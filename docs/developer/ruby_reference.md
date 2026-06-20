# Ruby Reference & Sanity Checks

This document maps functions to the canonical Ruby implementation (`../walheim-rb/`) and outlines verification steps.

---

## Ruby Source Location

When porting or comparing behaviors, check these source files in the sibling `walheim-rb` repository:

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

After making changes, run the Makefile targets to verify everything compiles, passes tests, and is linted cleanly:

```bash
# Build the whctl binary
make build

# Run unit tests and static analysis
make lint
make test

# Verify command responses
./whctl --help
./whctl version
./whctl get --help
./whctl context --help
```

The binary must compile cleanly. All unit tests and linters must pass. Help text should be readable and include examples.
