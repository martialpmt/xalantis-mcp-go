# xalantis-projects-mcp

[![CI](https://github.com/martialpmt/xalantis-mcp-go/actions/workflows/ci.yml/badge.svg)](https://github.com/martialpmt/xalantis-mcp-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/martialpmt/xalantis-mcp-go.svg)](https://pkg.go.dev/github.com/martialpmt/xalantis-mcp-go)

Serveur MCP local (stdio) pour l'API Xalantis. Il couvre 206 opérations :
projets et tâches, service desk (tickets, automatisations, catégories, tags,
SLA, escalades, réponses prédéfinies, disponibilité des agents) et catalogue
de services. Il complète le connecteur Xalantis existant.

> **Attention : écriture possible.** Le serveur expose aussi les opérations
> de création, de modification et de suppression. Ce que Claude peut faire
> dépend uniquement des scopes de la clé API. Pour un usage en lecture seule,
> utilisez une clé qui n'a que des scopes `:read`.
>
> **Fichiers locaux.** Le serveur ne lit et n'écrit des fichiers que dans un
> seul dossier : `XALANTIS_FILES_DIR` (défaut `~/Downloads/xalantis`, créé
> au démarrage). Pour envoyer un fichier, copiez-le d'abord dans ce dossier.
> Les liens symboliques qui sortent du dossier sont refusés.

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

- **Envoi** : `files` est un objet qui associe le nom du champ fichier
  (indiqué par `xalantis_describe_operation`, dans `body.file_fields`) à un
  chemin ou à une liste de chemins, qui doivent rester dans le dossier
  autorisé (`XALANTIS_FILES_DIR`) ; un chemin relatif est relatif à ce
  dossier. Par ex. pour les documents d'un projet :
  `{"files": {"files": ["cr.pdf"]}}` ; pour un import :
  `{"files": {"file": "taches.csv"}}`.
- **Réception** : un fichier reçu est enregistré dans ce même dossier par
  défaut (ou à `save_to`, qui doit aussi y rester). Un fichier existant
  n'est jamais écrasé : ` (1)`, ` (2)`… sont ajoutés au nom. L'outil renvoie
  le chemin, la taille et le type.
- Les réponses JSON ou texte (dont CSV) sont renvoyées directement.

## Prérequis

- Une clé API Xalantis avec les scopes voulus (Xalantis → paramètres de la clé API).
- macOS ou Linux (amd64 ou arm64) pour le script d'installation ; Windows via
  l'archive `.zip` des releases.

## Installation

### Script (macOS, Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/martialpmt/xalantis-mcp-go/main/install.sh | sh
```

Le script télécharge la release depuis GitHub, vérifie sa somme de contrôle
(SHA-256) et installe le binaire dans `~/.local/bin`, sans `sudo`.
Variables facultatives :

- `VERSION` : version à installer (défaut `latest`), par ex.
  `curl -fsSL … | VERSION=v0.1.0 sh` ;
- `INSTALL_DIR` : dossier d'installation (défaut `~/.local/bin`).

Vérification : `xalantis-projects-mcp --version`.

### Téléchargement manuel (dont Windows)

Sur la page [Releases](https://github.com/martialpmt/xalantis-mcp-go/releases),
télécharger l'archive du système (`xalantis-projects-mcp_<os>_<arch>.tar.gz`,
`.zip` pour Windows) et `checksums.txt`, puis :

```bash
shasum -a 256 -c checksums.txt --ignore-missing
tar -xzf xalantis-projects-mcp_darwin_arm64.tar.gz xalantis-projects-mcp
```

Placer le binaire dans un dossier du `PATH`. Sur macOS, un fichier téléchargé
avec le navigateur est mis en quarantaine :
`xattr -d com.apple.quarantine xalantis-projects-mcp`.

### Avec Go (≥ 1.24)

```bash
go install github.com/martialpmt/xalantis-mcp-go/cmd/xalantis-projects-mcp@latest
```

Le binaire est installé dans `$(go env GOPATH)/bin`.

## Configuration (Claude Desktop)

1. Claude Desktop → Réglages → Développeur → Éditer la config, ajouter dans
   `mcpServers` le chemin **absolu** du binaire (`~` n'est pas interprété) :
   ```json
   {
     "mcpServers": {
       "xalantis-projets": {
         "command": "/Users/VOTRE_LOGIN/.local/bin/xalantis-projects-mcp",
         "env": {
           "XALANTIS_API_KEY": "sk_live_VOTRE_CLE"
         }
       }
     }
   }
   ```
   (Si le fichier contient déjà le connecteur `xalantis`, ajouter simplement l'entrée
   `xalantis-projets` à côté, dans le même bloc `mcpServers`.)
2. Quitter complètement Claude Desktop et le rouvrir.

## Variables d'environnement

- `XALANTIS_API_KEY` (obligatoire) — clé API tenant `sk_live_…`.
  Elle ne quitte jamais votre machine : le serveur tourne en local et parle
  directement à `xalantis.com`.
- `XALANTIS_BASE_URL` (optionnel) — défaut `https://xalantis.com` ; à modifier
  uniquement si votre instance Xalantis est sur un autre domaine.
- `XALANTIS_FILES_DIR` (optionnel) — dossier autorisé pour tous les fichiers
  locaux, envoyés ou reçus ; défaut `~/Downloads/xalantis`, créé au
  démarrage. Un chemin relatif dans `files` ou `save_to` est relatif à ce
  dossier ; tout chemin qui en sort (y compris via un lien symbolique) est
  refusé.

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
- Vérifications locales (comme la CI) : `gofmt -l .`, `golangci-lint run`,
  `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`,
  `sh .github/scripts/next-version_test.sh`
- Build de release local : `goreleaser release --snapshot --clean` (résultat dans `dist/`).
- Publier une version : GitHub → Actions → **Release** → *Run workflow* sur
  `main`, choisir `patch`, `minor` ou `major`. Le workflow relance la CI, crée
  le tag `vX.Y.Z`, publie les binaires dans les Releases et enregistre le
  module sur pkg.go.dev. Les versions restent en `v0.x`/`v1.x` (une `v2`
  exigerait le suffixe `/v2` dans le chemin du module).

## Notes

- Rate limit API : 60 requêtes/minute par clé — le serveur remonte l'erreur avec le délai à attendre.
- Réponses limitées à 100 Mo (fichiers) et 10 Mo (texte renvoyé à Claude).
