# Write Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let MCP clients tell reads from writes (tool annotations + a read/write tool split), let users disable writes with `XALANTIS_READ_ONLY`, and generate a missing `Idempotency-Key`.

**Architecture:** `mcp.Tool` gains an `Annotations` field. `tools.GenericTools` gains a `readOnly` parameter and a new `xalantis_read_operation` tool; both it and `xalantis_call_operation` put a method check in front of the existing `callOperation`. `buildHeaders` generates a UUID v4 when the operation declares `Idempotency-Key` and the caller gave none. `main.go` parses `XALANTIS_READ_ONLY` and builds the server instructions.

**Tech Stack:** Go 1.24.7, standard library only (`crypto/rand`, `strconv`). Python 3 for the end-to-end test.

**Spec:** `docs/superpowers/specs/2026-09-17-write-safety-design.md`

## Global Constraints

- No new dependency: standard library only (`go.mod` has no `require`).
- User-facing text (errors, descriptions, README) is in French, like the existing code.
- Exact error messages:
  - `opération d'écriture : utilisez xalantis_call_operation`
  - `écriture désactivée (XALANTIS_READ_ONLY)`
  - `lecture : utilisez xalantis_read_operation`
  - suffix on a failed call with a generated key: ` (Idempotency-Key générée : <uuid> — réutilisez-la pour réessayer)`
