# Configuration

cream manages `~/.config/cream/config.toml` for you through its commands and the UI, so you rarely need to edit it. It never contains secrets and is safe to read.

```toml
version = 1
active = "work"
update_check = true     # at most once a day, only tells you about new versions
usage_check = false     # opt-in: ask Anthropic and OpenAI for your Claude Code and Codex plan usage
stale_login_days = 30   # warn about saved logins not refreshed for this long, 0 turns it off
log = true              # local activity log, never contains secrets
adapters = ["claude", "codex", "github", "gitlab", "git"]

[ui]
intro = true
dynamic_title = true
disabled_keys = []
click_twice = true

[vault]
backend = "auto"        # auto, keyring or file

[git]
strict = true           # with rules, repos that match none of them can't commit

[profiles.work]
description = "day job"
color = "#4FB3A9"
[profiles.work.git]
name = "Cream Work"
email = "cream@company.com"
[[profiles.work.rules]]
kind = "remote"
pattern = "gitlab.company.com"
[profiles.work.accounts.claude]
label = "cream@company.com"
```

A full example with every setting is in [`examples/config.toml`](../examples/config.toml).

## Settings

| Setting | Default | What it does |
| --- | --- | --- |
| `update_check` | `true` | check GitHub for a newer version, at most once a day |
| `usage_check` | `false` | ask Anthropic and OpenAI how much of their plan limits your saved Claude Code and Codex logins used, for `cream usage`, the UI and the switch hint on a rate limit. Sends each login's token to its own provider, see [SECURITY.md](SECURITY.md). |
| `stale_login_days` | `30` | warn about a saved login of another profile that hasn't been refreshed for this many days. `0` turns the warning off. |
| `log` | `true` | keep a local activity log in `cream.log` |
| `adapters` | every tool | the tools cream manages. Remove one and cream ignores it everywhere: it is never switched, locked, backed up or checked. |
| `ui.intro` | `true` | show the intro animation |
| `ui.dynamic_title` | `true` | show the active profile and what cream is doing in the terminal tab title |
| `ui.disabled_keys` | `[]` | keys that do nothing on the main screen, e.g. `["x", "ctrl+l"]`. They also leave the footer. |
| `ui.click_twice` | `true` | clicking a key shown on screen puts a box around it and a second click runs it. Set it to `false` to run it on the first click. |
| `vault.backend` | `auto` | `auto` uses the OS keyring when there is one and the encrypted file otherwise. `keyring` and `file` force one. |
| `git.strict` | `true` | once you have identity rules, repos that match none of them can't commit. Set it to `false` to keep the active profile as a fallback identity. With no rules the active profile is always the global identity. |
| `lock.all`, `lock.profiles` | empty | set by `cream lock` and cleared by `cream unlock`. Don't edit them by hand, the saved logins are in the vault. |

## Environment variables

| Variable | What it does |
| --- | --- |
| `CREAM_HOME` | where cream keeps its files (default `~/.config/cream`) |
| `XDG_CONFIG_HOME` | when set, cream uses `$XDG_CONFIG_HOME/cream` |
| `CREAM_NO_UPDATE_CHECK=1` | turns off the update check (it is also off in CI) |
| `CREAM_VAULT_PASSPHRASE` | passphrase for the encrypted file vault, for scripts |
| `CREAM_NO_INTRO=1` | skips the intro animation |
| `CREAM_PROFILE` | set by `cream run` inside the program it starts |

See also [using cream](USAGE.md) and [how it works](HOW_IT_WORKS.md).
