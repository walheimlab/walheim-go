# Manifest Reference

`whctl` uses Kubernetes-style manifests to declare resources. This document details the schema and provides complete examples for all supported resources.

---

## Supported Manifests

| Resource | Scope | API Version | Kind | Description |
|---|---|---|---|---|
| [Namespace](#1-namespace) | Cluster | `walheim/v1alpha1` | `Namespace` | Represents a physical machine |
| [App](#2-app) | Namespaced | `walheim/v1alpha1` | `App` | A Docker Compose application |
| [Secret](#3-secret) | Namespaced | `v1` | `Secret` | Base64 and plaintext environment secrets |
| [ConfigMap](#4-configmap) | Namespaced | `v1` | `ConfigMap` | Plaintext configuration values |
| [DaemonSet](#5-daemonset) | Cluster | `walheim/v1alpha1` | `DaemonSet` | Compose app deployed across nodes matching labels |
| [Job](#6-job) | Namespaced | `walheim/v1alpha1` | `Job` | A one-shot compose service run on demand |

---

## 1. Namespace

A `Namespace` represents a target physical machine reachable over SSH. It is a **cluster-scoped** resource.

### Specification Fields
- `metadata.name`: (Required) Unique name of the machine.
- `metadata.labels`: (Optional) Key-value pairs used for DaemonSet scheduling.
- `spec.hostname`: (Required) Hostname or IP address of the machine.
- `spec.username`: (Optional) SSH connection username. If omitted, uses your local SSH configurations (`~/.ssh/config`) or current user.
- `spec.baseDir`: (Optional) The remote folder where files are synced (defaults to `/data/walheim`).

### Example
```yaml
apiVersion: walheim/v1alpha1
kind: Namespace
metadata:
  name: prod-node-1
  labels:
    env: production
    tier: edge
spec:
  hostname: prod-1.example.com
  username: admin
  baseDir: /data/walheim
```

---

## 2. App

An `App` represents a Docker Compose application deployed to a namespace. It is a **namespaced** resource.

### Specification Fields
- `spec.compose`: (Required) Standard Docker Compose structure containing `services`, `volumes`, etc.
- `spec.envFrom`: (Optional) List of Secret and ConfigMap references to inject environment variables.
  - `secretRef.name`: Name of the Secret to read.
  - `configMapRef.name`: Name of the ConfigMap to read.
  - `serviceNames`: (Optional) Array of services to inject these variables into. If omitted, injects into all services.
- `spec.env`: (Optional) Directly declared environment variables. Supports `${VAR}` substitution.
  - `name`: Variable name.
  - `value`: Variable value.
  - `serviceNames`: (Optional) Array of services to target.

### Example
```yaml
apiVersion: walheim/v1alpha1
kind: App
metadata:
  name: web-stack
  namespace: prod-node-1
spec:
  compose:
    services:
      web:
        image: nginx:alpine
        ports:
          - "80:80"
      db:
        image: postgres:15
        volumes:
          - pgdata:/var/lib/postgresql/data

  # Inject environment variables from resources
  envFrom:
    - secretRef:
        name: database-credentials
      serviceNames: [db]   # Inject db password only into db service
    - configMapRef:
        name: global-configs
                           # Omitted serviceNames = inject into web and db

  # Direct env vars with system environment substitution
  env:
    - name: SITE_URL
      value: "https://${DOMAIN_NAME:-example.com}"
      serviceNames: [web]
```

---

## 3. Secret

A `Secret` stores base64 or plaintext sensitive configurations. Decrypted secrets are injected as env vars into target containers during compilation and are never saved in plaintext on target machines. It is a **namespaced** resource.

### Specification Fields
- `type`: Must be `Opaque`.
- `data`: (Optional) Base64-encoded key/value pairs.
- `stringData`: (Optional) Plaintext key/value pairs (auto-base64 encoded when written). If keys collide between `data` and `stringData`, `stringData` takes precedence.

### Example
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: database-credentials
  namespace: prod-node-1
type: Opaque
data:
  DB_PASSWORD: c3VwZXItc2VjcmV0LXBhc3N3b3Jk   # base64 encoded
stringData:
  DB_USER: db_admin                           # plaintext
```

---

## 4. ConfigMap

A `ConfigMap` stores non-sensitive plaintext configurations that can be injected as env vars. It is a **namespaced** resource.

### Specification Fields
- `data`: (Required) Plaintext key/value pairs.

### Example
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: global-configs
  namespace: prod-node-1
data:
  LOG_LEVEL: debug
  API_TIMEOUT: "30s"
```

---

## 5. DaemonSet

A `DaemonSet` runs a Docker Compose app across all namespaces that match its `namespaceSelector`. It is a **cluster-scoped** resource.

### Specification Fields
- `spec.namespaceSelector.matchLabels`: (Required) Key-value map to select namespaces.
- `spec.compose`: (Required) The compose services to launch on matched machines.

### Example
```yaml
apiVersion: walheim/v1alpha1
kind: DaemonSet
metadata:
  name: node-exporter
spec:
  namespaceSelector:
    matchLabels:
      env: production
  compose:
    services:
      exporter:
        image: prom/node-exporter:latest
        ports:
          - "9100:9100"
```

---

## 6. Job

A `Job` represents a one-shot container task executed on demand on a namespace. It is a **namespaced** resource.

### Specification Fields
- `spec.image`: (Required) Docker image to pull and execute.
- `spec.command`: (Optional) Command to run.
- `spec.args`: (Optional) Arguments for the command.

### Example
```yaml
apiVersion: walheim/v1alpha1
kind: Job
metadata:
  name: nightly-backup
  namespace: prod-node-1
spec:
  image: backup-utility:v1
  command: ["/bin/sh"]
  args:
    - "-c"
    - "tar -czf /backup/db.tar.gz /var/lib/postgresql/data"
```
