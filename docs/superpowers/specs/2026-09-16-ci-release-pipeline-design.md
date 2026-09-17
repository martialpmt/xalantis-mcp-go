# CI and release pipeline — design

Date: 2026-09-16
Status: approved

## Goal

Add GitHub Actions CI (formatting, lint, tests, security checks) and a
manually triggered release that tags the repository, publishes binaries on
a GitHub Release, and makes the module available on the Go module proxy and
pkg.go.dev. Users install the binary from GitHub Releases with a one-line
script, or with `go install`.

## Context

- Go module, standard library only (no `go.sum`), `go 1.24.7`.
- One binary: `cmd/xalantis-projects-mcp`. Tests in 5 packages.
- Public repository: `github.com/martialpmt/xalantis-mcp-go`.
- No CI, no lint config, no `.gitignore`, no tags, no LICENSE.
- The built binary `xalantis-projects-mcp` (11 MB) is committed.
- `go.mod` declares `paymetrust/xalantis-projects-mcp`, which has no domain,
  so the Go proxy cannot fetch it.
- `main.go` hardcodes `serverVersion = "2.0.0"` (used in MCP `serverInfo`
  and the HTTP `User-Agent`).

## Prerequisites (separate commits, before CI work)

1. **Module path.** Rename the module to
   `github.com/martialpmt/xalantis-mcp-go` and update every internal import.
2. **License.** Add `LICENSE` (MIT, "Copyright (c) 2026 Martial Anouman").
   pkg.go.dev hides docs without a recognised license.
3. **Ignore build output.** Add `.gitignore` (`/xalantis-projects-mcp`,
   `/dist/`, `coverage.out`) and `git rm --cached xalantis-projects-mcp`.
4. **Build version.** Replace the `serverVersion` constant with
   `var version = ""` in `main.go`, resolved at startup:
   1. `version` if set by `-ldflags "-X main.version=…"` (GoReleaser);
   2. else `debug.ReadBuildInfo().Main.Version` if it is not `(devel)`
      (set by `go install …@vX.Y.Z`);
   3. else `dev`.

   The resolved value feeds `serverInfo.version` and the `User-Agent`.
   A `--version` flag prints it and exits (used by the install smoke test).
   Covered by a unit test on the resolution function.

## Versioning

- Tags are `vX.Y.Z`. No tag yet means the base is `v0.0.0`.
- The first release is `v0.1.0` (dispatch with `minor`).
- Major versions stay at 0 or 1. A v2+ module requires a `/v2` path
  suffix, so the release workflow refuses any bump that yields `v2.0.0` or
  higher. Moving to v2 is a separate, deliberate change.
- A version on the Go proxy is permanent. A bad release is fixed with a new
  patch release, or with a `retract` directive in `go.mod`.

## Files

```
.github/
  workflows/
    ci.yml
    codeql.yml
    release.yml
  dependabot.yml
.golangci.yml
.goreleaser.yaml
.gitignore
LICENSE
install.sh
.github/scripts/next-version.sh       # bump logic, used by release.yml
.github/scripts/next-version_test.sh  # its tests
```

## `ci.yml`

Triggers: `push` to `main`, `pull_request`, `workflow_call`.
Permissions: `contents: read`. Go version: `test` runs a matrix of the
`go.mod` minimum (`go-version-file`) and `stable`; every other job, and the
release build, uses `stable` (so govulncheck checks a supported standard
library). Jobs run in parallel:

| Job | Checks |
|---|---|
| `format` | `gofmt -l .` prints nothing; `go mod tidy -diff` is clean |
| `lint` | `golangci-lint run` with `.golangci.yml` |
| `test` | `go test -race -coverprofile=coverage.out ./...`; coverage uploaded as an artifact |
| `vuln` | `govulncheck ./...` |
| `secrets` | `gitleaks` CLI, full history (`fetch-depth: 0`) |
| `build` | `go build ./...` |
| `scripts` | `shellcheck install.sh .github/scripts/*.sh`; `sh .github/scripts/next-version_test.sh` |

`test_mcp.py` is not run: it needs a live Xalantis API key.

## `.golangci.yml`

golangci-lint v2 config. Linters: defaults (`govet`, `staticcheck`,
`errcheck`, `ineffassign`, `unused`) plus `gosec`, `misspell`, `revive`.
`gosec` findings that are intended (e.g. file access already confined to
`XALANTIS_FILES_DIR`) are silenced inline with `//nolint:gosec // reason`,
never globally.

## `codeql.yml`

Triggers: `push` and `pull_request` to `main`, weekly cron.
Language `go`, build mode `autobuild`. Permissions:
`security-events: write`, `contents: read`. Results go to the Security tab.

## `release.yml`

Trigger: `workflow_dispatch` with input `bump` (`patch` | `minor` |
`major`, default `patch`). `concurrency: release`, no cancel.

```
gate     uses: ./.github/workflows/ci.yml
release  needs: gate
         if: github.ref == 'refs/heads/main'
         permissions: contents: write
  1. checkout, fetch-depth: 0
  2. compute next version from the latest vX.Y.Z tag and the bump input
  3. fail if the tag exists or the major version is >= 2
  4. git tag -a vX.Y.Z -m "Release vX.Y.Z"; git push origin vX.Y.Z
  5. goreleaser release --clean  (GITHUB_TOKEN)
  6. GOPROXY=proxy.golang.org go list -m github.com/martialpmt/xalantis-mcp-go@vX.Y.Z
  7. write a job summary: version, release URL, pkg.go.dev URL,
     install commands
install-smoke  needs: release
         matrix: ubuntu-latest, macos-latest
  1. checkout the new tag
  2. VERSION=vX.Y.Z INSTALL_DIR=$RUNNER_TEMP/bin sh install.sh
  3. assert `xalantis-projects-mcp --version` prints vX.Y.Z
```

