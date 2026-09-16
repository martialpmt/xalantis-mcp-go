# CI and release pipeline — design

Date: 2026-09-16
Status: approved

## Goal

Add GitHub Actions CI (formatting, lint, tests, security checks) and a
manually triggered release that tags the repository, publishes binaries on
a GitHub Release, and makes the module available on the Go module proxy and
pkg.go.dev.

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
   Covered by a unit test on the resolution function.

## Versioning

- Tags are `vX.Y.Z`. No tag yet means the base is `v0.0.0`.
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
```

## `ci.yml`

Triggers: `push` to `main`, `pull_request`, `workflow_call`.
Permissions: `contents: read`. Go version from `go.mod`
(`actions/setup-go` with `go-version-file`). Jobs run in parallel:

| Job | Checks |
|---|---|
| `format` | `gofmt -l .` prints nothing; `go mod tidy -diff` is clean |
| `lint` | `golangci-lint run` with `.golangci.yml` |
| `test` | `go test -race -coverprofile=coverage.out ./...`; coverage uploaded as an artifact |
| `vuln` | `govulncheck ./...` |
| `secrets` | `gitleaks` CLI, full history (`fetch-depth: 0`) |
| `build` | `go build ./...` |

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
     go install command
```

Dispatching from any branch other than `main` skips `release`; the
`gate` still runs, which is harmless.

If step 5 fails after the tag is pushed, the job summary prints the command
to delete the tag (`git push --delete origin vX.Y.Z`) so the release can be
fixed and re-run. Step 6 runs last, so a failed release never reaches the
proxy.

Tag commits use the `github-actions[bot]` identity.

## `.goreleaser.yaml`

- One build: `./cmd/xalantis-projects-mcp`, `CGO_ENABLED=0`,
  `-trimpath`, `-ldflags "-s -w -X main.version={{.Version}}"`.
- Targets: `linux`, `darwin`, `windows` × `amd64`, `arm64`.
- Archives: `tar.gz`, `zip` on Windows; include `README.md` and `LICENSE`.
- `checksums.txt` (SHA-256).
- Changelog from commits, grouped by Conventional Commit type; `docs:`,
  `test:`, `chore:` excluded.

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

## README

Add an installation section (`go install
github.com/martialpmt/xalantis-mcp-go/cmd/xalantis-projects-mcp@latest`,
or download from GitHub Releases) and CI / pkg.go.dev badges. Written in
French, like the rest of the README.

## Verification

Local, before pushing:
- `gofmt`, `go vet`, `go test -race ./...` pass after the module rename.
- `actionlint` passes on all workflows.
- `golangci-lint run` passes (fix or annotate findings).
- `goreleaser check` and `goreleaser release --snapshot --clean` produce
  6 archives and `checksums.txt`; the darwin/arm64 binary reports the
  snapshot version.

After pushing (user):
- CI and CodeQL are green on `main`.
- Dispatch `release` with `minor` → `v0.1.0` tag, GitHub Release with
  assets, module visible on pkg.go.dev, `go install …@v0.1.0` works.

## Out of scope

- Docker images, Homebrew tap, code signing / SLSA provenance.
- Running `test_mcp.py` in CI.
- Branch protection settings (configured by hand in GitHub).
