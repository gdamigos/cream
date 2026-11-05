# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

CRE.A.M (CREdentiAls Master), binary `cream`: a Go + Bubble Tea TUI plus Cobra CLI that switches CLI logins (Claude Code, Codex, GitHub CLI, GitLab CLI, git identity) between named profiles. The root keeps only README, LICENSE, CLAUDE.md, VERSION, the Go entry point and the tool configs. Everything else that is prose lives in `docs/`.

## Git

Never run `git add`, `git commit` or `git push`. The user does all git writes.

## How to work

- Do what was asked and nothing more. Don't add features, options, flags, config settings or refactors nobody asked for. Mention a good idea instead of building it.
- Keep it simple. Prefer the smallest change that fits the existing code over a new abstraction, interface, helper package or dependency. Three similar lines are fine. Don't build for hypothetical future needs.
- Edit existing files rather than creating new ones, unless the change really needs a new file.
- Reuse what exists: `core.Manager` for every operation, `fsutil` for file writes, `testutil` for tests.
- New behaviour gets a test, and the invariants below must keep passing. Before saying something is done, run `go build ./...`, `go test ./... -count=1`, `golangci-lint run ./...` and `gofmt -l .`, and report failures as they are.
- If a request is unclear or would break an invariant, ask instead of guessing.

## Code style

- Follow the surrounding code. Errors are wrapped with `%w`, user-facing errors say how to fix the problem (e.g. which command to run).

## Documentation

The docs, all in `docs/`:

| File | Holds |
| --- | --- |
| `USAGE.md` | UI keys, every command, `--only`, `use` vs `run`, undo |
| `HOW_IT_WORKS.md` | the files each adapter touches, where cream keeps its files, switch safety, git rules |
| `CONFIGURATION.md` | `config.toml` settings and environment variables |
| `SECURITY.md` | the promises cream makes, reporting, verifying releases |
| `CONTRIBUTING.md`, `AI-POLICY.md`, `CODE_OF_CONDUCT.md` | project policies (kept in `docs/`, GitHub still finds them) |
| `GITHUB_SETUP.md` | one-time GitHub, package-manager and signing setup, for maintainers |
| `TODO.md` | what is left to do |

When to update them:

- A new or changed key, command or flag goes in `USAGE.md`. A new file an adapter touches goes in the `HOW_IT_WORKS.md` table. A new setting or env var goes in `CONFIGURATION.md`. Remove items from `TODO.md` when they are done.
- Update the doc in the same change as the code. Don't create a new doc when a section in an existing one fits. A new doc must be added to the "Documentation" table in `README.md`.

How to write them:

- Plain, friendly and short. Say what something does and how to use it, then stop. One sentence beats a paragraph, a table beats a list of sentences.
- Don't explain the obvious, repeat what another doc already says (link to it), justify design choices, or walk through scenarios step by step unless the flow is genuinely confusing.
- No emojis, no marketing tone, no AI filler ("seamlessly", "robust", "it's worth noting", "in summary").
- Call the project "cream", and mention "CRE.A.M (CREdentiAls Master)" only where the name is explained. Where `cream` is already taken (packages), use `cream-go`.

## Commands

Dev machine is Windows/PowerShell 5.1. Go, goreleaser and golangci-lint are installed via scoop. `go.mod` targets Go 1.26.0.

```powershell
go build ./...
go test ./... -count=1                                  # all tests (real `git` used by identity-rule tests)
go test ./internal/adapters -run TestGitRulesWithRealGit -count=1   # single test
golangci-lint run ./...                                 # v2 config in .golangci.yml (gosec, errorlint, goimports…)
gofmt -l .
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
goreleaser check
goreleaser release --snapshot --clean --skip=sign,sbom,docker,publish   # all platforms into ./dist
```

To try the binary without touching real state, point it at a throwaway home: `$env:CREAM_HOME="$env:TEMP\cream-try"; $env:CREAM_NO_UPDATE_CHECK="1"; $env:CREAM_NO_INTRO="1"`.

To dump TUI frames as plain text for inspection, set `$env:CREAM_SNAPSHOT_DIR=<dir>` and run `go test ./internal/tui -run TestSnapshot -count=1`.

PowerShell 5.1's `Set-Content -Encoding utf8` writes a BOM, so use the Edit/Write tools for source files. Quote flags with dots, e.g. `go mod edit "-go=1.26.0"`.

## Architecture

The flow is `main.go` → `internal/cli` (Cobra) → `internal/core.Manager` ← `internal/tui`. Both the CLI and the TUI do everything through `core.Manager`; neither calls an adapter directly. `main.go` injects the TUI into the CLI via `cli.App.RunTUI`, which avoids an import cycle. Bare `cream` opens the TUI on a TTY and prints the status otherwise.

- **`internal/adapter`** defines the contract.
  - `Adapter` methods: `Status` (read-only, no prompts, no network), `Import` (live login → vault secret + `config.Account` label), `Activate(dryRun)` returning `[]Change`, `Files()` (every file `Activate` may modify), `SessionEnv` (env vars plus an optional `finish` that syncs refreshed tokens back to the vault after `cream run`), and `Verify`.
  - `Snapshotter` covers non-file state such as the macOS Keychain.
  - `ErrNotImported` and `SkipError` make the engine skip an adapter instead of failing.
  - `Env` bundles Paths, Vault, Runner, GOOS, Config and Getenv. Tests replace every field, so adapters must read the environment via `env.Env()` / `env.OS()`, never `os.Getenv` / `runtime.GOOS`.
