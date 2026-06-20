# whctl User Guide

`whctl` is a kubectl-inspired command line tool for deploying and managing Docker Compose applications across physical homelab machines over SSH. It requires no central control plane or scheduler — it operates purely using SSH, rsync, and `docker compose`.

---

## Core Concepts

- **Context** — Points `whctl` at a data directory (local path or S3-compatible bucket) containing all resource manifests.
- **Namespace** — One physical machine (hostname + SSH user). One namespace maps 1:1 to one physical host.
- **App** — A Docker Compose application deployed to a single namespace.
- **DaemonSet** — A compose app deployed to all namespaces matching a label selector.
- **Secret** — Base64-encoded key/value pairs injected as env vars into apps (never synced directly to hosts, only injected into compose env).
- **ConfigMap** — Plaintext key/value pairs injected as env vars into apps.
- **Job** — A one-shot container execution on demand.

---

## Global Flags

The following flags can be passed to most `whctl` commands:

```
--context STRING      Override active context
--whconfig STRING     Alternate config file (default: ~/.walheim/config, or $WHCONFIG)
-o, --output STRING   Output format: human|yaml|json (default: human)
-q, --quiet           Bare output, one item per line, no headers
-n, --namespace       Target namespace (required for namespaced resources)
-A, --all-namespaces  Operate across all namespaces
--dry-run             Show what would happen without making changes
--yes                 Skip confirmation prompts
-f, --filename        File, directory, or URL containing manifest(s)
```

---

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

---

## Manifest Formats

For a complete specification reference and examples, see [Manifest Reference](manifest_reference.md).

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

---

## Common Workflows

### Bootstrap a new homelab

```bash
# Set up a new local context
whctl context new home --data-dir ~/homelab

# Create your first namespace
whctl create namespace production --hostname prod.example.com --username admin

# Apply secrets, configmaps, and applications
whctl apply secret db-creds -n production -f secrets.yaml
whctl apply configmap app-config -n production -f config.yaml
whctl apply app myapp -n production -f app.yaml   # apply auto-starts the app

# Check deployment status
whctl get apps -n production -o json
```

### Deploy to multiple machines with DaemonSet

```bash
# Label namespaces
whctl label namespace node1 role=worker
whctl label namespace node2 role=worker

# Apply and start daemonset (targets all namespaces with role=worker)
whctl apply daemonset my-agent -f daemonset.yaml
whctl start daemonset my-agent
whctl describe daemonset my-agent -o json
```

### Backup and restore all manifests

```bash
# Export the entire context resources to a file
whctl context export > backup.yaml

# Restore by applying the exported backup in a new context
whctl context new restored --data-dir ~/restored
whctl apply -f backup.yaml
```
