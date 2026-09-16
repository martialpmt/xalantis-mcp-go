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
