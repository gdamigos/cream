<div align="center">

```
    ▄▄▄▄▄▄     ▄▄▄ ▄▄▄▄▄▄         ▄▄▄▄▄▄         ▄▄▓▄▄▄     ▄▄▄ ▄▄▄   ▄▄
 ▄███▀░░▀▓██▄  █████▀░░▀▓██▄   ▄███▀░░▀▓██▄   ▄███▀░ ▀███▄  █████▀░█▄█▀▓██▄
▐▓▓█▌    ▐█▓█▌ █▓▓█▌    ▐█▓█▌ ▐▓▓█▌    ▐█▓█▌ ▐▓▓█▌    ▐█▓█▌ █▓▓█▌ ▐▓▓▌ ▐█▓█▌
▓░░▓      ▓░▓█ ▓░░▓      ▓░▓█ ▓░░▓      ▓░▓█ ▓░░▓      ▓░▓█ ▓░░▓  ▐▒▓▌  ▓░▓█
░██░           ░██░      ░█░▓ ░██░▄          ░██░      ░█░▓ ░██░  ▐░░▌  ░█░▓
▓███           ▓███       ▀▀▀ ▓█████░▒▓█     ░░██      ░░█░ ▓███   ▀▀   ██░░
███░           ███░           ███░▀          ▓▓░░██▓▓░░▓▓░░ ███░       ░▓░▓░
░██▓      ░▓█▓ ░██▓           ░██▓      ░▓█▓ ██▓▓  ░░ ░▓█▓▓ ░██▓        ░▓▒█
▓░░█      ▓██▓ ░░░░           ▓░░█      ▓██▓ ▓███      ▓██▓ ░░░░        ▒▓█▓
▐▓▓█▌    ▐██▓▌ ▒▒▒▒           ▐▓▓█▌    ▐██▓▌ ▓▓▓█      ▒▓▓░ ▒▒▒▒        ▓█▓░
 ▀▓▓█▄░░▄█▓▓▀  █▓▓█            ▀▓▓█▄░░▄█▓▓▀  ▓██▓      ░▒▒▒ █▓▓█        ░▒▒▒
  ░░▀▀▀▀▀▀░░    ▓▓              ░░▀▀▀▀▀▀░░    ▓▓       ░  ░  ▓▓         ░  ░
```

### cream

**One switch for every CLI login you have.**
Move Claude Code, Codex, GitHub, GitLab and your git identity between `work` and `personal` with one key.

