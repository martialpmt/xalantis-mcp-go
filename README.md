# xalantis-projects-mcp

Serveur MCP **en lecture seule** pour le module « Projets et tâches » de l'API Xalantis.
Complète le connecteur Xalantis existant (service desk) en donnant accès aux tâches
des projets (tickets IAP, CAP, BMA, MAP, RDP, FDS, AAS, TQA…).

## Outils exposés (7, tous en GET — aucune écriture possible)

| Outil | Rôle |
|---|---|
| `xalantis_list_projects` | Liste des projets accessibles |
| `xalantis_list_tasks` | Recherche/pagination des tâches d'un projet (statuts, assignés, sprint, due_state…) |
| `xalantis_get_task` | Détail d'une tâche |
| `xalantis_task_activities` | Historique audité d'une tâche (changements de statut, etc.) |
| `xalantis_list_sprints` | Sprints d'un projet |
| `xalantis_list_statuses` | Statuts canoniques (UUID + libellé) |
| `xalantis_list_members` | Membres d'un projet (UUID + nom) |

## Prérequis

- La clé API Xalantis doit avoir le scope **`projects:read`** (Xalantis → paramètres de la clé API).
- macOS Apple Silicon pour le binaire fourni (`xalantis-projects-mcp`). Pour un autre
  système : `go build -o xalantis-projects-mcp main.go` (Go ≥ 1.21, aucune dépendance).

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

- `XALANTIS_API_KEY` (obligatoire) — clé API tenant `sk_live_…`, scope `projects:read`.
  Elle ne quitte jamais votre machine : le serveur tourne en local et parle
  directement à `xalantis.com`.
- `XALANTIS_BASE_URL` (optionnel) — défaut `https://xalantis.com` ; à modifier
  uniquement si votre instance Xalantis est sur un autre domaine.

## Notes

- Rate limit API : 60 requêtes/minute par clé — le serveur remonte l'erreur avec le délai à attendre.
- Tests : `python3 test_mcp.py` (mock de l'API + dialogue JSON-RPC complet).
