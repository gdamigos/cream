# Using cream

You can use cream in two ways: the full-screen terminal UI, or plain commands. Everything the UI can do, a command can do too, so you can script it.

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
| `u` | undo the last switch |
| `tab` or `1` to `3` | Status, Rules and Backups tabs. Left and right move between the profile list and the tab. |
| `D` | dry-run mode: every action only shows what it would change |
| `R` `?` `q` | refresh now, help, quit |

The UI keeps itself up to date. It re-reads every tool every few seconds and whenever its terminal gets focus again, and it picks up changes made by `cream` commands in other terminals.

Set `CREAM_NO_INTRO=1` to skip the intro animation. When cream isn't running in a terminal (for example in a pipe), `cream` prints the status instead of opening the UI.

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

You don't have to use every tool. A profile only switches the tools you saved into it, and the rest are left alone. Some examples:

- **Only git accounts.** You don't use Claude Code or Codex. Log in to gh, glab and set your git identity, save them into `work` and `personal`, and `enter` switches just those.
- **One git account, several Claude or Codex accounts.** In the UI, select a profile, press `→` to move into the Status list, pick Claude Code and press `i` to save only that login. Later, `→`, pick Claude Code and `enter` switches only Claude Code, and your git setup stays as it is.

From the command line the same thing is `--only`:

```sh
cream import personal --only claude
cream use personal --only claude,codex
cream use work --only github,gitlab,git
```

## Switching everything vs. one terminal

- `cream use work` changes the login files of every tool, so every new terminal and editor uses `work`.
- `cream run work claude` leaves everything else alone and starts only that one program as `work`. This lets you use two accounts side by side. Anything after the profile name is the command to run, and you can put `--` before it when the command has its own flags: `cream run personal -- gh pr list`.

A program can't change the environment of the shell that started it, which is why there are two modes.

## Undo

Every switch saves an encrypted backup first. `cream restore` goes back to the latest one, and `cream backups` lists them all so you can pick one with `cream restore <id>`. A restore saves a backup of the current state too, so you can undo it.

## Good to know

- Claude Code and Codex sessions that are already open keep the old account until you restart them.
- Headless Debian usually has no Secret Service, so cream uses the encrypted file vault and asks for a passphrase (or reads `CREAM_VAULT_PASSPHRASE`).
- glab tokens stored in the OS keyring instead of `config.yml` aren't supported yet.

See also [how it works](HOW_IT_WORKS.md) and [configuration](CONFIGURATION.md).
