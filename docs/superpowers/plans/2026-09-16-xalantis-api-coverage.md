# Xalantis API Coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the 206 Xalantis API operations of 12 selected areas through 3 generic spec-driven MCP tools, keep the 7 existing project tools, and restructure the Go code into focused packages.

**Architecture:** A stdlib-only Go binary. `internal/mcp` runs the JSON-RPC stdio loop, `internal/xalantis` is the HTTP client, `internal/openapi` embeds and queries the OpenAPI spec, `internal/tools` defines the 10 tools, and `cmd/xalantis-projects-mcp` wires them together.

**Tech Stack:** Go 1.24+ (standard library only), Python 3 for the end-to-end test.

**Spec:** `docs/superpowers/specs/2026-09-16-xalantis-api-coverage-design.md`

## Global Constraints

- No third-party Go dependencies. `go.mod` keeps `module paymetrust/xalantis-projects-mcp` and `go 1.24.7`.
- Default base URL: `https://xalantis.com`. `XALANTIS_BASE_URL` overrides it. Requests go to `<base>/api/v1<path>`.
- `serverVersion` = `2.0.0`, MCP protocol version = `2025-06-18`.
- Included OpenAPI tags (exact strings; `Politiques d’escalade` uses U+2019): `Projets et tâches`, `Tickets`, `Automatisations de tickets`, `Catégories de tickets`, `Tags de tickets`, `Politiques SLA`, `Violations SLA`, `Politiques d’escalade`, `Réponses prédéfinies`, `Disponibilité des agents`, `Catalogue de services`, `Administration du catalogue`. Expected operation count: 206.
- Writes are not gated in code; the API key's scopes decide.
- The 7 existing tools keep their names, input schemas, query mapping and validation.
- No reply to any JSON-RPC message without an `id`.
- Path segments reject empty values, `/?#&%`, `.` and `..`.
- User-facing messages, code comments and the README are in French.
- No interface with a single implementation, no DI framework, no extra layers.
- Build command: `go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp`. The binary is tracked in git at the repo root and must be rebuilt and committed at the end.
- Commit messages end with: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`

## Deviations from the spec (decided while planning)

These come from facts found in the OpenAPI file. Task 7 records them in the spec.

1. **`tools` imports `mcp`** for the `mcp.Tool` type. `mcp` stays a leaf package.
2. **Required header parameters are validated.** 70 operations declare `Idempotency-Key` as required, so `xalantis_call_operation` returns a tool error when it is missing.
3. **Query names are used as declared.** Array parameters are already named `status[]`, `type[]`, … in the spec. `query` accepts the declared name, the name without `[]`, and `base[sub]` keys for object parameters such as `custom_fields[clé]`. Unknown names are rejected.
4. **Response size limits.** The client reads up to 100 MB (documents can be 50 MB); text returned to Claude is limited to 10 MB and larger text is an error instead of silent truncation.
5. **Multipart array file fields are sent as `name[]`** (e.g. `files[]`), the form PHP/Laravel backends expect for several files.
6. **`save_to` never overwrites either**; it gets the same ` (1)` suffix rule as the default folder.
7. **"No filters → list areas" lives in the search tool**, not in `Catalog.Search`.
8. **Unknown path parameter keys are rejected.** Operations with no request body reject `body`.

## File Structure

| File | Responsibility |
|---|---|
| `cmd/xalantis-projects-mcp/main.go` | Reads env, builds client and catalog, registers tools, runs the server |
| `internal/mcp/server.go` | JSON-RPC 2.0 stdio loop, tool registry |
| `internal/mcp/server_test.go` | Handshake, tools, notifications, errors |
| `internal/xalantis/client.go` | HTTP client, error mapping, multipart builder |
| `internal/xalantis/client_test.go` | Request shape, errors, multipart |
| `internal/openapi/xalantis-openapi.json` | Full OpenAPI spec (embedded, unmodified) |
| `internal/openapi/catalog.go` | Parse, filter by area, search, describe, `$ref` resolution |
| `internal/openapi/catalog_test.go` | Counts, fields, search, describe, refs |
| `internal/tools/args.go` | Argument helpers, path segment check, value formatting |
| `internal/tools/projects.go` | The 7 existing project tools |
| `internal/tools/projects_test.go` | Test recorder + project tool tests |
| `internal/tools/generic.go` | search / describe / call tools, files, downloads |
| `internal/tools/generic_test.go` | Generic tool tests |
| `test_mcp.py` | End-to-end test against the built binary |
| `README.md` | User documentation (French) |
| `main.go` (root) | **Deleted** |

---

### Task 1: MCP server package

**Files:**
- Create: `internal/mcp/server.go`
- Test: `internal/mcp/server_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `const ProtocolVersion = "2025-06-18"`
  - `type Handler func(args map[string]any) (string, error)`
  - `type Tool struct { Name, Description string; InputSchema map[string]any; Handler Handler }` (JSON tags `name`, `description`, `inputSchema`; `Handler` not serialized)
  - `func NewServer(name, version, instructions string) *Server`
  - `func (s *Server) Register(tools ...Tool)`
  - `func (s *Server) Serve(r io.Reader, w io.Writer) error`

- [ ] **Step 1: Write the failing test**

Create `internal/mcp/server_test.go`:

```go
package mcp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// run envoie des lignes au serveur et renvoie les réponses indexées par id.
func run(t *testing.T, s *Server, lines ...string) map[string]map[string]any {
	t.Helper()
	var out strings.Builder
	if err := s.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	got := map[string]map[string]any{}
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("réponse illisible %q: %v", l, err)
		}
		got[string(mustJSON(t, m["id"]))] = m
	}
	return got
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testServer() *Server {
	s := NewServer("srv", "9.9.9", "fais ceci")
	s.Register(
		Tool{Name: "ok", InputSchema: map[string]any{"type": "object"}, Handler: func(a map[string]any) (string, error) {
			return "bonjour " + a["who"].(string), nil
		}},
		Tool{Name: "ko", InputSchema: map[string]any{"type": "object"}, Handler: func(map[string]any) (string, error) {
			return "", errors.New("boum")
		}},
	)
	return s
}

func TestInitializeAndPing(t *testing.T) {
	got := run(t, testServer(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
	)
	res := got["1"]["result"].(map[string]any)
	if res["protocolVersion"] != ProtocolVersion || res["instructions"] != "fais ceci" {
		t.Fatalf("initialize: %v", res)
	}
	if res["serverInfo"].(map[string]any)["version"] != "9.9.9" {
		t.Fatalf("serverInfo: %v", res)
	}
	if len(got["2"]["result"].(map[string]any)) != 0 {
		t.Fatalf("ping: %v", got["2"])
	}
}

func TestToolsListAndCall(t *testing.T) {
	got := run(t, testServer(),
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ok","arguments":{"who":"toi"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ko"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"absent"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":"pas un objet"}`,
	)
	tools := got["1"]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 || tools[0].(map[string]any)["name"] != "ok" {
		t.Fatalf("tools/list: %v", tools)
	}
	if _, leaked := tools[0].(map[string]any)["Handler"]; leaked {
		t.Fatal("le handler ne doit pas être sérialisé")
	}
	text := func(id string) (string, bool) {
		r := got[id]["result"].(map[string]any)
		return r["content"].([]any)[0].(map[string]any)["text"].(string), r["isError"].(bool)
	}
	if s, isErr := text("2"); s != "bonjour toi" || isErr {
		t.Fatalf("call ok: %q %v", s, isErr)
	}
	if s, isErr := text("3"); s != "Erreur : boum" || !isErr {
		t.Fatalf("call ko: %q %v", s, isErr)
	}
	if s, isErr := text("4"); !strings.Contains(s, "outil inconnu") || !isErr {
		t.Fatalf("call absent: %q %v", s, isErr)
	}
	if code := got["5"]["error"].(map[string]any)["code"].(float64); code != -32602 {
		t.Fatalf("params invalides: %v", got["5"])
	}
}

func TestNotificationsAndUnknownMethod(t *testing.T) {
	called := false
	s := NewServer("srv", "1", "")
	s.Register(Tool{Name: "spy", Handler: func(map[string]any) (string, error) { called = true; return "", nil }})
	got := run(t, s,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"spy"}}`,
		`{"jsonrpc":"2.0","id":null,"method":"tools/list"}`,
		`pas du json`,
		`{"jsonrpc":"2.0","id":"a","method":"unknown/method"}`,
	)
	if called {
		t.Fatal("une notification ne doit pas exécuter d'outil")
	}
	if len(got) != 1 {
		t.Fatalf("une seule réponse attendue, obtenu %v", got)
	}
	if code := got[`"a"`]["error"].(map[string]any)["code"].(float64); code != -32601 {
		t.Fatalf("méthode inconnue: %v", got)
	}
	init := run(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if _, ok := init["1"]["result"].(map[string]any)["instructions"]; ok {
		t.Fatal("instructions vides : champ absent attendu")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mcp/`
Expected: FAIL, compile errors such as `undefined: Server` and `undefined: NewServer`.

- [ ] **Step 3: Write the implementation**

Create `internal/mcp/server.go`:

```go
// Package mcp implémente un serveur MCP minimal (JSON-RPC 2.0 sur stdio)
// avec un registre d'outils. Il ne connaît rien de Xalantis.
package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// ProtocolVersion est la version du protocole MCP annoncée à l'initialisation.
const ProtocolVersion = "2025-06-18"

// Handler exécute un outil. Une erreur devient un résultat isError=true.
type Handler func(args map[string]any) (string, error)

// Tool décrit un outil exposé au client MCP.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     Handler        `json:"-"`
}

// Server est un serveur MCP stdio.
type Server struct {
	name, version, instructions string
	tools                       []Tool
	byName                      map[string]Tool
}

// NewServer crée un serveur sans outil.
func NewServer(name, version, instructions string) *Server {
	return &Server{name: name, version: version, instructions: instructions, byName: map[string]Tool{}}
}

// Register ajoute des outils, dans l'ordre donné.
func (s *Server) Register(tools ...Tool) {
	for _, t := range tools {
		s.tools = append(s.tools, t)
		s.byName[t.Name] = t
	}
}

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func textResult(text string, isErr bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	}
}

// Serve lit une requête JSON-RPC par ligne sur r et écrit les réponses sur w,
// jusqu'à la fin de r.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 0, 1<<20), 16<<20)
	out := bufio.NewWriter(w)
	enc := json.NewEncoder(out)
	send := func(resp response) error {
		resp.JSONRPC = "2.0"
		if err := enc.Encode(resp); err != nil {
			return err
		}
		return out.Flush()
	}

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue // ligne illisible : on ignore
		}
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue // notification : jamais de réponse
		}
		if err := send(s.handle(req)); err != nil {
			return err
		}
	}
	return in.Err()
}

func (s *Server) handle(req request) response {
	switch req.Method {
	case "initialize":
		result := map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		}
		if s.instructions != "" {
			result["instructions"] = s.instructions
		}
		return response{ID: req.ID, Result: result}

	case "ping":
		return response{ID: req.ID, Result: map[string]any{}}

	case "tools/list":
		return response{ID: req.ID, Result: map[string]any{"tools": s.tools}}

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return response{ID: req.ID, Error: &rpcError{Code: -32602, Message: "paramètres invalides"}}
		}
		tool, ok := s.byName[params.Name]
		if !ok {
			return response{ID: req.ID, Result: textResult("Erreur : outil inconnu : "+params.Name, true)}
		}
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		text, err := tool.Handler(params.Arguments)
		if err != nil {
			return response{ID: req.ID, Result: textResult("Erreur : "+err.Error(), true)}
		}
		return response{ID: req.ID, Result: textResult(text, false)}
	}
	return response{ID: req.ID, Error: &rpcError{Code: -32601, Message: "méthode non supportée : " + req.Method}}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l internal/mcp && go vet ./internal/mcp/ && go test ./internal/mcp/`
Expected: no gofmt output, then `ok  paymetrust/xalantis-projects-mcp/internal/mcp`.

The root `main.go` still builds on its own; leave it until Task 6.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp
git commit -m "feat(mcp): add stdio JSON-RPC server package

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Xalantis HTTP client package

