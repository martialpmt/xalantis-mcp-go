package mcp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
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
	s := NewServer("srv", "9.9.9", "fais ceci", 0)
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
	s := NewServer("srv", "1", "", 0)
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
	if len(got) != 2 {
		t.Fatalf("deux réponses attendues (erreur d'analyse + méthode inconnue), obtenu %v", got)
	}
	if code := got["null"]["error"].(map[string]any)["code"].(float64); code != -32700 {
		t.Fatalf("JSON illisible: %v", got["null"])
	}
	if code := got[`"a"`]["error"].(map[string]any)["code"].(float64); code != -32601 {
		t.Fatalf("méthode inconnue: %v", got)
	}
	bad := run(t, s, `["tableau"]`)
	if code := bad["null"]["error"].(map[string]any)["code"].(float64); code != -32600 {
		t.Fatalf("JSON valide mais pas une requête: %v", bad)
	}
	init := run(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if _, ok := init["1"]["result"].(map[string]any)["instructions"]; ok {
		t.Fatal("instructions vides : champ absent attendu")
	}
}

func TestToolsListAnnotations(t *testing.T) {
	s := NewServer("srv", "1", "", 0)
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

// TestCallsRunConcurrently : « rapide » doit pouvoir s'exécuter pendant que
// « lent » est bloqué. En traitement séquentiel, « lent » attend une libération
// qui ne viendra qu'après lui : il expire et le test échoue au lieu de boucler.
func TestCallsRunConcurrently(t *testing.T) {
	s := NewServer("srv", "1", "", 0)
	started, release := make(chan struct{}), make(chan struct{})
	s.Register(
		Tool{Name: "lent", InputSchema: map[string]any{"type": "object"}, Handler: func(map[string]any) (string, error) {
			close(started)
			select {
			case <-release:
				return "lent fini", nil
			case <-time.After(5 * time.Second):
				return "", errors.New("traitement séquentiel : « lent » bloque les appels suivants")
			}
		}},
		Tool{Name: "rapide", InputSchema: map[string]any{"type": "object"}, Handler: func(map[string]any) (string, error) {
			<-started // « lent » a forcément démarré : en séquentiel il a déjà rendu la main
			close(release)
			return "rapide fini", nil
		}},
	)
	got := run(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lent"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"rapide"}}`,
	)
	for id, want := range map[string]string{"1": "lent fini", "2": "rapide fini"} {
		r := got[id]["result"].(map[string]any)
		if s := r["content"].([]any)[0].(map[string]any)["text"].(string); s != want {
			t.Errorf("id %s : %q, attendu %q", id, s, want)
		}
	}
}

// TestOutputSizeLimit : le plafond s'applique à tout outil, y compris ceux
// qui ne passent par aucune réponse HTTP. Refus et non troncature : un JSON
// coupé serait invalide.
func TestOutputSizeLimit(t *testing.T) {
	s := NewServer("srv", "1", "", 100)
	s.Register(
		Tool{Name: "bavard", InputSchema: map[string]any{"type": "object"}, Handler: func(map[string]any) (string, error) {
			return strings.Repeat("x", 101), nil
		}},
		Tool{Name: "concis", InputSchema: map[string]any{"type": "object"}, Handler: func(map[string]any) (string, error) {
			return strings.Repeat("x", 100), nil
		}},
	)
	got := run(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"bavard"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"concis"}}`,
	)
	text := func(id string) (string, bool) {
		r := got[id]["result"].(map[string]any)
		return r["content"].([]any)[0].(map[string]any)["text"].(string), r["isError"].(bool)
	}
	out, isErr := text("1")
	if !isErr || !strings.Contains(out, "bavard") || !strings.Contains(out, "trop volumineuse") {
		t.Errorf("dépassement : %q %v", out, isErr)
	}
	if len(out) > 200 {
		t.Errorf("l'erreur ne doit pas reprendre la sortie : %d octets", len(out))
	}
	if out, isErr := text("2"); isErr || len(out) != 100 {
		t.Errorf("pile à la limite : %d octets, isError=%v", len(out), isErr)
	}
}
