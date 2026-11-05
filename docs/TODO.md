# TODO

What is left to do, roughly in order. The bigger picture is in the [roadmap](../README.md#roadmap).

## Scorecard

- [ ] Move `x/crypto` from the pinned fix commit to the next tagged release once it ships

## Packaging

- [ ] Create `gdamigos/homebrew-tap` and `gdamigos/scoop-bucket`, add their tokens
- [ ] Fork `winget-pkgs` and add `WINGET_GITHUB_TOKEN`
- [ ] macOS signing and notarisation (needs an Apple Developer account)
- [ ] Windows code signing (SignPath, Azure Trusted Signing or a certificate)
- [ ] An apt repository so `apt install cream-go` works with updates
- [ ] Codecov badge and PR comments

## Known gaps

- [ ] glab tokens kept in the OS keyring instead of `config.yml` aren't supported
- [ ] Tests for the real keyring backends only run where a keyring exists, add CI coverage for macOS and Linux Secret Service

## Next features

- [ ] Lock mode: `cream lock` and `cream unlock`
- [ ] `cream doctor` to check every tool's setup
- [ ] Optional pre-push check that the commit identity matches the remote
- [ ] Adapters for Gemini CLI, GitHub Copilot CLI and OpenCode
- [ ] API key profiles with `cream env`
- [ ] SSH keys and signing keys per profile
- [ ] Per-folder `.cream` files that pick a profile
- [ ] npm and PyPI registry tokens
- [ ] Warnings before tokens expire

## Later

- [ ] Cloud CLIs: AWS, gcloud, Azure, kubectl
- [ ] Token rotation
- [ ] A way for people to write their own adapters