Dispatching from any branch other than `main` skips `release`; the
`gate` still runs, which is harmless.

If step 5 fails after the tag is pushed, the job summary says not to delete or reuse the tag (it is public and the Go proxy may already have cached it): delete any partial GitHub Release, fix `main`, and dispatch again, which creates the next version; add a `retract` directive if the tagged code is broken. Step 6 runs last, so the workflow itself never publishes a failed release to the proxy.

Tags are created with the `github-actions[bot]` identity.

## `.goreleaser.yaml`

- One build: `./cmd/xalantis-projects-mcp`, `CGO_ENABLED=0`,
  `-trimpath`, `-ldflags "-s -w -X main.version=v{{.Version}}"` (GoReleaser's
  `.Version` has no `v`; the prefix keeps it identical to the tag and to
  `go install` builds).
- Targets: `linux`, `darwin`, `windows` × `amd64`, `arm64`.
- Archives: `tar.gz`, `zip` on Windows; include `README.md` and `LICENSE`.
  Name template without the version:
  `xalantis-projects-mcp_{{.Os}}_{{.Arch}}`, so
  `releases/latest/download/<asset>` always resolves to the newest release.
- `checksums.txt` (SHA-256), same fixed name.
- Changelog from commits, grouped by Conventional Commit type; `docs:`,
  `test:`, `chore:` excluded.

## `install.sh`

POSIX `sh` (`set -eu`), used as:

```
curl -fsSL https://raw.githubusercontent.com/martialpmt/xalantis-mcp-go/main/install.sh | sh
```

Settings (environment variables):

| Variable | Default | Role |
|---|---|---|
| `VERSION` | `latest` | Tag to install (`vX.Y.Z`) |
| `INSTALL_DIR` | `$HOME/.local/bin` | Destination, created if missing |
| `BASE_URL` | `https://github.com/martialpmt/xalantis-mcp-go/releases` | Override for local testing only |

Steps:
1. Detect OS (`uname -s`: Darwin → `darwin`, Linux → `linux`) and
   architecture (`uname -m`: `x86_64`/`amd64` → `amd64`,
   `arm64`/`aarch64` → `arm64`). Anything else exits with an error that
   points to the Releases page.
2. Download URL: `$BASE_URL/latest/download/<asset>` for `latest`, else
   `$BASE_URL/download/$VERSION/<asset>`. Uses `curl -fsSL`, falls back to
   `wget -qO`; neither → error.
3. Download the archive and `checksums.txt` into a `mktemp -d` directory
   (removed by `trap` on exit).
4. Verify SHA-256 with `sha256sum`, else `shasum -a 256`; mismatch or
   missing entry → error, nothing installed.
5. Extract only the binary and install it with mode `0755` into
   `$INSTALL_DIR`.
6. Print the installed version (`--version`). Warn if `$INSTALL_DIR` is not
   in `PATH`.

Windows is not supported by the script; the README documents the manual
zip download.

## `dependabot.yml`

Weekly updates for `github-actions` and `gomod`.

## Supply chain

- Every third-party action is pinned to a full commit SHA with a version
  comment; Dependabot keeps pins current.
- Workflow-level `permissions` default to `contents: read`; only the
  release job gets `contents: write`, only CodeQL gets
  `security-events: write`.
- Tool versions (golangci-lint, govulncheck, gitleaks, goreleaser) are
  pinned.
- `install.sh` never uses `sudo` and always verifies the checksum before
  installing.

## README

Add an installation section, in this order: the `install.sh` one-liner
(with `VERSION` / `INSTALL_DIR`), manual download from GitHub Releases
(including Windows), and `go install
github.com/martialpmt/xalantis-mcp-go/cmd/xalantis-projects-mcp@latest`.
Update the MCP client configuration example to the installed binary path.
Add CI and pkg.go.dev badges. Written in
French, like the rest of the README.

## Verification

Local, before pushing:
- `gofmt`, `go vet`, `go test -race ./...` pass after the module rename.
- `actionlint` passes on all workflows.
- `golangci-lint run` passes (fix or annotate findings).
- `goreleaser check` and `goreleaser release --snapshot --clean` produce
  6 archives and `checksums.txt` with version-free names; the darwin/arm64
  binary reports the snapshot version with `--version`.
- `shellcheck install.sh` passes. `install.sh` run with `BASE_URL` pointed
  at a local copy of the snapshot `dist/` (served with
  `python3 -m http.server`) installs into a temp dir, and a corrupted
  archive is rejected.

After pushing (user):
- CI and CodeQL are green on `main`.
- Dispatch `release` with `minor` → `v0.1.0` tag, GitHub Release with
  assets, `install-smoke` green, module visible on pkg.go.dev,
  `go install …@v0.1.0` works.

## Out of scope

- Docker images, Homebrew tap, code signing / SLSA provenance.
- A Windows install script.
- Running `test_mcp.py` in CI.
- Branch protection settings (configured by hand in GitHub).