[![CI](https://github.com/gdamigos/cream/actions/workflows/ci.yml/badge.svg)](https://github.com/gdamigos/cream/actions/workflows/ci.yml)
[![CodeQL](https://github.com/gdamigos/cream/actions/workflows/codeql.yml/badge.svg)](https://github.com/gdamigos/cream/actions/workflows/codeql.yml)
[![Release](https://img.shields.io/github/v/release/gdamigos/cream?sort=semver)](https://github.com/gdamigos/cream/releases)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/gdamigos/cream/badge)](https://scorecard.dev/viewer/?uri=github.com/gdamigos/cream)
[![Go Reference](https://pkg.go.dev/badge/github.com/gdamigos/cream.svg)](https://pkg.go.dev/github.com/gdamigos/cream)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

---

cream is short for **CRE.A.M**, the **CREdentiAls Master**. The command is `cream`.

## Why

If you have a work account and a personal one, you end up logging out and back in to half a dozen CLI tools every time you switch. Each tool keeps its login somewhere different, and none of them agree on who you are right now. On top of that, your git commit identity is separate from your login. You can push to the work GitLab with your work account while every commit carries your personal email.

cream groups one account per tool into a **profile** and switches all of them at once. You can do it from a full-screen terminal UI or from plain commands, on Windows, macOS and Debian. It is built to be safe: no telemetry, nothing running in the background, secrets in your OS keyring, and an encrypted backup before every change.

## Features

- **Full-screen terminal UI** with live status for every tool, step-by-step progress when switching, and a dry-run mode
- **Switch everything at once.** `cream use work` changes every tool for every new terminal and editor.
- **Or just one tool.** Only use git? Only switching Claude accounts? A profile only touches the tools you saved into it, and in the UI you can pick a single tool to switch.
- **Run one tool as another profile.** `cream run work claude` runs Claude Code as `work` in this terminal only, so two accounts can be used side by side.
- **Import once.** cream saves the account you are logged in to right now, so you never have to log in again.
- **Git identity that follows you.** Your git name and email switch with the profile. Or add rules so each repo picks its identity by folder or remote host, and repos that match no rule can't commit with the wrong email.
- **Safe storage.** Windows Credential Manager, macOS Keychain or Secret Service, with an [age](https://age-encryption.org) encrypted file when there is no keyring.
- **Backup and undo.** An encrypted backup is taken before every switch. If anything fails, everything is put back. `cream restore` undoes a switch.
- **`--dry-run` on every command** shows each file and keyring entry that would change.
- **No telemetry.** The only network call is an optional check for new versions.

## Install

| Platform | Command |
| --- | --- |
| **Windows** (Scoop) | `scoop bucket add gdamigos https://github.com/gdamigos/scoop-bucket` then `scoop install cream` |
| **Windows** (winget) | `winget install gdamigos.cream` |
| **macOS** (Homebrew) | `brew install --cask gdamigos/tap/cream` |
| **Debian / Ubuntu** | download `cream-go_<version>_<arch>.deb` from [Releases](https://github.com/gdamigos/cream/releases), then `sudo apt install ./cream-go_*.deb` |
| **Go** | `go install github.com/gdamigos/cream@latest` |
| **Binary** | download the zip or tar.gz for your OS from [Releases](https://github.com/gdamigos/cream/releases) (`cream.exe` on Windows) |
| **Container** | `docker run --rm ghcr.io/gdamigos/cream:latest --help` (for scripts, the UI needs a real terminal) |

The Debian package is called `cream-go` because Debian already has an unrelated package named `cream`. The command you run is still `cream`.

To check that a download is genuine, see [verifying releases](docs/SECURITY.md#verifying-releases).

## Quick start

```sh
# 1. create your profiles (the git identity is optional and can be added later)
cream profile add work     --git-name "Cream Work" --git-email cream@company.com
cream profile add personal --git-name "Cream"      --git-email cream@example.org

# 2. log in to your tools as WORK the normal way
#    (claude /login, codex login, gh auth login, glab auth login), then save them
cream import work

# 3. log in as PERSONAL and save again
cream import personal

# 4. from now on, switch everything with one command
cream use work
cream use personal

# or run a single tool as another profile, in this terminal only
cream run work claude

# 5. let each repo pick its git identity
cream rule add work     --remote gitlab.company.com
cream rule add personal --dir ~/code/personal
```

Or just run **`cream`** and do all of this from the terminal UI.

## Documentation

| Guide | What is in it |
| --- | --- |
| [Using cream](docs/USAGE.md) | the terminal UI and its keys, every command, undo |
| [How it works](docs/HOW_IT_WORKS.md) | what each tool's adapter changes, where files live, how switches stay safe, git rules |
| [Configuration](docs/CONFIGURATION.md) | `config.toml` and environment variables |
| [Security](docs/SECURITY.md) | what cream promises, reporting a problem, verifying downloads |
| [Contributing](docs/CONTRIBUTING.md) | reporting bugs, setting up, sending changes, adding a tool |
| [AI policy](docs/AI-POLICY.md) | rules for AI-assisted contributions |
| [Code of conduct](docs/CODE_OF_CONDUCT.md) | how we treat each other |
| [GitHub setup](docs/GITHUB_SETUP.md) | releases, packages and signing, for maintainers |
| [TODO](docs/TODO.md) | what is left to do |

## Roadmap

This first version covers the essentials. Next up: a lock mode (`cream lock` and `cream unlock`), `cream doctor` and an optional pre-push check, adapters for Gemini, Copilot and OpenCode, API key profiles (`cream env`), SSH and signing keys per profile, per-folder `.cream` files, npm and PyPI tokens, and warnings before tokens expire. Later: cloud CLIs (AWS, gcloud, Azure, kubectl), token rotation and a way to write your own adapters.

**Not planned:** telemetry, self-updating, background daemons, cloud sync, a GUI, or automating interactive logins.

The detailed list is in [TODO.md](docs/TODO.md).

## Contributing

Bug reports, fixes and new adapters are welcome. Start with the [contributing guide](docs/CONTRIBUTING.md), and if you use AI tools, read the [AI policy](docs/AI-POLICY.md) first.

## License

[MIT](LICENSE), Giorgos Damigos