**Files:**
- Create: `internal/xalantis/client.go`
- Test: `internal/xalantis/client_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `const DefaultBaseURL = "https://xalantis.com"`, `const MaxBodyBytes = 100 << 20`
  - `type Config struct { BaseURL, APIKey, UserAgent string }`
  - `func NewClient(cfg Config) *Client`
  - `type Response struct { Status int; Header http.Header; Body []byte }`
  - `func (c *Client) Do(method, path string, query url.Values, headers map[string]string, body io.Reader, contentType string) (*Response, error)` — non-2xx responses are errors: `rate limit atteint (60 req/min) — réessayez dans <Retry-After> s` for 429, `API Xalantis HTTP <code> : <body, trimmed, max 500 chars>` otherwise; missing key → `XALANTIS_API_KEY manquante : …` with no HTTP call.
  - `func (c *Client) Get(path string, query url.Values) (string, error)`
  - `type FilePart struct { Field, Path string }`
  - `func Multipart(fields url.Values, files []FilePart) (io.Reader, string, error)` — returns body and `Content-Type`.

- [ ] **Step 1: Write the failing test**

Create `internal/xalantis/client_test.go`:

```go
package xalantis

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoSendsAuthQueryHeadersAndBody(t *testing.T) {
	var got *http.Request
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL + "/", APIKey: "sk_live_x", UserAgent: "ua/1"})
	resp, err := c.Do(http.MethodPost, "/tickets", url.Values{"a": {"1"}},
		map[string]string{"Idempotency-Key": "k1"}, strings.NewReader(`{"x":1}`), "application/json")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 201 || string(resp.Body) != `{"success":true}` {
		t.Fatalf("réponse: %+v", resp)
	}
	checks := map[string]string{
		"method":          got.Method,
		"path":            got.URL.Path,
		"query":           got.URL.RawQuery,
		"Authorization":   got.Header.Get("Authorization"),
		"Accept":          got.Header.Get("Accept"),
		"User-Agent":      got.Header.Get("User-Agent"),
		"Content-Type":    got.Header.Get("Content-Type"),
		"Idempotency-Key": got.Header.Get("Idempotency-Key"),
		"body":            gotBody,
	}
	want := map[string]string{
		"method": "POST", "path": "/api/v1/tickets", "query": "a=1",
		"Authorization": "Bearer sk_live_x", "Accept": "application/json", "User-Agent": "ua/1",
		"Content-Type": "application/json", "Idempotency-Key": "k1", "body": `{"x":1}`,
	}
	for k, w := range want {
		if checks[k] != w {
			t.Errorf("%s = %q, attendu %q", k, checks[k], w)
		}
	}
}

func TestDefaultBaseURL(t *testing.T) {
	if c := NewClient(Config{}); c.baseURL != "https://xalantis.com" {
		t.Fatalf("baseURL = %q", c.baseURL)
	}
}

func TestDoMissingKeyMakesNoCall(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer srv.Close()
	_, err := NewClient(Config{BaseURL: srv.URL}).Get("/projects", nil)
	if err == nil || !strings.Contains(err.Error(), "XALANTIS_API_KEY manquante") || calls != 0 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestDoErrors(t *testing.T) {
	long := strings.Repeat("x", 600)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/limited":
			w.Header().Set("Retry-After", "42")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/api/v1/forbidden":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`  {"message":"scope manquant"}  `))
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(long))
		}
	}))
	defer srv.Close()
	c := NewClient(Config{BaseURL: srv.URL, APIKey: "k"})

	if _, err := c.Get("/limited", nil); err == nil || !strings.Contains(err.Error(), "réessayez dans 42 s") {
		t.Errorf("429: %v", err)
	}
	if _, err := c.Get("/forbidden", nil); err == nil || err.Error() != `API Xalantis HTTP 403 : {"message":"scope manquant"}` {
		t.Errorf("403: %v", err)
	}
	_, err := c.Get("/long", nil)
	if err == nil || err.Error() != "API Xalantis HTTP 422 : "+long[:500] {
		t.Errorf("422 tronqué: %v", err)
	}
}

