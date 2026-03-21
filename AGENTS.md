# Agent Handover — walheim-go

This document is for AI agents picking up work on this project. Read it fully before writing any code.

---

## What This Project Is

`walheim-go` is a Go rewrite of [`walheim-rb`](https://github.com/walheimlab/walheim-rb), a kubectl-style CLI for managing Docker-based homelab infrastructure across physical machines. The binary is called `whctl`.

**The core concept:** namespaces map 1:1 to physical machines (not logical groups like in Kubernetes). You deploy Docker Compose apps to those machines over SSH using rsync. There is no central control plane, no scheduler, no kubelet — just SSH, rsync, and `docker compose up`.

**The user's mental model is kubectl:** `whctl get namespaces`, `whctl apply app myapp -n production`, `whctl describe namespace production`. Anyone who's used kubectl should feel at home.

**This is a homelab tool, not production infra.** Simplicity beats completeness. WONTDO: RBAC, scheduling, port-forward, rollout history, resource quotas.

---

## Repository Layout

```
walheim-go/
├── cmd/
│   └── whctl/          ← CLI entry point and all cobra commands
├── internal/
│   ├── config/         ← ~/.walheim/config management
│   ├── exitcode/       ← canonical exit code constants
│   ├── fs/             ← filesystem abstraction (local impl; S3 planned)
│   ├── labels/         ← label get/set operations on manifests
│   ├── output/         ← table + JSON rendering, structured error format
│   ├── registry/       ← resource kind registry (maps kind strings to factories)
│   ├── resource/       ← base types: ClusterBase, NamespacedBase, ResourceMeta
│   ├── resources/
│   │   ├── apps/       ← App resource (most complex)
│   │   └── namespaces/ ← Namespace resource (cluster-scoped)
│   ├── rsync/          ← thin rsync wrapper
│   ├── ssh/            ← thin SSH wrapper
│   └── version/        ← build-time version vars (injected via ldflags)
├── plans/              ← execution plans for agents (read before implementing)
│   ├── plan-01-foundation.md
│   ├── plan-02-resource-framework.md
│   └── (plan-03 through plan-05 not yet written — see todo below)
├── go.mod
├── AGENTS.md           ← this file
└── README.md
```

---

## Intentional Breaks from the Ruby Implementation

The Go rewrite deliberately drops deprecated and legacy designs. Do not port these.

### 1. `--data-dir` flag — dropped

The Ruby CLI supports `--data-dir` as a global flag that bypasses the context system entirely, printing a deprecation warning when used. In Go, it does not exist. If a user has no config, the error message tells them to run `whctl context new`. There is no escape hatch.

### 2. Flat namespace manifest format — dropped

The Go implementation only knows one namespace manifest format:

```yaml
apiVersion: walheim/v1alpha1
kind: Namespace
metadata:
  name: production
spec:
  hostname: prod.example.com
  username: admin
```

`spec.hostname` and `spec.username` are the only fields the code reads. There is no fallback, no dual-format logic, no awareness that any other format ever existed.

### 3. Split `context` command routing — dropped

In the Ruby binary, `context` commands are routed to a completely separate `LegacyContext` module (OptionParser-based), while all other commands go through Thor. This is visible in `bin/whctl`:

```ruby
if ARGV[0] == 'context'
  Walheim::LegacyContext.execute(ARGV)
else
  Walheim::CLI.start(ARGV)
end
```

This split exists because Thor could not cleanly handle the context subcommands at the time. In Go, all commands — including `context` — go through cobra uniformly. `whctl context list`, `whctl context use`, etc. are regular cobra subcommands with no special routing.

---

## General Direction

1. **Correctness first.** The Go implementation must be a faithful port of the Ruby behaviour. Don't invent new semantics. When in doubt, read the Ruby source at `../walheim-rb/`.

2. **Agent-friendly on top.** After porting each command correctly, apply the patterns from the `agent-friendly-cli` skill: `--output json`, structured errors, exit codes, `--dry-run`, `--yes`, non-TTY detection, `--quiet`. These are additive — they don't change the human-facing behaviour.

3. **Filesystem abstraction from day one.** All resource logic must go through `internal/fs.FS` (never call `os.*` directly in resource code). The interface is designed to allow an S3 backend later without touching resource logic.

4. **Plans are the execution order.** Work through `plans/plan-01-*.md`, then `plan-02`, etc. Each plan is independently executable. Plan-01 must be done before Plan-02, Plan-02 before Plan-03, etc. Plans 03–05 still need to be written (see todo).

---

## Gap: Ruby vs Go — Todo List

### Plans not yet written
- [ ] **plan-03-namespaces.md** — Port `Resources::Namespaces` (cluster-scoped resource, includes SSH status checks in `describe`)
- [ ] **plan-04-apps.md** — Port `Resources::Apps` (most complex: compose generation, env injection, parallel SSH status, lifecycle commands)
- [ ] **plan-05-release.md** — GoReleaser + GitHub Actions + Homebrew tap (follow the `go-release` skill at `.pi/skills/go-release/`)

### Features from Ruby not yet accounted for in plans
- [ ] **`whctl import app`** — Wraps an existing `docker-compose.yml` into a Walheim App manifest (`.app.yaml`) without deploying. Covered in plan-04 scope but not yet written.
- [ ] **`whctl exec app`** — Interactive exec into a container. Must use `syscall.Exec` (process replacement, not `exec.Command`) so signals and TTY work correctly.
- [ ] **`whctl logs app --follow`** — Same as exec: must use `syscall.Exec` for proper `Ctrl+C` handling.
- [ ] **`whctl pull app`** — Pull latest images without restarting containers.
- [ ] **`whctl start` / `pause` / `stop` app** — App lifecycle commands beyond `apply`.
- [ ] **`whctl label`** — Set/remove/list labels on any resource manifest.
- [ ] **`whctl describe namespace`** — Live SSH connectivity check, Docker version probe, deployed container summary, disk usage. All happens at command time via SSH (not cached state).
- [ ] **`whctl describe app`** — Runs `docker compose ps` + `docker stats` via SSH.
- [ ] **Secret and ConfigMap resources** — Simple namespaced resources (no lifecycle hooks). Secrets: `.secret.yaml`, ConfigMaps: `.configmap.yaml`.
- [ ] **Parallel SSH status fetching for `get apps`** — When listing apps, batch-query all unique hosts concurrently (one SSH call per host, not one per app). See non-obvious rules below.
- [ ] **`spec.envFrom` injection** — Apps can inject environment variables from Secrets and ConfigMaps. Precedence rules are important (see non-obvious rules).
- [ ] **`spec.env` with variable substitution** — `${VAR_NAME}` in values is substituted from already-resolved env vars at compile time.
- [ ] **`-o yaml` output** — `get <kind> <name>` (single resource by name) should print the raw YAML manifest. Not a table.

### Planned in Ruby README but not yet implemented in Ruby either
- [ ] `edit` — Interactive resource editing
- [ ] `patch` — Partial updates
- [ ] Label selectors (`-l key=value`)
- [ ] `-o wide` output
- [ ] `attach`, `cp`
- [ ] Annotations

These are fine to defer to a future milestone.

---

## Resource Scopes: Cluster vs Namespaced

Every resource kind is either **cluster-scoped** or **namespaced**. This affects filesystem layout, CLI flag requirements, and which base type to embed.

### Filesystem layout

```
<dataDir>/
├── namespaces/              ← cluster-scoped: Namespace resources live here
│   ├── production/
│   │   ├── .namespace.yaml          ← Namespace manifest
│   │   ├── apps/            ← namespaced: App resources
│   │   │   └── myapp/
│   │   │       └── .app.yaml
│   │   ├── secrets/         ← namespaced: Secret resources
│   │   │   └── db-creds/
│   │   │       └── .secret.yaml
│   │   └── configmaps/      ← namespaced: ConfigMap resources
│   │       └── app-config/
│   │           └── .configmap.yaml
│   └── staging/
│       └── .namespace.yaml
└── (future cluster-scoped kinds sit here, alongside namespaces/)
```

Cluster-scoped resources are organised at the same level as the `namespaces/` directory — they are siblings of it, not children. Namespaced resources are always nested inside a namespace directory.

Path formulas:
- **Cluster-scoped:** `<dataDir>/<kind-plural>/<name>/<manifest>`
- **Namespaced:** `<dataDir>/namespaces/<namespace>/<kind-plural>/<name>/<manifest>`

Today the only cluster-scoped kind is `namespaces` itself, so `<kind-plural>` happens to be `namespaces` and the path is `<dataDir>/namespaces/<name>/`. This is not a special case — it falls out of the formula naturally.

### CLI differences

| | Cluster-scoped | Namespaced |
|---|---|---|
| `-n` / `--namespace` flag | Never present | Required for most operations |
| `-A` / `--all-namespaces` | N/A (no namespace concept) | Lists across all namespaces |
| `get` with no name | Lists all resources of that kind | Requires `-n` or `-A` |

### Base types

- Cluster-scoped resources embed `resource.ClusterBase`
- Namespaced resources embed `resource.NamespacedBase`

These base types implement the path formulas above and provide `ListAll`, `Get`, `Exists`, `ReadManifest`, `WriteManifest`, `EnsureDir`, `RemoveDir`. Resource packages call these rather than constructing paths manually.

### Discovering namespaces for `--all`

When a namespaced resource lists with `-A`, it scans `<dataDir>/namespaces/` for valid namespace directories. A directory is only considered a valid namespace if it contains a `.namespace.yaml` file. Bare directories (e.g. `.git`, temp dirs) are silently skipped.

---

## Command Registration Pattern

This is the most architecturally important thing to understand before touching the CLI layer.

**Commands are not statically declared.** There is no `cmd_get_namespaces.go` with hardcoded flag definitions for each resource. Instead:

1. Each resource package (e.g. `internal/resources/namespaces`) calls `registry.Register()` in its `init()` function, providing its `KindInfo` (plural, singular, aliases) and a factory function.

2. `cmd/whctl/main.go` blank-imports those packages to trigger their `init()` calls:
   ```go
   import (
       _ "github.com/walheimlab/walheim-go/internal/resources/namespaces"
       _ "github.com/walheimlab/walheim-go/internal/resources/apps"
   )
   ```

3. The verb commands (`get`, `apply`, `delete`, etc.) are generic cobra commands that accept `<kind>` as their first argument, look it up in the registry at runtime, and dispatch to the handler. There is one `get` command, not one per resource.

4. Resource-specific *extra* flags (e.g. `--hostname` for `create namespace`, `--follow` for `logs app`) are registered by the resource package itself, not by the generic verb command. The resource package may add its own cobra subcommands or hook into the generic verb via the registry entry.

This means: **adding a new resource type requires zero changes to the CLI layer.** You write the resource package, register it, blank-import it — and `whctl get <newkind>`, `whctl apply <newkind>`, etc. work automatically.

The Ruby equivalent is `HandlerRegistry` + `ResourceCommand.register_operation` in `lib/walheim/cli/resource_command.rb`. Thor commands are defined dynamically via `define_method` in a loop over all registered operations. The Go port must preserve this same extensibility.

### All verbs are equal

There is no distinction between "standard" verbs and "resource-specific" verbs at the framework level. Every verb — `get`, `apply`, `delete`, `start`, `pause`, `stop`, `logs`, `pull`, `import`, `exec` — is just an operation declared by a resource package. The framework registers a cobra command for a verb if and only if at least one registered resource declares it as an operation.

In the Ruby implementation, `operation_info` on the handler class drives all of this through a single `register_operation` loop. The same loop that wires `get` also wires `start` and `logs`. There is no hardcoded list of verbs in the CLI layer.

The Go port must follow the same model:
- Each resource package declares its complete set of operations in a structured way (analogous to `operation_info`)
- The framework iterates all registered resources, collects the union of all declared operations, and creates one cobra command per operation
- Each cobra command accepts `<kind>` as its first argument, looks up the resource in the registry, and dispatches to the handler method

This means `whctl start`, `whctl logs`, `whctl pull`, etc. are first-class top-level commands — not subcommands of a resource group — and they work for any resource that declares them. Today only Apps declares `start`; if a future resource also declares `start`, it just works.

### What this means practically

- The framework in plan-02 must not hardcode any verb names. Build the cobra command tree by iterating `registry.AllOperations()` — whatever operations the registered resources declare, those become the commands.
- Each operation declaration includes its flags. The framework merges flags across all resources that share a verb (e.g. both Namespaces and Apps declare `describe` with different flags — the cobra command gets the union).
- Resource packages are the only place where operations and their flags are defined. The CLI layer is purely mechanical wiring.
- `exec` was hardcoded in the Ruby CLI as an exception because Thor couldn't handle variadic args cleanly. In Go, cobra handles `--` separator natively, so `exec` should go through the same operation declaration path as everything else.

---

## Non-Obvious Rules

These are things you cannot infer from skimming the code. Get them wrong and the tool silently breaks compatibility with existing data directories.

### 1. Manifest filenames follow a consistent dot-prefix convention

Each resource type uses a dot-prefixed hidden filename inside its directory:

| Resource | Manifest filename |
|---|---|
| Namespace | `.namespace.yaml` |
| App | `.app.yaml` |
| Secret | `.secret.yaml` |
| ConfigMap | `.configmap.yaml` |

All manifests are hidden files. Do not change these filenames — existing data directories depend on them. Do not unify them to a generic name like `manifest.yaml`.

### 2. The `dataDir` is a parent — namespaces live inside it

The `dataDir` from config points to the root of a homelab context. Namespaces are discovered at `<dataDir>/namespaces/<name>/`. When a user stores namespaces at `~/homelab/namespaces/prod/`, their `dataDir` is `~/homelab/`, not `~/homelab/namespaces/`.

Despite `Namespace` being a `ClusterResource` with `plural = "namespaces"`, the ClusterBase path formula `<dataDir>/<plural>/<name>` accidentally gives `<dataDir>/namespaces/<name>/` — which is correct. This is not a coincidence, it's intentional.

### 3. apiVersion differs between config and resource manifests

- **Config file** (`~/.walheim/config`): `apiVersion: walheim.io/v1`
- **Resource manifests** (`.namespace.yaml`, `.app.yaml`, etc.): `apiVersion: walheim/v1alpha1`

These are different strings. Validate them accordingly.

### 4. Namespace detection requires the manifest file to exist

When listing resources with `--all`, Walheim only recognises a namespace if `<dataDir>/namespaces/<name>/.namespace.yaml` exists. A bare directory without the manifest is silently skipped. This prevents picking up `.git` or other non-namespace directories.

### 5. `stop` = `pause` then delete remote files

`whctl stop app` does two things:
1. Calls `pause` first (runs `docker compose down` on the remote)
2. Then runs `ssh rm -rf /data/walheim/apps/<name>` on the remote

`pause` only stops containers and keeps the files. `stop` cleans up entirely. The `pre_delete` hook in the Ruby code is what triggers `pause` before a delete.

### 6. Remote base dir is hardcoded as `/data/walheim`

The remote machine always gets files synced to `/data/walheim/apps/<name>/`. This is not configurable per-namespace. Apps are at `/data/walheim/apps/`, (future resources may add other subdirs). Don't make this a flag — keep it consistent so the tool is predictable.

### 7. Container status is fetched per-host, not per-app

When `get apps` runs across multiple namespaces, it would be catastrophically slow to SSH once per app. Instead:
1. Group all namespaces by their SSH host
2. SSH to each unique host exactly once with a single `docker ps -a --filter label=walheim.managed=true` query
3. Parse the output to extract all apps on that host
4. Do all host queries concurrently (goroutines, not sequential)

The Ruby implementation uses the `parallel` gem. In Go, use `sync.WaitGroup` + goroutines + a mutex-protected map for results.

### 8. Walheim labels are the source of truth for runtime state

There is no local database of "what's deployed where." The only runtime state lives in Docker labels on the remote containers:
- `walheim.managed=true` — this container is managed by Walheim
- `walheim.namespace=<name>` — which namespace
- `walheim.app=<name>` — which app
- `walheim.injected-env.secret.<name>=KEY1,KEY2` — which secret keys were injected (for audit)
- `walheim.injected-env.configmap.<name>=KEY1,KEY2` — same for configmaps
- `walheim.injected-env.override=KEY1,KEY2` — keys set by `spec.env`

Without these labels, `get apps` shows `NotFound` even if containers are running.

### 9. Environment variable injection precedence (highest to lowest)

When generating the final `docker-compose.yml` for an app:
1. **Existing `environment:` in the compose spec** — never overwritten
2. **`spec.env[]`** — direct env vars; always overwrite lower precedence
3. **`spec.envFrom[]`** — from secrets/configmaps; only set if key not already present

`spec.env` supports `${VAR_NAME}` substitution using the already-resolved environment at time of substitution. If a variable isn't found, keep the literal `${VAR_NAME}` string (don't error).

