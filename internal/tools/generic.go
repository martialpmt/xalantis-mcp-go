package tools

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/martialpmt/xalantis-mcp-go/internal/mcp"
	"github.com/martialpmt/xalantis-mcp-go/internal/openapi"
	"github.com/martialpmt/xalantis-mcp-go/internal/xalantis"
)

// MaxTextBytes borne une réponse texte renvoyée au client MCP.
const MaxTextBytes = 10 << 20

func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// GenericTools renvoie les outils de recherche, description, lecture et
// écriture des opérations du catalogue. filesDir est le seul dossier dans
// lequel le serveur lit (envoi) et écrit (save_to, téléchargements) des
// fichiers. readOnly retire l'outil d'écriture et limite la recherche aux GET.
func GenericTools(c *xalantis.Client, cat *openapi.Catalog, filesDir string, readOnly bool) []mcp.Tool {
	areas := make([]any, len(openapi.Areas))
	for i, a := range openapi.Areas {
		areas[i] = a
	}
	next, listMethod := "xalantis_read_operation (GET) ou xalantis_call_operation (écritures)", ""
	if readOnly {
		next, listMethod = "xalantis_read_operation (écritures désactivées)", "GET"
	}
	opID := map[string]any{"operation_id": prop("string", "operation_id renvoyé par xalantis_search_operations.")}
	readProps := merge(opID, map[string]any{
		"path_params": map[string]any{"type": "object", "description": "Paramètres de chemin, ex. {\"projectUuid\": \"…\"}."},
		"query":       map[string]any{"type": "object", "description": "Paramètres de requête. Tableau = valeurs répétées (nom[]), objet = nom[clé]."},
		"headers":     map[string]any{"type": "object", "description": "En-têtes déclarés par l'opération (If-Match ; Idempotency-Key est générée si absente)."},
		"save_to":     prop("string", "Chemin où enregistrer la réponse (fichier ou texte, ex. un export CSV), dans le dossier autorisé (XALANTIS_FILES_DIR ; chemin relatif = relatif à ce dossier). Sans save_to, les fichiers reçus sont enregistrés dans ce dossier et le texte est renvoyé directement."),
	})
	tools := []mcp.Tool{
		{
			Name: "xalantis_search_operations",
			Description: "Étape 1/3 pour toute opération Xalantis sans outil dédié (projets, tickets, SLA, catalogue…). " +
				"Cherche les opérations par mots-clés (accents ignorés), domaine et méthode HTTP. " +
				"Sans aucun filtre, renvoie la liste des domaines. Ensuite : xalantis_describe_operation.",
			InputSchema: schema(nil, map[string]any{
				"query":  prop("string", "Mots-clés, tous requis (ex. « créer ticket »)."),
				"area":   map[string]any{"type": "string", "enum": areas, "description": "Domaine fonctionnel."},
				"method": map[string]any{"type": "string", "enum": []any{"GET", "POST", "PUT", "PATCH", "DELETE"}, "description": "Méthode HTTP."},
			}),
			Annotations: readOnlyHint,
			Handler: func(raw map[string]any) (string, error) {
				a := args(raw)
				query, area, method := a.str("query"), a.str("area"), a.str("method")
				if query == "" && area == "" && method == "" {
					return toJSON(map[string]any{"areas": cat.AreaCounts(listMethod)})
				}
				if readOnly {
					if method != "" && !strings.EqualFold(method, "GET") {
						return toJSON(map[string]any{"count": 0, "operations": []openapi.Summary{}})
					}
					method = "GET"
				}
				res := cat.Search(query, area, method)
				return toJSON(map[string]any{"count": len(res), "operations": res})
			},
		},
		{
			Name: "xalantis_describe_operation",
			Description: "Étape 2/3 : paramètres (chemin, requête, en-têtes) et schéma du corps d'une opération Xalantis. " +
				"Ensuite : " + next + ".",
			InputSchema: schema([]string{"operation_id"}, opID),
			Annotations: readOnlyHint,
			Handler: func(raw map[string]any) (string, error) {
				d, err := cat.Describe(args(raw).str("operation_id"))
				if err != nil {
					return "", err
				}
				return toJSON(d)
			},
		},
		{
			Name: "xalantis_read_operation",
			Description: "Étape 3/3 pour une lecture : exécute une opération GET décrite par xalantis_describe_operation. " +
				"Ne modifie aucune donnée. Les fichiers reçus sont enregistrés localement.",
			InputSchema: schema([]string{"operation_id"}, readProps),
			Annotations: readOnlyHint,
			Handler: func(raw map[string]any) (string, error) {
				a := args(raw)
				if err := checkMethod(cat, a.str("operation_id"), true, readOnly); err != nil {
					return "", err
				}
				return callOperation(c, cat, filesDir, a)
			},
		},
	}
	if readOnly {
		return tools
	}
	return append(tools, mcp.Tool{
		Name: "xalantis_call_operation",
		Description: "Étape 3/3 pour une écriture : exécute une opération POST, PUT, PATCH ou DELETE décrite par xalantis_describe_operation. " +
			"Peut créer, modifier ou supprimer des données selon les scopes de la clé API. " +
			"Les fichiers envoyés sont des chemins locaux ; les fichiers reçus sont enregistrés localement.",
		InputSchema: schema([]string{"operation_id"}, merge(readProps, map[string]any{
			"body":  map[string]any{"type": "object", "description": "Corps JSON, ou champs texte pour une opération multipart."},
			"files": map[string]any{"type": "object", "description": "Opérations multipart : champ → chemin ou liste de chemins, dans le dossier autorisé (XALANTIS_FILES_DIR ; chemin relatif = relatif à ce dossier)."},
		})),
		Annotations: destructiveHint,
		Handler: func(raw map[string]any) (string, error) {
			a := args(raw)
			if err := checkMethod(cat, a.str("operation_id"), false, readOnly); err != nil {
				return "", err
			}
			return callOperation(c, cat, filesDir, a)
		},
	})
}

