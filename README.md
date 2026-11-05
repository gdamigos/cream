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

With a work and a personal account, switching means logging out and back in to every CLI tool, and your git commit email doesn't follow along. cream groups one account per tool into a **profile** and switches them all at once, on Windows, macOS and Debian.

## Features

- **Terminal UI or plain commands.** Everything the UI does, a command does too.
- **Switch everything or one tool.** `cream use work`, or pick a single tool in the UI.
- **Run one tool as another profile** in this terminal only: `cream run work claude`.
- **Import once.** cream saves the login you have now, so you never log in twice.
- **Git identity that follows you**, or rules that pick it per repo by folder or remote host.
- **Safe.** Secrets in your OS keyring, an encrypted backup before every change, rollback on failure, `--dry-run` everywhere, no telemetry.

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
| [Contributing](docs/CONTRIBUTING.md) | reporting bugs, setting up, sending changes, adding a tool, releasing |
| [AI policy](docs/AI-POLICY.md) | rules for AI-assisted contributions |
| [Code of conduct](docs/CODE_OF_CONDUCT.md) | how we treat each other |
| [TODO](docs/TODO.md) | what is left to do |

## Contributing

Bug reports, fixes and new adapters are welcome. See the [contributing guide](docs/CONTRIBUTING.md) and the [AI policy](docs/AI-POLICY.md).

## License

[MIT](LICENSE), Giorgos Damigos