- **`internal/adapters`** holds the five MVP adapters; `All()` sets the display order.
  - GitHub stores no secret: it calls `gh auth switch` and reads the token via `gh auth token` stdout.
  - GitLab edits glab's `config.yml` as a yaml.v3 node tree so comments survive.
  - Git identity owns a delimited block at the end of `~/.gitconfig`; `RenderBlock`, `ReplaceBlock` and `Sync` live in `gitid.go`. Remote rules expand to several explicit `hasconfig:remote.*.url:` globs because `*` doesn't cross `/` there. `Fallback` decides the global identity: with no rules the active profile always is, and strict mode (the default) only removes it once a rule exists.
- The TUI's Status tab is focusable (`→`). `enter` and `i` on a tool row pass `only` to `Use` and `Import`, so a single tool can be switched or imported.
- **`internal/core`** orchestrates operations. `Use` runs in this order:
  1. `syncOut` re-imports the outgoing profile so refreshed tokens are saved.
  2. `backup.Capture` snapshots every adapter's `Files()` plus any Snapshotter state, encrypted.
  3. Each adapter's `Activate` runs.
  4. On any failure, `rollback` restores everything and the active profile stays unchanged.

  Progress is reported through the `Progress func(Event)` callback, which the TUI turns into messages.
- **`internal/vault`**:
  - `Keyring` (go-keyring) base64-encodes values and splits them into 1500-char chunks (`key#i`) to stay under the Windows Credential Manager and macOS `security -i` limits.
  - `File` is an age/scrypt-encrypted JSON map.
  - `Open` with backend `auto` probes the keyring and falls back to the file.
  - Key format: `profile:<name>:<adapter>`. The backup encryption identity is stored under `BackupIdentityKey`.
  - The Claude macOS Keychain item is written by the claude adapter itself through `security -i` stdin with `-X` hex. go-keyring would add its own encoding prefix, which Claude Code can't read.
- **`internal/backup`**: age X25519-encrypted snapshots in `<cream>/backups`, keeps the last 20, `RestoreFiles` deletes files that didn't exist at snapshot time.
- **`internal/tui`**: one `Model` with value semantics.
  - Forms (huh) bind to the heap-allocated `*formData` so their pointers stay valid when the model is copied.
  - Long operations run in a goroutine that feeds a channel, which is read with `waitOp`.
  - Animations are driven by a 30fps `frameMsg` tick: gradient logo, harmonica spring cursor, toasts.
  - Modals are composited onto a dimmed background by `overlay()`, and `clamp()` keeps every frame within the terminal size.
  - The big logo is the embedded `logo.txt`, an ANSI-stripped copy of `ASCII.txt`.
- **`internal/update`**: the only network call. GitHub Releases, cached for 24h, notify-only, disabled by config, flag, `CREAM_NO_UPDATE_CHECK` or CI env vars.
- **`internal/testutil`**: `NewEnv(t, goos)` gives an isolated temp home, an in-memory vault and a `FakeRunner` with canned outputs keyed by `"name arg1 arg2"`. `Passthrough` runs a binary (e.g. `git`) for real.

## Invariants that are enforced by tests

- Secrets never appear in argv, stdout, logs or `config.toml`. Pass them via stdin or the child process's environment.
- `--dry-run` changes nothing (no files, no backups) and reports every change.
- File writes go through `fsutil.WriteFileAtomic`, which preserves existing permissions.
- Every file an adapter touches must be in its `Files()` so it gets backed up; document new ones in the table in `docs/HOW_IT_WORKS.md`.
- No new network calls and no telemetry.
- All CRE.A.M state lives under `paths.Resolve()`, which honours `CREAM_HOME` and `XDG_CONFIG_HOME` and defaults to `~/.config/cream` on every OS.

## Release/packaging

- GoReleaser (`.goreleaser.yaml`) builds CGO-free and reproducible.
- Where the name `cream` is taken (the Debian package), the name `cream-go` is used instead.
- Scoop and Homebrew uploads are skipped when their token secrets are unset.
- Checksums are signed with cosign; GHCR images are built via `dockers_v2` and the `Dockerfile`.
- `VERSION` (plain semver, currently `0.1.0`) is the single source of truth. `main.go` embeds it, so dev builds report `v<VERSION>-dev`, and GoReleaser overrides it with `-X main.version=v{{.Version}}`.
- `release.yml` fires when `VERSION` changes on `main` (or on manual dispatch). If tag `v<VERSION>` doesn't exist, it tests on three OSes, creates the tag and runs GoReleaser in the same run, because tags pushed with `GITHUB_TOKEN` can't trigger other workflows. It deletes the tag if publishing fails. After a successful publish, `archive-old` renames every stable release that isn't the newest patch of its `major.minor` to `[ARCHIVED] vX.Y.Z` and marks it a pre-release.
- Never create release tags manually; bump `VERSION` instead.
- Release notes come from three places:
  - GitHub's native generated notes (`changelog.use: github-native`), with categories from `.github/release.yml`.
  - PR labels set from conventional PR titles by `pr-labels.yml`, which decide those categories.
  - A collapsible `vPREV...vNEW` commit list, passed to the GoReleaser footer through the `RELEASE_COMMITS` env var.
- The annotated tag message lists the same commit range.
- `ci.yml` runs tests on three OSes, lint (including a VERSION semver check), govulncheck and a snapshot build.