# Security policy

cream holds every developer login on a machine, so security problems get top priority.

## Reporting a problem

Please don't open a public issue. Report it privately [here](https://github.com/gdamigos/cream/security/advisories/new) with your version (`cream version`), your OS and the steps to reproduce it.

## Supported versions

Only the latest minor release gets security fixes.

## What cream promises

Anything that breaks one of these is a security bug.

- **No telemetry.** The only request cream makes on its own is an optional update check, `GET https://api.github.com/repos/gdamigos/cream/releases/latest`, at most once a day. It only tells you about a new version and never downloads anything. Pressing `U` in the UI shows the update command for your package manager or opens the release page in your browser, and you run the update yourself. Turn it off with `update_check = false`, `--no-update-check` or `CREAM_NO_UPDATE_CHECK=1`. It is off in CI automatically.
- **Secrets are stored safely.** They live in your OS keyring (Windows Credential Manager, macOS Keychain, Secret Service) or, when there is none, in a file encrypted with [age](https://age-encryption.org). cream never writes them in plain text to its own files.
- **Backups are encrypted.** Every backup of a credential file is encrypted with age and readable only by you.
- **Secrets never show up** in output, logs, `config.toml` or command arguments. Keychain writes go through `security -i` on stdin, and tokens for `cream run` only go into the child process's environment.
- **Nothing runs in the background.** No daemon, no open ports, no scheduled tasks, no admin rights.
- **`--dry-run` works everywhere** and shows every file and keyring entry that would change.

## Known limits

- cream swaps the logins your tools already store. It doesn't change how Claude Code, Codex, gh or glab protect their own live credentials.
- It isn't a defence against someone who already has access to your user account.

## Verifying releases

Release checksums are signed with Sigstore cosign, without a long-lived key, and every file has a GitHub build provenance attestation:

```sh
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/gdamigos/cream/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
sha256sum --ignore-missing -c checksums.txt
gh attestation verify cream_*_linux_amd64.tar.gz --repo gdamigos/cream
```