// checkMethod refuse une opération qui ne correspond pas à l'outil : read
// n'accepte que GET, l'outil d'écriture tout sauf GET. Une opération inconnue
// passe : callOperation renvoie alors l'erreur « opération inconnue ».
func checkMethod(cat *openapi.Catalog, id string, read, readOnly bool) error {
	op, ok := cat.Get(id)
	if !ok {
		return nil
	}
	isGet := op.Method == "GET"
	switch {
	case read && !isGet && readOnly:
		return errors.New("écriture désactivée (XALANTIS_READ_ONLY)")
	case read && !isGet:
		return errors.New("opération d'écriture : utilisez xalantis_call_operation")
	case !read && isGet:
		return errors.New("lecture : utilisez xalantis_read_operation")
	}
	return nil
}

func callOperation(c *xalantis.Client, cat *openapi.Catalog, filesDir string, a args) (string, error) {
	id := a.str("operation_id")
	op, ok := cat.Get(id)
	if !ok {
		return "", fmt.Errorf("opération inconnue : %s (utilisez xalantis_search_operations)", id)
	}
	pathParams, err := a.obj("path_params")
	if err != nil {
		return "", err
	}
	queryArgs, err := a.obj("query")
	if err != nil {
		return "", err
	}
	headerArgs, err := a.obj("headers")
	if err != nil {
		return "", err
	}
	body, err := a.obj("body")
	if err != nil {
		return "", err
	}
	fileArgs, err := a.obj("files")
	if err != nil {
		return "", err
	}

	path, err := buildPath(op, pathParams)
	if err != nil {
		return "", err
	}
	query, err := buildQuery(op, queryArgs)
	if err != nil {
		return "", err
	}
	headers, generatedKey, err := buildHeaders(op, headerArgs)
	if err != nil {
		return "", err
	}
	// save_to est résolu et validé avant tout appel HTTP, comme les autres
	// entrées : un chemin hors du dossier autorisé ne doit jamais déclencher
	// de requête à l'API.
	saveTo := a.str("save_to")
	var saveTarget string
	if saveTo != "" {
		saveTarget, err = resolveSaveTo(filesDir, saveTo)
		if err != nil {
			return "", err
		}
	}

	var reader io.Reader
	contentType := ""
	switch op.BodyType {
	case "multipart/form-data":
		fields := url.Values{}
		for k, v := range body {
			if err := addValue(fields, k, v, true); err != nil {
				return "", err
			}
		}
		files, err := buildFiles(op, filesDir, fileArgs)
		if err != nil {
			return "", err
		}
		reader, contentType, err = xalantis.Multipart(fields, files)
		if err != nil {
			return "", err
		}
	case "application/json":
		if len(fileArgs) > 0 {
			return "", fmt.Errorf("l'opération %s n'accepte pas de fichiers", op.ID)
		}
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return "", err
			}
			reader, contentType = bytes.NewReader(b), "application/json"
		}
	default:
		if len(fileArgs) > 0 {
			return "", fmt.Errorf("l'opération %s n'accepte pas de fichiers", op.ID)
		}
		if body != nil {
			return "", fmt.Errorf("l'opération %s n'accepte pas de corps", op.ID)
		}
	}

	resp, err := c.Do(op.Method, path, query, headers, reader, contentType)
	if err != nil {
		if generatedKey != "" {
			return "", fmt.Errorf("%w (Idempotency-Key générée : %s — réutilisez-la pour réessayer)", err, generatedKey)
		}
		return "", err
	}
	if len(resp.Body) == 0 {
		return fmt.Sprintf("Succès (HTTP %d), réponse vide.", resp.Status), nil
	}
	if saveTo == "" && isText(resp.Header.Get("Content-Type")) {
		if len(resp.Body) > MaxTextBytes {
			return "", fmt.Errorf("réponse texte trop volumineuse (%d octets) : affinez les filtres ou paginez", len(resp.Body))
		}
		return string(resp.Body), nil
	}
	return saveDownload(filesDir, saveTarget, op.ID, resp)
}

