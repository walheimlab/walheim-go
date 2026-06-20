# Agent Handover — walheim-go

This document outlines key conventions and non-obvious rules for AI agents.
For comprehensive developer documentation, see:
- [Architecture & Layout](docs/developer/architecture.md)
- [Design Constraints & Conventions](docs/developer/design_constraints.md)
- [Roadmap & Todo](docs/developer/todo.md)
- [Intentional Breaks from Ruby](docs/developer/intentional_breaks.md)
- [Ruby Reference](docs/developer/ruby_reference.md)
- [Git Conventions](docs/developer/git_conventions.md)

---

## Core Conventions

1. **FS Abstraction**: Never call `os.*` directly in resource packages. Use `internal/fs.FS`.
2. **Hidden Manifests**: Filenames follow dot-prefix convention: `.namespace.yaml`, `.app.yaml`, `.secret.yaml`, `.configmap.yaml`.
3. **Cluster vs Namespaced**: Cluster-scoped embed `resource.ClusterBase` (placed at `<dataDir>/namespaces/<name>/.namespace.yaml`), namespaced embed `resource.NamespacedBase` (nested).
4. **Command Registration**: Cobra commands and verbs are not hardcoded. They are dynamically registered via `registry.Register()` and built from `registry.AllOperations()`.
5. **Process Replacement**: `exec` and `logs --follow` must replace the process via `syscall.Exec`.
6. **YAML Output**: `get` for a single resource must print raw YAML, not a table.
7. **APIVersion**: Config uses `walheim.io/v1`, resource manifests use `walheim/v1alpha1`.

---

## Git Conventions

- **Conventional Commit**: Use conventional commits with a maximum of 1 scope.
- **Subject**: Keep the first line brief and clear.
- **Description**: Explain the "why" of changes; explain "how" only if the diff is non-obvious. Keep overall messages brief.
- **No Issues**: Do not mention or reference external issue trackers.

---

## Design Constraints & Conventions (Quick Reference)

- **`dataDir` parent**: `dataDir` points to context root. Namespaces live at `<dataDir>/namespaces/`.
- **Namespace check**: A namespace requires `.namespace.yaml` to exist to be recognized.
- **`stop` hook**: `stop` triggers `pause` (remote compose down) then deletes remote files. Remote base dir is `/data/walheim`.
- **Parallel SSH**: Listing apps (`get apps -A`) groups namespaces by host and runs concurrent single `docker ps` queries.
- **Env Precedence**: Compose environment > `spec.env[]` > `spec.envFrom[]`. Supports `${VAR}` substitution.
- **Secrets**: Supports base64 `data` and plaintext `stringData` (precedence if keys collide). ConfigMaps only use plaintext `data`.
- **Non-TTY**: Destructive commands fail without `--yes` in non-interactive/agent sessions.
- **Hooks**: `apply` triggers `PostCreate/PostUpdate` ("start"), `delete` triggers `PreDelete` ("stop").
- **`serviceNames`**: Targets specific services for injection; empty targets all.
