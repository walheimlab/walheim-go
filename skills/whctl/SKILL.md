---
name: whctl
description: Teach AI agents to use whctl, a kubectl-style CLI for managing Docker Compose applications across homelab machines over SSH. Covers commands, resource types, manifest formats, workflows, and agent-friendly patterns.
---

# whctl — Agent Usage Guide

`whctl` is a kubectl-inspired CLI for deploying and managing Docker Compose applications across physical homelab machines over SSH. No control plane, no scheduler — just SSH, rsync, and `docker compose`.

## Core Concepts

| Concept | Description |
|---|---|
| **Context** | Points whctl at a data directory (local path or S3-compatible bucket) containing all manifests |
| **Namespace** | One physical machine (hostname + SSH user). One namespace = one host |
| **App** | A Docker Compose application deployed to a single namespace |
| **DaemonSet** | A Compose app deployed to all namespaces matching a label selector |
| **Secret** | Base64-encoded key/value pairs injected as env vars into apps. Never synced to hosts |
| **ConfigMap** | Plaintext key/value pairs injected as env vars into apps. Never synced to hosts |
| **Job** | A one-shot container execution on demand |

## Global Flags

```
--context STRING      Override active context
--whconfig STRING     Alternate config file (default: ~/.walheim/config, or $WHCONFIG)
-o, --output STRING   Output format: human|yaml|json (default: human)
-q, --quiet           Bare output, one item per line, no headers
-n, --namespace       Target namespace (required for namespaced resources)
-A, --all-namespaces  Operate across all namespaces
--dry-run             Show what would happen without making changes
--yes                 Skip confirmation prompts (required in non-TTY/agent contexts)
-f, --filename        File, directory, or URL containing manifest(s)
```

## Agent-Friendly Patterns

When running whctl non-interactively (scripts, agents, CI):

- Always pass `-o json` for structured, parseable output
- Always pass `--yes` on destructive commands (`delete`, `stop`) — without it, whctl will error rather than hang waiting for input
- Check exit code: `0` = success, non-zero = failure. Error details go to stderr as JSON when `-o json` is set
- Use `-q` to get plain name lists for piping into other commands
- Use `--dry-run` to validate before applying changes

```bash
# List all apps as JSON
whctl get apps -A -o json

# Delete without prompting (agent-safe)
whctl delete app myapp -n production --yes

# Get a single resource as YAML manifest
whctl get app myapp -n production -o yaml

# Quiet list for scripting
whctl get namespaces -q
```

## Commands Reference

### Context Management

```bash
# Add a local context
whctl context new home --data-dir ~/homelab

# Add an S3-backed context (Cloudflare R2, DigitalOcean Spaces, MinIO)
whctl context new prod \
  --backend s3 \
  --s3-endpoint https://account.r2.cloudflarestorage.com \
  --s3-region auto \
  --s3-bucket my-homelab \
  --s3-prefix walheim

# Switch context
whctl context use home

# Show active context
whctl context current

# List all contexts
whctl context list

# Export all resources as multi-document YAML
whctl context export > backup.yaml
```

### Namespaces (cluster-scoped)

```bash
whctl create namespace production --hostname prod.example.com --username admin
whctl get namespaces
whctl get namespace production -o yaml
whctl describe namespace production   # live SSH check + docker info
whctl delete namespace production --yes
whctl doctor namespaces               # validate all namespace manifests
```

### Apps (namespaced)

```bash
whctl apply app myapp -n production -f app.yaml   # create/update + auto-start
whctl get apps -n production
whctl get apps -A                                  # all namespaces
whctl get app myapp -n production -o yaml          # raw manifest
whctl describe app myapp -n production             # live docker compose ps
whctl start app myapp -n production
whctl pause app myapp -n production                # stop containers, keep remote files
whctl stop app myapp -n production                 # stop + delete remote files
whctl pull app myapp -n production                 # pull latest images
whctl logs app myapp -n production --follow --tail 100 --service web
whctl exec app myapp -n production --service web --cmd "sh"
whctl delete app myapp -n production --yes
```

### Secrets (namespaced)

```bash
whctl apply secret db-creds -n production -f secret.yaml
whctl get secrets -n production
whctl delete secret db-creds -n production --yes
```

### ConfigMaps (namespaced, alias: cm)

```bash
whctl apply configmap app-config -n production -f configmap.yaml
whctl get cm -n production
whctl delete configmap app-config -n production --yes
```

### DaemonSets (cluster-scoped, alias: ds)

```bash
whctl apply daemonset monitoring -f daemonset.yaml
whctl get ds
whctl start daemonset monitoring      # deploys to all matching namespaces
whctl stop daemonset monitoring --yes
whctl describe daemonset monitoring   # per-namespace status
```

