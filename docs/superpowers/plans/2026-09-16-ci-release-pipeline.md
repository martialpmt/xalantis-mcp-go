# CI and Release Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add GitHub Actions CI (format, lint, tests, security) and a manually dispatched release that tags `vX.Y.Z`, publishes binaries on GitHub Releases, installs via `install.sh`, and registers the module on the Go proxy / pkg.go.dev.

**Architecture:** Three workflows: `ci.yml` (push/PR, also reusable), `codeql.yml`, `release.yml` (dispatch → CI gate → tag → GoReleaser → proxy → install smoke test). The bump logic lives in a tested POSIX shell script. The binary gets a build-time version with a `--version` flag. Before any of that, the module is renamed to its fetchable GitHub path, and a license and `.gitignore` are added.

**Tech Stack:** Go (stdlib only), GitHub Actions, golangci-lint v2, govulncheck, gitleaks, CodeQL, GoReleaser v2, POSIX sh, shellcheck, actionlint.

**Spec:** `docs/superpowers/specs/2026-09-16-ci-release-pipeline-design.md`

## Global Constraints

- Module path: `github.com/martialpmt/xalantis-mcp-go`. Binary name: `xalantis-projects-mcp`. Main package: `./cmd/xalantis-projects-mcp`.
- `go.mod` stays `go 1.24.7`; no new Go dependencies.
- Tags are `vX.Y.Z`, major version 0 or 1 only; first release `v0.1.0`.
- Every third-party action is pinned to a full commit SHA with a `# vX.Y.Z` comment. Use exactly these:
  - `actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1`
  - `actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0`
  - `actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1`
  - `golangci/golangci-lint-action@ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a # v9.3.0`
  - `github/codeql-action/{init,autobuild,analyze}@b96794f015dfd88f77b49b1c93e0fa7110f94c63 # v4.38.0`
  - `goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3`
- Pinned tool versions: golangci-lint `v2.13.2`, govulncheck `v1.8.0`, gitleaks `8.30.1` (linux_x64 SHA-256 `551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb`), GoReleaser `v2.18.1`.
- Workflow-level `permissions: contents: read`; `contents: write` only on the release job; `security-events: write` only on CodeQL.
- Code comments, user-facing messages and README text are in **French** (match existing code). Workflow step names and commit messages are in English (Conventional Commits).
- Commit messages end with `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.
- Work happens on branch `ci-release-pipeline`. Do not push; the user pushes.

## Local tools

Install once before Task 5 (local only; CI pins its own versions):

```bash
brew install golangci-lint goreleaser gitleaks shellcheck actionlint
```

govulncheck runs through `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0`.

## File Structure

| File | Responsibility |
|---|---|
| `go.mod`, all `*.go` importing `paymetrust/...` | Module rename |
| `LICENSE` | MIT license |
| `.gitignore` | Build output |
| `cmd/xalantis-projects-mcp/version.go` | Version resolution |
| `cmd/xalantis-projects-mcp/version_test.go` | Its tests |
| `cmd/xalantis-projects-mcp/main.go` | `--version` flag, uses resolved version |
| `.github/scripts/next-version.sh` | Compute next tag from latest tag + bump |
| `.github/scripts/next-version_test.sh` | Its tests |
| `.golangci.yml` | Lint config |
| `.github/workflows/ci.yml` | CI checks (push, PR, reusable) |
| `.github/workflows/codeql.yml` | CodeQL |
| `.github/dependabot.yml` | Action and module updates |
| `.goreleaser.yaml` | Cross-builds, archives, checksums, changelog |
| `install.sh` | Install from GitHub Releases |
| `.github/workflows/release.yml` | Manual release |
| `README.md` | Install docs, badges |

---

### Task 1: Rename the module

**Files:**
- Modify: `go.mod:1`
- Modify: `cmd/xalantis-projects-mcp/main.go:19-22`, `internal/tools/generic.go:18-20`, `internal/tools/generic_test.go:11-12`, `internal/tools/projects.go:6-7`, `internal/tools/projects_test.go:11-12`

**Interfaces:**
- Produces: import prefix `github.com/martialpmt/xalantis-mcp-go/internal/...` used by every later task.

- [ ] **Step 1: Confirm the baseline passes**

Run: `go test ./...`
Expected: all 5 packages `ok`.

- [ ] **Step 2: Rewrite the module path and imports**

```bash
go mod edit -module github.com/martialpmt/xalantis-mcp-go
grep -rl '"paymetrust/xalantis-projects-mcp/' --include='*.go' . \
  | xargs sed -i '' 's#"paymetrust/xalantis-projects-mcp/#"github.com/martialpmt/xalantis-mcp-go/#'
gofmt -w cmd internal
```

(`sed -i ''` is the macOS form; on Linux use `sed -i`.) `gofmt -w` re-sorts imports if needed.

- [ ] **Step 3: Verify no old path remains and tests pass**

Run: `grep -rn 'paymetrust/xalantis-projects-mcp' --include='*.go' --include=go.mod . ; go vet ./... && go test ./...`
Expected: grep prints nothing; vet is silent; all packages `ok`.

- [ ] **Step 4: Commit**

```bash
git add go.mod cmd internal
git commit -m "refactor: rename module to github.com/martialpmt/xalantis-mcp-go

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: License, .gitignore, untrack the binary

