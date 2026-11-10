# How cream works

Each tool is handled by an **adapter** that knows where that tool keeps its login and how to swap it safely. A **profile** (like `work` or `personal`) holds one saved account per adapter.

## What each adapter touches

| Adapter | What a switch changes | `cream run` | Stored in the vault as |
| --- | --- | --- | --- |
| **Claude Code** | Windows and Linux: `~/.claude/.credentials.json` and the `oauthAccount` part of `~/.claude.json` (everything else in it is kept). macOS: the Keychain item `Claude Code-credentials` and `~/.claude.json`. Honours `CLAUDE_CONFIG_DIR`. | `CLAUDE_CONFIG_DIR=<cream>/sessions/<profile>/claude`. Refreshed tokens are saved back when the session ends. | `profile:<name>:claude` |
| **Codex** | `$CODEX_HOME/auth.json` (default `~/.codex`). Needs `cli_auth_credentials_store = "file"` in Codex's `config.toml`, and `cream status` warns if it is missing. | `CODEX_HOME=<cream>/sessions/<profile>/codex` | `profile:<name>:codex` |
| **GitHub CLI** | Runs `gh auth switch --hostname H --user U`, using gh's own support for several accounts. cream stores no GitHub secret, except for [password protected profiles](#password-protected-profiles). `hosts.yml` is backed up. | `GH_TOKEN`, read from `gh auth token` | nothing, or `profile:<name>:github` for a protected profile that isn't active |
| **GitLab CLI** | The `hosts.<host>` part of glab's `config.yml`, edited in place so your comments are kept | `GITLAB_TOKEN`, `GITLAB_HOST` | `profile:<name>:gitlab` |
| **Git identity** | A marked block at the end of `~/.gitconfig` (or `$GIT_CONFIG_GLOBAL`) plus `<cream>/git/<profile>.gitconfig`. Nothing outside the block is touched. | nothing, the identity follows the switch or the repo's rules | nothing |

## Where cream keeps its files

Everything lives in **`~/.config/cream/`** on every OS. You can move it with `CREAM_HOME` or `XDG_CONFIG_HOME`.

```text
~/.config/cream/
├── config.toml          profiles, account names, rules and settings, never secrets
├── backups/*.age        encrypted backups, only readable by you, the last 20 are kept
│                        (*~<profile>.age: only that protected profile's password opens it)
├── git/<profile>.gitconfig
├── sessions/<profile>/  separate homes for `cream run`
├── vault.age            only when there is no OS keyring
├── update-check.json    the last update check result
├── usage.json           the last usage check results, no tokens (only with usage_check = true)
└── cream.log            a local activity log without secrets
```

Secrets are stored in your OS keyring (Windows Credential Manager, macOS Keychain or Secret Service). When there is no keyring, they go into `vault.age`, a file encrypted with [age](https://age-encryption.org) and a passphrase.

## How a switch stays safe

1. Tokens that the outgoing profile refreshed are saved back first, so switching back later never uses an old token.
2. Every file the adapters might touch is saved into an **encrypted backup**.
3. Each adapter switches its tool to the new account.
4. If any step fails, **everything is put back** from the backup and the old profile stays active. `cream restore` undoes a switch that worked.

The UI does step 1 for the active profile too, when its terminal loses focus and when you quit, so logging out of a tool elsewhere doesn't leave the profile with an old token.

## Lock mode

`cream lock` takes a backup, saves each live login to the vault as `lock:*:<tool>` (or `lock:<profile>:<tool>`), then logs the tool out:

| Tool | Lock | Unlock |
| --- | --- | --- |
| Claude Code | deletes the credentials and the `oauthAccount` block | writes them back |
| Codex | deletes `auth.json` | writes it back |
| GitHub CLI | reads each token with `gh auth token`, then `gh auth logout` (every account, or only the profile's) | `gh auth login --with-token` with the token on stdin, then `gh auth switch` |
| GitLab CLI | removes the `token` from that host's entry in `config.yml` | puts the saved entry back |
| Git identity | drops the locked profiles' rules from the managed block, and the global identity when everything is locked | puts them back |

If any tool fails to lock, everything already locked is put back. The lock state is kept in the `[lock]` section of `config.toml`.

## Git identity

With no rules, git identity simply follows the switch: `cream use personal` makes the personal name and email your global git identity, and `cream use work` changes it back.

## Git identity rules

If you'd rather have each repo pick its own identity, add rules. `cream rule add` writes a marked block like this into `~/.gitconfig`:

```gitconfig
# >>> cream managed block (do not edit; run `cream rule`) >>>
[user]
	useConfigOnly = true
[includeIf "hasconfig:remote.*.url:https://gitlab.company.com/**"]
	path = "~/.config/cream/git/work.gitconfig"
[includeIf "hasconfig:remote.*.url:*@gitlab.company.com:*/**"]
	path = "~/.config/cream/git/work.gitconfig"
[includeIf "gitdir:~/code/personal/"]
	path = "~/.config/cream/git/personal.gitconfig"
# <<< cream managed block <<<
```

A repo with a work GitLab remote **always** commits as work, whichever profile is active. Once you have rules, a repo that matches none of them **can't commit** at all, so you never commit with the wrong email by accident. This is strict mode, and it is on by default. Remote rules need git 2.36 or newer.

If you also set `user.email` in your own `~/.gitconfig`, it defeats strict mode and `cream status` warns you. If you'd rather keep a global fallback identity even with rules, set `[git] strict = false` (see [configuration](CONFIGURATION.md)) and the active profile's identity is used for repos that match no rule.

## Network and privacy

No telemetry and nothing in the background. By default the only network call is an optional daily update check. cream only talks to Anthropic and OpenAI when you turn on the usage check (`usage_check = true`), and never to GitHub or GitLab itself. Details are in [SECURITY.md](SECURITY.md).
