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

	esc := c.Search("", "politiques d'escalade", "")
	if len(esc) != 3 {
		t.Errorf("escalade (apostrophe droite) = %d, attendu 3", len(esc))
	}
	for _, r := range esc {
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
