package tools

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martialpmt/xalantis-mcp-go/internal/mcp"
	"github.com/martialpmt/xalantis-mcp-go/internal/openapi"
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
	return find(t, GenericTools(rec.client(), catalog(t), dir, false), name)
}

// realDir résout dir via les liens symboliques, comme le fait la validation
// des chemins. Sur macOS, t.TempDir() vit sous /var/folders/…, lui-même un
// lien vers /private/var/folders/… : comparer les chemins résolus évite un
// faux échec.
func realDir(t *testing.T, dir string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
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
	rec := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true,"data":{"uuid":"new"}}`))
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
	if err := json.Unmarshal([]byte(rec.bodies[0]), &body); err != nil {
		t.Fatalf("corps %s : %v", rec.bodies[0], err)
	}
	if body["title"] != "Nouvelle tâche" {
		t.Errorf("corps = %s", rec.bodies[0])
	}
}

func TestCallQueryMapping(t *testing.T) {
	rec := newRecorder(t, nil)
	call := genericTool(t, rec, t.TempDir(), "xalantis_read_operation")
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
	read := genericTool(t, rec, dir, "xalantis_read_operation")
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil { //nolint:gosec // G306: fichier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}

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
		tool := call
		if strings.HasPrefix(c["operation_id"].(string), "get_") {
			tool = read
		}
		if _, err := tool.Handler(c); err == nil {
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
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
	if err := os.WriteFile(a, []byte("A"), 0o644); err != nil { //nolint:gosec // G306: fichier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("B"), 0o644); err != nil { //nolint:gosec // G306: fichier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}
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
	return newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		if disposition != "" {
			w.Header().Set("Content-Disposition", disposition)
		}
		_, _ = w.Write([]byte("PDFDATA"))
	})
}

func download(t *testing.T, rec *recorder, dir string, extra map[string]any) map[string]any {
	t.Helper()
	call := genericTool(t, rec, dir, "xalantis_read_operation")
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
	data, _ := os.ReadFile(filepath.Join(dir, "rapport.pdf")) //nolint:gosec // G304: chemin de test dans t.TempDir(), non contrôlé par un attaquant
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
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // G301: dossier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}
	res := download(t, downloadServer(t, ""), dir, map[string]any{"save_to": target})
	want := filepath.Join(realDir(t, dir), "sous", "x.pdf")
	if res["saved_to"] != want {
		t.Errorf("save_to : %v, attendu %s", res["saved_to"], want)
	}
}

func TestCallEmptyAndTextResponses(t *testing.T) {
	rec := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte("a,b\n1,2\n"))
	})
	call := genericTool(t, rec, t.TempDir(), "xalantis_call_operation")
	read := genericTool(t, rec, t.TempDir(), "xalantis_read_operation")
	out, err := call.Handler(map[string]any{"operation_id": "delete_tickets_By_uuid", "path_params": map[string]any{"uuid": "t-1"}})
	if err != nil || out != "Succès (HTTP 204), réponse vide." {
		t.Errorf("DELETE : %v %q", err, out)
	}
	out, err = read.Handler(map[string]any{"operation_id": "get_projects_By_projectUuid_exports", "path_params": map[string]any{"projectUuid": "p"}, "query": map[string]any{"format": "csv"}})
	if err != nil || out != "a,b\n1,2\n" {
		t.Errorf("CSV : %v %q", err, out)
	}
}

func TestSaveToAppliesToText(t *testing.T) {
	rec := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n1,2\n"))
	})
	dir := t.TempDir()
	call := genericTool(t, rec, dir, "xalantis_read_operation")
	target := filepath.Join(dir, "export.csv")
	in := map[string]any{
		"operation_id": "get_projects_By_projectUuid_exports",
		"path_params":  map[string]any{"projectUuid": "p"},
		"query":        map[string]any{"format": "csv"},
		"save_to":      target,
	}

	out, err := call.Handler(in)
	if err != nil {
		t.Fatal(err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("sortie %s : %v", out, err)
	}
	wantFirst := filepath.Join(realDir(t, dir), "export.csv")
	if res["saved_to"] != wantFirst {
		t.Errorf("saved_to = %v, attendu %s", res["saved_to"], wantFirst)
	}
	data, err := os.ReadFile(target) //nolint:gosec // G304: chemin de test dans t.TempDir(), non contrôlé par un attaquant
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a,b\n1,2\n" {
		t.Errorf("contenu = %q", data)
	}

	out2, err := call.Handler(in)
	if err != nil {
		t.Fatal(err)
	}
	var res2 map[string]any
	if err := json.Unmarshal([]byte(out2), &res2); err != nil {
		t.Fatalf("sortie %s : %v", out2, err)
	}
	wantSecond := filepath.Join(realDir(t, dir), "export (1).csv")
	if res2["saved_to"] != wantSecond {
		t.Errorf("saved_to (2e appel) = %v, attendu %s", res2["saved_to"], wantSecond)
	}
}

// uploadCall appelle post_projects_By_projectUuid_documents avec le champ
// fichier "files" pointant sur path, dans le dossier dir.
func uploadCall(t *testing.T, rec *recorder, dir, path string) error {
	t.Helper()
	call := genericTool(t, rec, dir, "xalantis_call_operation")
	_, err := call.Handler(map[string]any{
		"operation_id": "post_projects_By_projectUuid_documents",
		"path_params":  map[string]any{"projectUuid": "p"},
		"headers":      map[string]any{"Idempotency-Key": "k"},
		"files":        map[string]any{"files": path},
	})
	return err
}

// assertOutsideFolder vérifie que err rejette un chemin hors du dossier
// autorisé, avec le message attendu (et non un message de type « fichier
// introuvable », qui révélerait l'existence du chemin visé).
func assertOutsideFolder(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "hors du dossier autorisé") {
		t.Fatalf("attendu « hors du dossier autorisé », obtenu : %v", err)
	}
}

func TestCallUploadOutsideFolderRejected(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil { //nolint:gosec // G306: fichier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}
	assertOutsideFolder(t, uploadCall(t, rec, dir, outside))
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

func TestCallUploadOutsideFolderNonexistentHidesExistence(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "does-not-exist.txt")
	assertOutsideFolder(t, uploadCall(t, rec, dir, outside))
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

func TestCallUploadParentTraversalRejected(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "evil.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil { //nolint:gosec // G306: fichier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(outside) //nolint:gosec // G104: nettoyage de test, échec sans conséquence
	})
	assertOutsideFolder(t, uploadCall(t, rec, dir, "../evil.txt"))
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

func TestCallUploadSymlinkEscapeRejected(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	outsideDir := t.TempDir()
	target := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(target, []byte("s"), 0o644); err != nil { //nolint:gosec // G306: fichier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	assertOutsideFolder(t, uploadCall(t, rec, dir, link))
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

func TestCallUploadRelativePathAccepted(t *testing.T) {
	rec := newRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart: %v", err)
		}
		if n := len(r.MultipartForm.File["files[]"]); n != 1 {
			t.Errorf("%d fichiers sous files[]", n)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.pdf"), []byte("A"), 0o644); err != nil { //nolint:gosec // G306: fichier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}
	if err := uploadCall(t, rec, dir, "a.pdf"); err != nil {
		t.Fatal(err)
	}
	if len(rec.requests) != 1 {
		t.Fatalf("%d appels", len(rec.requests))
	}
}

func TestSaveToOutsideFolderRejected(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "export.csv")
	call := genericTool(t, rec, dir, "xalantis_read_operation")
	_, err := call.Handler(map[string]any{
		"operation_id": "get_projects_By_projectUuid_exports",
		"path_params":  map[string]any{"projectUuid": "p"},
		"query":        map[string]any{"format": "csv"},
		"save_to":      outside,
	})
	assertOutsideFolder(t, err)
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

func TestSaveToOutsideFolderNonexistentParentHidesExistence(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	outsideParent := filepath.Join(t.TempDir(), "does-not-exist-dir")
	call := genericTool(t, rec, dir, "xalantis_read_operation")
	_, err := call.Handler(map[string]any{
		"operation_id": "get_projects_By_projectUuid_exports",
		"path_params":  map[string]any{"projectUuid": "p"},
		"query":        map[string]any{"format": "csv"},
		"save_to":      filepath.Join(outsideParent, "export.csv"),
	})
	assertOutsideFolder(t, err)
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

func TestSaveToSymlinkParentEscapeRejected(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	outsideDir := t.TempDir()
	link := filepath.Join(dir, "out")
	if err := os.Symlink(outsideDir, link); err != nil {
		t.Fatal(err)
	}
	call := genericTool(t, rec, dir, "xalantis_read_operation")
	_, err := call.Handler(map[string]any{
		"operation_id": "get_projects_By_projectUuid_exports",
		"path_params":  map[string]any{"projectUuid": "p"},
		"query":        map[string]any{"format": "csv"},
		"save_to":      filepath.Join(link, "export.csv"),
	})
	assertOutsideFolder(t, err)
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

func TestSaveToRelativeAccepted(t *testing.T) {
	rec := newRecorder(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("a,b\n1,2\n"))
	})
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sous"), 0o755); err != nil { //nolint:gosec // G301: dossier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}
	call := genericTool(t, rec, dir, "xalantis_read_operation")
	out, err := call.Handler(map[string]any{
		"operation_id": "get_projects_By_projectUuid_exports",
		"path_params":  map[string]any{"projectUuid": "p"},
		"query":        map[string]any{"format": "csv"},
		"save_to":      "sous/x.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("sortie %s : %v", out, err)
	}
	want := filepath.Join(realDir(t, filepath.Join(dir, "sous")), "x.csv")
	if res["saved_to"] != want {
		t.Errorf("saved_to = %v, attendu %s", res["saved_to"], want)
	}
	data, err := os.ReadFile(want) //nolint:gosec // G304: chemin de test dans t.TempDir(), non contrôlé par un attaquant
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a,b\n1,2\n" {
		t.Errorf("contenu = %q", data)
	}
}

func TestSaveToMissingParentDir(t *testing.T) {
	rec := newRecorder(t, nil)
	dir := t.TempDir()
	call := genericTool(t, rec, dir, "xalantis_read_operation")
	_, err := call.Handler(map[string]any{
		"operation_id": "get_projects_By_projectUuid_exports",
		"path_params":  map[string]any{"projectUuid": "p"},
		"query":        map[string]any{"format": "csv"},
		"save_to":      "absent/x.csv",
	})
	if err == nil || !strings.Contains(err.Error(), "introuvable") {
		t.Fatalf("attendu « introuvable », obtenu : %v", err)
	}
	if len(rec.requests) != 0 {
		t.Errorf("%d appels API, attendu 0", len(rec.requests))
	}
}

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
		Count      int               `json:"count"`
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