var pathParamPattern = regexp.MustCompile(`\{([^}]+)\}`)

func buildPath(op *openapi.Operation, values map[string]any) (string, error) {
	for k := range values {
		if _, ok := op.Param("path", k); !ok {
			return "", fmt.Errorf("paramètre de chemin inconnu pour %s : %s", op.ID, k)
		}
	}
	var firstErr error
	path := pathParamPattern.ReplaceAllStringFunc(op.Path, func(m string) string {
		name := m[1 : len(m)-1]
		v, _ := values[name].(string)
		v = strings.TrimSpace(v)
		if err := checkSegment(name, v); err != nil && firstErr == nil {
			firstErr = err
		}
		return url.PathEscape(v)
	})
	return path, firstErr
}

// queryName retrouve le nom déclaré d'un paramètre de requête : nom exact,
// nom + "[]", ou nom de base d'un paramètre objet (ex. custom_fields[clé]).
func queryName(op *openapi.Operation, key string) (string, bool) {
	if _, ok := op.Param("query", key); ok {
		return key, true
	}
	if _, ok := op.Param("query", key+"[]"); ok {
		return key + "[]", true
	}
	base, _, _ := strings.Cut(key, "[")
	for _, p := range op.Params {
		if p.In != "query" {
			continue
		}
		if pb, _, found := strings.Cut(p.Name, "["); found && pb == base && !strings.HasSuffix(p.Name, "[]") {
			return key, true
		}
	}
	return "", false
}

func buildQuery(op *openapi.Operation, values map[string]any) (url.Values, error) {
	q := url.Values{}
	for k, v := range values {
		name, ok := queryName(op, k)
		if !ok {
			return nil, fmt.Errorf("paramètre de requête inconnu pour %s : %s", op.ID, k)
		}
		if err := addValue(q, name, v, false); err != nil {
			return nil, err
		}
	}
	for _, p := range op.Params {
		if p.In == "query" && p.Required && !q.Has(p.Name) {
			return nil, fmt.Errorf("paramètre de requête requis manquant : %s", p.Name)
		}
	}
	return q, nil
}

