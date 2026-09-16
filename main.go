// xalantis-projects-mcp — Serveur MCP (stdio) en lecture seule pour le module
// « Projets et tâches » de l'API Xalantis (api/v1).
//
// Outils exposés (tous en GET, aucune écriture possible) :
//   - xalantis_list_projects   : liste des projets accessibles
//   - xalantis_list_tasks      : recherche/pagination des tâches d'un projet
//   - xalantis_get_task        : détail d'une tâche
//   - xalantis_task_activities : historique audité d'une tâche
//   - xalantis_list_sprints    : sprints d'un projet
//   - xalantis_list_statuses   : statuts canoniques des tâches d'un projet
//   - xalantis_list_members    : membres d'un projet
//
// Configuration par variables d'environnement :
//
//	XALANTIS_API_KEY  (obligatoire) — clé API tenant, scope requis : projects:read
//	XALANTIS_BASE_URL (optionnel)   — défaut : https://xalantis.com
//
// Compilation : go build -o xalantis-projects-mcp main.go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	serverName    = "xalantis-projects-mcp"
	serverVersion = "1.0.0"
	protocolVer   = "2025-06-18"
)

// ---------------------------------------------------------------- JSON-RPC

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------- Outils

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func prop(t, desc string) map[string]any {
	return map[string]any{"type": t, "description": desc}
}

