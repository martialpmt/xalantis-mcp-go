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
