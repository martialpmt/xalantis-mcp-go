package tools

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martialpmt/xalantis-mcp-go/internal/mcp"
	"github.com/martialpmt/xalantis-mcp-go/internal/xalantis"
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
		_, _ = w.Write([]byte(`{"success":true}`))
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

// TestPerPageCapped : une demande démesurée est ramenée au plafond de l'API
// plutôt que d'aller chercher une page que tooLarge refuserait ensuite.
func TestPerPageCapped(t *testing.T) {
	rec := newRecorder(t, nil)
	tool := find(t, ProjectTools(rec.client()), "xalantis_list_projects")
	if _, err := tool.Handler(map[string]any{"per_page": float64(5000)}); err != nil {
		t.Fatal(err)
	}
	if got := rec.requests[0].URL.Query().Get("per_page"); got != "100" {
		t.Errorf("per_page = %s, attendu 100", got)
	}
}

// TestDedicatedToolsCapResponseSize : les outils dédiés appliquent le même
// plafond que les outils génériques, sinon une liste géante sature le contexte.
func TestDedicatedToolsCapResponseSize(t *testing.T) {
	rec := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"x":"` + strings.Repeat("y", MaxTextBytes) + `"}`))
	})
	tool := find(t, ProjectTools(rec.client()), "xalantis_list_projects")
	_, err := tool.Handler(map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "trop volumineuse") {
		t.Fatalf("plafond non appliqué : %v", err)
	}
	if !strings.Contains(err.Error(), "per_page") {
		t.Errorf("l'erreur doit dire comment s'en sortir : %v", err)
	}
}
