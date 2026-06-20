# walheim-go Architecture & Design

This document details the architectural patterns, repository layout, and core designs of `walheim-go`.

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
├── docs/               ← user and developer documentation
├── go.mod
├── AGENTS.md           ← brief agent instruction file
└── README.md
```

---

## Resource Scopes: Cluster vs Namespaced

Every resource kind is either **cluster-scoped** or **namespaced**. This affects filesystem layout, CLI flag requirements, and which base type to embed.

### Filesystem Layout

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

### CLI Differences

| | Cluster-scoped | Namespaced |
|---|---|---|
| `-n` / `--namespace` flag | Never present | Required for most operations |
| `-A` / `--all-namespaces` | N/A (no namespace concept) | Lists across all namespaces |
| `get` with no name | Lists all resources of that kind | Requires `-n` or `-A` |

### Base Types

- Cluster-scoped resources embed [resource.ClusterBase](../../internal/resource/base.go) (or equivalent base type)
- Namespaced resources embed [resource.NamespacedBase](../../internal/resource/base.go)

These base types implement the path formulas above and provide `ListAll`, `Get`, `Exists`, `ReadManifest`, `WriteManifest`, `EnsureDir`, `RemoveDir`. Resource packages call these rather than constructing paths manually.

### Discovering Namespaces for `--all`

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

### All Verbs are Equal

There is no distinction between "standard" verbs and "resource-specific" verbs at the framework level. Every verb — `get`, `apply`, `delete`, `start`, `pause`, `stop`, `logs`, `pull`, `import`, `exec` — is just an operation declared by a resource package. The framework registers a cobra command for a verb if and only if at least one registered resource declares it as an operation.

In the Ruby implementation, `operation_info` on the handler class drives all of this through a single `register_operation` loop. The same loop that wires `get` also wires `start` and `logs`. There is no hardcoded list of verbs in the CLI layer.

The Go port must follow the same model:
- Each resource package declares its complete set of operations in a structured way (analogous to `operation_info`)
- The framework iterates all registered resources, collects the union of all declared operations, and creates one cobra command per operation
- Each cobra command accepts `<kind>` as its first argument, looks up the resource in the registry, and dispatches to the handler method

This means `whctl start`, `whctl logs`, `whctl pull`, etc. are first-class top-level commands — not subcommands of a resource group — and they work for any resource that declares them. Today only Apps declares `start`; if a future resource also declares `start`, it just works.

### What This Means Practically

- The framework must not hardcode any verb names. Build the cobra command tree by iterating `registry.AllOperations()` — whatever operations the registered resources declare, those become the commands.
- Each operation declaration includes its flags. The framework merges flags across all resources that share a verb (e.g. both Namespaces and Apps declare `describe` with different flags — the cobra command gets the union).
- Resource packages are the only place where operations and their flags are defined. The CLI layer is purely mechanical wiring.
- `exec` was hardcoded in the Ruby CLI as an exception because Thor couldn't handle variadic args cleanly. In Go, cobra handles `--` separator natively, so `exec` should go through the same operation declaration path as everything else.
