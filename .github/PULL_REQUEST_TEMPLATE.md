## What and why

<!-- the title must be a conventional commit, like feat: fix: sec: docs: chore: -->

## How I tested it

<!-- the commands you ran, their output or a screenshot -->

## AI usage

<!-- which tool you used and for what, or "none". see docs/AI-POLICY.md -->

## Checklist

- [ ] `go test ./...` passes and tests were added or updated
- [ ] I ran the change for real with a throwaway `CREAM_HOME`
- [ ] No new network calls, telemetry or dependencies (or discussed in an issue first)
- [ ] Secrets never reach command arguments, logs, output or `config.toml`
- [ ] Files touched by an adapter are listed in its `Files()` and in docs/HOW_IT_WORKS.md
- [ ] `--dry-run` shows every change and changes nothing
- [ ] I follow the [AI policy](../docs/AI-POLICY.md)
