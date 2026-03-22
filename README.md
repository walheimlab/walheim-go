# walheim-go

[![codecov](https://codecov.io/gh/walheimlab/walheim-go/graph/badge.svg)](https://codecov.io/gh/walheimlab/walheim-go)

`whctl` is a kubectl-style CLI for managing Docker Compose apps across physical homelab machines over SSH. No scheduler, no control plane — just SSH, rsync, and `docker compose up`. Manifests are stored on a local filesystem or any S3-compatible object store (Cloudflare R2, DigitalOcean Spaces, MinIO, etc.).

## Install

**Homebrew (macOS/Linux):**
```bash
brew install walheimlab/tap/whctl
```

**Go install:**
```bash
go install github.com/walheimlab/walheim-go/cmd/whctl@latest
```

Or download a pre-built binary from the [releases page](https://github.com/walheimlab/walheim-go/releases).

## Concepts

- **Namespace** — a physical machine (hostname + SSH user). One namespace = one host.
- **App** — a Docker Compose application deployed to a namespace.
- **Secret / ConfigMap** — key/value stores injected as environment variables into apps.
- **DaemonSet** — a compose app deployed to every namespace (like a sidecar on all hosts).
- **Job** — a one-shot compose service run on demand.
- **Context** — points `whctl` at a data directory (local path or S3-compatible bucket) where your manifests live.

## Quick Start

```bash
# Create a context pointing at your homelab data dir
whctl context new home --data-dir ~/homelab

# Register a machine
whctl apply namespace production <<EOF
apiVersion: walheim/v1alpha1
kind: Namespace
metadata:
  name: production
spec:
  hostname: prod.example.com
  username: admin
EOF

# Deploy an app
whctl apply app myapp -n production -f myapp.yaml

# Check status
whctl get apps -n production
whctl describe app myapp -n production
```

## Commands

```
Resource commands:
  apply       Create or update a resource from a manifest file
  create      Create a resource
  delete      Delete a resource
  describe    Show detailed resource information
  doctor      Check resources for issues
  exec        Execute a command inside a running container
  get         List or get resources
  import      Wrap an existing docker-compose.yml into an App manifest
  logs        Print or follow container logs
  pause       Stop containers (keep remote files)
  pull        Pull latest images without restarting
  run         Run a Job on its target host
  start       Start (or restart) containers
  stop        Stop containers and remove remote files

Management:
  context     Manage whctl contexts (local or S3)
  label       Set, remove, or list labels on any resource
  version     Show version info
```

### Resources

| Kind | Scope | Description |
|---|---|---|
| `namespace` | cluster | A physical machine |
| `app` | namespaced | A Docker Compose app |
| `secret` | namespaced | Base64-encoded key/value env store |
| `configmap` | namespaced | Plaintext key/value env store |
| `daemonset` | cluster | A compose app deployed to all namespaces |
| `job` | namespaced | A one-shot compose service |

### Output formats

```bash
whctl get apps -n production          # table (default)
whctl get apps -n production -o json  # JSON array
whctl get app myapp -n production     # single resource — prints raw YAML
whctl get apps -A                     # all namespaces
```

## App Manifest

```yaml
apiVersion: walheim/v1alpha1
kind: App
metadata:
  name: myapp
  namespace: production
spec:
  compose:
    services:
      web:
        image: nginx:alpine
        environment:
          - LOG_LEVEL=info

  # Inject env vars from a Secret into specific services
  envFrom:
    - secretRef:
        name: db-creds
      serviceNames: [web]

  # Override env vars (support ${VAR} substitution)
  env:
    - name: APP_ENV
      value: production
```

## Contexts

```bash
# Local data directory
whctl context new home --data-dir ~/homelab

# S3-compatible storage (Cloudflare R2, DigitalOcean Spaces, etc.)
whctl context new remote --backend s3 \
  --s3-bucket my-homelab \
  --s3-endpoint https://account.r2.cloudflarestorage.com \
  --s3-region auto \
  --s3-access-key-id KEY \
  --s3-secret-access-key SECRET

whctl context list
whctl context use remote
```

## Data Directory Layout

```
<dataDir>/
└── namespaces/
    └── production/
        ├── .namespace.yaml
        ├── apps/myapp/.app.yaml
        ├── secrets/db-creds/.secret.yaml
        └── configmaps/app-config/.configmap.yaml
```

The data directory can be a local path (suitable as a Git repo) or an S3-compatible bucket — configured per context. Generated `docker-compose.yml` files should be in `.gitignore` when using a local Git repo.

## Labels

```bash
whctl label namespace production env=prod team=platform
whctl label app myapp -n production tier=backend
whctl label app myapp -n production old-label-   # remove
whctl label namespace production --list
```

## License

MIT
