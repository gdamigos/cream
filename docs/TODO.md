# TODO

Ideas and known problems in no particular ((execution)) order.

## Improvements

- [ ] Optional pre-push check that the commit identity matches the remote
- [ ] Signing key per profile (GPG or SSH)
- [ ] SSH keys per profile (host aliases in `~/.ssh/config`, loaded into ssh-agent)
- [ ] Adapters for Gemini CLI, GitHub Copilot CLI and OpenCode
- [ ] API key profiles (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, ...) with `cream env work`
- [ ] Per-folder `.cream` files that pick a profile
- [ ] npm and PyPI tokens
- [ ] Cloud CLIs: AWS, gcloud, Azure, kubectl
- [ ] Encrypted export and import of profiles between machines
- [ ] Token rotation
- [ ] winget repository
- [ ] apt repository
- [ ] macOS notarisation and Windows code signing
- [ ] CI coverage for the real keyring backends on macOS and Linux Secret Service
- [ ] User-defined adapters

## Bugs

- [ ] glab tokens stored in the OS keyring instead of `config.yml` aren't supported
- [ ] An empty update check result (before the first release exists) is cached for 24 hours
- [ ] `x/crypto` is pinned to a fix commit instead of a tagged release
- [ ] some times claude defaults to API billing after switch (instead of Pro/Max/Team)
