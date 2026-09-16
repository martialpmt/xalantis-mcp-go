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