// buildHeaders valide les en-têtes fournis. Si l'opération déclare
// Idempotency-Key et qu'aucune valeur n'est fournie, une clé est générée et
// renvoyée en second résultat ("" sinon).
func buildHeaders(op *openapi.Operation, values map[string]any) (map[string]string, string, error) {
	h := map[string]string{}
	for k, v := range values {
		var name string
		for _, p := range op.Params {
			if p.In == "header" && strings.EqualFold(p.Name, k) {
				name = p.Name
			}
		}
		if name == "" {
			return nil, "", fmt.Errorf("en-tête non autorisé pour %s : %s", op.ID, k)
		}
		s, err := scalar(name, v)
		if err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(s) != "" {
			h[name] = strings.TrimSpace(s)
		}
	}
	generated := ""
	for _, p := range op.Params {
		if p.In == "header" && strings.EqualFold(p.Name, "Idempotency-Key") && h[p.Name] == "" {
			generated = newUUID()
			h[p.Name] = generated
		}
	}
	for _, p := range op.Params {
		if p.In == "header" && p.Required && h[p.Name] == "" {
			return nil, "", fmt.Errorf("en-tête requis manquant : %s", p.Name)
		}
	}
	return h, generated, nil
}

// newUUID renvoie un UUID v4 aléatoire (RFC 9562).
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // depuis Go 1.24, ne renvoie jamais d'erreur (le programme s'arrête en cas d'échec)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func buildFiles(op *openapi.Operation, filesDir string, values map[string]any) ([]xalantis.FilePart, error) {
	names := make([]string, 0, len(values))
	for k := range values {
		names = append(names, k)
	}
	sort.Strings(names)
	var parts []xalantis.FilePart
	for _, name := range names {
		var field *openapi.FileField
		for i := range op.FileFields {
			if op.FileFields[i].Name == name {
				field = &op.FileFields[i]
			}
		}
		if field == nil {
			return nil, fmt.Errorf("champ fichier inconnu pour %s : %s", op.ID, name)
		}
		var paths []string
		switch v := values[name].(type) {
		case string:
			paths = []string{v}
		case []any:
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("champ fichier %s : chemins texte attendus", name)
				}
				paths = append(paths, s)
			}
		default:
			return nil, fmt.Errorf("champ fichier %s : chemin ou liste de chemins attendu", name)
		}
		if len(paths) > 1 && !field.Multiple {
			return nil, fmt.Errorf("champ fichier %s : un seul fichier accepté", name)
		}
		key := name
		if field.Multiple {
			key += "[]"
		}
		for _, p := range paths {
			resolved, err := resolveUpload(filesDir, p)
			if err != nil {
				return nil, err
			}
			parts = append(parts, xalantis.FilePart{Field: key, Path: resolved})
		}
	}
	return parts, nil
}

func isText(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json") || strings.HasPrefix(mt, "text/")
}

// safeFilename extrait un nom de fichier sans chemin de Content-Disposition.
func safeFilename(disposition string) string {
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	name := filepath.Base(strings.ReplaceAll(params["filename"], "\\", "/"))
	if name == "." || name == ".." || name == "/" {
		return ""
	}
	return name
}

// createUnique crée path, ou « nom (1).ext », « nom (2).ext »… s'il existe déjà.
func createUnique(path string) (*os.File, string, error) {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 0; i < 1000; i++ {
		p := path
		if i > 0 {
			p = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // G304: p vient de resolveSaveTo ou de filesDir+safeFilename, déjà confiné à XALANTIS_FILES_DIR
		if err == nil {
			return f, p, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("aucun nom libre pour %s", path)
}

// saveDownload écrit resp.Body dans target, ou, si target est vide, dans
// filesDir sous le nom déduit de Content-Disposition (ou de opID). target,
// quand il est fourni, a déjà été validé par resolveSaveTo.
func saveDownload(filesDir, target, opID string, resp *xalantis.Response) (string, error) {
	if target == "" {
		name := safeFilename(resp.Header.Get("Content-Disposition"))
		if name == "" {
			name = opID + ".bin"
		}
		target = filepath.Join(filesDir, name)
	}
	f, path, err := createUnique(target)
	if err != nil {
		return "", fmt.Errorf("enregistrement impossible : %v", err)
	}
	_, werr := f.Write(resp.Body)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		return "", fmt.Errorf("écriture de %s impossible : %v", path, errors.Join(werr, cerr))
	}
	return toJSON(map[string]any{
		"saved_to":     path,
		"size":         len(resp.Body),
		"content_type": resp.Header.Get("Content-Type"),
	})
}