### Jobs (namespaced)

```bash
whctl apply job db-backup -n production -f job.yaml
whctl run job db-backup -n production             # stream output
whctl run job db-backup -n production --detach    # background
whctl logs job db-backup -n production
whctl delete job db-backup -n production --yes
```

### Labels

```bash
whctl label namespace production env=prod team=platform
whctl label app myapp -n production version=v2 --overwrite
whctl label namespace production old-label-   # remove label (trailing dash)
whctl label namespace production --list
whctl label namespace production --list -o json
```

### Diagnostics

```bash
whctl doctor namespaces               # validate all namespace manifests
whctl doctor apps -n production       # validate apps in a namespace
whctl actions app                     # list all verbs available for a resource
whctl version
```

## Manifest Formats

### Config (`~/.walheim/config`)

```yaml
apiVersion: walheim.io/v1
kind: Config
currentContext: home
contexts:
  - name: home
    dataDir: ~/homelab
  - name: r2-prod
    s3:
      endpoint: https://account.r2.cloudflarestorage.com
      region: auto
      bucket: my-homelab
      prefix: walheim
      accessKeyID: KEY       # or use AWS_ACCESS_KEY_ID env var
      secretAccessKey: SECRET
```

### Namespace

```yaml
apiVersion: walheim/v1alpha1
kind: Namespace
metadata:
  name: production
  labels:
    env: prod
spec:
  hostname: prod.example.com
  username: admin           # optional; uses SSH config if omitted
  baseDir: /data/walheim    # optional; default is /data/walheim
```

### App

```yaml
apiVersion: walheim/v1alpha1
kind: App
metadata:
  name: myapp
  namespace: production
  labels:
    tier: backend
spec:
  compose:
    services:
      web:
        image: nginx:alpine
        ports:
          - "80:80"
      worker:
        image: worker:latest

  # Inject from Secrets and ConfigMaps
  envFrom:
    - secretRef:
        name: db-creds
      serviceNames: [web, worker]   # omit to inject into ALL services
    - configMapRef:
        name: app-config
      serviceNames: [web]

  # Direct environment variables (supports ${VAR} substitution)
  env:
    - name: APP_ENV
      value: production
      # no serviceNames = inject into all services
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: db-creds
  namespace: production
type: Opaque
data:
  DB_PASSWORD: cGFzc3dvcmQxMjM=   # base64-encoded
stringData:
  DB_USER: admin                   # plaintext (auto-encoded on save)
```

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: production
data:
  LOG_LEVEL: info
  MAX_CONNECTIONS: "100"
```

### DaemonSet

```yaml
apiVersion: walheim/v1alpha1
kind: DaemonSet
metadata:
  name: monitoring
spec:
  namespaceSelector:
    matchLabels:
      env: prod
  compose:
    services:
      agent:
        image: agent:latest
```

### Job

```yaml
apiVersion: walheim/v1alpha1
kind: Job
metadata:
  name: db-backup
  namespace: production
spec:
  image: backup-tool:latest
  command: /backup.sh
  args:
    - --compress
    - --keep
    - 7d
```

## Common Workflows

### Bootstrap a new homelab

```bash
whctl context new home --data-dir ~/homelab
whctl create namespace production --hostname prod.example.com --username admin
whctl apply secret db-creds -n production -f secrets.yaml
whctl apply configmap app-config -n production -f config.yaml
whctl apply app myapp -n production -f app.yaml   # apply auto-starts the app
whctl get apps -n production -o json
```

### Deploy to multiple machines with DaemonSet

```bash
# Label existing namespaces
whctl label namespace node1 role=worker
whctl label namespace node2 role=worker

# Apply and start daemonset (targets all namespaces with role=worker)
whctl apply daemonset my-agent -f daemonset.yaml
whctl start daemonset my-agent
whctl describe daemonset my-agent -o json
```

### Backup and restore all manifests

```bash
whctl context export > backup.yaml
# restore:
whctl context new restored --data-dir ~/restored
whctl apply -f backup.yaml
```

## Key Behaviours to Know

- `apply app` automatically triggers `start` — no separate step needed
- `stop app` = pause (docker compose down) + delete remote files; `pause` keeps files
- `get app myapp` (single resource by name) prints the raw YAML manifest, not a table
- `exec` and `logs --follow` replace the whctl process via `syscall.Exec` so signals work
- Multi-namespace operations (`get apps -A`, `start daemonset`) run in parallel
- Secrets are never synced to remote hosts — only their decrypted values are injected into `.env` files
- The data directory is designed to be a git repo; generated `docker-compose.yml` files should be in `.gitignore`
- `apiVersion` in config is `walheim.io/v1`; in resource manifests it is `walheim/v1alpha1` — these are different
