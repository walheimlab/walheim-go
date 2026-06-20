# Intentional Breaks from the Ruby Implementation

The Go rewrite deliberately drops deprecated and legacy designs. Do not port these when implementing features.

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
