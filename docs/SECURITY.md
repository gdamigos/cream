# Security policy

cream holds every developer login on a machine, so security problems get top priority.

## Reporting a problem

Please don't open a public issue. Report it privately [here](https://github.com/gdamigos/cream/security/advisories/new) with your version (`cream version`), your OS and the steps to reproduce it.

## Supported versions

Only the latest minor release gets security fixes.

## What cream promises

Anything that breaks one of these is a security bug.

- **No telemetry.** The only request cream makes on its own is an optional update check, `GET https://api.github.com/repos/gdamigos/cream/releases/latest`, at most once a day. It only tells you about a new version and never downloads anything. Pressing `U` in the UI shows the update command for your package manager or opens the release page in your browser, and you run the update yourself. Turn it off with `update_check = false`, `--no-update-check` or `CREAM_NO_UPDATE_CHECK=1`. It is off in CI automatically.
- **The usage check is opt-in.** Only with `usage_check = true` does cream ask how much of their plan limits your Claude Code and Codex logins used: `GET https://api.anthropic.com/api/oauth/usage` and `GET https://chatgpt.com/backend-api/wham/usage`. Each login's token goes only to its own provider, in the `Authorization` header, and the answers are cached for 5 minutes in `usage.json`, which holds no tokens. cream never refreshes a token itself.
- **Secrets are stored safely.** They live in your OS keyring (Windows Credential Manager, macOS Keychain, Secret Service) or, when there is none, in a file encrypted with [age](https://age-encryption.org). cream never writes them in plain text to its own files.
- **Backups are encrypted.** Every backup of a credential file is encrypted with age and readable only by you.
- **A password protected profile stays private.** Without its password nobody using cream can switch to it, run tools as it, restore its backups or read its saved logins, accounts, git identity or rules. They are encrypted to a key that only the password opens ([how](HOW_IT_WORKS.md#password-protected-profiles)).
- **Secrets never show up** in output, logs, `config.toml` or command arguments. Keychain writes go through `security -i` on stdin, and tokens for `cream run` only go into the child process's environment.
- **Nothing runs in the background.** No daemon, no open ports, no scheduled tasks, no admin rights.
- **`--dry-run` works everywhere** and shows every file and keyring entry that would change.

## Known limits

- cream swaps the logins your tools already store. It doesn't change how Claude Code, Codex, gh or glab protect their own live credentials.
- It isn't a defence against someone who already has access to your user account.
- A protected profile's password guards its saved data, not its live session. While it is active its logins sit in the tools' own files like any other, and someone at the same account can still delete it or change the files by hand.

## Verifying releases

Release checksums are signed with Sigstore cosign, without a long-lived key, and every file has a GitHub build provenance attestation:

```sh
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/gdamigos/cream/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
sha256sum --ignore-missing -c checksums.txt
gh attestation verify cream_*_linux_amd64.tar.gz --repo gdamigos/cream
```

The same attestation is attached to each release as `cream_<version>.intoto.jsonl`, so you can check it offline with `gh attestation verify <file> --repo gdamigos/cream --bundle cream_<version>.intoto.jsonl`.