**Files:**
- Create: `LICENSE`, `.gitignore`
- Delete from index: `xalantis-projects-mcp` (the file stays on disk)

- [ ] **Step 1: Write `LICENSE`**

```text
MIT License

Copyright (c) 2026 Martial Anouman

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Write `.gitignore`**

```gitignore
# Binaire compilé localement
/xalantis-projects-mcp
# Sorties GoReleaser et couverture
/dist/
coverage.out
```

- [ ] **Step 3: Stop tracking the binary**

Run: `git rm --cached xalantis-projects-mcp && git status --short`
Expected: `D  xalantis-projects-mcp`, `?? .gitignore`, `?? LICENSE`; the binary no longer appears as untracked, and `ls xalantis-projects-mcp` still finds it on disk.

- [ ] **Step 4: Commit**

```bash
git add LICENSE .gitignore
git commit -m "chore: add MIT license and gitignore, stop tracking built binary

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Build version and `--version` flag

**Files:**
- Create: `cmd/xalantis-projects-mcp/version.go`, `cmd/xalantis-projects-mcp/version_test.go`
- Modify: `cmd/xalantis-projects-mcp/main.go` (imports, `const` block, start of `main`, `UserAgent`, `NewServer` call)

**Interfaces:**
- Produces: `var version string` in package `main` (set by `-ldflags "-X main.version=…"`, used by GoReleaser in Task 8); `func resolveVersion(injected string, info *debug.BuildInfo, ok bool) string`; `func currentVersion() string`; CLI flag `--version` printing the version on one line then exiting 0 (used by Tasks 9 and 10).

- [ ] **Step 1: Write the failing test** — `cmd/xalantis-projects-mcp/version_test.go`

```go
package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	info := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: v}}
	}
	cases := []struct {
		name     string
		injected string
		info     *debug.BuildInfo
		ok       bool
		want     string
	}{
		{"valeur injectée prioritaire", "v0.1.0", info("v9.9.9"), true, "v0.1.0"},
		{"version du module (go install)", "", info("v0.2.0"), true, "v0.2.0"},
		{"build local sans version", "", info("(devel)"), true, "dev"},
		{"version de module vide", "", info(""), true, "dev"},
		{"pas d'informations de build", "", nil, false, "dev"},
		{"informations nulles", "", nil, true, "dev"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveVersion(c.injected, c.info, c.ok); got != c.want {
				t.Errorf("resolveVersion(%q, …) = %q, attendu %q", c.injected, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./cmd/xalantis-projects-mcp/ -run TestResolveVersion -v`
Expected: build failure `undefined: resolveVersion`.

- [ ] **Step 3: Implement** — `cmd/xalantis-projects-mcp/version.go`

```go
package main

import "runtime/debug"

// version est injectée à la compilation par GoReleaser
// (-ldflags "-X main.version=vX.Y.Z").
var version = ""

// resolveVersion choisit la version annoncée : la valeur injectée, sinon la
// version du module (go install …@vX.Y.Z), sinon "dev".
func resolveVersion(injected string, info *debug.BuildInfo, ok bool) string {
	if injected != "" {
		return injected
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// currentVersion renvoie la version du binaire en cours d'exécution.
func currentVersion() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(version, info, ok)
}
```

- [ ] **Step 4: Run the test to see it pass**

Run: `go test ./cmd/xalantis-projects-mcp/ -run TestResolveVersion -v`
Expected: `PASS`, 6 subtests.

- [ ] **Step 5: Wire it into `main.go`**

In the header comment, replace the `// Compilation : …` line with:

```go
// Compilation : go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp
// Version : xalantis-projects-mcp --version
```

Add `"flag"` to the imports (alphabetical, before `"fmt"`).

Replace the `const` block with (drop `serverVersion`):

```go
const (
	serverName   = "xalantis-projects-mcp"
	instructions = "Pour les lectures courantes de projets, utilisez les 7 outils dédiés (xalantis_list_projects, xalantis_list_tasks…). " +
		"Pour toute autre opération Xalantis (tickets, SLA, catalogue, écriture sur les projets…) : " +
		"xalantis_search_operations, puis xalantis_describe_operation, puis xalantis_call_operation."
)
```

At the very start of `main()`, before `openapi.Load()`:

```go
	showVersion := flag.Bool("version", false, "affiche la version et quitte")
	flag.Parse()
	ver := currentVersion()
	if *showVersion {
		fmt.Println(ver)
		return
	}

```

Replace `UserAgent: serverName + "/" + serverVersion,` with `UserAgent: serverName + "/" + ver,` and `mcp.NewServer(serverName, serverVersion, instructions)` with `mcp.NewServer(serverName, ver, instructions)`.

- [ ] **Step 6: Verify the flag and the injected version**

