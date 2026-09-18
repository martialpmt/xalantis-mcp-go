# xalantis-mcp-go

[![CI](https://github.com/martialpmt/xalantis-mcp-go/actions/workflows/ci.yml/badge.svg)](https://github.com/martialpmt/xalantis-mcp-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/martialpmt/xalantis-mcp-go.svg)](https://pkg.go.dev/github.com/martialpmt/xalantis-mcp-go)

Serveur MCP local (stdio) pour l'API Xalantis. Il couvre 206 opérations :
projets et tâches, service desk (tickets, automatisations, catégories, tags,
SLA, escalades, réponses prédéfinies, disponibilité des agents) et catalogue
de services. Il complète le connecteur Xalantis existant.

> **Attention : écriture possible.** Le serveur expose aussi les opérations
> de création, de modification et de suppression. Ce que Claude peut faire
> dépend des scopes de la clé API. Pour un usage en lecture seule, utilisez
> une clé qui n'a que des scopes `:read`, ou définissez
> `XALANTIS_READ_ONLY=1` : le serveur n'expose alors aucune écriture.
>
> **Fichiers locaux.** Le serveur ne lit et n'écrit des fichiers que dans un
> seul dossier : `XALANTIS_FILES_DIR` (défaut `~/Downloads/xalantis`, créé
> au démarrage). Pour envoyer un fichier, copiez-le d'abord dans ce dossier.
> Les liens symboliques qui sortent du dossier sont refusés.

## Outils exposés (11)

Outils dédiés aux lectures courantes de projets (lecture seule) :

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
| `xalantis_read_operation` | 3. Exécuter une lecture (GET) : `path_params`, `query`, `headers`, `save_to`. |
| `xalantis_call_operation` | 3. Exécuter une écriture (POST, PUT, PATCH, DELETE) : mêmes arguments, plus `body` et `files`. Absent avec `XALANTIS_READ_ONLY=1`. |

Les outils de lecture portent l'annotation MCP `readOnlyHint` et l'outil
d'écriture `destructiveHint` : le client peut approuver les lectures
automatiquement et demander confirmation avant une écriture.

L'en-tête `Idempotency-Key` des écritures est facultatif : s'il manque, le
serveur génère un UUID. Si l'appel échoue, l'erreur donne la clé générée ;
la repasser dans `headers` pour relancer la même requête sans créer de
doublon. Si le corps change, omettre la clé : la réutiliser avec un corps
différent est refusé (HTTP 409).

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
  `curl -fsSL … | VERSION=v0.2.0 sh` ;
- `INSTALL_DIR` : dossier d'installation (défaut `~/.local/bin`).

Vérification : `xalantis-mcp-go --version`.

> **Ancien nom.** Jusqu'à v0.1.0, le binaire et les archives s'appelaient
> `xalantis-projects-mcp` ; le script n'installe que les versions suivantes
> (v0.1.0 : téléchargement manuel). Après une mise à jour depuis v0.1.0,
> remplacez le chemin `command` dans la configuration de Claude Desktop et
> supprimez l'ancien binaire (`~/.local/bin/xalantis-projects-mcp`).

### Téléchargement manuel (dont Windows)

Sur la page [Releases](https://github.com/martialpmt/xalantis-mcp-go/releases),
télécharger l'archive du système (`xalantis-mcp-go_<os>_<arch>.tar.gz`,
`.zip` pour Windows) et `checksums.txt`, puis :

```bash
shasum -a 256 -c checksums.txt --ignore-missing
tar -xzf xalantis-mcp-go_darwin_arm64.tar.gz xalantis-mcp-go
```

Placer le binaire dans un dossier du `PATH`. Sur macOS, un fichier téléchargé
avec le navigateur est mis en quarantaine :
`xattr -d com.apple.quarantine xalantis-mcp-go`.

Sous Windows (PowerShell), comparer l'empreinte avec la ligne correspondante de `checksums.txt`, puis extraire :

```powershell
Get-FileHash .\xalantis-mcp-go_windows_amd64.zip -Algorithm SHA256
Expand-Archive .\xalantis-mcp-go_windows_amd64.zip -DestinationPath .
```

### Avec Go (≥ 1.24)

```bash
go install github.com/martialpmt/xalantis-mcp-go/cmd/xalantis-mcp-go@latest
```

Le binaire est installé dans `$(go env GOPATH)/bin`.

## Configuration (Claude Desktop)

1. Claude Desktop → Réglages → Développeur → Éditer la config, ajouter dans
   `mcpServers` le chemin **absolu** du binaire (`~` n'est pas interprété) :
   ```json
   {
     "mcpServers": {
       "xalantis-mcp-go": {
         "command": "/Users/VOTRE_LOGIN/.local/bin/xalantis-mcp-go",
         "env": {
           "XALANTIS_API_KEY": "sk_live_VOTRE_CLE"
         }
       }
     }
   }
   ```
   (Si le fichier contient déjà le connecteur `xalantis`, ajouter simplement l'entrée
   `xalantis-mcp-go` à côté, dans le même bloc `mcpServers`.)
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
- `XALANTIS_READ_ONLY` (optionnel) — `1`/`true` : le serveur n'expose
  pas `xalantis_call_operation` et la recherche ne renvoie que des
  lectures ; défaut `false`. Une valeur invalide empêche le démarrage.
- `XALANTIS_DEBUG` (optionnel) — `1`/`true` : trace chaque appel HTTP
  (méthode, URL, statut, durée) sur la sortie d'erreur, sans jamais la clé
  API ; défaut `false`. Utile pour diagnostiquer un outil qui échoue : dans
  Claude Desktop, la trace apparaît dans les journaux du serveur MCP.

## Développement

```
cmd/xalantis-mcp-go/         point d'entrée : configuration et assemblage
internal/mcp/                boucle JSON-RPC stdio et registre d'outils
internal/xalantis/           client HTTP de l'API
internal/openapi/            spécification embarquée, recherche et description
internal/tools/              outils dédiés et génériques
```

- Mettre à jour l'API : remplacer `internal/openapi/xalantis-openapi.json`
  puis recompiler.
- Tests unitaires : `go test ./...`
- Test de bout en bout : `go build -o xalantis-mcp-go ./cmd/xalantis-mcp-go && python3 test_mcp.py`
- Vérifications locales (comme la CI) : `gofmt -l .`, `golangci-lint run`,
  `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`,
  `sh .github/scripts/next-version_test.sh`
- Build de release local : `goreleaser release --snapshot --clean` (résultat dans `dist/`).
- Publier une version : GitHub → Actions → **Release** → *Run workflow* sur
  `main`, choisir `patch`, `minor` ou `major`. Le workflow relance la CI, crée
  le tag `vX.Y.Z`, publie les binaires dans les Releases et enregistre le
  module sur pkg.go.dev. Les versions restent en `v0.x`/`v1.x` (une `v2`
  exigerait le suffixe `/v2` dans le chemin du module).

## Notes

- Rate limit API : 60 requêtes/minute par clé. Le serveur rejoue la requête
  une fois si l'en-tête `Retry-After` demande 20 s ou moins ; au-delà, il
  remonte l'erreur avec le délai à attendre. Il rejoue aussi une fois après
  une erreur réseau passagère ou un 502/503/504 ; les écritures portant une
  `Idempotency-Key`, ce second essai ne crée pas de doublon. Un 500 n'est
  jamais rejoué.
- Réponses limitées à 100 Mo (fichiers) et 200 Ko (texte renvoyé à Claude,
  la contrainte étant la fenêtre de contexte). Au-delà : paginer
  (`per_page`, plafonné à 100), affiner les filtres, ou utiliser `save_to`
  pour enregistrer la réponse en fichier.
- Délais : 30 s pour les en-têtes de réponse, 5 minutes pour la requête
  entière (téléchargements). Un client MCP abandonne souvent un appel au
  bout d'une minute : un gros téléchargement peut donc être signalé comme
  annulé alors que le fichier a bien été enregistré. Les appels sont traités
  en parallèle : un téléchargement long ne retarde plus les suivants.
