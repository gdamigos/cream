# GitHub setup, releases and packages

For maintainers. The repo files are in place, so what's left is one-time settings and secrets.

| What | Works right away | Needs setup |
| --- | --- | --- |
| CI (tests on 3 OSes, lint, govulncheck, snapshot build) | yes | |
| Automatic releases when `VERSION` changes | yes | |
| Windows zip (`cream.exe`), macOS and Linux archives, `.deb` | yes | |
| Signed checksums, SBOMs, build provenance | yes | |
| Container image on ghcr.io | yes | make the package public ([section 4](#github-packages-ghcrio)) |
| CodeQL, dependency review, Scorecard, PR title check | yes | |
| Dependabot updates | yes | turn on alerts ([section 2](#2-repository-settings)) |
| Homebrew tap | | repo and secret ([section 5](#5-homebrew-tap)) |
| Scoop bucket | | repo and secret ([section 6](#6-scoop-bucket)) |
| winget | | fork and secret ([section 7](#7-winget)) |
| macOS signing and notarisation | | Apple Developer account and secrets ([section 8](#8-macos-signing-and-notarisation)) |
| Windows code signing | | a certificate or signing service ([section 9](#9-windows-code-signing)) |
| Branch protection | | a ruleset ([section 3](#3-branch-protection)) |
| Coverage reports (Codecov) | | optional ([section 10](#10-coverage)) |

---

## 1. First push

```sh
git push -u origin main
```

CI runs and uploads binaries for every platform on the run page. Release runs too, so **the first push publishes v0.1.0**.

## 2. Repository settings

**Settings, General**

- Features: turn on *Issues*. *Discussions* is optional.
- Pull Requests: turn on *Allow squash merging* and set its default message to *Pull request title*. PR titles are checked to be conventional commits, so squash commits stay clean and the release notes are grouped correctly.

**Settings, Actions, General**

- *Workflow permissions*: **Read repository contents** (the default). Each workflow asks for the extra permissions it needs.

**Settings, Code security**

- **Dependabot alerts** and **Dependabot security updates**: turn them on. Weekly version updates are already set up in `.github/dependabot.yml`.
- **Code scanning**: leave the CodeQL *default setup* **off**. The repo has its own `codeql.yml` and the two would clash.
- **Secret scanning** and **push protection**: turn both on.

**Scorecard**: `scorecard.yml` publishes its results, which only works for **public** repositories. For a private repo, set `publish_results: false` or delete the workflow.

## 3. Branch protection

Under Settings, Rules, Rulesets, create a **branch ruleset** for the default branch:

- Restrict deletions and block force pushes.
- Require a pull request before merging (0 or 1 approvals for a solo project).
- Require status checks to pass. Add `test (ubuntu-latest)`, `test (windows-latest)`, `test (macos-latest)`, `lint`, `vulncheck`, `snapshot build (all platforms)`, `analyze` and `semantic`. The names only show up in the picker after the checks have run once.
- Optionally require signed commits.

If you add a **tag ruleset** for `v*`, add **GitHub Actions** to its bypass list, because the release workflow creates the tag itself.

## 4. Releases

### Making a release

1. Bump the number in `VERSION`. It is plain semver without a `v`, for example `0.1.0` to `0.1.1`, `0.2.0` or `1.0.0`.
2. Push or merge to `main`.

The **Release** workflow then:

1. reads `VERSION`, checks it, and stops quietly if `v<VERSION>` already exists
2. runs the tests on Linux, macOS and Windows
3. creates the tag `v<VERSION>` on that commit
4. runs GoReleaser, which publishes the GitHub release with its notes, the archives, the `.deb`, signed checksums, SBOMs, the ghcr.io image, and the Homebrew, Scoop and winget updates if their secrets exist
5. adds build provenance that can be checked with `gh attestation verify`
6. archives superseded releases: only the newest patch of each `major.minor` stays a normal release, older ones are renamed `[ARCHIVED] vX.Y.Z` and turned into pre-releases (downloads stay)

Every bump is published as a normal release. If you ever want a test release, add a suffix by hand, like `0.2.0-rc.1`. That becomes a GitHub *pre-release* and the container image isn't tagged `latest`.

### Release notes and tag message

Each release description has:

1. **GitHub's own generated notes**: *What's Changed* (merged PRs grouped into Features, Bug fixes, Security, Dependencies and so on by `.github/release.yml`), *New Contributors*, and **Full Changelog: `vPREV...vNEW`**, which links to the comparison.
2. **A collapsible list of commits** from `vPREV` to `vNEW`, so commits pushed straight to `main` without a PR show up too.
3. Instructions for checking the download.

The **tag message** lists the same range, for example `Changes from v0.1.0 (a1b2c3d) to v0.2.0 (e4f5a6b):` followed by the commit titles. The first release says *Initial release*.

The groups come from PR labels, which `pr-labels.yml` sets from the PR title: `feat:` becomes feature, `fix:` bug, `sec:` security, `docs:` documentation, `chore:`, `ci:`, `build:`, `refactor:` and `test:` become maintenance, and a `!` adds breaking. Add the label `skip-changelog` to leave a PR out.

Local builds report `v<VERSION>-dev`, because `main.go` embeds the `VERSION` file. Release builds get the exact version from GoReleaser.

### If a release fails

- If it fails **before the tag is created** (for example a failing test), fix the problem and push again. A push that doesn't change `VERSION` won't start the workflow, so use **Actions, Release, Run workflow** to try again.
- If it fails **after the tag is created**, the `cleanup-on-failure` job deletes the tag for you. If GoReleaser already created part of a GitHub release, delete it under *Releases*, then use **Run workflow**.
- Never reuse a version number that was published. Bump it instead.

### GitHub Packages (ghcr.io)

The first release pushes `ghcr.io/gdamigos/cream`. New packages are **private** at first. To make it public, go to your profile, Packages, cream, Package settings, Change visibility, Public. On the same page, check that the package is linked to this repository. The image's `org.opencontainers.image.source` label normally does this for you.

## 5. Homebrew tap

1. Create a **public** repo `gdamigos/homebrew-tap` with a README.
2. Create a **fine-grained personal access token** under Settings, Developer settings, Fine-grained tokens. Give it access to `gdamigos/homebrew-tap` only, with *Contents: Read and write*.
3. In this repo, go to Settings, Secrets and variables, Actions, New secret, and save it as `HOMEBREW_TAP_GITHUB_TOKEN`.

Users then install with:

```sh
brew install --cask gdamigos/tap/cream
```

Until the binaries are notarised ([section 8](#8-macos-signing-and-notarisation)), macOS Gatekeeper blocks them. Users can get around that with `xattr -dr com.apple.quarantine $(which cream)`.

## 6. Scoop bucket

1. Create a **public** repo `gdamigos/scoop-bucket`.
2. Create a fine-grained token for that repo with *Contents: Read and write*.
3. Save it as the secret `SCOOP_BUCKET_GITHUB_TOKEN`.

```powershell
scoop bucket add gdamigos https://github.com/gdamigos/scoop-bucket
scoop install cream
```

## 7. winget

GoReleaser opens a PR on `microsoft/winget-pkgs` for every release.

1. Fork `microsoft/winget-pkgs` to `gdamigos/winget-pkgs`.
2. Create a **classic** token with the `public_repo` scope. Fine-grained tokens can't open PRs into Microsoft's repo.
3. Save it as the secret `WINGET_GITHUB_TOKEN`.

Microsoft's moderators review the first `gdamigos.cream` submission by hand, which can take a few days. Checks are stricter for unsigned binaries ([section 9](#9-windows-code-signing)). Once it is merged:

```powershell
winget install gdamigos.cream
```

## 8. macOS signing and notarisation

This needs an Apple Developer Program membership ($99 a year). Once the secrets exist, `.goreleaser.yaml` turns signing on by itself.

| Secret | Where to get it |
| --- | --- |
| `MACOS_SIGN_P12` | Export your **Developer ID Application** certificate and key from Keychain as a `.p12` file, then `base64 -i cert.p12 \| pbcopy` |
| `MACOS_SIGN_PASSWORD` | the password you chose for the export |
| `MACOS_NOTARY_KEY` | App Store Connect, Users and Access, Integrations, **Team key** (`.p8`), base64 encoded |
| `MACOS_NOTARY_KEY_ID` | that key's ID |
| `MACOS_NOTARY_ISSUER_ID` | the Issuer ID shown on the same page |

## 9. Windows code signing

This isn't set up yet. An unsigned `cream.exe` works, but SmartScreen may warn the first time it runs. Options:

- **SignPath Foundation**: free signing for open source projects, which plugs into GitHub Actions once approved.
- **Azure Trusted Signing**: cheap and easy to use from CI (`azure/trusted-signing-action`).
- **An OV or EV certificate** on a hardware token or a cloud HSM.

To use one of these, add a `signs:` entry with `artifacts: binary` and `ids: [cream]`, limited to Windows, in `.goreleaser.yaml` (or a `binary_signs:` block) that calls the signing tool, and add its secrets to `release.yml`.

## 10. Coverage

This is optional. CI already uploads `coverage.out` as a download. For a badge and PR comments:

1. Sign in at codecov.io with GitHub, add the repo and save its token as the secret `CODECOV_TOKEN`.
2. Add this after the *Test* step in `ci.yml`:

   ```yaml
   - uses: codecov/codecov-action@v5
     if: matrix.os == 'ubuntu-latest'
     with:
       files: coverage.out
       token: ${{ secrets.CODECOV_TOKEN }}
   ```

## 11. Debian apt repository

Optional. Releases already ship `cream-go_<version>_<arch>.deb`. For `apt install cream-go` with updates you need an apt repository, e.g. Cloudsmith or Gemfury, which GoReleaser can publish to.

## 12. Secrets

| Secret | Used for | Required |
| --- | --- | --- |
| `GITHUB_TOKEN` | releases, ghcr.io, attestations | automatic |
| `HOMEBREW_TAP_GITHUB_TOKEN` | Homebrew cask | optional |
| `SCOOP_BUCKET_GITHUB_TOKEN` | Scoop manifest | optional |
| `WINGET_GITHUB_TOKEN` | winget PR | optional |
| `MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD`, `MACOS_NOTARY_KEY`, `MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_ISSUER_ID` | macOS signing and notarisation | optional |
| `CODECOV_TOKEN` | coverage upload | optional |

A missing optional secret never fails a release. That step is just skipped.

To check a release, see [verifying releases](SECURITY.md#verifying-releases).

See also the [README](../README.md), [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md).