### 10. `exec` and `logs --follow` must replace the process

Both `whctl exec app` and `whctl logs app --follow` need to behave like the user is directly SSH'd in — signals (`Ctrl+C`, `SIGTERM`) must propagate, TTY must attach. Use `syscall.Exec(sshBinaryPath, args, os.Environ())` to replace the whctl process with the SSH process. Using `exec.Command(...).Run()` is wrong for these cases.

### 11. Single-resource `get` prints YAML, not a table

`whctl get apps -n production` → table  
`whctl get app myapp -n production` → raw YAML of the manifest (not a one-row table)

This matches kubectl behaviour. The Ruby code prints `YAML.dump(result[:manifest])` when the result is a single Hash rather than an Array.

### 12. `context new` creates `namespaces/` if missing

When `whctl context new <name> --data-dir <path>` is run and the `<path>/namespaces/` subdirectory doesn't exist, Walheim creates it automatically with a warning. If `<path>` itself doesn't exist, it errors (doesn't create it). The data directory must pre-exist; only the `namespaces/` subdirectory is auto-created.

### 13. The generated `docker-compose.yml` is a local artifact

After `whctl apply app` runs, it writes a generated `docker-compose.yml` into the local app directory (next to `.app.yaml`). This file is then synced to the remote. Users should not edit it by hand — it gets overwritten on every apply. Source of truth is `.app.yaml`.

### 14. Namespace manifest structure

The namespace manifest format:

```yaml
apiVersion: walheim/v1alpha1
kind: Namespace
metadata:
  name: production
spec:
  hostname: prod.example.com
  username: admin    # optional; uses SSH config if omitted
```

Hostname is read from `spec.hostname`. Username is read from `spec.username`.

### 15. SSH username is optional

If `username` is not set in the namespace config, the SSH connection is just `hostname` (no `user@`). SSH will use the local user or whatever is in `~/.ssh/config`. Don't error when username is absent — it's a valid configuration.

### 16. Non-TTY detection governs prompt behaviour

When `stdin` is not a TTY (i.e., the caller is a script or agent):
- Destructive commands (`delete`, `stop`) must either have `--yes` set or fail immediately with a clear error message pointing to `--yes`
- Never hang waiting for input
- In `--output json` mode, warnings about non-TTY go to stderr only

### 17. `apply` on apps auto-starts — lifecycle hooks are the mechanism

When `apply app` creates or updates an app manifest, it automatically triggers `start`. This is not a separate step the user takes — it happens inside the `apply` Run function via the hook system.

Apps declares: `PostCreate = "start"`, `PostUpdate = "start"`, `PreDelete = "stop"`.

The `apply` Run function calls the hook directly after writing the manifest. `delete` calls the `stop` hook before removing the local directory. If either hook fails, the overall command fails.

Hooks are declared per-resource in the `registry.Registration.Hooks` struct. The framework provides a helper to invoke them; the resource's operation Run function is responsible for calling it.

### 18. `serviceNames` in envFrom and env targets specific services

Both `spec.envFrom[]` and `spec.env[]` entries accept an optional `serviceNames` field:

```yaml
envFrom:
  - secretRef:
      name: db-creds
    serviceNames: [web, worker]  # only inject into these services
env:
  - name: LOG_LEVEL
    value: debug
    # no serviceNames = inject into ALL services
```

If `serviceNames` is absent or empty, the injection applies to every service in the compose spec. If present and non-empty, only the listed service names receive the injection. Services not in the list are untouched.

### 19. Secret `data` is base64-encoded; `stringData` is plaintext; both coexist

Kubernetes-style secrets support two fields:

```yaml
data:
  DB_PASSWORD: c2VjcmV0MTIz   # base64-encoded
stringData:
  API_KEY: plaintext-value     # not encoded
```

When loading a secret for env injection:
1. Decode every value in `data` with `base64.StdEncoding.DecodeString`
2. Take every value in `stringData` as-is
3. Merge both maps — `stringData` takes precedence if the same key appears in both

ConfigMaps only have `data` (plaintext, no encoding). No base64 involved.

### 21. The data directory is intended to be a Git repo

Users are expected to `git init` their data directories and commit `.namespace.yaml`, `.app.yaml`, etc. The generated `docker-compose.yml` files should be in `.gitignore`. This is the GitOps model: config in git, runtime state in Docker labels.

---

## Reference: Ruby Source Location

The canonical Ruby implementation is at `../walheim-rb/` (sibling directory). When porting behaviour, read:

| Topic | Ruby file |
|---|---|
| Config management | `lib/walheim/config.rb` |
| Namespace resource | `lib/walheim/resources/namespaces.rb` |
| App resource | `lib/walheim/resources/apps.rb` |
| Secret resource | `lib/walheim/resources/secrets.rb` |
| ConfigMap resource | `lib/walheim/resources/configmaps.rb` |
| Rsync/SSH sync | `lib/walheim/sync.rb` |
| Base resource types | `lib/walheim/resource.rb`, `cluster_resource.rb`, `namespaced_resource.rb` |
| CLI dispatch | `lib/walheim/cli/base_command.rb` |
| Table output | `lib/walheim/cli/helpers.rb` |
| Context subcommands | `lib/walheim/cli/legacy_context.rb` |
| Label operations | `lib/walheim/label_operations.rb` |

---

## Quick Sanity Check

After any implementation work, verify:

```bash
cd /path/to/walheim-go
go build ./...
go vet ./...
./whctl --help
./whctl version
./whctl get --help
./whctl context --help
```

The binary must compile cleanly. `go vet` must pass. Help text should be readable and include examples.
