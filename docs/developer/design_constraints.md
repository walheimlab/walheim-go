# Design Constraints & Conventions

These constraints represent core architectural behaviors and compatibility requirements with existing data directories. Get these wrong, and the tool will silently break or lose compatibility.

---

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

### 20. The data directory is intended to be a Git repo

Users are expected to `git init` their data directories and commit `.namespace.yaml`, `.app.yaml`, etc. The generated `docker-compose.yml` files should be in `.gitignore`. This is the GitOps model: config in git, runtime state in Docker labels.
