# Using cream

Use the terminal UI or plain commands. Everything the UI does, a command does too.

## The terminal UI

Run `cream` with no arguments.

```
 ▄▀▀ █▀▄ █▀▀   ▄▀█   █▀▄▀█   CREdentiAls Master  v0.1.0
 ▀▄▄ █▀▄ ██▄ ▄ █▀█ ▄ █ ▀ █   active ● work  ·  OS keyring        one switch, every login▌
╭──────────────────────────╮╭──────────────────────────────────────────────────────────╮
│ Profiles                 ││  1 Status   2 Rules   3 Backups                          │
│                          ││                                                          │
│ ▌ ● personal             ││ TOOL            LIVE NOW              PERSONAL           │
│ ▌   5/5 tools · 1 rule   ││ Claude Code     ● cream@company.com   → cream@example.org│
│   ● work active          ││ Codex           ● cream@company.com   → cream@example.org│
│     5/5 tools · 1 rule   ││ GitHub CLI      ● cream-work          → cream            │
│                          ││ GitLab CLI      ● cream.work          → cream            │
│                          ││ Git identity    ● cream@company.com   ✓ cream@example.org│
│                          ││                                                          │
│                          ││ enter switch everything to personal · r run one tool     │
╰──────────────────────────╯╰──────────────────────────────────────────────────────────╯
 ✓ now using work
 enter switch · r run · i import · n new · e edit · a rule · u undo · D dry-run · ? help · q quit
```

| Key | What it does |
| --- | --- |
| up and down, `j` `k` or the mouse wheel | pick a profile |
| `enter` | switch every tool to the selected profile (shows a preview first) |
| `→` then `enter` | on the Status tab, pick one tool and switch only that tool |
| `→` then `i` | on the Status tab, pick one tool and save only that tool's current login |
| `→` then `x` | on the Status tab, pick one tool and forget the account the profile saved for it |
| `r` | run Claude Code, Codex, gh, glab or a shell as the profile, in this terminal only |
| `i` | save the accounts you are logged in to now into the profile |
| `n` `e` `x` | new, edit or delete a profile (deleting also removes its saved logins) |
| `a` | add a git identity rule (remote host or folder) |
| `L` / `ctrl+l` | lock or unlock the selected profile / everything |
| `U` | when a new version is out: the update command for how you installed cream (Scoop, Homebrew, `.deb`, `go install`), to copy, or the release page to open |
| `u` | undo the last switch |
| `tab` or `1` to `3` | Status, Rules and Backups tabs. Left and right move between the profile list and the tab. |
| `D` | dry-run mode: every action only shows what it would change |
| `R` `?` `q` | refresh now, help, quit |

The UI refreshes itself every few seconds and when its terminal gets focus, so logins you make in another terminal show up. `CREAM_NO_INTRO=1` skips the intro. Outside a terminal, `cream` prints the status instead.

## Commands

Every command accepts `--dry-run` (`-n`) and `--no-update-check`.

```text
cream                                   open the UI (prints the status when not in a terminal)
cream profile add <name> [--git-name … --git-email … --signing-key … -d …]
cream profile set <name> [--git-name … --git-email … --signing-key … -d …]
cream profile list
cream profile remove <name> --yes       also deletes its saved logins
cream profile forget <name> --only claude[,…] --yes   drop saved logins, keep the profile
cream import <profile> [--only claude,codex,github,gitlab,git] [--gitlab-host HOST]
cream use <profile> [--only …]          switch every tool
cream run <profile> <command> [args…]   run one tool as a profile, this terminal only
cream lock [profile]                    log tools out on purpose and turn git identity off
cream unlock [profile]                  put everything back
cream status [--json]                   who is logged in to each tool
cream rule add <profile> --remote HOST | --dir PATH
cream rule list
cream rule remove <profile> <n>
cream restore [backup-id]               undo the last switch (or go back to a given backup)
cream backups                           list the encrypted backups
cream version [--check]                 show the version, --check asks GitHub for a newer one
cream completion bash|zsh|fish|powershell
```

Exit codes are `0` for success and `1` for an error. `cream run` returns the exit code of the program it ran.

## Switching only some tools

A profile only switches the tools you saved into it. In the UI, `→` picks one tool for `enter`, `i` or `x`. On the command line, use `--only`:

```sh
cream import personal --only claude
cream use personal --only claude,codex
cream use work --only github,gitlab,git
```

## Switching everything vs. one terminal

- `cream use work` changes every tool's login files, so every new terminal and editor uses `work`.
- `cream run work claude` starts only that program as `work` and leaves everything else alone, so two accounts can run side by side. Put `--` before a command with its own flags: `cream run personal -- gh pr list`.

## Lock mode

`cream lock` logs Claude Code, Codex, gh and glab out and removes your git identity, so nothing can push or commit until `cream unlock`. Logins are saved to the vault first, so unlocking needs no new login.

`cream lock work` only logs out the tools signed in as work's accounts and turns work's git rules off. `work` can't be switched to or run until `cream unlock work`, while other profiles keep working. Unlocking a profile never overwrites a login you made in the meantime.

It's a guardrail against mistakes, not a security boundary, and sessions that are already running keep working until restarted.

## Undo

Every switch saves an encrypted backup first. `cream restore` goes back to the latest one, or `cream restore <id>` to one from `cream backups`. A restore can be undone too.

## Good to know

- Claude Code and Codex sessions that are already open keep the old account until you restart them.
- Headless Debian usually has no Secret Service, so cream uses the encrypted file vault and asks for a passphrase (or reads `CREAM_VAULT_PASSPHRASE`).
- glab tokens stored in the OS keyring instead of `config.yml` aren't supported yet.

See also [how it works](HOW_IT_WORKS.md) and [configuration](CONFIGURATION.md).
