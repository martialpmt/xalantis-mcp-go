# xalantis-projects-mcp

Serveur MCP local (stdio) pour l'API Xalantis. Il couvre 206 opérations :
projets et tâches, service desk (tickets, automatisations, catégories, tags,
SLA, escalades, réponses prédéfinies, disponibilité des agents) et catalogue
de services. Il complète le connecteur Xalantis existant.

> **Attention : écriture possible.** Le serveur expose aussi les opérations
> de création, de modification et de suppression. Ce que Claude peut faire
> dépend uniquement des scopes de la clé API. Pour un usage en lecture seule,
> utilisez une clé qui n'a que des scopes `:read`.

## Outils exposés (10)

Outils dédiés aux lectures courantes de projets (inchangés) :

| Outil | Rôle |
|---|---|
| `xalantis_list_projects` | Liste des projets accessibles |
| `xalantis_list_tasks` | Recherche/pagination des tâches d'un projet (statuts, assignés, sprint, due_state…) |
| `xalantis_get_task` | Détail d'une tâche |
| `xalantis_task_activities` | Historique audité d'une tâche |
| `xalantis_list_sprints` | Sprints d'un projet |
| `xalantis_list_statuses` | Statuts canoniques (UUID + libellé) |
| `xalantis_list_members` | Membres d'un projet (UUID + nom) |

Outils génériques, pour toutes les autres opérations, à utiliser dans cet ordre :

| Outil | Rôle |
|---|---|
| `xalantis_search_operations` | 1. Trouver une opération (mots-clés, domaine, méthode). Sans filtre : liste des domaines. |
| `xalantis_describe_operation` | 2. Voir ses paramètres et le schéma de son corps. |
| `xalantis_call_operation` | 3. L'exécuter (`path_params`, `query`, `headers`, `body`, `files`, `save_to`). |

Les opérations d'écriture exigent souvent l'en-tête `Idempotency-Key`
(valeur unique par mutation, par ex. un UUID) : passez-le dans `headers`.

## Domaines couverts et scopes

| Domaine | Opérations | Scopes |
|---|---|---|
| Projets et tâches | 108 | `projects:read`, `projects:write`, `projects:delete`, `projects:export`, `projects:import`, `projects:track_time`, `projects:manage_assignees`, `projects:manage_automations`, `projects:manage_configuration`, `projects:manage_documents`, `projects:manage_integrations`, `projects:manage_members`, `projects:manage_portal` |
| Tickets | 57 | `tickets:read`, `tickets:write` |
| Automatisations de tickets | 8 | `tickets:read`, `tickets:manage_automations` |
| Catégories de tickets | 4 | `tickets:read`, `tickets:manage_categories` |
| Tags de tickets | 5 | `tickets:read`, `tickets:write`, `tickets:manage_categories` |
| Politiques SLA | 4 | `tickets:read`, `tickets:manage_sla` |
| Violations SLA | 2 | `tickets:read`, `tickets:write` |
| Politiques d’escalade | 3 | `tickets:read`, `tickets:manage_escalations` |
| Réponses prédéfinies | 4 | `tickets:read`, `tickets:write` |
| Disponibilité des agents | 2 | `tickets:read`, `tickets:write` |
| Catalogue de services | 3 | `tickets:read`, `tickets:write` |
| Administration du catalogue | 6 | `tickets:read`, `tickets:write` |

Le scope exact de chaque opération est indiqué par `xalantis_search_operations`
et `xalantis_describe_operation`. Sans le scope requis, l'API répond 403 et
l'outil renvoie l'erreur.

## Fichiers

- **Envoi** : `files` associe un champ à un chemin local (ou une liste de
  chemins), par ex. `{"files": ["/Users/moi/Documents/cr.pdf"]}`.
- **Réception** : un fichier reçu est enregistré dans `~/Downloads` (ou à
  `save_to`). Un fichier existant n'est jamais écrasé : ` (1)`, ` (2)`… sont
  ajoutés au nom. L'outil renvoie le chemin, la taille et le type.
- Les réponses JSON ou texte (dont CSV) sont renvoyées directement.

## Prérequis

- Une clé API Xalantis avec les scopes voulus (Xalantis → paramètres de la clé API).
- macOS Apple Silicon pour le binaire fourni (`xalantis-projects-mcp`). Pour un
  autre système : `go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp`
  (Go ≥ 1.24, aucune dépendance).

## Installation (Claude Desktop, macOS)

1. Poser ce dossier quelque part, par ex. `~/Documents/xalantis-projects-mcp/`.
2. Rendre le binaire exécutable et lever la quarantaine macOS (fichier téléchargé) :
   ```bash
   chmod +x ~/Documents/xalantis-projects-mcp/xalantis-projects-mcp
   xattr -d com.apple.quarantine ~/Documents/xalantis-projects-mcp/xalantis-projects-mcp 2>/dev/null
   ```
3. Claude Desktop → Réglages → Développeur → Éditer la config, ajouter dans `mcpServers` :
   ```json
   {
     "mcpServers": {
       "xalantis-projets": {
         "command": "/Users/VOTRE_LOGIN/Documents/xalantis-projects-mcp/xalantis-projects-mcp",
         "env": {
           "XALANTIS_API_KEY": "sk_live_VOTRE_CLE"
         }
       }
     }
   }
   ```
   (Si le fichier contient déjà le connecteur `xalantis`, ajouter simplement l'entrée
   `xalantis-projets` à côté, dans le même bloc `mcpServers`.)
4. Quitter complètement Claude Desktop et le rouvrir.

## Variables d'environnement

- `XALANTIS_API_KEY` (obligatoire) — clé API tenant `sk_live_…`.
  Elle ne quitte jamais votre machine : le serveur tourne en local et parle
  directement à `xalantis.com`.
- `XALANTIS_BASE_URL` (optionnel) — défaut `https://xalantis.com` ; à modifier
  uniquement si votre instance Xalantis est sur un autre domaine.

## Développement

```
cmd/xalantis-projects-mcp/   point d'entrée : configuration et assemblage
internal/mcp/                boucle JSON-RPC stdio et registre d'outils
internal/xalantis/           client HTTP de l'API
internal/openapi/            spécification embarquée, recherche et description
internal/tools/              outils dédiés et génériques
```

- Mettre à jour l'API : remplacer `internal/openapi/xalantis-openapi.json`
  puis recompiler.
- Tests unitaires : `go test ./...`
- Test de bout en bout : `go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp && python3 test_mcp.py`

## Notes

- Rate limit API : 60 requêtes/minute par clé — le serveur remonte l'erreur avec le délai à attendre.
- Réponses limitées à 100 Mo (fichiers) et 10 Mo (texte renvoyé à Claude).