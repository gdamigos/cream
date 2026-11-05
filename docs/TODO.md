# TODO

Ideas and known problems in no particular ((execution)) order.

## Improvements

- [ ] `cream doctor` to check each tool's setup and the current repo's author email against the profile that owns the remote
- [ ] Optional pre-push check that the commit identity matches the remote
- [ ] Signing key per profile (GPG or SSH)
- [ ] SSH keys per profile (host aliases in `~/.ssh/config`, loaded into ssh-agent)
- [ ] Adapters for Gemini CLI, GitHub Copilot CLI and OpenCode
- [ ] API key profiles (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, ...) with `cream env work`
- [ ] Lock mode: `cream lock` and `cream unlock` stop git and the CLIs from working, for everything or one profile
- [ ] Warnings before tokens expire
- [ ] Claude Code and Codex usage display, and switching accounts on a rate limit
- [ ] Per-folder `.cream` files that pick a profile
- [ ] npm and PyPI tokens
- [ ] Cloud CLIs: AWS, gcloud, Azure, kubectl
- [ ] Encrypted export and import of profiles between machines
- [ ] Token rotation
- [ ] Scoop bucket and Homebrew tap
- [ ] winget and an apt repository
- [ ] macOS notarisation and Windows code signing
- [ ] CI coverage for the real keyring backends on macOS and Linux Secret Service
- [ ] Check with a network monitor that cream makes no request besides the update check
- [ ] User-defined adapters

## Bugs

- [ ] glab tokens stored in the OS keyring instead of `config.yml` aren't supported
- [ ] An empty update check result (before the first release exists) is cached for 24 hours
- [ ] `x/crypto` is pinned to a fix commit instead of a tagged release
