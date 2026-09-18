package xalantis

import (
	"bytes"
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
		_, _ = w.Write([]byte(`{"success":true}`))
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
	long := strings.Repeat("x", MaxErrorBytes+100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/limited":
			w.Header().Set("Retry-After", "42")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/api/v1/forbidden":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`  {"message":"scope manquant"}  `))
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(long))
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
	if err == nil || err.Error() != "API Xalantis HTTP 422 : "+long[:MaxErrorBytes] {
		t.Errorf("422 tronqué: %v", err)
	}
}

// retryServer répond 429 à la première requête (avec Retry-After), puis 200.
// Il mémorise les corps reçus.
func retryServer(t *testing.T, retryAfter string) (*httptest.Server, *[]string) {
	t.Helper()
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.Header().Set("Retry-After", retryAfter)
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

func post(c *Client) (*Response, error) {
	return c.Do(http.MethodPost, "/tickets", nil, nil, strings.NewReader(`{"x":1}`), "application/json")
}

func TestDoRetriesOnceAfterShortRateLimit(t *testing.T) {
	srv, bodies := retryServer(t, "0")
	resp, err := post(NewClient(Config{BaseURL: srv.URL, APIKey: "k"}))
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != `{"success":true}` {
		t.Errorf("réponse = %s", resp.Body)
	}
	if len(*bodies) != 2 || (*bodies)[1] != `{"x":1}` {
		t.Errorf("corps reçus = %q, attendu deux fois {\"x\":1}", *bodies)
	}
}

func TestDoDoesNotRetryLongRateLimit(t *testing.T) {
	srv, bodies := retryServer(t, "21")
	_, err := post(NewClient(Config{BaseURL: srv.URL, APIKey: "k"}))
	if err == nil || !strings.Contains(err.Error(), "réessayez dans 21 s") {
		t.Errorf("erreur = %v", err)
	}
	if len(*bodies) != 1 {
		t.Errorf("%d requêtes, attendu 1", len(*bodies))
	}
}

func TestMultipart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("contenu"), 0o644); err != nil { //nolint:gosec // G306: fichier de test dans t.TempDir(), permissions sans conséquence
		t.Fatal(err)
	}

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

// TestRetryGatewayAndDebug vérifie la seconde tentative sur 503 et la trace
// de diagnostic : deux appels, une seule erreur remontée, deux lignes tracées.
func TestRetryGatewayAndDebug(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	var log bytes.Buffer
	c := NewClient(Config{BaseURL: srv.URL, APIKey: "k", Debug: &log})
	body, err := c.Get("/flaky", nil)
	if err != nil || body != `{"ok":true}` {
		t.Fatalf("503 rejoué : body=%q err=%v", body, err)
	}
	if calls != 2 {
		t.Errorf("appels = %d, attendu 2", calls)
	}
	if got := strings.Count(log.String(), "[xalantis] GET "); got != 2 {
		t.Errorf("trace = %d lignes, attendu 2 :\n%s", got, log.String())
	}
	if strings.Contains(log.String(), "k") && strings.Contains(log.String(), "Bearer") {
		t.Errorf("la trace ne doit pas porter la clé : %s", log.String())
	}
}

// TestNoRetryOn500 : une erreur applicative n'est pas rejouée.
func TestNoRetryOn500(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := NewClient(Config{BaseURL: srv.URL, APIKey: "k"}).Get("/boum", nil); err == nil {
		t.Fatal("500 doit rester une erreur")
	}
	if calls != 1 {
		t.Errorf("appels = %d, attendu 1", calls)
	}
}
