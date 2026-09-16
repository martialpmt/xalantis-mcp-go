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
