# Contributing to cream

Bug reports, fixes, new adapters and feature ideas are all welcome.

## Reporting bugs

[Open an issue](https://github.com/gdamigos/cream/issues) with your OS, how you installed cream, `cream version`, the steps to reproduce, and the output of `cream status` (plus `--dry-run` for a switch). Those outputs are safe to share. Never paste tokens or credential files.

## Suggesting features

Describe the problem, not just the feature. cream is a small, local, security-first tool, so small ideas that fit that land more easily. For a new tool, say where it keeps its login, since that is most of the work. Discuss anything big in an issue before writing code.

## Setting up

You need [Go](https://go.dev/dl/) 1.26 or newer and git. [golangci-lint](https://golangci-lint.run) v2 is needed for linting.

```sh
git clone https://github.com/gdamigos/cream.git
cd cream
go build ./...         # build everything
go test ./...          # all tests (the git identity tests run real git)
golangci-lint run      # lint
```

With `make` (Linux, macOS, or Git Bash on Windows) there are shortcuts:

```sh
make test              # all tests
make lint              # golangci-lint
make vuln              # govulncheck
make run               # the UI against a throwaway home, your real logins are untouched
make snapshot          # build every platform into ./dist with GoReleaser
```

To try it without touching your real setup:

```sh
export CREAM_HOME="$(mktemp -d)" CREAM_NO_UPDATE_CHECK=1
go run . status
```

On Windows PowerShell use `$env:CREAM_HOME="$env:TEMP\cream-try"` instead.

## Project layout

```text
main.go                   entry point, embeds VERSION
internal/cli              commands
internal/tui              terminal UI
internal/core             import, switch and rollback, restore, sessions, rules
internal/adapters         one file per tool: Claude Code, Codex, GitHub, GitLab, git identity
internal/adapter          what every adapter has to implement
internal/vault            OS keyring and the encrypted file vault
internal/backup           encrypted backups
internal/update           the update check
internal/testutil         fake runner and temporary environment for tests
docs/                     guides and project policies
.github/                  CI, release workflows and templates
```

[`CLAUDE.md`](../CLAUDE.md) at the repo root describes the architecture and the rules the code has to follow. Read it before a larger change. [How it works](HOW_IT_WORKS.md) lists every file each adapter touches.

## Making changes

- **Keep pull requests small**, and explain why the change is needed.
- **Add tests** for behaviour you change. `testutil.NewEnv` gives you a temporary home, a fake runner and an in-memory vault.
- **Test it for real** with a throwaway `CREAM_HOME`, not just in the tests.
- **Follow the safety rules.** They are not optional:
  - no new network calls and no telemetry of any kind
  - secrets never go in command arguments, logs, normal output or `config.toml`
  - every file write goes through `fsutil.WriteFileAtomic`
  - every file an adapter touches is listed in its `Files()` so it gets backed up, and in [how it works](HOW_IT_WORKS.md)
  - `--dry-run` changes nothing and shows every change
  - ask in an issue before adding a new dependency
- **Match the surrounding style** and run `gofmt`. Comments are short, lower case and only where the code is hard to follow.
- **Use a conventional PR title** like `feat: add gemini adapter`. The release notes are grouped from it.
- Using AI? Read the [AI policy](AI-POLICY.md) first.

### Adding an adapter

Implement `adapter.Adapter` in `internal/adapters/<tool>.go`, add it to `adapters.All()`, write tests with `testutil.NewEnv` and list the files it touches in [how it works](HOW_IT_WORKS.md).

## Releasing

Maintainers bump [`VERSION`](../VERSION) and merge to `main`, and the release workflow does the rest. Never tag by hand. One-time setup is in [GITHUB_SETUP.md](GITHUB_SETUP.md).

Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).
