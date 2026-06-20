# walheim-go Roadmap & Feature Gap

This todo list tracks features from the original Ruby codebase (`walheim-rb`) that are not yet implemented or mapped to execution plans in `walheim-go`.

---

## Plans to Write

- [ ] **plan-03-namespaces.md** — Port `Resources::Namespaces` (cluster-scoped resource, includes SSH status checks in `describe`)
- [ ] **plan-04-apps.md** — Port `Resources::Apps` (most complex: compose generation, env injection, parallel SSH status, lifecycle commands)
- [ ] **plan-05-release.md** — GoReleaser + GitHub Actions + Homebrew tap (following `go-release` patterns)

---

## Feature Gaps: Ruby vs Go

### App & Lifecycle Management
- [ ] **`whctl import app`** — Wraps an existing `docker-compose.yml` into a Walheim App manifest (`.app.yaml`) without deploying.
- [ ] **`whctl exec app`** — Interactive exec into a container. Must use `syscall.Exec` (process replacement, not `exec.Command`) so signals and TTY work correctly.
- [ ] **`whctl logs app --follow`** — Process replacement `syscall.Exec` for proper `Ctrl+C` handling.
- [ ] **`whctl pull app`** — Pull latest images without restarting containers.
- [ ] **`whctl start` / `pause` / `stop` app** — App lifecycle commands beyond `apply`.
- [ ] **`whctl describe app`** — Runs `docker compose ps` + `docker stats` via SSH.
- [ ] **Parallel SSH status fetching for `get apps`** — Batch-query all unique hosts concurrently when listing apps.

### Namespace & Diagnostics
- [ ] **`whctl describe namespace`** — Live SSH connectivity check, Docker version probe, deployed container summary, disk usage. Runs on demand (not cached).
- [ ] **`whctl label`** — Set/remove/list labels on any resource manifest.

### Resources
- [ ] **Secret and ConfigMap resources** — Simple namespaced resources. Secrets: `.secret.yaml`, ConfigMaps: `.configmap.yaml`.

### Env Injection & Manifest Compilation
- [ ] **`spec.envFrom` injection** — Inject variables from Secrets and ConfigMaps.
- [ ] **`spec.env` with variable substitution** — `${VAR_NAME}` substitution from already-resolved env vars at compile time.

### CLI Output & Formatting
- [ ] **`-o yaml` output** — `get <kind> <name>` (single resource by name) should print the raw YAML manifest (not a table).

---

## Planned/Deferred Features (from Ruby README, not yet in Ruby either)

- [ ] `edit` — Interactive resource editing
- [ ] `patch` — Partial updates
- [ ] Label selectors (`-l key=value`)
- [ ] `-o wide` output
- [ ] `attach`, `cp`
- [ ] Annotations