- Generated key: UUID v4 (RFC 9562), lowercase hex, `crypto/rand`.
- `XALANTIS_READ_ONLY`: empty = off, otherwise `strconv.ParseBool`; invalid value = stderr message and exit code 1.
- Local checks before each commit: `gofmt -l .` (empty output), `go vet ./...`, `go test ./...`, `golangci-lint run`.
- Commit messages: Conventional Commits, ending with `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.
- Branch: `write-safety`.

---

### Task 1: `annotations` field on MCP tools

**Files:**
- Modify: `internal/mcp/server.go` (struct `Tool`)
- Test: `internal/mcp/server_test.go`

**Interfaces:**
- Produces: `mcp.Tool.Annotations map[string]any`, serialized as `annotations`, omitted when nil.

- [ ] **Step 1: Write the failing test**

Append to `internal/mcp/server_test.go`:

```go
func TestToolsListAnnotations(t *testing.T) {
	s := NewServer("srv", "1", "")
	s.Register(
		Tool{Name: "lire", Annotations: map[string]any{"readOnlyHint": true}},
		Tool{Name: "brut"},
	)
	got := run(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	tools := got["1"]["result"].(map[string]any)["tools"].([]any)
	lire, brut := tools[0].(map[string]any), tools[1].(map[string]any)
	if ann, ok := lire["annotations"].(map[string]any); !ok || ann["readOnlyHint"] != true {
		t.Fatalf("annotations de lire : %v", lire)
	}
	if _, ok := brut["annotations"]; ok {
		t.Fatalf("annotations vides : champ absent attendu, obtenu %v", brut)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp -run TestToolsListAnnotations`
Expected: FAIL to compile with `unknown field Annotations in struct literal`.

- [ ] **Step 3: Write minimal implementation**

In `internal/mcp/server.go`, replace the `Tool` struct:

```go
// Tool décrit un outil exposé au client MCP. Annotations porte les indices
// MCP (readOnlyHint, destructiveHint…) ; nil = champ absent.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
	Handler     Handler        `json:"-"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mcp`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(mcp): expose tool annotations in tools/list

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: area counts filtered by method

**Files:**
- Modify: `internal/openapi/catalog.go` (`AreaCounts`, around line 212)
- Modify: `internal/tools/generic.go:55` (the only caller)
- Test: `internal/openapi/catalog_test.go`

**Interfaces:**
- Produces: `func (c *Catalog) AreaCounts(method string) []AreaCount`. Empty `method` = every operation; otherwise only operations whose `Method` equals `strings.ToUpper(method)`.

The embedded spec has 67 GET operations, including 38 in "Projets et tâches" and 17 in "Tickets".

- [ ] **Step 1: Write the failing test**

In `internal/openapi/catalog_test.go`, in `TestLoadFiltersAreas`, change `for _, a := range c.AreaCounts() {` to `for _, a := range c.AreaCounts("") {`, then append:

```go
func TestAreaCountsByMethod(t *testing.T) {
	c := loadEmbedded(t)
	got, total := map[string]int{}, 0
	for _, a := range c.AreaCounts("get") {
		got[a.Area] = a.Operations
		total += a.Operations
	}
	if got["Projets et tâches"] != 38 || got["Tickets"] != 17 || total != 67 {
		t.Errorf("GET : %v (total %d), attendu 38 / 17 / 67", got, total)
	}
	if n := len(c.AreaCounts("TRACE")); n != 0 {
		t.Errorf("méthode absente : %d domaines, attendu 0", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/openapi`
Expected: FAIL to compile with `too many arguments in call to c.AreaCounts`.

- [ ] **Step 3: Write minimal implementation**

In `internal/openapi/catalog.go`, replace `AreaCounts`:

```go
// AreaCounts renvoie les domaines dans l'ordre de Areas, en ne comptant que
// les opérations de la méthode method (vide = toutes).
func (c *Catalog) AreaCounts(method string) []AreaCount {
	counts := map[string]int{}
	for _, op := range c.ops {
		if method == "" || op.Method == strings.ToUpper(method) {
			counts[op.Area]++
		}
	}
	var out []AreaCount
	for _, a := range Areas {
		if counts[a] > 0 {
			out = append(out, AreaCount{Area: a, Operations: counts[a]})
		}
	}
	return out
}
```

In `internal/tools/generic.go`, change `cat.AreaCounts()` to `cat.AreaCounts("")`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: all `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/openapi/catalog.go internal/openapi/catalog_test.go internal/tools/generic.go
git commit -m "feat(openapi): filter area counts by HTTP method

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: read/write tool split, read-only mode in tools, annotations

**Files:**
- Modify: `internal/tools/projects.go` (hint maps near the helpers; `ProjectTools` return)
- Modify: `internal/tools/generic.go` (`GenericTools`, new `checkMethod`)
- Modify: `cmd/xalantis-mcp-go/main.go` (call site only: add `false`)
- Test: `internal/tools/generic_test.go`, `internal/tools/projects_test.go`

**Interfaces:**
- Consumes: `mcp.Tool.Annotations` (Task 1), `Catalog.AreaCounts(method string)` (Task 2).
- Produces:
  - `func GenericTools(c *xalantis.Client, cat *openapi.Catalog, filesDir string, readOnly bool) []mcp.Tool`. Order: search, describe, read, then call (call omitted when `readOnly`).
  - Package vars `readOnlyHint = map[string]any{"readOnlyHint": true}` and `destructiveHint = map[string]any{"destructiveHint": true}`.
  - `func checkMethod(cat *openapi.Catalog, id string, read, readOnly bool) error`.

- [ ] **Step 1: Write the failing tests**

In `internal/tools/generic_test.go`, change the `genericTool` helper body to:

```go
	return find(t, GenericTools(rec.client(), catalog(t), dir, false), name)
```

Append:

```go
func TestToolAnnotations(t *testing.T) {
	want := map[string]string{
		"xalantis_search_operations":  "readOnlyHint",
		"xalantis_describe_operation": "readOnlyHint",
		"xalantis_read_operation":     "readOnlyHint",
		"xalantis_call_operation":     "destructiveHint",
	}
	tools := GenericTools(nil, catalog(t), t.TempDir(), false)
	if len(tools) != len(want) {
		t.Fatalf("%d outils génériques, attendu %d", len(tools), len(want))
	}
	for _, tool := range tools {
		if hint := want[tool.Name]; hint == "" || tool.Annotations[hint] != true {
			t.Errorf("%s : annotations %v", tool.Name, tool.Annotations)
		}
	}
	for _, tool := range ProjectTools(nil) {
		if tool.Annotations["readOnlyHint"] != true {
			t.Errorf("%s : readOnlyHint attendu, obtenu %v", tool.Name, tool.Annotations)
		}
	}
}

func TestReadAndCallRejectWrongMethod(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	read := genericTool(t, rec, dir, "xalantis_read_operation")
	call := genericTool(t, rec, dir, "xalantis_call_operation")

	if _, err := read.Handler(map[string]any{"operation_id": "post_tickets"}); err == nil || err.Error() != "opération d'écriture : utilisez xalantis_call_operation" {
		t.Errorf("lecture d'un POST : %v", err)
	}
	if _, err := call.Handler(map[string]any{"operation_id": "get_tickets"}); err == nil || err.Error() != "lecture : utilisez xalantis_read_operation" {
		t.Errorf("écriture d'un GET : %v", err)
	}
	if _, err := read.Handler(map[string]any{"operation_id": "nope"}); err == nil || !strings.Contains(err.Error(), "opération inconnue") {
		t.Errorf("opération inconnue : %v", err)
	}
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

func TestReadOnlyMode(t *testing.T) {
	rec := newRecorder(t, nil)
	tools := GenericTools(rec.client(), catalog(t), t.TempDir(), true)
	if len(tools) != 3 {
		t.Fatalf("%d outils en lecture seule, attendu 3", len(tools))
	}
	for _, tool := range tools {
		if tool.Name == "xalantis_call_operation" {
			t.Fatal("xalantis_call_operation ne doit pas être exposé en lecture seule")
		}
	}

	search := find(t, tools, "xalantis_search_operations")
	out, err := search.Handler(map[string]any{"query": "ticket"})
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Count      int                `json:"count"`
		Operations []openapi.Summary `json:"operations"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Count == 0 {
		t.Fatalf("recherche : %v %s", err, out)
	}
	for _, op := range res.Operations {
		if op.Method != "GET" {
			t.Errorf("opération non GET en lecture seule : %+v", op)
		}
	}
	if out, err := search.Handler(map[string]any{"query": "ticket", "method": "POST"}); err != nil || out != `{"count":0,"operations":[]}` {
		t.Errorf("filtre POST : %v %s", err, out)
	}
	if out, err := search.Handler(map[string]any{}); err != nil || !strings.Contains(out, `{"area":"Tickets","operations":17}`) {
		t.Errorf("domaines : %v %s", err, out)
	}

	read := find(t, tools, "xalantis_read_operation")
	if _, err := read.Handler(map[string]any{"operation_id": "post_tickets"}); err == nil || err.Error() != "écriture désactivée (XALANTIS_READ_ONLY)" {
		t.Errorf("écriture en lecture seule : %v", err)
	}
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}
```

Then move the existing GET calls to the read tool (otherwise they now fail with `lecture : utilisez xalantis_read_operation`):

1. `TestCallQueryMapping`: `genericTool(t, rec, t.TempDir(), "xalantis_call_operation")` → `"xalantis_read_operation"`.
2. `download` helper: `genericTool(t, rec, dir, "xalantis_call_operation")` → `"xalantis_read_operation"`.
3. `TestSaveToAppliesToText`, `TestSaveToOutsideFolderRejected`, `TestSaveToOutsideFolderNonexistentParentHidesExistence`, `TestSaveToSymlinkParentEscapeRejected`, `TestSaveToRelativeAccepted`, `TestSaveToMissingParentDir`: in each, `genericTool(t, rec, dir, "xalantis_call_operation")` → `"xalantis_read_operation"`. (All use `get_projects_By_projectUuid_exports`. Leave `uploadCall` on the call tool: it posts.)
4. `TestCallEmptyAndTextResponses`: after the `call := …` line add
   ```go
   	read := genericTool(t, rec, t.TempDir(), "xalantis_read_operation")
   ```
   and change the CSV call `out, err = call.Handler(map[string]any{"operation_id": "get_projects_By_projectUuid_exports", …` to `out, err = read.Handler(…` (same arguments).
5. `TestCallRejectsBadInputWithoutCallingAPI`: after the `call := …` line add
   ```go
   	read := genericTool(t, rec, dir, "xalantis_read_operation")
   ```
   and replace the loop with:
   ```go
   	for name, c := range cases {
   		tool := call
   		if strings.HasPrefix(c["operation_id"].(string), "get_") {
   			tool = read
   		}
   		if _, err := tool.Handler(c); err == nil {
   			t.Errorf("%s : erreur attendue", name)
   		}
   	}
   ```

Check nothing else sends a GET through the call tool:

Run: `grep -n '"operation_id": *"get_' internal/tools/generic_test.go`
Expected: every hit is in one of the tests above, or in `download`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tools`
Expected: FAIL to compile with `too many arguments in call to GenericTools`.

- [ ] **Step 3: Write the implementation**

In `internal/tools/projects.go`, after `pagingProps`, add:

```go
// Indices MCP : un client peut approuver les lectures automatiquement et
// demander confirmation avant une écriture.
var (
	readOnlyHint    = map[string]any{"readOnlyHint": true}
	destructiveHint = map[string]any{"destructiveHint": true}
)
```

In `ProjectTools`, change `return []mcp.Tool{` to `tools := []mcp.Tool{`. At the end of the function, after the `xalantis_list_members` entry, replace

```go
		},
	}
}
```

with

```go
		},
	}
	for i := range tools {
		tools[i].Annotations = readOnlyHint
	}
	return tools
}
```

In `internal/tools/generic.go`, replace the whole `GenericTools` function with:

```go
// GenericTools renvoie les outils de recherche, description, lecture et
// écriture des opérations du catalogue. filesDir est le seul dossier dans
// lequel le serveur lit (envoi) et écrit (save_to, téléchargements) des
// fichiers. readOnly retire l'outil d'écriture et limite la recherche aux GET.
func GenericTools(c *xalantis.Client, cat *openapi.Catalog, filesDir string, readOnly bool) []mcp.Tool {
	areas := make([]any, len(openapi.Areas))
	for i, a := range openapi.Areas {
		areas[i] = a
	}
	next, listMethod := "xalantis_read_operation (GET) ou xalantis_call_operation (écritures)", ""
	if readOnly {
		next, listMethod = "xalantis_read_operation (écritures désactivées)", "GET"
	}
	opID := map[string]any{"operation_id": prop("string", "operation_id renvoyé par xalantis_search_operations.")}
	readProps := merge(opID, map[string]any{
		"path_params": map[string]any{"type": "object", "description": "Paramètres de chemin, ex. {\"projectUuid\": \"…\"}."},
		"query":       map[string]any{"type": "object", "description": "Paramètres de requête. Tableau = valeurs répétées (nom[]), objet = nom[clé]."},
		"headers":     map[string]any{"type": "object", "description": "En-têtes déclarés par l'opération (If-Match ; Idempotency-Key est générée si absente)."},
		"save_to":     prop("string", "Chemin où enregistrer la réponse (fichier ou texte, ex. un export CSV), dans le dossier autorisé (XALANTIS_FILES_DIR ; chemin relatif = relatif à ce dossier). Sans save_to, les fichiers reçus sont enregistrés dans ce dossier et le texte est renvoyé directement."),
	})
	tools := []mcp.Tool{
		{
			Name: "xalantis_search_operations",
			Description: "Étape 1/3 pour toute opération Xalantis sans outil dédié (projets, tickets, SLA, catalogue…). " +
				"Cherche les opérations par mots-clés (accents ignorés), domaine et méthode HTTP. " +
				"Sans aucun filtre, renvoie la liste des domaines. Ensuite : xalantis_describe_operation.",
			InputSchema: schema(nil, map[string]any{
				"query":  prop("string", "Mots-clés, tous requis (ex. « créer ticket »)."),
				"area":   map[string]any{"type": "string", "enum": areas, "description": "Domaine fonctionnel."},
				"method": map[string]any{"type": "string", "enum": []any{"GET", "POST", "PUT", "PATCH", "DELETE"}, "description": "Méthode HTTP."},
			}),
			Annotations: readOnlyHint,
			Handler: func(raw map[string]any) (string, error) {
				a := args(raw)
				query, area, method := a.str("query"), a.str("area"), a.str("method")
				if query == "" && area == "" && method == "" {
					return toJSON(map[string]any{"areas": cat.AreaCounts(listMethod)})
				}
				if readOnly {
					if method != "" && !strings.EqualFold(method, "GET") {
						return toJSON(map[string]any{"count": 0, "operations": []openapi.Summary{}})
					}
					method = "GET"
				}
				res := cat.Search(query, area, method)
				return toJSON(map[string]any{"count": len(res), "operations": res})
			},
		},
		{
			Name: "xalantis_describe_operation",
			Description: "Étape 2/3 : paramètres (chemin, requête, en-têtes) et schéma du corps d'une opération Xalantis. " +
				"Ensuite : " + next + ".",
			InputSchema: schema([]string{"operation_id"}, opID),
			Annotations: readOnlyHint,
			Handler: func(raw map[string]any) (string, error) {
				d, err := cat.Describe(args(raw).str("operation_id"))
				if err != nil {
					return "", err
				}
				return toJSON(d)
			},
		},
		{
			Name: "xalantis_read_operation",
			Description: "Étape 3/3 pour une lecture : exécute une opération GET décrite par xalantis_describe_operation. " +
				"Ne modifie aucune donnée. Les fichiers reçus sont enregistrés localement.",
			InputSchema: schema([]string{"operation_id"}, readProps),
			Annotations: readOnlyHint,
			Handler: func(raw map[string]any) (string, error) {
				a := args(raw)
				if err := checkMethod(cat, a.str("operation_id"), true, readOnly); err != nil {
					return "", err
				}
				return callOperation(c, cat, filesDir, a)
			},
		},
	}
	if readOnly {
		return tools
	}
	return append(tools, mcp.Tool{
		Name: "xalantis_call_operation",
		Description: "Étape 3/3 pour une écriture : exécute une opération POST, PUT, PATCH ou DELETE décrite par xalantis_describe_operation. " +
			"Peut créer, modifier ou supprimer des données selon les scopes de la clé API. " +
			"Les fichiers envoyés sont des chemins locaux ; les fichiers reçus sont enregistrés localement.",
		InputSchema: schema([]string{"operation_id"}, merge(readProps, map[string]any{
			"body":  map[string]any{"type": "object", "description": "Corps JSON, ou champs texte pour une opération multipart."},
			"files": map[string]any{"type": "object", "description": "Opérations multipart : champ → chemin ou liste de chemins, dans le dossier autorisé (XALANTIS_FILES_DIR ; chemin relatif = relatif à ce dossier)."},
		})),
		Annotations: destructiveHint,
		Handler: func(raw map[string]any) (string, error) {
			a := args(raw)
			if err := checkMethod(cat, a.str("operation_id"), false, readOnly); err != nil {
				return "", err
			}
			return callOperation(c, cat, filesDir, a)
		},
	})
}

// checkMethod refuse une opération qui ne correspond pas à l'outil : read
// n'accepte que GET, l'outil d'écriture tout sauf GET. Une opération inconnue
// passe : callOperation renvoie alors l'erreur « opération inconnue ».
func checkMethod(cat *openapi.Catalog, id string, read, readOnly bool) error {
	op, ok := cat.Get(id)
	if !ok {
		return nil
	}
	isGet := op.Method == "GET"
	switch {
	case read && !isGet && readOnly:
		return errors.New("écriture désactivée (XALANTIS_READ_ONLY)")
	case read && !isGet:
		return errors.New("opération d'écriture : utilisez xalantis_call_operation")
	case !read && isGet:
		return errors.New("lecture : utilisez xalantis_read_operation")
	}
	return nil
}
```

(`errors` and `strings` are already imported in `generic.go`.)

In `cmd/xalantis-mcp-go/main.go`, change
`srv.Register(tools.GenericTools(client, cat, filesDir)...)` to
`srv.Register(tools.GenericTools(client, cat, filesDir, false)...)`.
Task 4 replaces `false`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`
Expected: no gofmt output, all `ok`, `0 issues.`

- [ ] **Step 5: Commit**

```bash
git add internal/tools cmd/xalantis-mcp-go/main.go
git commit -m "feat(tools): split read and write operation tools with MCP annotations

xalantis_read_operation runs GET operations (readOnlyHint);
xalantis_call_operation now only runs writes (destructiveHint).
GenericTools takes a readOnly flag that hides the write tool and
limits search to GET operations.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: `XALANTIS_READ_ONLY` and server instructions

**Files:**
- Modify: `cmd/xalantis-mcp-go/main.go`
- Create: `cmd/xalantis-mcp-go/main_test.go`

**Interfaces:**
- Consumes: `tools.GenericTools(..., readOnly bool)` (Task 3).
- Produces: `func parseReadOnly(v string) (bool, error)`, `func serverInstructions(readOnly bool) string` (package `main`).

- [ ] **Step 1: Write the failing test**

Create `cmd/xalantis-mcp-go/main_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestParseReadOnly(t *testing.T) {
	cases := []struct {
		in      string
		want    bool
		wantErr bool
	}{
		{"", false, false},
		{"  ", false, false},
		{"1", true, false},
		{"true", true, false},
		{"TRUE", true, false},
		{" 1 ", true, false},
		{"0", false, false},
		{"false", false, false},
		{"oui", false, true},
	}
	for _, c := range cases {
		got, err := parseReadOnly(c.in)
		if got != c.want || (err != nil) != c.wantErr {
			t.Errorf("parseReadOnly(%q) = %v, %v ; attendu %v, erreur %v", c.in, got, err, c.want, c.wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), "XALANTIS_READ_ONLY") {
			t.Errorf("parseReadOnly(%q) : l'erreur doit nommer la variable : %v", c.in, err)
		}
	}
}

func TestServerInstructions(t *testing.T) {
	rw := serverInstructions(false)
	if !strings.Contains(rw, "xalantis_read_operation") || !strings.Contains(rw, "xalantis_call_operation") {
		t.Errorf("mode normal : %s", rw)
	}
	ro := serverInstructions(true)
	if strings.Contains(ro, "xalantis_call_operation") || !strings.Contains(ro, "XALANTIS_READ_ONLY") {
		t.Errorf("lecture seule : %s", ro)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/xalantis-mcp-go`
Expected: FAIL to compile with `undefined: parseReadOnly` and `undefined: serverInstructions`.

- [ ] **Step 3: Write the implementation**

In `cmd/xalantis-mcp-go/main.go`:

1. In the package comment, after the `XALANTIS_FILES_DIR` lines, add:
   ```go
   //	XALANTIS_READ_ONLY (optionnel)   — true/1 : n'expose aucune opération d'écriture ; défaut : false
   ```
2. Add `"strconv"` and `"strings"` to the imports.
3. Replace the `const` block with:
   ```go
   const serverName = "xalantis-mcp-go"

   // serverInstructions renvoie les consignes d'initialisation MCP.
   func serverInstructions(readOnly bool) string {
   	s := "Pour les lectures courantes de projets, utilisez les 7 outils dédiés (xalantis_list_projects, xalantis_list_tasks…). " +
   		"Pour toute autre opération Xalantis (tickets, SLA, catalogue…) : " +
   		"xalantis_search_operations, puis xalantis_describe_operation, puis "
   	if readOnly {
   		return s + "xalantis_read_operation. Mode lecture seule (XALANTIS_READ_ONLY) : les écritures sont désactivées."
   	}
   	return s + "xalantis_read_operation pour une lecture (GET) ou xalantis_call_operation pour une écriture."
   }

   // parseReadOnly lit XALANTIS_READ_ONLY : vide = désactivé.
   func parseReadOnly(v string) (bool, error) {
   	v = strings.TrimSpace(v)
   	if v == "" {
   		return false, nil
   	}
   	b, err := strconv.ParseBool(v)
   	if err != nil {
   		return false, fmt.Errorf("XALANTIS_READ_ONLY invalide (%q) : utilisez true/false ou 1/0", v)
   	}
   	return b, nil
   }
   ```
4. In `main`, right after the `--version` block (before `cat, err := openapi.Load()`), add:
   ```go
   	readOnly, err := parseReadOnly(os.Getenv("XALANTIS_READ_ONLY"))
   	if err != nil {
   		fmt.Fprintln(os.Stderr, err)
   		os.Exit(1)
   	}
   ```
5. Replace `srv := mcp.NewServer(serverName, ver, instructions)` with
   `srv := mcp.NewServer(serverName, ver, serverInstructions(readOnly))`
   and `tools.GenericTools(client, cat, filesDir, false)` with
   `tools.GenericTools(client, cat, filesDir, readOnly)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`
Expected: no gofmt output, all `ok`, `0 issues.`

Also: `go build -o xalantis-mcp-go ./cmd/xalantis-mcp-go && XALANTIS_READ_ONLY=oui ./xalantis-mcp-go < /dev/null; echo "exit=$?"; rm -f xalantis-mcp-go`
Expected: `XALANTIS_READ_ONLY invalide ("oui") : utilisez true/false ou 1/0` then `exit=1`.

- [ ] **Step 5: Commit**

```bash
git add cmd/xalantis-mcp-go
git commit -m "feat: add XALANTIS_READ_ONLY to disable write operations

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: generate a missing `Idempotency-Key`

**Files:**
- Modify: `internal/tools/generic.go` (`buildHeaders`, `callOperation`, new `newUUID`)
- Test: `internal/tools/generic_test.go`

**Interfaces:**
- Produces:
  - `func buildHeaders(op *openapi.Operation, values map[string]any) (map[string]string, string, error)`. The second result is the generated key, `""` when none was generated.
  - `func newUUID() string` (UUID v4).

- [ ] **Step 1: Write the failing tests**

In `internal/tools/generic_test.go`, add `"regexp"` to the imports, and in `TestCallRejectsBadInputWithoutCallingAPI` delete the `"en-tête requis": …` case (that call now succeeds with a generated key). Append:

```go
var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func newTaskInput() map[string]any {
	return map[string]any{
		"operation_id": "post_projects_By_projectUuid_tasks",
		"path_params":  map[string]any{"projectUuid": "p-1"},
		"body":         map[string]any{"title": "x"},
	}
}

func TestCallGeneratesIdempotencyKey(t *testing.T) {
	rec := newRecorder(t, nil)
	call := genericTool(t, rec, t.TempDir(), "xalantis_call_operation")
	blank := newTaskInput()
	blank["headers"] = map[string]any{"Idempotency-Key": "  "}
	for _, in := range []map[string]any{newTaskInput(), newTaskInput(), blank} {
		if _, err := call.Handler(in); err != nil {
			t.Fatal(err)
		}
	}
	keys := make([]string, len(rec.requests))
	for i, r := range rec.requests {
		keys[i] = r.Header.Get("Idempotency-Key")
		if !uuidV4.MatchString(keys[i]) {
			t.Errorf("appel %d : clé %q, UUID v4 attendu", i, keys[i])
		}
	}
	if keys[0] == keys[1] {
		t.Errorf("deux appels, même clé générée : %s", keys[0])
	}
}

func TestCallFailureReportsGeneratedKey(t *testing.T) {
	rec := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boum"}`))
	})
	call := genericTool(t, rec, t.TempDir(), "xalantis_call_operation")

	_, err := call.Handler(newTaskInput())
	sent := rec.requests[0].Header.Get("Idempotency-Key")
	want := "(Idempotency-Key générée : " + sent + " — réutilisez-la pour réessayer)"
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") || !strings.HasSuffix(err.Error(), want) {
		t.Errorf("erreur = %v, attendu le suffixe %q", err, want)
	}

	given := newTaskInput()
	given["headers"] = map[string]any{"Idempotency-Key": "idem-1"}
	if _, err := call.Handler(given); err == nil || strings.Contains(err.Error(), "générée") {
		t.Errorf("clé fournie : %v", err)
	}
}