Run:
```bash
go vet ./... && go test ./...
go run ./cmd/xalantis-projects-mcp --version
go build -ldflags "-X main.version=v9.9.9" -o "$TMPDIR/xpm" ./cmd/xalantis-projects-mcp && "$TMPDIR/xpm" --version
```
Expected: tests `ok`; the `go run` line prints `dev` or a VCS pseudo-version (e.g. `v0.0.0-2026…-abcdef+dirty`); the last line prints exactly `v9.9.9`.

- [ ] **Step 7: Commit**

```bash
git add cmd/xalantis-projects-mcp
git commit -m "feat: report build version and add --version flag

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Next-version script

**Files:**
- Create: `.github/scripts/next-version.sh`, `.github/scripts/next-version_test.sh`

**Interfaces:**
- Produces: `sh .github/scripts/next-version.sh <latest-tag-or-empty> <patch|minor|major>` prints `vX.Y.Z` on stdout and exits 0; exits 1 with a message on stderr for an invalid tag, an invalid bump, or a result with major ≥ 2. Used by `release.yml` (Task 10) and the CI `scripts` job (Task 6).

- [ ] **Step 1: Write the failing test** — `.github/scripts/next-version_test.sh`

```sh
#!/bin/sh
# Tests de next-version.sh. Usage : sh .github/scripts/next-version_test.sh
set -u

script="$(dirname "$0")/next-version.sh"
fail=0

# expect <attendu> <arguments…> ; "ERREUR" attend un code de sortie non nul.
expect() {
	want=$1
	shift
	got=$(sh "$script" "$@" 2>/dev/null) || got=ERREUR
	if [ "$got" != "$want" ]; then
		echo "ÉCHEC : next-version.sh $* => $got, attendu $want"
		fail=1
	fi
}

expect v0.0.1 "" patch
expect v0.1.0 "" minor
expect v1.0.0 "" major
expect v0.1.1 v0.1.0 patch
expect v0.2.0 v0.1.3 minor
expect v1.0.0 v0.9.9 major
expect v1.10.0 v1.9.4 minor
expect v1.2.10 v1.2.9 patch
expect ERREUR v1.2.0 major
expect ERREUR v0.1.0 bogus
expect ERREUR v0.1.0
expect ERREUR 1.2.3 patch
expect ERREUR v1.2.3-rc.1 patch

if [ "$fail" -eq 0 ]; then
	echo OK
fi
exit "$fail"
```

- [ ] **Step 2: Run it to see it fail**

Run: `sh .github/scripts/next-version_test.sh`
Expected: exit 1; `ÉCHEC` lines for every valid case (the script does not exist yet, so everything returns `ERREUR`).

- [ ] **Step 3: Implement** — `.github/scripts/next-version.sh`

```sh
#!/bin/sh
# Calcule la prochaine version vX.Y.Z.
# Usage : next-version.sh <dernier tag, vide si aucun> <patch|minor|major>
set -eu

latest=${1:-v0.0.0}
bump=${2:-}

if ! printf '%s\n' "$latest" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "tag invalide : $latest (attendu vX.Y.Z)" >&2
	exit 1
fi

rest=${latest#v}
major=${rest%%.*}
rest=${rest#*.}
minor=${rest%%.*}
patch=${rest#*.}

case $bump in
patch) patch=$((patch + 1)) ;;
minor)
	minor=$((minor + 1))
	patch=0
	;;
major)
	major=$((major + 1))
	minor=0
	patch=0
	;;
*)
	echo "bump invalide : '$bump' (attendu patch, minor ou major)" >&2
	exit 1
	;;
esac

if [ "$major" -ge 2 ]; then
	echo "v$major.$minor.$patch refusée : une version v2+ exige le suffixe /v$major dans le chemin du module" >&2
	exit 1
fi

