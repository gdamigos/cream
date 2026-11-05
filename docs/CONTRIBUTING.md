# Contributing to cream

Thanks for your interest in cream (CRE.A.M, short for CREdentiAls Master). It's a small terminal tool written in Go that switches your CLI logins (Claude Code, Codex, GitHub CLI, GitLab CLI and your git identity) between profiles like `work` and `personal`. Bug reports, fixes, new adapters and feature ideas are all welcome.

## Reporting bugs

[Open an issue](https://github.com/gdamigos/cream/issues) and include:

- your OS (e.g. Windows 11, macOS 15, Debian 12) and how you installed cream (Scoop, winget, Homebrew, the `.deb`, a release zip or from source)
- the output of `cream version`
- the steps to reproduce, what you expected and what happened instead
- the output of `cream status`, and of the command with `--dry-run` if it is about a switch

Never paste tokens, credential files or anything from your vault. The output of `cream status` and `--dry-run` is safe to share.

Search the existing issues first so you don't file a duplicate.

## Suggesting features

Open an issue describing the problem the feature solves, not just the feature itself. Keep in mind what cream is: a small, local, security-first switch for CLI logins. It has no telemetry, no daemon and no cloud sync. Small, well-scoped ideas that fit that are much more likely to land than large ones. If you want a new tool supported, say where that tool keeps its login (files, keyring items, environment variables), since that is most of the work. For anything big (see the roadmap in the [README](../README.md#roadmap)), open an issue to discuss it before writing code.

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

To try the binary by hand without touching your real setup, point it at a temporary home first:

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

- **Keep pull requests small and focused.** One change at a time is far easier to review.
- **Explain why** the change is needed and link any related issue.
- **Run `go test ./...`** and make sure it passes. Add or update tests for behaviour you change. The tests use `testutil.NewEnv`, which gives you a temporary home, a fake command runner and an in-memory vault, so they never touch your real logins.
- **Test it for real**, not just in the tests. Use a throwaway `CREAM_HOME` and actually run the command or open the TUI and check the thing you touched works.
- **Follow the safety rules.** cream holds every login on a machine, so these are not optional:
  - no new network calls and no telemetry of any kind
  - secrets never go in command arguments, logs, normal output or `config.toml`
  - every file write goes through `fsutil.WriteFileAtomic`
  - every file an adapter touches is listed in its `Files()` so it gets backed up, and in [how it works](HOW_IT_WORKS.md)
  - `--dry-run` changes nothing and shows every change
  - ask in an issue before adding a new dependency
- **Match the surrounding style.** Run `gofmt` before committing. Comments are short, lower case and only where the code is hard to follow. Mirror what's already there.
- **Use a conventional PR title** like `feat: add gemini adapter` or `fix: keep glab comments`. The release notes are grouped from it.
- **Mark the PR as a draft** until you've confirmed it works, then mark it ready.

### Adding an adapter

An adapter teaches cream about one tool. Implement `adapter.Adapter` in `internal/adapters/<tool>.go`, add it to `adapters.All()`, write tests with `testutil.NewEnv` and add the files it touches to the table in [how it works](HOW_IT_WORKS.md). The contract for each method is described in `internal/adapter/adapter.go` and in `CLAUDE.md`.

### AI-assisted contributions

Using AI tools is fine, but you are responsible for what you submit. Read the [AI policy](AI-POLICY.md) before opening a PR. In short: say which tool you used, understand every line, and test it yourself.

## Releasing

This is for maintainers. Bump the number in [`VERSION`](../VERSION) (for example `0.1.0` to `0.2.0`) and merge it to `main`. The release workflow tags the commit and publishes everything. Don't create release tags by hand. The one-time GitHub setup is in [GITHUB_SETUP.md](GITHUB_SETUP.md).

## Questions

If you're unsure whether a change is wanted, open an issue and ask before putting in the work. Everyone taking part is expected to follow the [code of conduct](CODE_OF_CONDUCT.md).