func TestBuildHeadersRequiredHeader(t *testing.T) {
	op := &openapi.Operation{ID: "op", Params: []openapi.Param{{Name: "If-Match", In: "header", Required: true}}}
	if _, _, err := buildHeaders(op, nil); err == nil || err.Error() != "en-tête requis manquant : If-Match" {
		t.Errorf("If-Match manquant : %v", err)
	}
	h, generated, err := buildHeaders(op, map[string]any{"if-match": "v1"})
	if err != nil || h["If-Match"] != "v1" || generated != "" {
		t.Errorf("If-Match fourni : %v %v %q", h, err, generated)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tools`
Expected: FAIL to compile with `assignment mismatch: 3 variables but buildHeaders returns 2 values`.

- [ ] **Step 3: Write the implementation**

In `internal/tools/generic.go`, add `"crypto/rand"` to the imports and replace `buildHeaders` with:

```go
// buildHeaders valide les en-têtes fournis. Si l'opération déclare
// Idempotency-Key et qu'aucune valeur n'est fournie, une clé est générée et
// renvoyée en second résultat ("" sinon).
func buildHeaders(op *openapi.Operation, values map[string]any) (map[string]string, string, error) {
	h := map[string]string{}
	for k, v := range values {
		var name string
		for _, p := range op.Params {
			if p.In == "header" && strings.EqualFold(p.Name, k) {
				name = p.Name
			}
		}
		if name == "" {
			return nil, "", fmt.Errorf("en-tête non autorisé pour %s : %s", op.ID, k)
		}
		s, err := scalar(name, v)
		if err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(s) != "" {
			h[name] = strings.TrimSpace(s)
		}
	}
	generated := ""
	for _, p := range op.Params {
		if p.In == "header" && strings.EqualFold(p.Name, "Idempotency-Key") && h[p.Name] == "" {
			generated = newUUID()
			h[p.Name] = generated
		}
	}
	for _, p := range op.Params {
		if p.In == "header" && p.Required && h[p.Name] == "" {
			return nil, "", fmt.Errorf("en-tête requis manquant : %s", p.Name)
		}
	}
	return h, generated, nil
}

// newUUID renvoie un UUID v4 aléatoire (RFC 9562).
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // depuis Go 1.24, ne renvoie jamais d'erreur (le programme s'arrête en cas d'échec)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
```

In `callOperation`, change

```go
	headers, err := buildHeaders(op, headerArgs)
```

to

```go
	headers, generatedKey, err := buildHeaders(op, headerArgs)
```

and change

```go
	resp, err := c.Do(op.Method, path, query, headers, reader, contentType)
	if err != nil {
		return "", err
	}
```

to

```go
	resp, err := c.Do(op.Method, path, query, headers, reader, contentType)
	if err != nil {
		if generatedKey != "" {
			return "", fmt.Errorf("%w (Idempotency-Key générée : %s — réutilisez-la pour réessayer)", err, generatedKey)
		}
		return "", err
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run`
Expected: no gofmt output, all `ok`, `0 issues.` If gosec flags the ignored `rand.Read` result, add `//nolint:gosec // G104: crypto/rand.Read ne renvoie jamais d'erreur depuis Go 1.24` on that line (the repo already uses this `//nolint:gosec // Gxxx: …` style).

- [ ] **Step 5: Commit**

```bash
git add internal/tools/generic.go internal/tools/generic_test.go
git commit -m "feat(tools): generate a missing Idempotency-Key

When an operation declares Idempotency-Key and none is given, send a
UUID v4 and report it in the error if the call fails, so a retry can
reuse it.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: end-to-end test and README

**Files:**
- Modify: `test_mcp.py`
- Modify: `README.md`

**Interfaces:**
- Consumes: everything above, through the built binary.

- [ ] **Step 1: Update the end-to-end test**

In `test_mcp.py`:

1. Change the first import line to `import json, os, re, subprocess, sys, tempfile, threading`.
2. In `msgs`, change the `"name"` of message id 14 (the download) from `xalantis_call_operation` to `xalantis_read_operation`. Replace the comment of message id 15 with `# Idempotency-Key absente -> générée`, and add after it:
   ```python
       {"jsonrpc": "2.0", "id": 16, "method": "tools/call",
        "params": {"name": "xalantis_read_operation", "arguments": {
            "operation_id": "post_projects_By_projectUuid_tasks", "path_params": {"projectUuid": "p-1"}}}},  # écriture via l'outil de lecture -> erreur sans appel API
   ```
3. Replace the block from `stdin_data = "".join(...)` through the loop that fills `resp` with:
   ```python
   home = tempfile.mkdtemp()

   def run_server(messages, **env):
       return subprocess.run(
           ["./xalantis-mcp-go"],
           input="".join(json.dumps(m) + "\n" for m in messages), capture_output=True, text=True, timeout=30,
           env={"XALANTIS_API_KEY": "sk_live_test", "XALANTIS_BASE_URL": f"http://127.0.0.1:{port}",
                "PATH": "/usr/bin", "HOME": home, **env},
       )

   def responses(p):
       return {d.get("id"): d for d in (json.loads(l) for l in p.stdout.splitlines() if l.strip())}

   proc = run_server(msgs)
   resp = responses(proc)
   ```
   (Delete the old `home = tempfile.mkdtemp()` line above it, if it's separate.)
4. Replace the `tools = …` and `check(len(tools) == 10 …)` lines with:
   ```python
   annotations = {t["name"]: t.get("annotations", {}) for t in resp[3]["result"]["tools"]}
   check(len(annotations) == 11, f"tools/list ({len(annotations)} outils)")
   check(annotations["xalantis_read_operation"] == {"readOnlyHint": True}
         and annotations["xalantis_list_tasks"] == {"readOnlyHint": True}
         and annotations["xalantis_call_operation"] == {"destructiveHint": True}, "annotations des outils")
   ```
   and change the instructions check to
   `check("xalantis_read_operation" in resp[1]["result"].get("instructions", ""), "instructions d'initialisation")`.
5. Replace the `check(resp[15] …)` line with:
   ```python
   check(not resp[15]["result"]["isError"] and len(POSTS) == 2
         and re.fullmatch(r"[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}", POSTS[1]["idem"] or "")
         and POSTS[1]["body"] == {"title": "sans clé"}, "Idempotency-Key générée")
   check(resp[16]["result"]["isError"] and "xalantis_call_operation" in resp[16]["result"]["content"][0]["text"],
         "read_operation refuse une écriture")
   ```
6. Change `check(len(REQUESTS) == 5, …)` to `check(len(REQUESTS) == 6, …)`.
7. Just before `srv.shutdown()`, add:
   ```python
   ro_msgs = [msgs[0], {"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
              {"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {
                  "name": "xalantis_call_operation", "arguments": {"operation_id": "post_projects_By_projectUuid_tasks"}}}]
   ro = responses(run_server(ro_msgs, XALANTIS_READ_ONLY="1"))
   ro_tools = {t["name"] for t in ro[2]["result"]["tools"]}
   check(len(ro_tools) == 10 and "xalantis_call_operation" not in ro_tools, "lecture seule : outil d'écriture absent")
   check(ro[3]["result"]["isError"] and "outil inconnu" in ro[3]["result"]["content"][0]["text"],
         "lecture seule : appel d'écriture refusé")
   check("XALANTIS_READ_ONLY" in ro[1]["result"]["instructions"], "lecture seule : instructions")
   check(len(REQUESTS) == 6, "lecture seule : aucun appel API")
   bad = run_server(msgs[:1], XALANTIS_READ_ONLY="oui")
   check(bad.returncode == 1 and "XALANTIS_READ_ONLY" in bad.stderr, "XALANTIS_READ_ONLY invalide refusée")
   ```

- [ ] **Step 2: Run the end-to-end test**

Run: `go build -o xalantis-mcp-go ./cmd/xalantis-mcp-go && python3 test_mcp.py; echo "exit=$?"; rm -f xalantis-mcp-go`
Expected: only `PASS` lines, `exit=0`.

- [ ] **Step 3: Update the README**

In `README.md`:

1. In the "Attention : écriture possible" block, replace
   ```
   > dépend uniquement des scopes de la clé API. Pour un usage en lecture seule,
   > utilisez une clé qui n'a que des scopes `:read`.
   ```
   with
   ```
   > dépend des scopes de la clé API. Pour un usage en lecture seule, utilisez
   > une clé qui n'a que des scopes `:read`, ou définissez
   > `XALANTIS_READ_ONLY=1` : le serveur n'expose alors aucune écriture.
   ```
2. Change `## Outils exposés (10)` to `## Outils exposés (11)` and
   `Outils dédiés aux lectures courantes de projets (inchangés) :` to
   `Outils dédiés aux lectures courantes de projets (lecture seule) :`.
3. Replace the generic tools table and the `Idempotency-Key` paragraph below it (from `| \`xalantis_search_operations\`` through `(valeur unique par mutation, par ex. un UUID) : passez-le dans \`headers\`.`) with:
   ```markdown
   | `xalantis_search_operations` | 1. Trouver une opération (mots-clés, domaine, méthode). Sans filtre : liste des domaines. |
   | `xalantis_describe_operation` | 2. Voir ses paramètres et le schéma de son corps. |
   | `xalantis_read_operation` | 3. Exécuter une lecture (GET) : `path_params`, `query`, `headers`, `save_to`. |
   | `xalantis_call_operation` | 3. Exécuter une écriture (POST, PUT, PATCH, DELETE) : mêmes arguments, plus `body` et `files`. Absent avec `XALANTIS_READ_ONLY=1`. |

   Les outils de lecture portent l'annotation MCP `readOnlyHint` et l'outil
   d'écriture `destructiveHint` : le client peut approuver les lectures
   automatiquement et demander confirmation avant une écriture.

   L'en-tête `Idempotency-Key` des écritures est facultatif : s'il manque, le
   serveur génère un UUID. Si l'appel échoue, l'erreur donne la clé générée ;
   la repasser dans `headers` pour réessayer sans créer de doublon.
   ```
4. In "Variables d'environnement", after the `XALANTIS_FILES_DIR` item, add:
   ```markdown
   - `XALANTIS_READ_ONLY` (optionnel) — `1`/`true` : le serveur n'expose
     pas `xalantis_call_operation` et la recherche ne renvoie que des
     lectures ; défaut `false`. Une valeur invalide empêche le démarrage.
   ```

Check: `grep -n "call_operation\|READ_ONLY\|Idempotency" README.md` and read each hit; the README must no longer say the key is required.

- [ ] **Step 4: Run all checks**

Run: `gofmt -l . && go vet ./... && go test ./... && golangci-lint run && go build -o xalantis-mcp-go ./cmd/xalantis-mcp-go && python3 test_mcp.py; rm -f xalantis-mcp-go`
Expected: no gofmt output, all `ok`, `0 issues.`, only `PASS` lines.

- [ ] **Step 5: Commit**

```bash
git add test_mcp.py README.md
git commit -m "docs: document read/write tools, read-only mode and generated keys

Also extend the end-to-end test to cover them.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```