echo "v$major.$minor.$patch"
```

- [ ] **Step 4: Run the tests and shellcheck**

Run: `sh .github/scripts/next-version_test.sh && shellcheck .github/scripts/*.sh`
Expected: `OK`, exit 0; shellcheck prints nothing.

- [ ] **Step 5: Commit**

```bash
git add .github/scripts
git commit -m "ci: add next-version script for release bumps

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: golangci-lint config and fixes

**Files:**
- Create: `.golangci.yml`
- Modify: any Go files the linters flag

- [ ] **Step 1: Write `.golangci.yml`**

```yaml
version: "2"

linters:
  # Défauts : errcheck, govet, ineffassign, staticcheck, unused.
  enable:
    - gosec
    - misspell
    - revive
  exclusions:
    presets:
      - std-error-handling
```

- [ ] **Step 2: Run the linter**

Run: `golangci-lint run ./...`
Expected: either `0 issues.` or a list of findings.

- [ ] **Step 3: Resolve every finding**

Apply these rules:
- A real bug (unchecked error that matters, wrong format verb, etc.) → fix the code.
- `gosec` finding that is intended behaviour → annotate the line with the rule and the reason, for example:
  ```go
  f, err := os.Open(path) //nolint:gosec // G304: chemin déjà confiné à XALANTIS_FILES_DIR
  ```
  Never disable gosec globally or per file. Before annotating a file-path finding, check that the path really goes through the `XALANTIS_FILES_DIR` confinement in `internal/tools/files.go`; if it does not, it is a real bug: stop and report it.
- `revive` style rule that conflicts with the existing codebase convention across many files (e.g. comment wording on exported identifiers written in French) → disable only that rule in `.golangci.yml`, with a comment:
  ```yaml
  linters:
    settings:
      revive:
        rules:
          - name: <rule-name>
            disabled: true # <raison>
  ```
  (Listing rules in `settings.revive.rules` replaces revive's default set, so if you add this block, also list the default rules you keep. Run `golangci-lint run` again to confirm.)
- `misspell` on a French word → add it under `linters.settings.misspell.ignore-rules`.

- [ ] **Step 4: Verify**

Run: `golangci-lint run ./... && go test ./...`
Expected: `0 issues.`; tests `ok`.

- [ ] **Step 5: Commit**

```bash
git add .golangci.yml cmd internal
git commit -m "ci: add golangci-lint config with gosec and fix findings

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: `.golangci.yml` (Task 5), `.github/scripts/*` (Task 4), `install.sh` (Task 9; the `scripts` job lists it, so until Task 9 lands the job would fail — this is fine because nothing is pushed before Task 12).
- Produces: reusable workflow (`on: workflow_call`) called by `release.yml` as `uses: ./.github/workflows/ci.yml`.

- [ ] **Step 1: Scan the git history for secrets locally**

Run: `gitleaks git --redact --verbose .`
Expected: `no leaks found`. If there are findings: for a real secret, **stop and tell the user** (rotating it is their call). For a confirmed false positive, add its `Fingerprint:` line to a new `.gitleaksignore` at the repo root and rerun until clean.

- [ ] **Step 2: Write `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
  workflow_call:

permissions:
  contents: read

jobs:
  format:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: stable
      - name: gofmt
        run: |
          unformatted=$(gofmt -l .)
          if [ -n "$unformatted" ]; then
            echo "::error::Fichiers à formater avec gofmt -w :"
            echo "$unformatted"
            exit 1
          fi
      - name: go mod tidy
        run: go mod tidy -diff

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: stable
      - uses: golangci/golangci-lint-action@ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a # v9.3.0
        with:
          version: v2.13.2

  test:
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        go: ["go.mod", "stable"]
    name: test (${{ matrix.go }})
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: ${{ matrix.go == 'stable' && 'stable' || '' }}
          go-version-file: ${{ matrix.go == 'go.mod' && 'go.mod' || '' }}
      - name: go test
        run: go test -race -coverprofile=coverage.out ./...
      - uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1
        if: matrix.go == 'stable'
        with:
          name: coverage
          path: coverage.out

  vuln:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: stable
      - name: govulncheck
        run: go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

  secrets:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
      - name: Install gitleaks
        env:
          GITLEAKS_VERSION: "8.30.1"
          GITLEAKS_SHA256: 551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
        run: |
          archive="gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
          curl -fsSLO "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/${archive}"
          echo "${GITLEAKS_SHA256}  ${archive}" | sha256sum -c -
          tar -xzf "$archive" -C "$RUNNER_TEMP" gitleaks
      - name: gitleaks
        run: '"$RUNNER_TEMP/gitleaks" git --redact --verbose .'

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: stable
      - name: go build
        run: go build ./...

  scripts:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - name: shellcheck
        run: shellcheck install.sh .github/scripts/*.sh
      - name: next-version tests
        run: sh .github/scripts/next-version_test.sh
```

- [ ] **Step 3: Lint the workflow and run the same checks locally**

Run:
```bash
actionlint .github/workflows/ci.yml
test -z "$(gofmt -l .)" && go mod tidy -diff && go test -race ./... && go build ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```
Expected: actionlint prints nothing; the chain exits 0; govulncheck prints `No vulnerabilities found.` If govulncheck reports a standard-library vulnerability that only affects your local toolchain, note it but continue (CI uses `stable`); if it reports one in this module's code, stop and report it.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git add .gitleaksignore 2>/dev/null || true
git commit -m "ci: add CI workflow with format, lint, tests and security checks

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: CodeQL and Dependabot

**Files:**
- Create: `.github/workflows/codeql.yml`, `.github/dependabot.yml`

- [ ] **Step 1: Write `.github/workflows/codeql.yml`**

```yaml
name: CodeQL

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  schedule:
    - cron: "27 4 * * 1"

permissions:
  contents: read

jobs:
  analyze:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: stable
      - uses: github/codeql-action/init@b96794f015dfd88f77b49b1c93e0fa7110f94c63 # v4.38.0
        with:
          languages: go
          build-mode: autobuild
      - uses: github/codeql-action/analyze@b96794f015dfd88f77b49b1c93e0fa7110f94c63 # v4.38.0
        with:
          category: /language:go
```

- [ ] **Step 2: Write `.github/dependabot.yml`**

```yaml
version: 2
updates:
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: weekly
```

- [ ] **Step 3: Lint**

Run: `actionlint .github/workflows/codeql.yml && ruby -ryaml -e 'YAML.load_file(".github/dependabot.yml")' && echo valid`
Expected: `valid`, nothing else.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/codeql.yml .github/dependabot.yml
git commit -m "ci: add CodeQL analysis and Dependabot updates

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: GoReleaser config

**Files:**
- Create: `.goreleaser.yaml`

**Interfaces:**
- Consumes: `main.version` (Task 3), `LICENSE` (Task 2).
- Produces: release assets `xalantis-projects-mcp_<os>_<arch>.tar.gz` (`.zip` for windows) and `checksums.txt` (format `<sha256>  <filename>`); each archive holds `xalantis-projects-mcp` (`.exe` on windows) at its root, plus `README.md` and `LICENSE`. Used by `install.sh` (Task 9).

- [ ] **Step 1: Write `.goreleaser.yaml`**

```yaml
version: 2

project_name: xalantis-projects-mcp

builds:
  - id: xalantis-projects-mcp
    main: ./cmd/xalantis-projects-mcp
    binary: xalantis-projects-mcp
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w -X main.version=v{{ .Version }}
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]

archives:
  - id: default
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - README.md
      - LICENSE

checksum:
  name_template: checksums.txt
  algorithm: sha256

snapshot:
  version_template: "{{ incpatch .Version }}-snapshot"

changelog:
  use: git
  sort: asc
  filters:
    exclude:
      - "^docs"
      - "^test"
      - "^chore"
  groups:
    - title: Features
      regexp: '^.*?feat(\(.+\))??!?:.+$'
      order: 0
    - title: Bug fixes
      regexp: '^.*?fix(\(.+\))??!?:.+$'
      order: 1
    - title: Other changes
      order: 999

release:
  github:
    owner: martialpmt
    name: xalantis-mcp-go
```

- [ ] **Step 2: Validate and build a snapshot**

Run:
```bash
goreleaser check
goreleaser release --snapshot --clean
ls dist/*.tar.gz dist/*.zip dist/checksums.txt
cat dist/checksums.txt
```
Expected: `check` reports the config is valid, with no deprecation warnings (fix any it reports); `ls` shows 4 `.tar.gz` (linux, darwin × amd64, arm64) and 2 `.zip` (windows × amd64, arm64), each named without a version (e.g. `xalantis-projects-mcp_darwin_arm64.tar.gz`); `checksums.txt` lists all 6.

- [ ] **Step 3: Check archive layout and embedded version**

Run:
```bash
tar -tzf dist/xalantis-projects-mcp_darwin_arm64.tar.gz
tar -xzf dist/xalantis-projects-mcp_darwin_arm64.tar.gz -C "$TMPDIR" xalantis-projects-mcp
"$TMPDIR/xalantis-projects-mcp" --version
```
Expected: listing has `xalantis-projects-mcp`, `README.md`, `LICENSE` at the root; version prints `v0.0.1-snapshot`. (On a non-darwin/arm64 machine, use the archive matching `uname -s`/`uname -m`.)

- [ ] **Step 4: Commit** (`dist/` is ignored)

```bash
git add .goreleaser.yaml
git commit -m "build: add GoReleaser config for cross-platform release archives

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: `install.sh`

**Files:**
- Create: `install.sh` (mode 0755)

**Interfaces:**
- Consumes: asset names and `checksums.txt` format from Task 8; `--version` from Task 3.
- Produces: `VERSION`, `INSTALL_DIR`, `BASE_URL` env contract; used by `release.yml` `install-smoke` (Task 10) as `VERSION=vX.Y.Z INSTALL_DIR=… sh install.sh`.

- [ ] **Step 1: Prepare a local release server from the snapshot** (requires `dist/` from Task 8; rerun `goreleaser release --snapshot --clean` if missing)

```bash
T=$(mktemp -d)
archive="xalantis-projects-mcp_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz"
mkdir -p "$T/srv/latest/download" "$T/srv/download/v0.0.1-snapshot"
cp "dist/$archive" dist/checksums.txt "$T/srv/latest/download/"
cp "dist/$archive" dist/checksums.txt "$T/srv/download/v0.0.1-snapshot/"
echo "$T"
```

Start the server in the background (keep it running for Steps 2–5): `python3 -m http.server 8765 --directory "$T/srv"`

- [ ] **Step 2: Confirm the test fails before the script exists**

Run: `BASE_URL=http://127.0.0.1:8765 INSTALL_DIR="$T/bin" sh install.sh`
Expected: `sh: install.sh: No such file or directory`, non-zero exit.

- [ ] **Step 3: Write `install.sh`**

```sh
#!/bin/sh
# Installe xalantis-projects-mcp depuis les GitHub Releases (macOS, Linux).
#
#   curl -fsSL https://raw.githubusercontent.com/martialpmt/xalantis-mcp-go/main/install.sh | sh
#
# Variables facultatives :
#   VERSION      version à installer : latest (défaut) ou vX.Y.Z
#   INSTALL_DIR  dossier d'installation (défaut : $HOME/.local/bin)
#   BASE_URL     URL des releases (tests locaux uniquement)
set -eu

BINARY=xalantis-projects-mcp
RELEASES_URL=https://github.com/martialpmt/xalantis-mcp-go/releases
VERSION=${VERSION:-latest}
INSTALL_DIR=${INSTALL_DIR:-$HOME/.local/bin}
BASE_URL=${BASE_URL:-$RELEASES_URL}

fail() {
	echo "install.sh : $*" >&2
	exit 1
}

case $(uname -s) in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "système non pris en charge : $(uname -s). Téléchargez l'archive sur $RELEASES_URL" ;;
esac

case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "architecture non prise en charge : $(uname -m). Téléchargez l'archive sur $RELEASES_URL" ;;
esac

case $VERSION in
latest) url=$BASE_URL/latest/download ;;
v[0-9]*) url=$BASE_URL/download/$VERSION ;;
*) fail "VERSION invalide : $VERSION (attendu : latest ou vX.Y.Z)" ;;
esac

if command -v curl >/dev/null 2>&1; then
	download() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -qO "$2" "$1"; }
else
	fail "curl ou wget est nécessaire"
fi

if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
	fail "sha256sum ou shasum est nécessaire"
fi

archive=${BINARY}_${os}_${arch}.tar.gz
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Téléchargement de $archive ($VERSION)…"
download "$url/$archive" "$tmp/$archive" || fail "téléchargement impossible : $url/$archive"
download "$url/checksums.txt" "$tmp/checksums.txt" || fail "téléchargement impossible : $url/checksums.txt"

expected=$(awk -v f="$archive" '$2 == f {print $1}' "$tmp/checksums.txt")
[ -n "$expected" ] || fail "$archive absent de checksums.txt"
[ "$(sha256 "$tmp/$archive")" = "$expected" ] || fail "somme de contrôle invalide pour $archive, rien n'a été installé"

tar -xzf "$tmp/$archive" -C "$tmp" "$BINARY"
mkdir -p "$INSTALL_DIR"
cp "$tmp/$BINARY" "$INSTALL_DIR/$BINARY.tmp"
chmod 0755 "$INSTALL_DIR/$BINARY.tmp"
mv -f "$INSTALL_DIR/$BINARY.tmp" "$INSTALL_DIR/$BINARY"

echo "Installé : $INSTALL_DIR/$BINARY ($("$INSTALL_DIR/$BINARY" --version))"
case :$PATH: in
*:"$INSTALL_DIR":*) ;;
*)
	echo "Attention : $INSTALL_DIR n'est pas dans PATH. Ajoutez par exemple à votre profil :"
	echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
	;;
esac
```

Then: `chmod 0755 install.sh && shellcheck install.sh` — expected: no output.

- [ ] **Step 4: Test the success paths**

Run:
```bash
BASE_URL=http://127.0.0.1:8765 INSTALL_DIR="$T/bin" sh install.sh
"$T/bin/xalantis-projects-mcp" --version
VERSION=v0.0.1-snapshot BASE_URL=http://127.0.0.1:8765 INSTALL_DIR="$T/bin-pinned" sh install.sh
cat install.sh | BASE_URL=http://127.0.0.1:8765 INSTALL_DIR="$T/bin-pipe" sh
```
Expected: each run prints `Installé : …/xalantis-projects-mcp (v0.0.1-snapshot)` and the PATH warning; the explicit `--version` prints `v0.0.1-snapshot`; `ls "$T"/bin*` shows three installed binaries and no `.tmp` files.

- [ ] **Step 5: Test the failure paths**

Run:
```bash
VERSION=bogus INSTALL_DIR="$T/bad1" sh install.sh; echo "exit=$?"
VERSION=v9.9.9 BASE_URL=http://127.0.0.1:8765 INSTALL_DIR="$T/bad2" sh install.sh; echo "exit=$?"
printf x >> "$T/srv/latest/download/$archive"
BASE_URL=http://127.0.0.1:8765 INSTALL_DIR="$T/bad3" sh install.sh; echo "exit=$?"
ls "$T"/bad1 "$T"/bad2 "$T"/bad3 2>&1
```
Expected: `VERSION invalide`, exit=1; `téléchargement impossible`, exit=1; `somme de contrôle invalide`, exit=1; the three `ls` calls report `No such file or directory` (nothing installed). Then stop the HTTP server and `rm -rf "$T"`.

- [ ] **Step 6: Commit**

```bash
git add install.sh
git commit -m "feat: add install script for GitHub Release binaries

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Release workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Interfaces:**
- Consumes: `ci.yml` via `workflow_call` (Task 6); `next-version.sh` contract (Task 4); `.goreleaser.yaml` (Task 8); `install.sh` env contract and `--version` (Tasks 9, 3).

- [ ] **Step 1: Write `.github/workflows/release.yml`**

````yaml
name: Release

on:
  workflow_dispatch:
    inputs:
      bump:
        description: Partie de la version à incrémenter
        type: choice
        options: [patch, minor, major]
        default: patch

permissions:
  contents: read

concurrency:
  group: release
  cancel-in-progress: false

env:
  MODULE: github.com/martialpmt/xalantis-mcp-go

jobs:
  gate:
    uses: ./.github/workflows/ci.yml

  release:
    needs: gate
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    permissions:
      contents: write
    outputs:
      version: ${{ steps.version.outputs.version }}
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          ref: ${{ github.sha }}
          fetch-depth: 0
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: stable

      - name: Compute version
        id: version
        env:
          BUMP: ${{ inputs.bump }}
        run: |
          latest=$(git tag --list 'v*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n1 || true)
          version=$(sh .github/scripts/next-version.sh "$latest" "$BUMP")
          if git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
            echo "::error::Le tag $version existe déjà"
            exit 1
          fi
          echo "Version : ${latest:-aucune} -> $version"
          echo "version=$version" >> "$GITHUB_OUTPUT"

      - name: Create tag
        id: tag
        env:
          VERSION: ${{ steps.version.outputs.version }}
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
          git tag -a "$VERSION" -m "Release $VERSION"
          git push origin "$VERSION"

      - name: GoReleaser
        id: goreleaser
        uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3
        with:
          distribution: goreleaser
          version: v2.18.1
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Publish to the Go module proxy
        id: proxy
        working-directory: ${{ runner.temp }}
        env:
          VERSION: ${{ steps.version.outputs.version }}
          GOPROXY: https://proxy.golang.org
        run: |
          for attempt in 1 2 3 4 5; do
            if go list -m "$MODULE@$VERSION"; then
              exit 0
            fi
            sleep $((attempt * 10))
          done
          echo "::error::Le proxy Go ne trouve pas $MODULE@$VERSION"
          exit 1

      - name: Summary
        env:
          VERSION: ${{ steps.version.outputs.version }}
        run: |
          {
            echo "## Release $VERSION"
            echo
            echo "- GitHub Release : $GITHUB_SERVER_URL/$GITHUB_REPOSITORY/releases/tag/$VERSION"
            echo "- pkg.go.dev : https://pkg.go.dev/$MODULE@$VERSION"
            echo
            echo '```sh'
            echo "curl -fsSL https://raw.githubusercontent.com/$GITHUB_REPOSITORY/main/install.sh | VERSION=$VERSION sh"
            echo "go install $MODULE/cmd/xalantis-projects-mcp@$VERSION"
            echo '```'
          } >> "$GITHUB_STEP_SUMMARY"

      - name: Recovery instructions (GoReleaser failed)
        if: failure() && steps.goreleaser.outcome == 'failure'
        env:
          VERSION: ${{ steps.version.outputs.version }}
        run: |
          {
            echo "## Échec de GoReleaser après la création du tag $VERSION"
            echo
            echo "Rien n'a été publié sur le proxy Go. Supprimez la release GitHub partielle s'il y en a une, puis le tag, corrigez et relancez :"
            echo
            echo '```sh'
            echo "git push --delete origin $VERSION"
            echo '```'
          } >> "$GITHUB_STEP_SUMMARY"

      - name: Recovery instructions (proxy failed)
        if: failure() && steps.proxy.outcome == 'failure'
        env:
          VERSION: ${{ steps.version.outputs.version }}
        run: |
          {
            echo "## La release $VERSION est publiée, mais le proxy Go ne l'a pas encore"
            echo
            echo "Ne supprimez pas le tag. Réessayez plus tard :"
            echo
            echo '```sh'
            echo "GOPROXY=https://proxy.golang.org go list -m $MODULE@$VERSION"
            echo '```'
          } >> "$GITHUB_STEP_SUMMARY"

  install-smoke:
    needs: release
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          ref: ${{ needs.release.outputs.version }}
          persist-credentials: false
      - name: Install from the release
        env:
          VERSION: ${{ needs.release.outputs.version }}
        run: |
          INSTALL_DIR="$RUNNER_TEMP/bin" sh install.sh
          got=$("$RUNNER_TEMP/bin/xalantis-projects-mcp" --version)
          if [ "$got" != "$VERSION" ]; then
            echo "::error::Version installée $got, attendue $VERSION"
            exit 1
          fi
````

- [ ] **Step 2: Lint all workflows**

Run: `actionlint`
Expected: no output (actionlint also runs shellcheck on every `run:` block; fix anything it reports).

- [ ] **Step 3: Dry-run the version step locally**

Run:
```bash
latest=$(git tag --list 'v*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n1 || true)
sh .github/scripts/next-version.sh "$latest" minor
```
Expected: `v0.1.0` (the repo has no tags yet).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add manual release workflow with Go proxy publish and install smoke test

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: README

**Files:**
- Modify: `README.md` (title area; replace the `## Prérequis` and `## Installation (Claude Desktop, macOS)` sections; `## Développement` section)

- [ ] **Step 1: Add badges** directly under `# xalantis-projects-mcp`, followed by a blank line:

```markdown
[![CI](https://github.com/martialpmt/xalantis-mcp-go/actions/workflows/ci.yml/badge.svg)](https://github.com/martialpmt/xalantis-mcp-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/martialpmt/xalantis-mcp-go.svg)](https://pkg.go.dev/github.com/martialpmt/xalantis-mcp-go)
```

- [ ] **Step 2: Replace everything from `## Prérequis` up to (not including) `## Variables d'environnement`** with:

~~~markdown
## Prérequis

- Une clé API Xalantis avec les scopes voulus (Xalantis → paramètres de la clé API).
- macOS ou Linux (amd64 ou arm64) pour le script d'installation ; Windows via
  l'archive `.zip` des releases.

## Installation

### Script (macOS, Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/martialpmt/xalantis-mcp-go/main/install.sh | sh
```

Le script télécharge la release depuis GitHub, vérifie sa somme de contrôle
(SHA-256) et installe le binaire dans `~/.local/bin`, sans `sudo`.
Variables facultatives :

- `VERSION` : version à installer (défaut `latest`), par ex.
  `curl -fsSL … | VERSION=v0.1.0 sh` ;
- `INSTALL_DIR` : dossier d'installation (défaut `~/.local/bin`).

Vérification : `xalantis-projects-mcp --version`.

### Téléchargement manuel (dont Windows)

Sur la page [Releases](https://github.com/martialpmt/xalantis-mcp-go/releases),
télécharger l'archive du système (`xalantis-projects-mcp_<os>_<arch>.tar.gz`,
`.zip` pour Windows) et `checksums.txt`, puis :

```bash
shasum -a 256 -c checksums.txt --ignore-missing
tar -xzf xalantis-projects-mcp_darwin_arm64.tar.gz xalantis-projects-mcp
```

Placer le binaire dans un dossier du `PATH`. Sur macOS, un fichier téléchargé
avec le navigateur est mis en quarantaine :
`xattr -d com.apple.quarantine xalantis-projects-mcp`.

### Avec Go (≥ 1.24)

```bash
go install github.com/martialpmt/xalantis-mcp-go/cmd/xalantis-projects-mcp@latest
```

Le binaire est installé dans `$(go env GOPATH)/bin`.

## Configuration (Claude Desktop)

1. Claude Desktop → Réglages → Développeur → Éditer la config, ajouter dans
   `mcpServers` le chemin **absolu** du binaire (`~` n'est pas interprété) :
   ```json
   {
     "mcpServers": {
       "xalantis-projets": {
         "command": "/Users/VOTRE_LOGIN/.local/bin/xalantis-projects-mcp",
         "env": {
           "XALANTIS_API_KEY": "sk_live_VOTRE_CLE"
         }
       }
     }
   }
   ```
   (Si le fichier contient déjà le connecteur `xalantis`, ajouter simplement l'entrée
   `xalantis-projets` à côté, dans le même bloc `mcpServers`.)
2. Quitter complètement Claude Desktop et le rouvrir.

~~~

- [ ] **Step 3: Extend `## Développement`** — after the `- Test de bout en bout : …` bullet, add:

```markdown
- Vérifications locales (comme la CI) : `gofmt -l .`, `golangci-lint run`,
  `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`,
  `sh .github/scripts/next-version_test.sh`
- Build de release local : `goreleaser release --snapshot --clean` (résultat dans `dist/`).
- Publier une version : GitHub → Actions → **Release** → *Run workflow* sur
  `main`, choisir `patch`, `minor` ou `major`. Le workflow relance la CI, crée
  le tag `vX.Y.Z`, publie les binaires dans les Releases et enregistre le
  module sur pkg.go.dev. Les versions restent en `v0.x`/`v1.x` (une `v2`
  exigerait le suffixe `/v2` dans le chemin du module).
```

- [ ] **Step 4: Check the README**

Run: `grep -n 'paymetrust\|Apple Silicon pour le binaire fourni\|Documents/xalantis-projects-mcp' README.md`
Expected: no output (old instructions are gone).

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: document install script, releases and go install

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: Final verification

No new files. Every command must pass before reporting completion.

- [ ] **Step 1: Run the full local check suite**

```bash
test -z "$(gofmt -l .)" && echo gofmt-ok
go mod tidy -diff && echo tidy-ok
go vet ./... && go test -race ./...
golangci-lint run ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
gitleaks git --redact .
shellcheck install.sh .github/scripts/*.sh
sh .github/scripts/next-version_test.sh
actionlint
goreleaser check
goreleaser release --snapshot --clean
go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp && python3 test_mcp.py
```

Expected: every command exits 0 (`test_mcp.py` needs `XALANTIS_API_KEY`; if it is not set, record the test as skipped rather than passed).

- [ ] **Step 2: Check the branch contents**

Run: `git status --short && git log --oneline main..HEAD`
Expected: clean tree; commits for the spec plus Tasks 1–11; `git ls-files | grep -x xalantis-projects-mcp` prints nothing.

- [ ] **Step 3: Hand over to the user** with these post-push steps (the agent does not push):
  1. Push the branch, open a PR, and confirm `CI` and `CodeQL` are green.
  2. Merge to `main`.
  3. Actions → Release → Run workflow on `main` with `minor` → expect tag `v0.1.0`, a GitHub Release with 6 archives + `checksums.txt`, green `install-smoke` on ubuntu and macOS.
  4. Check `https://pkg.go.dev/github.com/martialpmt/xalantis-mcp-go` (indexing can take a few minutes) and `go install github.com/martialpmt/xalantis-mcp-go/cmd/xalantis-projects-mcp@v0.1.0`.
  5. Optional: add branch protection on `main` requiring the CI jobs.
