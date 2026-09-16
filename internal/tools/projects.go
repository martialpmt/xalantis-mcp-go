package tools

import (
	"net/url"

	"paymetrust/xalantis-projects-mcp/internal/mcp"
	"paymetrust/xalantis-projects-mcp/internal/xalantis"
)

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

func merge(ms ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

var pagingProps = map[string]any{
	"page":     prop("integer", "Numéro de page (défaut 1)."),
	"per_page": prop("integer", "Résultats par page, 1 à 100."),
}

// ProjectTools renvoie les 7 outils de lecture des projets.
func ProjectTools(c *xalantis.Client) []mcp.Tool {
	projectUUID := map[string]any{"project_uuid": prop("string", "UUID du projet.")}
	taskUUID := map[string]any{"task_uuid": prop("string", "UUID de la tâche.")}
	return []mcp.Tool{
		{
			Name:        "xalantis_list_projects",
			Description: "Liste les projets Xalantis accessibles (lecture seule). Filtres : recherche, statut, projets archivés.",
			InputSchema: schema(nil, merge(pagingProps, map[string]any{
				"search":           prop("string", "Filtre sur la clé, le nom ou la description."),
				"status":           prop("string", "Filtre exact par statut du projet."),
				"include_archived": prop("boolean", "Inclut les projets archivés."),
			})),
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
				a.paging(q)
				a.addStr(q, "search", "search")
				a.addStr(q, "status", "status")
				a.addBool(q, "include_archived", "include_archived")
				return c.Get("/projects", q)
			},
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
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
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
				return c.Get("/projects/"+p+"/tasks", q)
			},
		},
		{
			Name:        "xalantis_get_task",
			Description: "Détail relationnel d'une tâche Xalantis par UUID (lecture seule).",
			InputSchema: schema([]string{"project_uuid", "task_uuid"}, merge(projectUUID, taskUUID)),
			Handler: func(raw map[string]any) (string, error) {
				a := args(raw)
				p, err := requireUUID(a, "project_uuid")
				if err != nil {
					return "", err
				}
				t, err := requireUUID(a, "task_uuid")
				if err != nil {
					return "", err
				}
				return c.Get("/projects/"+p+"/tasks/"+t, nil)
			},
		},
		{
			Name:        "xalantis_task_activities",
			Description: "Historique audité d'une tâche Xalantis (changements de statut, d'assigné, etc.), paginé (lecture seule).",
			InputSchema: schema([]string{"project_uuid", "task_uuid"}, merge(pagingProps, projectUUID, taskUUID)),
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
				p, err := requireUUID(a, "project_uuid")
				if err != nil {
					return "", err
				}
				t, err := requireUUID(a, "task_uuid")
				if err != nil {
					return "", err
				}
				a.paging(q)
				return c.Get("/projects/"+p+"/tasks/"+t+"/activities", q)
			},
		},
		{
			Name:        "xalantis_list_sprints",
			Description: "Liste les sprints d'un projet Xalantis, du plus récent au plus ancien (lecture seule).",
			InputSchema: schema([]string{"project_uuid"}, merge(pagingProps, projectUUID)),
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
				p, err := requireUUID(a, "project_uuid")
				if err != nil {
					return "", err
				}
				a.paging(q)
				return c.Get("/projects/"+p+"/sprints", q)
			},
		},
		{
			Name:        "xalantis_list_statuses",
			Description: "Liste les statuts canoniques des tâches d'un projet Xalantis (UUID + libellé), utiles pour filtrer list_tasks (lecture seule).",
			InputSchema: schema([]string{"project_uuid"}, projectUUID),
			Handler: func(raw map[string]any) (string, error) {
				p, err := requireUUID(args(raw), "project_uuid")
				if err != nil {
					return "", err
				}
				return c.Get("/projects/"+p+"/statuses", nil)
			},
		},
		{
			Name:        "xalantis_list_members",
			Description: "Liste les membres d'un projet Xalantis (UUID + nom), utiles pour filtrer list_tasks par assigné (lecture seule).",
			InputSchema: schema([]string{"project_uuid"}, merge(pagingProps, projectUUID)),
			Handler: func(raw map[string]any) (string, error) {
				a, q := args(raw), url.Values{}
				p, err := requireUUID(a, "project_uuid")
				if err != nil {
					return "", err
				}
				a.paging(q)
				return c.Get("/projects/"+p+"/members", q)
			},
		},
	}
}