func TestMultipart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	os.WriteFile(path, []byte("contenu"), 0o644)

	body, ct, err := Multipart(url.Values{"label": {"doc"}}, []FilePart{{Field: "files[]", Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", ct)
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	if req.FormValue("label") != "doc" {
		t.Errorf("champ label = %q", req.FormValue("label"))
	}
	fh := req.MultipartForm.File["files[]"]
	if len(fh) != 1 || fh[0].Filename != "note.txt" {
		t.Fatalf("fichiers: %+v", fh)
	}
	f, _ := fh[0].Open()
	data, _ := io.ReadAll(f)
	if string(data) != "contenu" {
		t.Errorf("contenu = %q", data)
	}

	if _, _, err := Multipart(nil, []FilePart{{Field: "file", Path: filepath.Join(dir, "absent")}}); err == nil {
		t.Error("fichier absent : erreur attendue")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/xalantis/`
Expected: FAIL, compile errors such as `undefined: NewClient`.

- [ ] **Step 3: Write the implementation**

Create `internal/xalantis/client.go`:

```go
// Package xalantis est le client HTTP de l'API Xalantis (api/v1).
package xalantis

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultBaseURL est utilisée quand Config.BaseURL est vide.
const DefaultBaseURL = "https://xalantis.com"

// MaxBodyBytes borne la taille d'une réponse lue en mémoire.
// ponytail: réponse entière en mémoire ; passer au streaming si des fichiers > 100 Mo apparaissent.
const MaxBodyBytes = 100 << 20

// Config regroupe les paramètres du client, lus une seule fois par main.
type Config struct {
	BaseURL   string
	APIKey    string
	UserAgent string
}

// Client appelle l'API Xalantis.
type Client struct {
	baseURL, apiKey, userAgent string
	http                       *http.Client
}

// NewClient crée un client ; BaseURL vide = DefaultBaseURL.
func NewClient(cfg Config) *Client {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		baseURL:   base,
		apiKey:    cfg.APIKey,
		userAgent: cfg.UserAgent,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// Response est une réponse HTTP 2xx lue en entier.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Do envoie une requête. path est relatif à /api/v1 et déjà échappé.
// body peut être nil. Toute réponse hors 2xx devient une erreur.
func (c *Client) Do(method, path string, query url.Values, headers map[string]string, body io.Reader, contentType string) (*Response, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("XALANTIS_API_KEY manquante : ajoutez-la dans la configuration du serveur MCP")
	}
	u := c.baseURL + "/api/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("appel API impossible : %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limit atteint (60 req/min) — réessayez dans %s s", resp.Header.Get("Retry-After"))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return nil, fmt.Errorf("API Xalantis HTTP %d : %s", resp.StatusCode, msg)
	}
	if len(data) > MaxBodyBytes {
		return nil, fmt.Errorf("réponse trop volumineuse (plus de %d Mo)", MaxBodyBytes>>20)
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

// Get est un raccourci pour un GET dont la réponse est du texte.
func (c *Client) Get(path string, query url.Values) (string, error) {
	resp, err := c.Do(http.MethodGet, path, query, nil, nil, "")
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}

// FilePart est un fichier local à joindre sous le champ Field.
type FilePart struct {
	Field string
	Path  string
}

// Multipart construit un corps multipart/form-data. Renvoie le corps et son Content-Type.
func Multipart(fields url.Values, files []FilePart) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, values := range fields {
		for _, v := range values {
			if err := w.WriteField(name, v); err != nil {
				return nil, "", err
			}
		}
	}
	for _, f := range files {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, "", fmt.Errorf("lecture de %s impossible : %v", f.Path, err)
		}
		part, err := w.CreateFormFile(f.Field, filepath.Base(f.Path))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l internal/xalantis && go vet ./internal/xalantis/ && go test ./internal/xalantis/`
Expected: no gofmt output, then `ok  paymetrust/xalantis-projects-mcp/internal/xalantis`.

- [ ] **Step 5: Commit**

```bash
git add internal/xalantis
git commit -m "feat(xalantis): add HTTP client package

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: OpenAPI catalog package

**Files:**
- Create: `internal/openapi/xalantis-openapi.json` (copy)
- Create: `internal/openapi/catalog.go`
- Test: `internal/openapi/catalog_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `var Areas []string` (the 12 tags, in the order of the Global Constraints)
  - `type Param struct { Name, In string; Required bool; Type, Description string }` (JSON: `name`, `in`, `required`, `type`, `description`)
  - `type FileField struct { Name string; Multiple bool }` (JSON: `name`, `multiple`)
  - `type Operation struct { ID, Method, Path, Area, Summary, Scope string; Params []Param; BodyType string; BodyRequired bool; BodySchema map[string]any; FileFields []FileField }` — `Method` is upper case; `BodyType` is `""`, `"application/json"` or `"multipart/form-data"`.
  - `func (o *Operation) Param(in, name string) (Param, bool)`
  - `func Load() (*Catalog, error)`, `func Parse(data []byte, areas []string) (*Catalog, error)`
  - `func (c *Catalog) Len() int`, `func (c *Catalog) Get(id string) (*Operation, bool)`
  - `type AreaCount struct { Area string; Operations int }` (JSON: `area`, `operations`); `func (c *Catalog) AreaCounts() []AreaCount`
  - `type Summary struct { OperationID, Method, Path, Summary, Scope string }` (JSON: `operation_id`, `method`, `path`, `summary`, `scope`)
  - `const MaxResults = 50`; `func (c *Catalog) Search(query, area, method string) []Summary` — never returns nil.
  - `type Description struct { OperationID, Method, Path, Area, Summary, Scope string; Parameters []Param; Body map[string]any }` — `Body` keys: `content_type`, `required`, `schema`, and `file_fields` when present.
  - `const MaxRefDepth = 3`; `func (c *Catalog) Describe(id string) (*Description, error)` — unknown id → error containing `opération inconnue`.

- [ ] **Step 1: Copy the OpenAPI file**

```bash
mkdir -p internal/openapi
cp /Users/martialanouman/Downloads/xalantis-openapi.json internal/openapi/xalantis-openapi.json
python3 -c "import json; d=json.load(open('internal/openapi/xalantis-openapi.json')); print(d['info']['version'], len(d['paths']))"
```

Expected: `2.11.0` followed by the number of paths.

- [ ] **Step 2: Write the failing test**

Create `internal/openapi/catalog_test.go`:

```go
package openapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func loadEmbedded(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func TestLoadFiltersAreas(t *testing.T) {
	c := loadEmbedded(t)
	if c.Len() != 206 {
		t.Fatalf("Len = %d, attendu 206", c.Len())
	}
	want := map[string]int{
		"Projets et tâches": 108, "Tickets": 57, "Automatisations de tickets": 8,
		"Catégories de tickets": 4, "Tags de tickets": 5, "Politiques SLA": 4,
		"Violations SLA": 2, "Politiques d’escalade": 3, "Réponses prédéfinies": 4,
		"Disponibilité des agents": 2, "Catalogue de services": 3, "Administration du catalogue": 6,
	}
	got := map[string]int{}
	for _, a := range c.AreaCounts() {
		got[a.Area] = a.Operations
	}
	for area, n := range want {
		if got[area] != n {
			t.Errorf("%s = %d, attendu %d", area, got[area], n)
		}
	}
	if _, ok := c.Get("get_contracts"); ok {
		t.Error("les contrats sont hors périmètre")
	}
}

func TestOperationFields(t *testing.T) {
	c := loadEmbedded(t)
	op, ok := c.Get("post_tickets")
	if !ok {
		t.Fatal("post_tickets absent")
	}
	if op.Method != "POST" || op.Path != "/tickets" || op.Area != "Tickets" || op.Scope != "tickets:write" {
		t.Errorf("post_tickets: %+v", op)
	}
	if op.BodyType != "application/json" || !op.BodyRequired {
		t.Errorf("corps: %q %v", op.BodyType, op.BodyRequired)
	}

	up, _ := c.Get("post_projects_By_projectUuid_documents")
	if p, ok := up.Param("header", "Idempotency-Key"); !ok || !p.Required {
		t.Errorf("Idempotency-Key: %+v %v", p, ok)
	}
	if up.BodyType != "multipart/form-data" || len(up.FileFields) != 1 || up.FileFields[0] != (FileField{Name: "files", Multiple: true}) {
		t.Errorf("upload multiple: %+v", up.FileFields)
	}
	imp, _ := c.Get("post_projects_By_projectUuid_imports")
	if len(imp.FileFields) != 1 || imp.FileFields[0] != (FileField{Name: "file"}) {
		t.Errorf("upload simple: %+v", imp.FileFields)
	}
}

func TestSearch(t *testing.T) {
	c := loadEmbedded(t)

	res := c.Search("creer ticket", "", "post")
	found := false
	for _, r := range res {
		if r.OperationID == "post_tickets" {
			found = true
		}
		if r.Method != "POST" {
			t.Errorf("filtre méthode ignoré : %+v", r)
		}
	}
	if !found {
		t.Errorf("post_tickets introuvable sans accents : %+v", res)
	}

	for _, r := range c.Search("", "politiques d'escalade", "") {
		if !strings.Contains(r.Path, "escalation") {
			t.Errorf("filtre domaine ignoré : %+v", r)
		}
	}
	if n := len(c.Search("", "Politiques d’escalade", "")); n != 3 {
		t.Errorf("escalade = %d, attendu 3", n)
	}
	if n := len(c.Search("", "", "")); n != MaxResults {
		t.Errorf("limite = %d, attendu %d", n, MaxResults)
	}
	if res := c.Search("introuvable-xyz", "", ""); res == nil || len(res) != 0 {
		t.Errorf("aucun résultat : liste vide attendue, obtenu %#v", res)
	}
}

func TestDescribe(t *testing.T) {
	c := loadEmbedded(t)
	d, err := c.Describe("post_projects_By_projectUuid_documents")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(d)
	s := string(b)
	for _, want := range []string{`"operation_id":"post_projects_By_projectUuid_documents"`, `"name":"projectUuid"`, `"content_type":"multipart/form-data"`, `"file_fields":[{"name":"files","multiple":true}]`} {
		if !strings.Contains(s, want) {
			t.Errorf("describe ne contient pas %s : %s", want, s)
		}
	}
	if _, err := c.Describe("absent"); err == nil || !strings.Contains(err.Error(), "opération inconnue") {
		t.Errorf("absent: %v", err)
	}
	get, _ := c.Describe("get_tickets")
	if get.Body != nil {
		t.Errorf("GET sans corps : %v", get.Body)
	}
}

func TestResolveRefs(t *testing.T) {
	spec := `{
	  "paths": {"/x": {"post": {"operationId": "post_x", "tags": ["T"],
	    "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/A"}}}}}}},
	  "components": {"schemas": {
	    "A": {"type": "object", "properties": {"b": {"$ref": "#/components/schemas/B"}}},
	    "B": {"type": "object", "properties": {"c": {"$ref": "#/components/schemas/C"}}},
	    "C": {"type": "object", "properties": {"d": {"$ref": "#/components/schemas/D"}}},
	    "D": {"type": "string"}
	  }}
	}`
	c, err := Parse([]byte(spec), []string{"T"})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Describe("post_x")
	b, _ := json.Marshal(d.Body["schema"])
	want := `{"properties":{"b":{"properties":{"c":{"properties":{"d":{"$ref":"#/components/schemas/D"}},"type":"object"}},"type":"object"}},"type":"object"}`
	if string(b) != want {
		t.Errorf("schéma résolu :\n%s\nattendu :\n%s", b, want)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/openapi/`
Expected: FAIL, compile errors such as `undefined: Load`.

- [ ] **Step 4: Write the implementation**

Create `internal/openapi/catalog.go`:

```go
// Package openapi charge la spécification OpenAPI embarquée de Xalantis,
// garde les opérations des domaines retenus et permet de les chercher et
// de les décrire.
package openapi

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

//go:embed xalantis-openapi.json
var specJSON []byte

// Areas liste les tags OpenAPI retenus. « Politiques d’escalade » utilise
// l'apostrophe typographique U+2019, comme la spécification.
var Areas = []string{
	"Projets et tâches",
	"Tickets",
	"Automatisations de tickets",
	"Catégories de tickets",
	"Tags de tickets",
	"Politiques SLA",
	"Violations SLA",
	"Politiques d’escalade",
	"Réponses prédéfinies",
	"Disponibilité des agents",
	"Catalogue de services",
	"Administration du catalogue",
}

// Param est un paramètre de chemin, de requête ou d'en-tête.
type Param struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

// FileField est un champ binaire d'un corps multipart.
type FileField struct {
	Name     string `json:"name"`
	Multiple bool   `json:"multiple"`
}

// Operation est une opération de l'API.
type Operation struct {
	ID           string
	Method       string // en majuscules
	Path         string // ex. /projects/{projectUuid}/tasks
	Area         string
	Summary      string
	Scope        string
	Params       []Param
	BodyType     string // "", "application/json" ou "multipart/form-data"
	BodyRequired bool
	BodySchema   map[string]any
	FileFields   []FileField
}

// Param renvoie le paramètre nommé name à l'emplacement in.
func (o *Operation) Param(in, name string) (Param, bool) {
	for _, p := range o.Params {
		if p.In == in && p.Name == name {
			return p, true
		}
	}
	return Param{}, false
}

// Catalog est l'ensemble des opérations retenues.
type Catalog struct {
	ops        []*Operation
	byID       map[string]*Operation
	components map[string]any
}

// Load analyse la spécification embarquée avec les domaines Areas.
func Load() (*Catalog, error) {
	return Parse(specJSON, Areas)
}

type rawSpec struct {
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]any `json:"schemas"`
	} `json:"components"`
}

type rawOperation struct {
	OperationID string                `json:"operationId"`
	Summary     string                `json:"summary"`
	Tags        []string              `json:"tags"`
	Security    []map[string][]string `json:"security"`
	Parameters  []struct {
		Name        string         `json:"name"`
		In          string         `json:"in"`
		Required    bool           `json:"required"`
		Description string         `json:"description"`
		Schema      map[string]any `json:"schema"`
	} `json:"parameters"`
	RequestBody *struct {
		Required bool `json:"required"`
		Content  map[string]struct {
			Schema map[string]any `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
}

var httpMethods = []string{"get", "post", "put", "patch", "delete"}

// Parse analyse une spécification et garde les opérations dont le premier tag est dans areas.
func Parse(data []byte, areas []string) (*Catalog, error) {
	var spec rawSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, a := range areas {
		keep[a] = true
	}
	c := &Catalog{byID: map[string]*Operation{}, components: spec.Components.Schemas}
	for path, item := range spec.Paths {
		for _, m := range httpMethods {
			raw, ok := item[m]
			if !ok {
				continue
			}
			var ro rawOperation
			if err := json.Unmarshal(raw, &ro); err != nil {
				return nil, fmt.Errorf("%s %s : %v", m, path, err)
			}
			if len(ro.Tags) == 0 || !keep[ro.Tags[0]] {
				continue
			}
			op := &Operation{
				ID:      ro.OperationID,
				Method:  strings.ToUpper(m),
				Path:    path,
				Area:    ro.Tags[0],
				Summary: ro.Summary,
			}
			for _, s := range ro.Security {
				for _, scopes := range s {
					op.Scope = strings.Join(scopes, " ")
				}
			}
			for _, p := range ro.Parameters {
				t, _ := p.Schema["type"].(string)
				op.Params = append(op.Params, Param{Name: p.Name, In: p.In, Required: p.Required, Type: t, Description: p.Description})
			}
			if rb := ro.RequestBody; rb != nil {
				op.BodyRequired = rb.Required
				for _, ct := range []string{"application/json", "multipart/form-data"} {
					if content, ok := rb.Content[ct]; ok {
						op.BodyType = ct
						op.BodySchema = content.Schema
						break
					}
				}
				if op.BodyType == "multipart/form-data" {
					op.FileFields = fileFields(op.BodySchema)
				}
			}
			if op.ID == "" || c.byID[op.ID] != nil {
				return nil, fmt.Errorf("operationId absent ou dupliqué : %s %s", m, path)
			}
			c.ops = append(c.ops, op)
			c.byID[op.ID] = op
		}
	}
	sort.Slice(c.ops, func(i, j int) bool { return c.ops[i].ID < c.ops[j].ID })
	return c, nil
}

func fileFields(schema map[string]any) []FileField {
	props, _ := schema["properties"].(map[string]any)
	var out []FileField
	for name, v := range props {
		p, _ := v.(map[string]any)
		if p["format"] == "binary" {
			out = append(out, FileField{Name: name})
			continue
		}
		if items, _ := p["items"].(map[string]any); p["type"] == "array" && items["format"] == "binary" {
			out = append(out, FileField{Name: name, Multiple: true})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Len renvoie le nombre d'opérations.
func (c *Catalog) Len() int { return len(c.ops) }

// Get renvoie une opération par operationId.
func (c *Catalog) Get(id string) (*Operation, bool) {
	op, ok := c.byID[id]
	return op, ok
}

// AreaCount est un domaine et son nombre d'opérations.
type AreaCount struct {
	Area       string `json:"area"`
	Operations int    `json:"operations"`
}

// AreaCounts renvoie les domaines dans l'ordre de Areas.
func (c *Catalog) AreaCounts() []AreaCount {
	counts := map[string]int{}
	for _, op := range c.ops {
		counts[op.Area]++
	}
	var out []AreaCount
	for _, a := range Areas {
		if counts[a] > 0 {
			out = append(out, AreaCount{Area: a, Operations: counts[a]})
		}
	}
	return out
}

// Summary est une ligne de résultat de recherche.
type Summary struct {
	OperationID string `json:"operation_id"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Summary     string `json:"summary"`
	Scope       string `json:"scope"`
}

// MaxResults borne le nombre de résultats de Search.
const MaxResults = 50

var foldReplacer = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i",
	"ô", "o", "ö", "o",
	"ù", "u", "û", "u", "ü", "u",
	"ç", "c", "œ", "oe", "’", "'",
)

// fold met en minuscules et retire les accents français.
func fold(s string) string {
	return foldReplacer.Replace(strings.ToLower(s))
}

// Search renvoie les opérations dont l'identifiant, le chemin ou le résumé
// contiennent tous les mots de query, filtrées par domaine et méthode.
func (c *Catalog) Search(query, area, method string) []Summary {
	words := strings.Fields(fold(query))
	out := []Summary{}
	for _, op := range c.ops {
		if area != "" && fold(op.Area) != fold(area) {
			continue
		}
		if method != "" && op.Method != strings.ToUpper(method) {
			continue
		}
		hay := fold(op.ID + " " + op.Path + " " + op.Summary)
		match := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		out = append(out, Summary{OperationID: op.ID, Method: op.Method, Path: op.Path, Summary: op.Summary, Scope: op.Scope})
		if len(out) == MaxResults {
			break
		}
	}
	return out
}

// Description est la vue détaillée d'une opération.
type Description struct {
	OperationID string         `json:"operation_id"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	Area        string         `json:"area"`
	Summary     string         `json:"summary"`
	Scope       string         `json:"scope"`
	Parameters  []Param        `json:"parameters"`
	Body        map[string]any `json:"body,omitempty"`
}

// MaxRefDepth borne la résolution des $ref dans Describe.
const MaxRefDepth = 3

// Describe renvoie la description d'une opération, $ref résolus.
func (c *Catalog) Describe(id string) (*Description, error) {
	op, ok := c.byID[id]
	if !ok {
		return nil, fmt.Errorf("opération inconnue : %s (utilisez xalantis_search_operations)", id)
	}
	d := &Description{
		OperationID: op.ID, Method: op.Method, Path: op.Path, Area: op.Area,
		Summary: op.Summary, Scope: op.Scope, Parameters: op.Params,
	}
	if d.Parameters == nil {
		d.Parameters = []Param{}
	}
	if op.BodyType != "" {
		d.Body = map[string]any{
			"content_type": op.BodyType,
			"required":     op.BodyRequired,
			"schema":       c.resolve(op.BodySchema, 0),
		}
		if len(op.FileFields) > 0 {
			d.Body["file_fields"] = op.FileFields
		}
	}
	return d, nil
}

var refPattern = regexp.MustCompile(`^#/components/schemas/(.+)$`)

// resolve remplace les {"$ref": "#/components/schemas/X"} par le schéma X,
// jusqu'à MaxRefDepth niveaux ; au-delà, le $ref reste tel quel.
func (c *Catalog) resolve(v any, depth int) any {
	switch t := v.(type) {
	case map[string]any:
		if ref, ok := t["$ref"].(string); ok {
			m := refPattern.FindStringSubmatch(ref)
			if m == nil || depth >= MaxRefDepth || c.components[m[1]] == nil {
				return t
			}
			return c.resolve(c.components[m[1]], depth+1)
		}
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = c.resolve(x, depth)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = c.resolve(x, depth)
		}
		return out
	}
	return v
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l internal/openapi && go vet ./internal/openapi/ && go test ./internal/openapi/`
Expected: no gofmt output, then `ok  paymetrust/xalantis-projects-mcp/internal/openapi`.

If `TestLoadFiltersAreas` reports `Politiques d’escalade = 0`, the apostrophe in `Areas` is not U+2019; copy it from the Global Constraints.

- [ ] **Step 6: Commit**

```bash
git add internal/openapi
git commit -m "feat(openapi): embed Xalantis spec and add operation catalog

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Project tools on the new packages

**Files:**
- Create: `internal/tools/args.go`
- Create: `internal/tools/projects.go`
- Test: `internal/tools/projects_test.go`

**Interfaces:**
- Consumes: `mcp.Tool` (Task 1); `xalantis.Client`, `xalantis.NewClient`, `xalantis.Config`, `(*Client).Get` (Task 2).
- Produces:
  - `func ProjectTools(c *xalantis.Client) []mcp.Tool` — the 7 tools in this order: `xalantis_list_projects`, `xalantis_list_tasks`, `xalantis_get_task`, `xalantis_task_activities`, `xalantis_list_sprints`, `xalantis_list_statuses`, `xalantis_list_members`.
  - Package-internal helpers used by Task 5: `type args map[string]any` with `str(k) string`, `obj(k) (map[string]any, error)`; `checkSegment(key, v string) error`; `scalar(key string, v any) (string, error)` (bool → `1`/`0`, numbers without trailing decimals); `addValue(dst url.Values, key string, v any, arrayKey bool) error`; `prop`, `arrProp`, `schema`, `merge`.
  - Test helpers used by Task 5 (in `projects_test.go`): `type recorder` with fields `srv`, `requests []*http.Request`, `bodies []string`; `newRecorder(t, h http.HandlerFunc) *recorder` (nil `h` → responds `{"success":true}` as JSON); `(*recorder).client() *xalantis.Client`; `find(t, tools []mcp.Tool, name string) mcp.Tool`.

- [ ] **Step 1: Write the failing test**

Create `internal/tools/projects_test.go`:

```go
package tools

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"paymetrust/xalantis-projects-mcp/internal/mcp"
	"paymetrust/xalantis-projects-mcp/internal/xalantis"
)

// recorder est un faux serveur Xalantis qui mémorise les requêtes reçues.
type recorder struct {
	srv      *httptest.Server
	requests []*http.Request
	bodies   []string
}

func newRecorder(t *testing.T, h http.HandlerFunc) *recorder {
	t.Helper()
	r := &recorder{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
		r.requests = append(r.requests, req)
		r.bodies = append(r.bodies, string(body))
		if h != nil {
			h(w, req)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *recorder) client() *xalantis.Client {
	return xalantis.NewClient(xalantis.Config{BaseURL: r.srv.URL, APIKey: "k"})
}

func find(t *testing.T, tools []mcp.Tool, name string) mcp.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("outil %s absent", name)
	return mcp.Tool{}
}

func TestProjectToolsNames(t *testing.T) {
	tools := ProjectTools(nil)
	want := []string{"xalantis_list_projects", "xalantis_list_tasks", "xalantis_get_task", "xalantis_task_activities", "xalantis_list_sprints", "xalantis_list_statuses", "xalantis_list_members"}
	if len(tools) != len(want) {
		t.Fatalf("%d outils", len(tools))
	}
	for i, name := range want {
		if tools[i].Name != name {
			t.Errorf("outil %d = %s, attendu %s", i, tools[i].Name, name)
		}
	}
}

func TestListTasksQuery(t *testing.T) {
	rec := newRecorder(t, nil)
	tool := find(t, ProjectTools(rec.client()), "xalantis_list_tasks")
	_, err := tool.Handler(map[string]any{
		"project_uuid": "p-1", "statuses": []any{"s-1", " "}, "due_state": "overdue",
		"per_page": float64(50), "page": float64(2), "include_archived": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	u := rec.requests[0].URL
	if u.Path != "/api/v1/projects/p-1/tasks" {
		t.Errorf("path = %s", u.Path)
	}
	q := u.Query()
	if strings.Join(q["status[]"], ",") != "s-1" || q.Get("due_state") != "overdue" ||
		q.Get("per_page") != "50" || q.Get("page") != "2" || q.Get("include_archived") != "1" {
		t.Errorf("query = %v", q)
	}
}

func TestProjectToolsRejectBadUUID(t *testing.T) {
	rec := newRecorder(t, nil)
	tool := find(t, ProjectTools(rec.client()), "xalantis_get_task")
	cases := []map[string]any{
		{},
		{"project_uuid": "p-1"},
		{"project_uuid": "p-1", "task_uuid": "../secret"},
		{"project_uuid": "p-1", "task_uuid": ".."},
		{"project_uuid": ".", "task_uuid": "t-1"},
	}
	for _, c := range cases {
		if _, err := tool.Handler(c); err == nil {
			t.Errorf("%v : erreur attendue", c)
		}
	}
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tools/`
Expected: FAIL, compile error `undefined: ProjectTools`.

- [ ] **Step 3: Write the argument helpers**

Create `internal/tools/args.go`:

```go
// Package tools définit les outils MCP exposés par le serveur.
package tools

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// args enveloppe les arguments d'un appel d'outil.
type args map[string]any

func (a args) str(k string) string {
	if v, ok := a[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (a args) obj(k string) (map[string]any, error) {
	switch v := a[k].(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return v, nil
	}
	return nil, fmt.Errorf("argument %s invalide : objet attendu", k)
}

func (a args) addStr(q url.Values, argKey, queryKey string) {
	if v := a.str(argKey); v != "" {
		q.Set(queryKey, v)
	}
}

func (a args) addBool(q url.Values, argKey, queryKey string) {
	if v, ok := a[argKey].(bool); ok && v {
		q.Set(queryKey, "1")
	}
}

func (a args) addInt(q url.Values, argKey, queryKey string) {
	if v, ok := a[argKey].(float64); ok && v > 0 {
		q.Set(queryKey, fmt.Sprintf("%d", int(v)))
	}
}

func (a args) addArr(q url.Values, argKey, queryKey string) {
	raw, ok := a[argKey].([]any)
	if !ok {
		return
	}
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			q.Add(queryKey, strings.TrimSpace(s))
		}
	}
}

func (a args) paging(q url.Values) {
	a.addInt(q, "page", "page")
	a.addInt(q, "per_page", "per_page")
}

// checkSegment valide une valeur destinée à un segment de chemin d'URL.
func checkSegment(key, v string) error {
	if v == "" {
		return fmt.Errorf("argument requis manquant : %s", key)
	}
	if strings.ContainsAny(v, "/?#&%") || v == "." || v == ".." {
		return fmt.Errorf("argument %s invalide", key)
	}
	return nil
}

func requireUUID(a args, key string) (string, error) {
	v := a.str(key)
	if err := checkSegment(key, v); err != nil {
		return "", err
	}
	return v, nil
}

// scalar convertit une valeur JSON simple en texte pour une requête ou un formulaire.
func scalar(key string, v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case bool:
		if t {
			return "1", nil
		}
		return "0", nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	}
	return "", fmt.Errorf("valeur invalide pour %s : texte, nombre ou booléen attendu", key)
}

// addValue ajoute v sous key : les tableaux deviennent des valeurs répétées
// (sous key[] si arrayKey est vrai), les objets des clés key[sous-clé].
func addValue(dst url.Values, key string, v any, arrayKey bool) error {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		k := key
		if arrayKey && !strings.HasSuffix(k, "[]") {
			k += "[]"
		}
		for _, item := range t {
			s, err := scalar(key, item)
			if err != nil {
				return err
			}
			dst.Add(k, s)
		}
		return nil
	case map[string]any:
		for sub, item := range t {
			s, err := scalar(key+"["+sub+"]", item)
			if err != nil {
				return err
			}
			dst.Add(key+"["+sub+"]", s)
		}
		return nil
	}
	s, err := scalar(key, v)
	if err != nil {
		return err
	}
	dst.Add(key, s)
	return nil
}
```

- [ ] **Step 4: Write the project tools**

Create `internal/tools/projects.go`. Names, descriptions, schemas and query mapping are copied from the root `main.go` (`tools()` and `callTool()`); only the plumbing changes.

```go
package tools

import (
	"net/url"

	"paymetrust/xalantis-projects-mcp/internal/mcp"
	"paymetrust/xalantis-projects-mcp/internal/xalantis"
)

func prop(t, desc string) map[string]any {
	return map[string]any{"type": t, "description": desc}
}

func arrProp(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

func schema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func merge(ms ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

var pagingProps = map[string]any{
	"page":     prop("integer", "Numéro de page (défaut 1)."),
	"per_page": prop("integer", "Résultats par page, 1 à 100."),
}

// ProjectTools renvoie les 7 outils de lecture des projets.
func ProjectTools(c *xalantis.Client) []mcp.Tool {
	projectUUID := map[string]any{"project_uuid": prop("string", "UUID du projet.")}
	taskUUID := map[string]any{"task_uuid": prop("string", "UUID de la tâche.")}
	return []mcp.Tool{
		{
			Name:        "xalantis_list_projects",
			Description: "Liste les projets Xalantis accessibles (lecture seule). Filtres : recherche, statut, projets archivés.",
			InputSchema: schema(nil, merge(pagingProps, map[string]any{
				"search":           prop("string", "Filtre sur la clé, le nom ou la description."),
				"status":           prop("string", "Filtre exact par statut du projet."),
				"include_archived": prop("boolean", "Inclut les projets archivés."),
			})),
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
				a.paging(q)
				a.addStr(q, "search", "search")
				a.addStr(q, "status", "status")
				a.addBool(q, "include_archived", "include_archived")
				return c.Get("/projects", q)
			},
		},
		{
			Name:        "xalantis_list_tasks",
			Description: "Recherche et pagine les tâches d'un projet Xalantis (lecture seule). Filtres : texte, statuts, assignés, priorités, sprint, epic, jalon, échéance (due_state), archivées.",
			InputSchema: schema([]string{"project_uuid"}, merge(pagingProps, projectUUID, map[string]any{
				"search":           prop("string", "Recherche textuelle (255 caractères max), ex. une référence de ticket."),
				"statuses":         arrProp("UUID de statuts (jusqu'à 50)."),
				"assignees":        arrProp("UUID de membres (jusqu'à 50)."),
				"labels":           arrProp("UUID de labels (jusqu'à 50)."),
				"priorities":       arrProp("Priorités de tâche."),
				"types":            arrProp("Types de tâche."),
				"epic_id":          prop("string", "UUID public de l'Epic."),
				"sprint":           prop("string", "UUID public du sprint."),
				"milestone":        prop("string", "UUID public du jalon."),
				"due_state":        prop("string", "all, overdue, due_soon, no_due ou has_due."),
				"include_archived": prop("boolean", "Inclut les tâches archivées."),
				"sort_by":          prop("string", "Axe de tri autorisé par l'API."),
				"sort_direction":   prop("string", "asc ou desc."),
			})),
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
				p, err := requireUUID(a, "project_uuid")
				if err != nil {
					return "", err
				}
				a.paging(q)
				a.addStr(q, "search", "search")
				a.addArr(q, "statuses", "status[]")
				a.addArr(q, "assignees", "assignees[]")
				a.addArr(q, "labels", "labels[]")
				a.addArr(q, "priorities", "priorities[]")
				a.addArr(q, "types", "type[]")
				a.addStr(q, "epic_id", "epic_id")
				a.addStr(q, "sprint", "sprint")
				a.addStr(q, "milestone", "milestone")
				a.addStr(q, "due_state", "due_state")
				a.addBool(q, "include_archived", "include_archived")
				a.addStr(q, "sort_by", "sort_by")
				a.addStr(q, "sort_direction", "sort_direction")
				return c.Get("/projects/"+p+"/tasks", q)
			},
		},
		{
			Name:        "xalantis_get_task",
			Description: "Détail relationnel d'une tâche Xalantis par UUID (lecture seule).",
			InputSchema: schema([]string{"project_uuid", "task_uuid"}, merge(projectUUID, taskUUID)),
			Handler: func(raw map[string]any) (string, error) {
				a := args(raw)
				p, err := requireUUID(a, "project_uuid")
				if err != nil {
					return "", err
				}
				t, err := requireUUID(a, "task_uuid")
				if err != nil {
					return "", err
				}
				return c.Get("/projects/"+p+"/tasks/"+t, nil)
			},
		},
		{
			Name:        "xalantis_task_activities",
			Description: "Historique audité d'une tâche Xalantis (changements de statut, d'assigné, etc.), paginé (lecture seule).",
			InputSchema: schema([]string{"project_uuid", "task_uuid"}, merge(pagingProps, projectUUID, taskUUID)),
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
				p, err := requireUUID(a, "project_uuid")
				if err != nil {
					return "", err
				}
				t, err := requireUUID(a, "task_uuid")
				if err != nil {
					return "", err
				}
				a.paging(q)
				return c.Get("/projects/"+p+"/tasks/"+t+"/activities", q)
			},
		},
		{
			Name:        "xalantis_list_sprints",
			Description: "Liste les sprints d'un projet Xalantis, du plus récent au plus ancien (lecture seule).",
			InputSchema: schema([]string{"project_uuid"}, merge(pagingProps, projectUUID)),
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
				p, err := requireUUID(a, "project_uuid")
				if err != nil {
					return "", err
				}
				a.paging(q)
				return c.Get("/projects/"+p+"/sprints", q)
			},
		},
		{
			Name:        "xalantis_list_statuses",
			Description: "Liste les statuts canoniques des tâches d'un projet Xalantis (UUID + libellé), utiles pour filtrer list_tasks (lecture seule).",
			InputSchema: schema([]string{"project_uuid"}, projectUUID),
			Handler: func(raw map[string]any) (string, error) {
				p, err := requireUUID(args(raw), "project_uuid")
				if err != nil {
					return "", err
				}
				return c.Get("/projects/"+p+"/statuses", nil)
			},
		},
		{
			Name:        "xalantis_list_members",
			Description: "Liste les membres d'un projet Xalantis (UUID + nom), utiles pour filtrer list_tasks par assigné (lecture seule).",
			InputSchema: schema([]string{"project_uuid"}, merge(pagingProps, projectUUID)),
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
				p, err := requireUUID(a, "project_uuid")
				if err != nil {
					return "", err
				}
				a.paging(q)
				return c.Get("/projects/"+p+"/members", q)
			},
		},
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l internal/tools && go vet ./internal/tools/ && go test ./internal/tools/`
Expected: no gofmt output, then `ok  paymetrust/xalantis-projects-mcp/internal/tools`.

- [ ] **Step 6: Commit**

```bash
git add internal/tools
git commit -m "feat(tools): port the 7 project tools to the new packages

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Generic search / describe / call tools

**Files:**
- Create: `internal/tools/generic.go`
- Test: `internal/tools/generic_test.go`

**Interfaces:**
- Consumes: everything listed as produced by Tasks 1–4.
- Produces:
  - `const MaxTextBytes = 10 << 20`
  - `func GenericTools(c *xalantis.Client, cat *openapi.Catalog, downloadDir string) []mcp.Tool` — tools in this order: `xalantis_search_operations`, `xalantis_describe_operation`, `xalantis_call_operation`.
  - Call tool results: response body as text for JSON / `+json` / `text/*`; `Succès (HTTP <code>), réponse vide.` for an empty body; otherwise JSON `{"saved_to": <path>, "size": <bytes>, "content_type": <type>}`.

Test operation IDs used below (all exist in the embedded spec): `post_tickets`, `get_tickets`, `delete_tickets_By_uuid`, `get_contracts` (out of scope on purpose), `get_projects_By_projectUuid_tasks`, `post_projects_By_projectUuid_tasks`, `post_projects_By_projectUuid_documents`, `post_projects_By_projectUuid_imports`, `get_projects_By_projectUuid_exports`, `get_projects_By_projectUuid_documents_By_attachmentUuid_download`.

- [ ] **Step 1: Write the failing test**

Create `internal/tools/generic_test.go`:

```go
package tools

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"paymetrust/xalantis-projects-mcp/internal/mcp"
	"paymetrust/xalantis-projects-mcp/internal/openapi"
)

func catalog(t *testing.T) *openapi.Catalog {
	t.Helper()
	c, err := openapi.Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func genericTool(t *testing.T, rec *recorder, dir, name string) mcp.Tool {
	t.Helper()
	return find(t, GenericTools(rec.client(), catalog(t), dir), name)
}

func TestSearchTool(t *testing.T) {
	rec := newRecorder(t, nil)
	search := genericTool(t, rec, t.TempDir(), "xalantis_search_operations")

	out, err := search.Handler(map[string]any{})
	if err != nil || !strings.Contains(out, `"areas"`) || !strings.Contains(out, `"Tickets"`) {
		t.Fatalf("sans filtre : %v %s", err, out)
	}
	out, err = search.Handler(map[string]any{"query": "creer ticket", "method": "POST"})
	if err != nil || !strings.Contains(out, `"operation_id":"post_tickets"`) {
		t.Fatalf("recherche : %v %s", err, out)
	}
}

func TestDescribeTool(t *testing.T) {
	rec := newRecorder(t, nil)
	describe := genericTool(t, rec, t.TempDir(), "xalantis_describe_operation")
	out, err := describe.Handler(map[string]any{"operation_id": "post_tickets"})
	if err != nil || !strings.Contains(out, `"content_type":"application/json"`) {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := describe.Handler(map[string]any{"operation_id": "nope"}); err == nil {
		t.Fatal("opération inconnue : erreur attendue")
	}
}

func TestCallJSON(t *testing.T) {
	rec := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"success":true,"data":{"uuid":"new"}}`))
	})
	call := genericTool(t, rec, t.TempDir(), "xalantis_call_operation")
	out, err := call.Handler(map[string]any{
		"operation_id": "post_projects_By_projectUuid_tasks",
		"path_params":  map[string]any{"projectUuid": "p 1"},
		"headers":      map[string]any{"idempotency-key": "idem-1"},
		"body":         map[string]any{"title": "Nouvelle tâche"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"success":true,"data":{"uuid":"new"}}` {
		t.Errorf("sortie = %s", out)
	}
	req := rec.requests[0]
	if req.Method != "POST" || req.URL.EscapedPath() != "/api/v1/projects/p%201/tasks" {
		t.Errorf("requête = %s %s", req.Method, req.URL.EscapedPath())
	}
	if req.Header.Get("Idempotency-Key") != "idem-1" || req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("en-têtes = %v", req.Header)
	}
	var body map[string]any
	json.Unmarshal([]byte(rec.bodies[0]), &body)
	if body["title"] != "Nouvelle tâche" {
		t.Errorf("corps = %s", rec.bodies[0])
	}
}

func TestCallQueryMapping(t *testing.T) {
	rec := newRecorder(t, nil)
	call := genericTool(t, rec, t.TempDir(), "xalantis_call_operation")
	_, err := call.Handler(map[string]any{
		"operation_id": "get_projects_By_projectUuid_tasks",
		"path_params":  map[string]any{"projectUuid": "p-1"},
		"query": map[string]any{
			"status":           []any{"s-1", "s-2"},
			"include_archived": false,
			"per_page":         float64(25),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	q := rec.requests[0].URL.Query()
	if strings.Join(q["status[]"], ",") != "s-1,s-2" || q.Get("include_archived") != "0" || q.Get("per_page") != "25" {
		t.Errorf("query = %v", q)
	}
}

func TestCallRejectsBadInputWithoutCallingAPI(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	call := genericTool(t, rec, dir, "xalantis_call_operation")
	file := filepath.Join(dir, "a.txt")
	os.WriteFile(file, []byte("x"), 0o644)

	cases := map[string]map[string]any{
		"opération inconnue":    {"operation_id": "get_contracts"},
		"chemin manquant":       {"operation_id": "get_projects_By_projectUuid_tasks"},
		"chemin avec slash":     {"operation_id": "get_projects_By_projectUuid_tasks", "path_params": map[string]any{"projectUuid": "a/b"}},
		"chemin ..":             {"operation_id": "get_projects_By_projectUuid_tasks", "path_params": map[string]any{"projectUuid": ".."}},
		"chemin inconnu":        {"operation_id": "get_projects_By_projectUuid_tasks", "path_params": map[string]any{"projectUuid": "p", "x": "y"}},
		"requête inconnue":      {"operation_id": "get_tickets", "query": map[string]any{"nope": "1"}},
		"requête objet":         {"operation_id": "get_tickets", "query": "pas un objet"},
		"en-tête interdit":      {"operation_id": "get_tickets", "headers": map[string]any{"Authorization": "x"}},
		"en-tête requis":        {"operation_id": "post_projects_By_projectUuid_documents", "path_params": map[string]any{"projectUuid": "p"}, "files": map[string]any{"files": file}},
		"fichier sur JSON":      {"operation_id": "post_tickets", "files": map[string]any{"file": file}},
		"corps sur GET":         {"operation_id": "get_tickets", "body": map[string]any{"a": 1}},
		"fichier absent":        {"operation_id": "post_projects_By_projectUuid_imports", "path_params": map[string]any{"projectUuid": "p"}, "headers": map[string]any{"Idempotency-Key": "k"}, "files": map[string]any{"file": filepath.Join(dir, "absent")}},
		"dossier":               {"operation_id": "post_projects_By_projectUuid_imports", "path_params": map[string]any{"projectUuid": "p"}, "headers": map[string]any{"Idempotency-Key": "k"}, "files": map[string]any{"file": dir}},
		"deux fichiers":         {"operation_id": "post_projects_By_projectUuid_imports", "path_params": map[string]any{"projectUuid": "p"}, "headers": map[string]any{"Idempotency-Key": "k"}, "files": map[string]any{"file": []any{file, file}}},
		"champ fichier inconnu": {"operation_id": "post_projects_By_projectUuid_imports", "path_params": map[string]any{"projectUuid": "p"}, "headers": map[string]any{"Idempotency-Key": "k"}, "files": map[string]any{"autre": file}},
	}
	for name, c := range cases {
		if _, err := call.Handler(c); err == nil {
			t.Errorf("%s : erreur attendue", name)
		}
	}
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

func TestCallMultipartUpload(t *testing.T) {
	rec := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart: %v", err)
		}
		if n := len(r.MultipartForm.File["files[]"]); n != 2 {
			t.Errorf("%d fichiers sous files[]", n)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true}`))
	})
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
	os.WriteFile(a, []byte("A"), 0o644)
	os.WriteFile(b, []byte("B"), 0o644)
	call := genericTool(t, rec, dir, "xalantis_call_operation")
	_, err := call.Handler(map[string]any{
		"operation_id": "post_projects_By_projectUuid_documents",
		"path_params":  map[string]any{"projectUuid": "p-1"},
		"headers":      map[string]any{"Idempotency-Key": "k"},
		"files":        map[string]any{"files": []any{a, b}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.requests) != 1 {
		t.Fatalf("%d appels", len(rec.requests))
	}
}

func downloadServer(t *testing.T, disposition string) *recorder {
	return newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		if disposition != "" {
			w.Header().Set("Content-Disposition", disposition)
		}
		w.Write([]byte("PDFDATA"))
	})
}

func download(t *testing.T, rec *recorder, dir string, extra map[string]any) map[string]any {
	t.Helper()
	call := genericTool(t, rec, dir, "xalantis_call_operation")
	in := map[string]any{
		"operation_id": "get_projects_By_projectUuid_documents_By_attachmentUuid_download",
		"path_params":  map[string]any{"projectUuid": "p-1", "attachmentUuid": "a-1"},
	}
	for k, v := range extra {
		in[k] = v
	}
	out, err := call.Handler(in)
	if err != nil {
		t.Fatal(err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("sortie %s : %v", out, err)
	}
	return res
}

func TestDownloadDefaultDirNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	rec := downloadServer(t, `attachment; filename="rapport.pdf"`)
	first := download(t, rec, dir, nil)
	second := download(t, rec, dir, nil)
	if first["saved_to"] != filepath.Join(dir, "rapport.pdf") || second["saved_to"] != filepath.Join(dir, "rapport (1).pdf") {
		t.Fatalf("chemins : %v / %v", first["saved_to"], second["saved_to"])
	}
	if first["size"].(float64) != 7 || first["content_type"] != "application/octet-stream" {
		t.Errorf("métadonnées : %v", first)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "rapport.pdf"))
	if string(data) != "PDFDATA" {
		t.Errorf("contenu : %q", data)
	}
}

func TestDownloadMaliciousFilenameStaysInDir(t *testing.T) {
	dir := t.TempDir()
	for _, cd := range []string{`attachment; filename="../../evil.sh"`, `attachment; filename="..\\..\\evil.sh"`} {
		res := download(t, downloadServer(t, cd), dir, nil)
		if filepath.Dir(res["saved_to"].(string)) != dir {
			t.Errorf("%s : enregistré hors du dossier : %v", cd, res["saved_to"])
		}
	}
	res := download(t, downloadServer(t, `attachment; filename=".."`), dir, nil)
	if res["saved_to"] != filepath.Join(dir, "get_projects_By_projectUuid_documents_By_attachmentUuid_download.bin") {
		t.Errorf("nom de repli : %v", res["saved_to"])
	}
}

func TestDownloadSaveTo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sous", "x.pdf")
	os.MkdirAll(filepath.Dir(target), 0o755)
	res := download(t, downloadServer(t, ""), t.TempDir(), map[string]any{"save_to": target})
	if res["saved_to"] != target {
		t.Errorf("save_to : %v", res["saved_to"])
	}
}

func TestCallEmptyAndTextResponses(t *testing.T) {
	rec := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Write([]byte("a,b\n1,2\n"))
	})
	call := genericTool(t, rec, t.TempDir(), "xalantis_call_operation")
	out, err := call.Handler(map[string]any{"operation_id": "delete_tickets_By_uuid", "path_params": map[string]any{"uuid": "t-1"}})
	if err != nil || out != "Succès (HTTP 204), réponse vide." {
		t.Errorf("DELETE : %v %q", err, out)
	}
	out, err = call.Handler(map[string]any{"operation_id": "get_projects_By_projectUuid_exports", "path_params": map[string]any{"projectUuid": "p"}, "query": map[string]any{"format": "csv"}})
	if err != nil || out != "a,b\n1,2\n" {
		t.Errorf("CSV : %v %q", err, out)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tools/`
Expected: FAIL, compile error `undefined: GenericTools`.

- [ ] **Step 3: Write the implementation**

Create `internal/tools/generic.go`:

```go
package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"paymetrust/xalantis-projects-mcp/internal/mcp"
	"paymetrust/xalantis-projects-mcp/internal/openapi"
	"paymetrust/xalantis-projects-mcp/internal/xalantis"
)

// MaxTextBytes borne une réponse texte renvoyée au client MCP.
const MaxTextBytes = 10 << 20

func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// GenericTools renvoie les outils de recherche, description et appel des
// opérations du catalogue. downloadDir reçoit les fichiers téléchargés sans save_to.
func GenericTools(c *xalantis.Client, cat *openapi.Catalog, downloadDir string) []mcp.Tool {
	areas := make([]any, len(openapi.Areas))
	for i, a := range openapi.Areas {
		areas[i] = a
	}
	opID := map[string]any{"operation_id": prop("string", "operation_id renvoyé par xalantis_search_operations.")}
	return []mcp.Tool{
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
			Handler: func(raw map[string]any) (string, error) {
				a := args(raw)
				query, area, method := a.str("query"), a.str("area"), a.str("method")
				if query == "" && area == "" && method == "" {
					return toJSON(map[string]any{"areas": cat.AreaCounts()})
				}
				res := cat.Search(query, area, method)
				return toJSON(map[string]any{"count": len(res), "operations": res})
			},
		},
		{
			Name: "xalantis_describe_operation",
			Description: "Étape 2/3 : paramètres (chemin, requête, en-têtes) et schéma du corps d'une opération Xalantis. " +
				"Ensuite : xalantis_call_operation.",
			InputSchema: schema([]string{"operation_id"}, opID),
			Handler: func(raw map[string]any) (string, error) {
				d, err := cat.Describe(args(raw).str("operation_id"))
				if err != nil {
					return "", err
				}
				return toJSON(d)
			},
		},
		{
			Name: "xalantis_call_operation",
			Description: "Étape 3/3 : exécute une opération Xalantis décrite par xalantis_describe_operation. " +
				"Peut créer, modifier ou supprimer des données selon les scopes de la clé API. " +
				"Les fichiers envoyés sont des chemins locaux ; les fichiers reçus sont enregistrés localement.",
			InputSchema: schema([]string{"operation_id"}, merge(opID, map[string]any{
				"path_params": map[string]any{"type": "object", "description": "Paramètres de chemin, ex. {\"projectUuid\": \"…\"}."},
				"query":       map[string]any{"type": "object", "description": "Paramètres de requête. Tableau = valeurs répétées (nom[]), objet = nom[clé]."},
				"headers":     map[string]any{"type": "object", "description": "En-têtes déclarés par l'opération (Idempotency-Key, If-Match)."},
				"body":        map[string]any{"type": "object", "description": "Corps JSON, ou champs texte pour une opération multipart."},
				"files":       map[string]any{"type": "object", "description": "Opérations multipart : champ → chemin local ou liste de chemins."},
				"save_to":     prop("string", "Chemin local où enregistrer un fichier reçu (défaut : dossier Téléchargements)."),
			})),
			Handler: func(raw map[string]any) (string, error) {
				return callOperation(c, cat, downloadDir, args(raw))
			},
		},
	}
}

func callOperation(c *xalantis.Client, cat *openapi.Catalog, downloadDir string, a args) (string, error) {
	id := a.str("operation_id")
	op, ok := cat.Get(id)
	if !ok {
		return "", fmt.Errorf("opération inconnue : %s (utilisez xalantis_search_operations)", id)
	}
	pathParams, err := a.obj("path_params")
	if err != nil {
		return "", err
	}
	queryArgs, err := a.obj("query")
	if err != nil {
		return "", err
	}
	headerArgs, err := a.obj("headers")
	if err != nil {
		return "", err
	}
	body, err := a.obj("body")
	if err != nil {
		return "", err
	}
	fileArgs, err := a.obj("files")
	if err != nil {
		return "", err
	}

	path, err := buildPath(op, pathParams)
	if err != nil {
		return "", err
	}
	query, err := buildQuery(op, queryArgs)
	if err != nil {
		return "", err
	}
	headers, err := buildHeaders(op, headerArgs)
	if err != nil {
		return "", err
	}

	var reader io.Reader
	contentType := ""
	switch op.BodyType {
	case "multipart/form-data":
		fields := url.Values{}
		for k, v := range body {
			if err := addValue(fields, k, v, true); err != nil {
				return "", err
			}
		}
		files, err := buildFiles(op, fileArgs)
		if err != nil {
			return "", err
		}
		reader, contentType, err = xalantis.Multipart(fields, files)
		if err != nil {
			return "", err
		}
	case "application/json":
		if len(fileArgs) > 0 {
			return "", fmt.Errorf("l'opération %s n'accepte pas de fichiers", op.ID)
		}
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return "", err
			}
			reader, contentType = bytes.NewReader(b), "application/json"
		}
	default:
		if len(fileArgs) > 0 {
			return "", fmt.Errorf("l'opération %s n'accepte pas de fichiers", op.ID)
		}
		if body != nil {
			return "", fmt.Errorf("l'opération %s n'accepte pas de corps", op.ID)
		}
	}

	resp, err := c.Do(op.Method, path, query, headers, reader, contentType)
	if err != nil {
		return "", err
	}
	if len(resp.Body) == 0 {
		return fmt.Sprintf("Succès (HTTP %d), réponse vide.", resp.Status), nil
	}
	if isText(resp.Header.Get("Content-Type")) {
		if len(resp.Body) > MaxTextBytes {
			return "", fmt.Errorf("réponse texte trop volumineuse (%d octets) : affinez les filtres ou paginez", len(resp.Body))
		}
		return string(resp.Body), nil
	}
	return saveDownload(downloadDir, a.str("save_to"), op.ID, resp)
}

var pathParamPattern = regexp.MustCompile(`\{([^}]+)\}`)

func buildPath(op *openapi.Operation, values map[string]any) (string, error) {
	for k := range values {
		if _, ok := op.Param("path", k); !ok {
			return "", fmt.Errorf("paramètre de chemin inconnu pour %s : %s", op.ID, k)
		}
	}
	var firstErr error
	path := pathParamPattern.ReplaceAllStringFunc(op.Path, func(m string) string {
		name := m[1 : len(m)-1]
		v, _ := values[name].(string)
		v = strings.TrimSpace(v)
		if err := checkSegment(name, v); err != nil && firstErr == nil {
			firstErr = err
		}
		return url.PathEscape(v)
	})
	return path, firstErr
}

// queryName retrouve le nom déclaré d'un paramètre de requête : nom exact,
// nom + "[]", ou nom de base d'un paramètre objet (ex. custom_fields[clé]).
func queryName(op *openapi.Operation, key string) (string, bool) {
	if _, ok := op.Param("query", key); ok {
		return key, true
	}
	if _, ok := op.Param("query", key+"[]"); ok {
		return key + "[]", true
	}
	base, _, _ := strings.Cut(key, "[")
	for _, p := range op.Params {
		if p.In != "query" {
			continue
		}
		if pb, _, found := strings.Cut(p.Name, "["); found && pb == base && !strings.HasSuffix(p.Name, "[]") {
			return key, true
		}
	}
	return "", false
}

func buildQuery(op *openapi.Operation, values map[string]any) (url.Values, error) {
	q := url.Values{}
	for k, v := range values {
		name, ok := queryName(op, k)
		if !ok {
			return nil, fmt.Errorf("paramètre de requête inconnu pour %s : %s", op.ID, k)
		}
		if err := addValue(q, name, v, false); err != nil {
			return nil, err
		}
	}
	for _, p := range op.Params {
		if p.In == "query" && p.Required && !q.Has(p.Name) {
			return nil, fmt.Errorf("paramètre de requête requis manquant : %s", p.Name)
		}
	}
	return q, nil
}

func buildHeaders(op *openapi.Operation, values map[string]any) (map[string]string, error) {
	h := map[string]string{}
	for k, v := range values {
		var name string
		for _, p := range op.Params {
			if p.In == "header" && strings.EqualFold(p.Name, k) {
				name = p.Name
			}
		}
		if name == "" {
			return nil, fmt.Errorf("en-tête non autorisé pour %s : %s", op.ID, k)
		}
		s, err := scalar(name, v)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(s) != "" {
			h[name] = strings.TrimSpace(s)
		}
	}
	for _, p := range op.Params {
		if p.In == "header" && p.Required && h[p.Name] == "" {
			return nil, fmt.Errorf("en-tête requis manquant : %s (valeur unique par mutation, ex. un UUID)", p.Name)
		}
	}
	return h, nil
}

func buildFiles(op *openapi.Operation, values map[string]any) ([]xalantis.FilePart, error) {
	names := make([]string, 0, len(values))
	for k := range values {
		names = append(names, k)
	}
	sort.Strings(names)
	var parts []xalantis.FilePart
	for _, name := range names {
		var field *openapi.FileField
		for i := range op.FileFields {
			if op.FileFields[i].Name == name {
				field = &op.FileFields[i]
			}
		}
		if field == nil {
			return nil, fmt.Errorf("champ fichier inconnu pour %s : %s", op.ID, name)
		}
		var paths []string
		switch v := values[name].(type) {
		case string:
			paths = []string{v}
		case []any:
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("champ fichier %s : chemins texte attendus", name)
				}
				paths = append(paths, s)
			}
		default:
			return nil, fmt.Errorf("champ fichier %s : chemin ou liste de chemins attendu", name)
		}
		if len(paths) > 1 && !field.Multiple {
			return nil, fmt.Errorf("champ fichier %s : un seul fichier accepté", name)
		}
		key := name
		if field.Multiple {
			key += "[]"
		}
		for _, p := range paths {
			info, err := os.Stat(p)
			if err != nil || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("fichier introuvable ou invalide : %s", p)
			}
			parts = append(parts, xalantis.FilePart{Field: key, Path: p})
		}
	}
	return parts, nil
}

func isText(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json") || strings.HasPrefix(mt, "text/")
}

// safeFilename extrait un nom de fichier sans chemin de Content-Disposition.
func safeFilename(disposition string) string {
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	name := filepath.Base(strings.ReplaceAll(params["filename"], "\\", "/"))
	if name == "." || name == ".." || name == "/" {
		return ""
	}
	return name
}

// createUnique crée path, ou « nom (1).ext », « nom (2).ext »… s'il existe déjà.
func createUnique(path string) (*os.File, string, error) {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 0; i < 1000; i++ {
		p := path
		if i > 0 {
			p = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, p, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("aucun nom libre pour %s", path)
}

func saveDownload(downloadDir, saveTo, opID string, resp *xalantis.Response) (string, error) {
	target := saveTo
	if target == "" {
		name := safeFilename(resp.Header.Get("Content-Disposition"))
		if name == "" {
			name = opID + ".bin"
		}
		target = filepath.Join(downloadDir, name)
	}
	f, path, err := createUnique(target)
	if err != nil {
		return "", fmt.Errorf("enregistrement impossible : %v", err)
	}
	_, werr := f.Write(resp.Body)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		return "", fmt.Errorf("écriture de %s impossible : %v", path, errors.Join(werr, cerr))
	}
	return toJSON(map[string]any{
		"saved_to":     path,
		"size":         len(resp.Body),
		"content_type": resp.Header.Get("Content-Type"),
	})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l internal && go vet ./internal/... && go test ./internal/...`
Expected: no gofmt output, then `ok` for `mcp`, `openapi`, `tools` and `xalantis`.

- [ ] **Step 5: Commit**

```bash
git add internal/tools
git commit -m "feat(tools): add generic search, describe and call tools

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Entry point, end-to-end test, binary

**Files:**
- Create: `cmd/xalantis-projects-mcp/main.go`
- Delete: `main.go`
- Modify: `test_mcp.py` (full replacement below)
- Modify: `xalantis-projects-mcp` (rebuilt binary)

**Interfaces:**
- Consumes: `mcp.NewServer`, `(*Server).Register`, `(*Server).Serve`; `xalantis.NewClient`, `xalantis.Config`; `openapi.Load`; `tools.ProjectTools`, `tools.GenericTools`.
- Produces: the binary. `initialize` returns `serverInfo.version = "2.0.0"` and an `instructions` string that names `xalantis_search_operations`.

- [ ] **Step 1: Write the failing end-to-end test**

Replace the whole content of `test_mcp.py` with:

```python
#!/usr/bin/env python3
"""Test du serveur MCP : mock de l'API Xalantis + dialogue JSON-RPC via stdio."""
import json, os, subprocess, sys, tempfile, threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse, parse_qs

REQUESTS = []

class Mock(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        u = urlparse(self.path)
        REQUESTS.append((self.path, self.headers.get("Authorization")))
        routes = {
            "/api/v1/projects": {"success": True, "data": [
                {"uuid": "p-1", "key": "IAP", "name": "Intégrations & tests"},
                {"uuid": "p-2", "key": "CAP", "name": "Core API"}]},
            "/api/v1/projects/p-1/tasks": {"success": True, "data": [
                {"uuid": "t-1", "reference": "IAP-102", "title": "Test du mapping des codes d'erreur", "status": {"name": "Bloqué"}}],
                "meta": {"total": 1}},
            "/api/v1/projects/p-1/tasks/t-1": {"success": True, "data": {"uuid": "t-1", "reference": "IAP-102"}},
            "/api/v1/projects/p-1/tasks/t-1/activities": {"success": True, "data": [
                {"type": "status_changed", "from": "En cours", "to": "Bloqué", "at": "2026-08-28"}]},
            "/api/v1/projects/p-1/sprints": {"success": True, "data": []},
            "/api/v1/projects/p-1/statuses": {"success": True, "data": [{"uuid": "s-1", "name": "Bloqué"}]},
            "/api/v1/projects/p-1/members": {"success": True, "data": [{"uuid": "m-1", "name": "Salif Ka"}]},
        }
        if u.path == "/api/v1/projects/p-1/documents/a-1/download":
            self.send_response(200)
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("Content-Disposition", 'attachment; filename="rapport.pdf"')
            self.end_headers()
            self.wfile.write(b"%PDF-test")
            return
        if u.path == "/api/v1/forbidden":
            self.send_response(403); self.end_headers()
            self.wfile.write(b'{"message":"This API key does not have the required permission."}')
            return
        body = routes.get(u.path)
        if body is None:
            self.send_response(404); self.end_headers(); self.wfile.write(b'{"message":"not found"}')
            return
        payload = json.dumps({**body, "_query": parse_qs(u.query)}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(payload)

    def do_POST(self):
        u = urlparse(self.path)
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        REQUESTS.append((self.path, self.headers.get("Authorization")))
        POSTS.append({"path": u.path, "idem": self.headers.get("Idempotency-Key"),
                      "type": self.headers.get("Content-Type"), "body": json.loads(body or b"null")})
        payload = json.dumps({"success": True, "data": {"uuid": "tk-1"}}).encode()
        self.send_response(201)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(payload)

POSTS = []
srv = HTTPServer(("127.0.0.1", 0), Mock)
port = srv.server_address[1]
threading.Thread(target=srv.serve_forever, daemon=True).start()

msgs = [
    {"jsonrpc": "2.0", "id": 1, "method": "initialize",
     "params": {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "test", "version": "0"}}},
    {"jsonrpc": "2.0", "method": "notifications/initialized"},
    {"jsonrpc": "2.0", "id": 2, "method": "ping"},
    {"jsonrpc": "2.0", "id": 3, "method": "tools/list"},
    {"jsonrpc": "2.0", "id": 4, "method": "tools/call",
     "params": {"name": "xalantis_list_projects", "arguments": {"search": "IAP"}}},
    {"jsonrpc": "2.0", "id": 5, "method": "tools/call",
     "params": {"name": "xalantis_list_tasks", "arguments": {
         "project_uuid": "p-1", "statuses": ["s-1"], "due_state": "overdue", "per_page": 50, "page": 2}}},
    {"jsonrpc": "2.0", "id": 6, "method": "tools/call",
     "params": {"name": "xalantis_task_activities", "arguments": {"project_uuid": "p-1", "task_uuid": "t-1"}}},
    {"jsonrpc": "2.0", "id": 7, "method": "tools/call",
     "params": {"name": "xalantis_list_tasks", "arguments": {}}},  # project_uuid manquant -> erreur outil
    {"jsonrpc": "2.0", "id": 8, "method": "tools/call",
     "params": {"name": "xalantis_get_task", "arguments": {"project_uuid": "p-1", "task_uuid": "../secret"}}},  # injection path
    {"jsonrpc": "2.0", "id": 9, "method": "unknown/method"},
    {"jsonrpc": "2.0", "id": 10, "method": "tools/call",
     "params": {"name": "xalantis_get_task", "arguments": {"project_uuid": "p-1", "task_uuid": ".."}}},  # segment ".."
    {"jsonrpc": "2.0", "method": "tools/call",
     "params": {"name": "xalantis_list_projects", "arguments": {}}},  # notification : ni réponse ni appel API
    {"jsonrpc": "2.0", "id": 11, "method": "tools/call",
     "params": {"name": "xalantis_search_operations", "arguments": {"query": "creer tache", "method": "POST"}}},
    {"jsonrpc": "2.0", "id": 12, "method": "tools/call",
     "params": {"name": "xalantis_describe_operation", "arguments": {"operation_id": "post_projects_By_projectUuid_tasks"}}},
    {"jsonrpc": "2.0", "id": 13, "method": "tools/call",
     "params": {"name": "xalantis_call_operation", "arguments": {
         "operation_id": "post_projects_By_projectUuid_tasks", "path_params": {"projectUuid": "p-1"},
         "headers": {"Idempotency-Key": "idem-42"}, "body": {"title": "Nouvelle tâche"}}}},
    {"jsonrpc": "2.0", "id": 14, "method": "tools/call",
     "params": {"name": "xalantis_call_operation", "arguments": {
         "operation_id": "get_projects_By_projectUuid_documents_By_attachmentUuid_download",
         "path_params": {"projectUuid": "p-1", "attachmentUuid": "a-1"}}}},
    {"jsonrpc": "2.0", "id": 15, "method": "tools/call",
     "params": {"name": "xalantis_call_operation", "arguments": {
         "operation_id": "post_projects_By_projectUuid_tasks", "path_params": {"projectUuid": "p-1"},
         "body": {"title": "sans clé"}}}},  # Idempotency-Key requise -> erreur sans appel API
]
stdin_data = "".join(json.dumps(m) + "\n" for m in msgs)

home = tempfile.mkdtemp()
os.mkdir(os.path.join(home, "Downloads"))
proc = subprocess.run(
    ["./xalantis-projects-mcp"],
    input=stdin_data, capture_output=True, text=True, timeout=30,
    env={"XALANTIS_API_KEY": "sk_live_test", "XALANTIS_BASE_URL": f"http://127.0.0.1:{port}", "PATH": "/usr/bin", "HOME": home},
)
resp = {}
for line in proc.stdout.splitlines():
    if line.strip():
        d = json.loads(line)
        resp[d.get("id")] = d

ok = True
def check(cond, label):
    global ok
    print(("PASS " if cond else "FAIL ") + label)
    ok = ok and cond

check(resp[1]["result"]["serverInfo"]["name"] == "xalantis-projects-mcp", "initialize")
check(resp[2]["result"] == {}, "ping")
tools = {t["name"] for t in resp[3]["result"]["tools"]}
check(len(tools) == 10 and {"xalantis_list_tasks", "xalantis_call_operation"} <= tools, f"tools/list ({len(tools)} outils)")
check("xalantis_search_operations" in resp[1]["result"].get("instructions", ""), "instructions d'initialisation")
r4 = json.loads(resp[4]["result"]["content"][0]["text"])
check(r4["_query"] == {"search": ["IAP"]} and r4["data"][0]["key"] == "IAP", "list_projects + query")
r5 = json.loads(resp[5]["result"]["content"][0]["text"])
q5 = r5["_query"]
check(q5.get("status[]") == ["s-1"] and q5.get("due_state") == ["overdue"]
      and q5.get("per_page") == ["50"] and q5.get("page") == ["2"], "list_tasks filtres status[]/due_state/pagination")
r6 = json.loads(resp[6]["result"]["content"][0]["text"])
check(r6["data"][0]["type"] == "status_changed", "task_activities")
check(resp[7]["result"]["isError"] and "project_uuid" in resp[7]["result"]["content"][0]["text"], "argument requis manquant")
check(resp[8]["result"]["isError"] and "invalide" in resp[8]["result"]["content"][0]["text"], "rejet injection de chemin")
check(resp[9]["error"]["code"] == -32601, "méthode inconnue")
check(resp[10]["result"]["isError"] and "invalide" in resp[10]["result"]["content"][0]["text"], "rejet segment ..")
check(None not in resp, "pas de réponse à une notification tools/call")
r11 = json.loads(resp[11]["result"]["content"][0]["text"])
check(any(o["operation_id"] == "post_projects_By_projectUuid_tasks" for o in r11["operations"]), "search_operations")
r12 = json.loads(resp[12]["result"]["content"][0]["text"])
check(r12["method"] == "POST" and r12["body"]["content_type"] == "application/json"
      and any(p["name"] == "Idempotency-Key" and p["required"] for p in r12["parameters"]), "describe_operation")
r13 = json.loads(resp[13]["result"]["content"][0]["text"])
check(r13["data"]["uuid"] == "tk-1" and POSTS and POSTS[0] == {
    "path": "/api/v1/projects/p-1/tasks", "idem": "idem-42", "type": "application/json",
    "body": {"title": "Nouvelle tâche"}}, "call_operation POST JSON")
r14 = json.loads(resp[14]["result"]["content"][0]["text"])
saved = os.path.join(home, "Downloads", "rapport.pdf")
check(r14["saved_to"] == saved and open(saved, "rb").read() == b"%PDF-test", "call_operation téléchargement")
check(resp[15]["result"]["isError"] and "Idempotency-Key" in resp[15]["result"]["content"][0]["text"], "Idempotency-Key requise")
check(all(a == "Bearer sk_live_test" for _, a in REQUESTS), "header Authorization sur chaque appel")
check(len(REQUESTS) == 5, f"{len(REQUESTS)} appels API (les entrées invalides n'atteignent pas l'API)")
check(proc.stderr.strip() == "", "stderr vide")

srv.shutdown()
sys.exit(0 if ok else 1)
```

- [ ] **Step 2: Run it against the current binary to verify it fails**

Run: `go build -o xalantis-projects-mcp main.go && python3 test_mcp.py`
Expected: exit code 1. The output shows `FAIL tools/list (7 outils)` and `FAIL instructions d'initialisation`, then a `json.decoder.JSONDecodeError` traceback at the `r11 = …` line, because the old binary answers `Erreur : outil inconnu` for the new tools.

- [ ] **Step 3: Write the entry point and remove the old file**

Create `cmd/xalantis-projects-mcp/main.go`:

```go
// xalantis-projects-mcp — serveur MCP (stdio) pour l'API Xalantis (api/v1) :
// projets et tâches, service desk et catalogue de services.
//
// Configuration par variables d'environnement :
//
//	XALANTIS_API_KEY  (obligatoire) — clé API tenant ; les scopes décident des opérations permises
//	XALANTIS_BASE_URL (optionnel)   — défaut : https://xalantis.com
//
// Compilation : go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"paymetrust/xalantis-projects-mcp/internal/mcp"
	"paymetrust/xalantis-projects-mcp/internal/openapi"
	"paymetrust/xalantis-projects-mcp/internal/tools"
	"paymetrust/xalantis-projects-mcp/internal/xalantis"
)

const (
	serverName    = "xalantis-projects-mcp"
	serverVersion = "2.0.0"
	instructions  = "Pour les lectures courantes de projets, utilisez les 7 outils dédiés (xalantis_list_projects, xalantis_list_tasks…). " +
		"Pour toute autre opération Xalantis (tickets, SLA, catalogue, écriture sur les projets…) : " +
		"xalantis_search_operations, puis xalantis_describe_operation, puis xalantis_call_operation."
)

func main() {
	cat, err := openapi.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "spécification OpenAPI embarquée illisible :", err)
		os.Exit(1)
	}
	client := xalantis.NewClient(xalantis.Config{
		BaseURL:   os.Getenv("XALANTIS_BASE_URL"),
		APIKey:    os.Getenv("XALANTIS_API_KEY"),
		UserAgent: serverName + "/" + serverVersion,
	})
	home, _ := os.UserHomeDir()

	srv := mcp.NewServer(serverName, serverVersion, instructions)
	srv.Register(tools.ProjectTools(client)...)
	srv.Register(tools.GenericTools(client, cat, filepath.Join(home, "Downloads"))...)
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

Then:

```bash
git rm main.go
```

- [ ] **Step 4: Build and run every test**

```bash
gofmt -l . && go vet ./... && go test ./... && go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp && python3 test_mcp.py
```

Expected: no gofmt output, `ok` for the four `internal` packages (`cmd/xalantis-projects-mcp` reports `[no test files]`), then 20 `PASS` lines from `test_mcp.py` and exit code 0.

- [ ] **Step 5: Commit**

```bash
git add cmd test_mcp.py xalantis-projects-mcp
git commit -m "feat: wire generic tools into new entry point, drop root main.go

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Documentation

**Files:**
- Modify: `README.md` (full replacement below)
- Modify: `docs/superpowers/specs/2026-09-16-xalantis-api-coverage-design.md` (append a section)

- [ ] **Step 1: Replace the README**

Replace the whole content of `README.md` with:

````markdown
# xalantis-projects-mcp

Serveur MCP local (stdio) pour l'API Xalantis. Il couvre 206 opérations :
projets et tâches, service desk (tickets, automatisations, catégories, tags,
SLA, escalades, réponses prédéfinies, disponibilité des agents) et catalogue
de services. Il complète le connecteur Xalantis existant.

> **Attention : écriture possible.** Le serveur expose aussi les opérations
> de création, de modification et de suppression. Ce que Claude peut faire
> dépend uniquement des scopes de la clé API. Pour un usage en lecture seule,
> utilisez une clé qui n'a que des scopes `:read`.

## Outils exposés (10)

Outils dédiés aux lectures courantes de projets (inchangés) :

| Outil | Rôle |
|---|---|
| `xalantis_list_projects` | Liste des projets accessibles |
| `xalantis_list_tasks` | Recherche/pagination des tâches d'un projet (statuts, assignés, sprint, due_state…) |
| `xalantis_get_task` | Détail d'une tâche |
| `xalantis_task_activities` | Historique audité d'une tâche |
| `xalantis_list_sprints` | Sprints d'un projet |
| `xalantis_list_statuses` | Statuts canoniques (UUID + libellé) |
| `xalantis_list_members` | Membres d'un projet (UUID + nom) |

Outils génériques, pour toutes les autres opérations, à utiliser dans cet ordre :

| Outil | Rôle |
|---|---|
| `xalantis_search_operations` | 1. Trouver une opération (mots-clés, domaine, méthode). Sans filtre : liste des domaines. |
| `xalantis_describe_operation` | 2. Voir ses paramètres et le schéma de son corps. |
| `xalantis_call_operation` | 3. L'exécuter (`path_params`, `query`, `headers`, `body`, `files`, `save_to`). |

Les opérations d'écriture exigent souvent l'en-tête `Idempotency-Key`
(valeur unique par mutation, par ex. un UUID) : passez-le dans `headers`.

## Domaines couverts et scopes

| Domaine | Opérations | Scopes |
|---|---|---|
| Projets et tâches | 108 | `projects:read`, `projects:write`, `projects:delete`, `projects:export`, `projects:import`, `projects:track_time`, `projects:manage_assignees`, `projects:manage_automations`, `projects:manage_configuration`, `projects:manage_documents`, `projects:manage_integrations`, `projects:manage_members`, `projects:manage_portal` |
| Tickets | 57 | `tickets:read`, `tickets:write` |
| Automatisations de tickets | 8 | `tickets:read`, `tickets:manage_automations` |
| Catégories de tickets | 4 | `tickets:read`, `tickets:manage_categories` |
| Tags de tickets | 5 | `tickets:read`, `tickets:write`, `tickets:manage_categories` |
| Politiques SLA | 4 | `tickets:read`, `tickets:manage_sla` |
| Violations SLA | 2 | `tickets:read`, `tickets:write` |
| Politiques d’escalade | 3 | `tickets:read`, `tickets:manage_escalations` |
| Réponses prédéfinies | 4 | `tickets:read`, `tickets:write` |
| Disponibilité des agents | 2 | `tickets:read`, `tickets:write` |
| Catalogue de services | 3 | `tickets:read`, `tickets:write` |
| Administration du catalogue | 6 | `tickets:read`, `tickets:write` |

Le scope exact de chaque opération est indiqué par `xalantis_search_operations`
et `xalantis_describe_operation`. Sans le scope requis, l'API répond 403 et
l'outil renvoie l'erreur.

## Fichiers

- **Envoi** : `files` associe un champ à un chemin local (ou une liste de
  chemins), par ex. `{"files": ["/Users/moi/Documents/cr.pdf"]}`.
- **Réception** : un fichier reçu est enregistré dans `~/Downloads` (ou à
  `save_to`). Un fichier existant n'est jamais écrasé : ` (1)`, ` (2)`… sont
  ajoutés au nom. L'outil renvoie le chemin, la taille et le type.
- Les réponses JSON ou texte (dont CSV) sont renvoyées directement.

## Prérequis

- Une clé API Xalantis avec les scopes voulus (Xalantis → paramètres de la clé API).
- macOS Apple Silicon pour le binaire fourni (`xalantis-projects-mcp`). Pour un
  autre système : `go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp`
  (Go ≥ 1.24, aucune dépendance).

## Installation (Claude Desktop, macOS)

1. Poser ce dossier quelque part, par ex. `~/Documents/xalantis-projects-mcp/`.
2. Rendre le binaire exécutable et lever la quarantaine macOS (fichier téléchargé) :
   ```bash
   chmod +x ~/Documents/xalantis-projects-mcp/xalantis-projects-mcp
   xattr -d com.apple.quarantine ~/Documents/xalantis-projects-mcp/xalantis-projects-mcp 2>/dev/null
   ```
3. Claude Desktop → Réglages → Développeur → Éditer la config, ajouter dans `mcpServers` :
   ```json
   {
     "mcpServers": {
       "xalantis-projets": {
         "command": "/Users/VOTRE_LOGIN/Documents/xalantis-projects-mcp/xalantis-projects-mcp",
         "env": {
           "XALANTIS_API_KEY": "sk_live_VOTRE_CLE"
         }
       }
     }
   }
   ```
   (Si le fichier contient déjà le connecteur `xalantis`, ajouter simplement l'entrée
   `xalantis-projets` à côté, dans le même bloc `mcpServers`.)
4. Quitter complètement Claude Desktop et le rouvrir.

## Variables d'environnement

- `XALANTIS_API_KEY` (obligatoire) — clé API tenant `sk_live_…`.
  Elle ne quitte jamais votre machine : le serveur tourne en local et parle
  directement à `xalantis.com`.
- `XALANTIS_BASE_URL` (optionnel) — défaut `https://xalantis.com` ; à modifier
  uniquement si votre instance Xalantis est sur un autre domaine.

## Développement

```
cmd/xalantis-projects-mcp/   point d'entrée : configuration et assemblage
internal/mcp/                boucle JSON-RPC stdio et registre d'outils
internal/xalantis/           client HTTP de l'API
internal/openapi/            spécification embarquée, recherche et description
internal/tools/              outils dédiés et génériques
```

- Mettre à jour l'API : remplacer `internal/openapi/xalantis-openapi.json`
  puis recompiler.
- Tests unitaires : `go test ./...`
- Test de bout en bout : `go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp && python3 test_mcp.py`

## Notes

- Rate limit API : 60 requêtes/minute par clé — le serveur remonte l'erreur avec le délai à attendre.
- Réponses limitées à 100 Mo (fichiers) et 10 Mo (texte renvoyé à Claude).
````

- [ ] **Step 2: Check the README against the code**

```bash
grep -c "app.xalantis" README.md cmd/xalantis-projects-mcp/main.go internal/xalantis/client.go
grep -n "go build" README.md
```

Expected: every count is `0`. Every `go build` line uses `./cmd/xalantis-projects-mcp`.

- [ ] **Step 3: Record the planning deviations in the spec**

Append to the end of `docs/superpowers/specs/2026-09-16-xalantis-api-coverage-design.md`:

```markdown
## Amendments (implementation planning, 2026-09-16)

Facts found in the OpenAPI file changed these details:

1. `tools` imports `mcp` for the `mcp.Tool` type; `mcp` stays a leaf package.
2. Required header parameters are validated like required query parameters.
   70 operations require `Idempotency-Key`.
3. Query parameter names are used as declared (`status[]`, `type[]`, …).
   `query` also accepts the name without `[]` and `base[sub]` keys for object
   parameters (`custom_fields[clé]`). Unknown names are rejected.
4. The client reads responses up to 100 MB (documents reach 50 MB). Text
   returned to Claude is limited to 10 MB; larger text is an error.
5. Multipart array file fields are sent as `name[]` (e.g. `files[]`).
6. `save_to` never overwrites either; it uses the same ` (1)` suffix rule.
7. "No filters → list areas" is handled by the search tool, not by
   `Catalog.Search`.
8. Unknown path parameter keys are rejected; operations without a request
   body reject `body`.
9. No `$ref` appears in request bodies or parameters of the 206 operations
   (only in responses). The resolver is kept as a safeguard.
```

- [ ] **Step 4: Final verification**

```bash
go test ./... && go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp && python3 test_mcp.py && git status --short
```

Expected: all tests pass, 20 `PASS` lines, and `git status` lists only `README.md` and the spec file as modified (the binary is unchanged since Task 6).

- [ ] **Step 5: Commit**

```bash
git add README.md docs/superpowers/specs/2026-09-16-xalantis-api-coverage-design.md
git commit -m "docs: document generic tools, areas, scopes and file handling

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

