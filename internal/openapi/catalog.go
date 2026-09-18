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
// contiennent tous les mots de query, filtrées par domaine et méthode, ainsi
// que le nombre total de correspondances. Au plus MaxResults sont renvoyées :
// sans le total, une liste coupée est indiscernable d'une liste complète et
// le modèle choisit dans un sous-ensemble en croyant avoir tout vu.
func (c *Catalog) Search(query, area, method string) ([]Summary, int) {
	words := strings.Fields(fold(query))
	out, total := []Summary{}, 0
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
		total++
		if len(out) < MaxResults {
			out = append(out, Summary{OperationID: op.ID, Method: op.Method, Path: op.Path, Summary: op.Summary, Scope: op.Scope})
		}
	}
	return out, total
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