func arrProp(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

func schema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var pagingProps = map[string]any{
	"page":     prop("integer", "Numéro de page (défaut 1)."),
	"per_page": prop("integer", "Résultats par page, 1 à 100."),
}

func tools() []toolDef {
	merge := func(ms ...map[string]any) map[string]any {
		out := map[string]any{}
		for _, m := range ms {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}
	projectUUID := map[string]any{"project_uuid": prop("string", "UUID du projet.")}
	taskUUID := map[string]any{"task_uuid": prop("string", "UUID de la tâche.")}
	return []toolDef{
		{
			Name:        "xalantis_list_projects",
			Description: "Liste les projets Xalantis accessibles (lecture seule). Filtres : recherche, statut, projets archivés.",
			InputSchema: schema(nil, merge(pagingProps, map[string]any{
				"search":           prop("string", "Filtre sur la clé, le nom ou la description."),
				"status":           prop("string", "Filtre exact par statut du projet."),
				"include_archived": prop("boolean", "Inclut les projets archivés."),
			})),
		},
		{
			Name:        "xalantis_list_tasks",
			Description: "Recherche et pagine les tâches d'un projet Xalantis (lecture seule). Filtres : texte, statuts, assignés, priorités, sprint, epic, jalon, échéance (due_state), archivées.",
			InputSchema: schema([]string{"project_uuid"}, merge(pagingProps, projectUUID, map[string]any{
				"search":           prop("string", "Recherche textuelle (255 caractères max), ex. une référence de ticket."),
				"statuses":         arrProp("UUID de statuts (jusqu'à 50)."),
				"assignees":        arrProp("UUID de membres (jusqu'à 50)."),
				"labels":           arrProp("UUID de labels (jusqu'à 50)."),
				"priorities":       arrProp("Priorités de tâche."),
				"types":            arrProp("Types de tâche."),
				"epic_id":          prop("string", "UUID public de l'Epic."),
				"sprint":           prop("string", "UUID public du sprint."),
				"milestone":        prop("string", "UUID public du jalon."),
				"due_state":        prop("string", "all, overdue, due_soon, no_due ou has_due."),
				"include_archived": prop("boolean", "Inclut les tâches archivées."),
				"sort_by":          prop("string", "Axe de tri autorisé par l'API."),
				"sort_direction":   prop("string", "asc ou desc."),
			})),
		},
		{
			Name:        "xalantis_get_task",
			Description: "Détail relationnel d'une tâche Xalantis par UUID (lecture seule).",
			InputSchema: schema([]string{"project_uuid", "task_uuid"}, merge(projectUUID, taskUUID)),
		},
		{
			Name:        "xalantis_task_activities",
			Description: "Historique audité d'une tâche Xalantis (changements de statut, d'assigné, etc.), paginé (lecture seule).",
			InputSchema: schema([]string{"project_uuid", "task_uuid"}, merge(pagingProps, projectUUID, taskUUID)),
		},
		{
			Name:        "xalantis_list_sprints",
			Description: "Liste les sprints d'un projet Xalantis, du plus récent au plus ancien (lecture seule).",
			InputSchema: schema([]string{"project_uuid"}, merge(pagingProps, projectUUID)),
		},
		{
			Name:        "xalantis_list_statuses",
			Description: "Liste les statuts canoniques des tâches d'un projet Xalantis (UUID + libellé), utiles pour filtrer list_tasks (lecture seule).",
			InputSchema: schema([]string{"project_uuid"}, projectUUID),
		},
		{
			Name:        "xalantis_list_members",
			Description: "Liste les membres d'un projet Xalantis (UUID + nom), utiles pour filtrer list_tasks par assigné (lecture seule).",
			InputSchema: schema([]string{"project_uuid"}, merge(pagingProps, projectUUID)),
		},
	}
}

// ---------------------------------------------------------------- Client API

var httpClient = &http.Client{Timeout: 30 * time.Second}

func apiGet(path string, query url.Values) (string, error) {
	base := strings.TrimRight(os.Getenv("XALANTIS_BASE_URL"), "/")
	if base == "" {
		base = "https://xalantis.com"
	}
	key := os.Getenv("XALANTIS_API_KEY")
	if key == "" {
		return "", fmt.Errorf("XALANTIS_API_KEY manquante : ajoutez-la dans la configuration du serveur MCP")
	}
	u := base + "/api/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", serverName+"/"+serverVersion)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("appel API impossible : %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 Mo max
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("rate limit atteint (60 req/min) — réessayez dans %s s", resp.Header.Get("Retry-After"))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return "", fmt.Errorf("API Xalantis HTTP %d : %s", resp.StatusCode, msg)
	}
	return string(body), nil
}

// ---------------------------------------------------------------- Arguments

type args map[string]any

func (a args) str(k string) string {
	if v, ok := a[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
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

func requireUUID(a args, key string) (string, error) {
	v := a.str(key)
	if v == "" {
		return "", fmt.Errorf("argument requis manquant : %s", key)
	}
	if strings.ContainsAny(v, "/?#&%") || v == "." || v == ".." {
		return "", fmt.Errorf("argument %s invalide", key)
	}
	return v, nil
}

// ---------------------------------------------------------------- Dispatch

func callTool(name string, a args) (string, error) {
	q := url.Values{}
	switch name {
	case "xalantis_list_projects":
		a.paging(q)
		a.addStr(q, "search", "search")
		a.addStr(q, "status", "status")
		a.addBool(q, "include_archived", "include_archived")
		return apiGet("/projects", q)

	case "xalantis_list_tasks":
		p, err := requireUUID(a, "project_uuid")
		if err != nil {
			return "", err
		}
		a.paging(q)
		a.addStr(q, "search", "search")
		a.addArr(q, "statuses", "status[]")
		a.addArr(q, "assignees", "assignees[]")
		a.addArr(q, "labels", "labels[]")
		a.addArr(q, "priorities", "priorities[]")
		a.addArr(q, "types", "type[]")
		a.addStr(q, "epic_id", "epic_id")
		a.addStr(q, "sprint", "sprint")
		a.addStr(q, "milestone", "milestone")
		a.addStr(q, "due_state", "due_state")
		a.addBool(q, "include_archived", "include_archived")
		a.addStr(q, "sort_by", "sort_by")
		a.addStr(q, "sort_direction", "sort_direction")
		return apiGet("/projects/"+p+"/tasks", q)

	case "xalantis_get_task":
		p, err := requireUUID(a, "project_uuid")
		if err != nil {
			return "", err
		}
		t, err := requireUUID(a, "task_uuid")
		if err != nil {
			return "", err
		}
		return apiGet("/projects/"+p+"/tasks/"+t, nil)

	case "xalantis_task_activities":
		p, err := requireUUID(a, "project_uuid")
		if err != nil {
			return "", err
		}
		t, err := requireUUID(a, "task_uuid")
		if err != nil {
			return "", err
		}
		a.paging(q)
		return apiGet("/projects/"+p+"/tasks/"+t+"/activities", q)

	case "xalantis_list_sprints":
		p, err := requireUUID(a, "project_uuid")
		if err != nil {
			return "", err
		}
		a.paging(q)
		return apiGet("/projects/"+p+"/sprints", q)

	case "xalantis_list_statuses":
		p, err := requireUUID(a, "project_uuid")
		if err != nil {
			return "", err
		}
		return apiGet("/projects/"+p+"/statuses", nil)

	case "xalantis_list_members":
		p, err := requireUUID(a, "project_uuid")
		if err != nil {
			return "", err
		}
		a.paging(q)
		return apiGet("/projects/"+p+"/members", q)
	}
	return "", fmt.Errorf("outil inconnu : %s", name)
}

// ---------------------------------------------------------------- Boucle MCP

func textResult(text string, isErr bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 16<<20)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	send := func(resp rpcResponse) {
		resp.JSONRPC = "2.0"
		_ = enc.Encode(resp)
		_ = out.Flush()
	}

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue // ligne illisible : on ignore
		}
		isNotification := len(req.ID) == 0 || string(req.ID) == "null"

		switch req.Method {
		case "initialize":
			send(rpcResponse{ID: req.ID, Result: map[string]any{
				"protocolVersion": protocolVer,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
			}})

		case "notifications/initialized", "notifications/cancelled", "notifications/roots/list_changed":
			// rien à faire

		case "ping":
			if !isNotification {
				send(rpcResponse{ID: req.ID, Result: map[string]any{}})
			}

		case "tools/list":
			if isNotification {
				continue
			}
			send(rpcResponse{ID: req.ID, Result: map[string]any{"tools": tools()}})

		case "tools/call":
			if isNotification {
				continue
			}
			var params struct {
				Name      string `json:"name"`
				Arguments args   `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				send(rpcResponse{ID: req.ID, Error: &rpcError{Code: -32602, Message: "paramètres invalides"}})
				continue
			}
			if params.Arguments == nil {
				params.Arguments = args{}
			}
			body, err := callTool(params.Name, params.Arguments)
			if err != nil {
				send(rpcResponse{ID: req.ID, Result: textResult("Erreur : "+err.Error(), true)})
				continue
			}
			send(rpcResponse{ID: req.ID, Result: textResult(body, false)})

		default:
			if !isNotification {
				send(rpcResponse{ID: req.ID, Error: &rpcError{Code: -32601, Message: "méthode non supportée : " + req.Method}})
			}
		}
	}
}
