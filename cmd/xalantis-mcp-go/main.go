// xalantis-mcp-go — serveur MCP (stdio) pour l'API Xalantis (api/v1) :
// projets et tâches, service desk et catalogue de services.
//
// Configuration par variables d'environnement :
//
//	XALANTIS_API_KEY   (obligatoire) — clé API tenant ; les scopes décident des opérations permises
//	XALANTIS_BASE_URL  (optionnel)   — défaut : https://xalantis.com
//	XALANTIS_FILES_DIR (optionnel)   — dossier autorisé pour les fichiers locaux (envoi, save_to,
//	                                    téléchargements) ; défaut : <dossier personnel>/Downloads/xalantis
//	XALANTIS_READ_ONLY (optionnel)   — true/1 : n'expose aucune opération d'écriture ; défaut : false
//
// Compilation : go build -o xalantis-mcp-go ./cmd/xalantis-mcp-go
// Version : xalantis-mcp-go --version
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/martialpmt/xalantis-mcp-go/internal/mcp"
	"github.com/martialpmt/xalantis-mcp-go/internal/openapi"
	"github.com/martialpmt/xalantis-mcp-go/internal/tools"
	"github.com/martialpmt/xalantis-mcp-go/internal/xalantis"
)

const serverName = "xalantis-mcp-go"

// serverInstructions renvoie les consignes d'initialisation MCP.
func serverInstructions(readOnly bool) string {
	s := "Pour les lectures courantes de projets, utilisez les 7 outils dédiés (xalantis_list_projects, xalantis_list_tasks…). " +
		"Pour toute autre opération Xalantis (tickets, SLA, catalogue…) : " +
		"xalantis_search_operations, puis xalantis_describe_operation, puis "
	if readOnly {
		return s + "xalantis_read_operation. Mode lecture seule (XALANTIS_READ_ONLY) : les écritures sont désactivées."
	}
	return s + "xalantis_read_operation pour une lecture (GET) ou xalantis_call_operation pour une écriture."
}

// parseBoolEnv lit une variable d'environnement booléenne : vide = désactivée.
func parseBoolEnv(name, v string) (bool, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s invalide (%q) : utilisez true/false ou 1/0", name, v)
	}
	return b, nil
}

func main() {
	showVersion := flag.Bool("version", false, "affiche la version et quitte")
	flag.Parse()
	ver := currentVersion()
	if *showVersion {
		fmt.Println(ver)
		return
	}

	readOnly, err := parseBoolEnv("XALANTIS_READ_ONLY", os.Getenv("XALANTIS_READ_ONLY"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	debug, err := parseBoolEnv("XALANTIS_DEBUG", os.Getenv("XALANTIS_DEBUG"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cat, err := openapi.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "spécification OpenAPI embarquée illisible :", err)
		os.Exit(1)
	}
	cfg := xalantis.Config{
		BaseURL:   os.Getenv("XALANTIS_BASE_URL"),
		APIKey:    os.Getenv("XALANTIS_API_KEY"),
		UserAgent: serverName + "/" + ver,
	}
	if debug {
		// stdout porte le protocole MCP : la trace va sur stderr.
		cfg.Debug = os.Stderr
	}
	client := xalantis.NewClient(cfg)

	filesDir := os.Getenv("XALANTIS_FILES_DIR")
	if filesDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "dossier personnel introuvable, définissez XALANTIS_FILES_DIR :", err)
			os.Exit(1)
		}
		filesDir = filepath.Join(home, "Downloads", "xalantis")
	}
	if filesDir, err = filepath.Abs(filesDir); err != nil {
		fmt.Fprintln(os.Stderr, "chemin de XALANTIS_FILES_DIR invalide :", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "création du dossier autorisé impossible ("+filesDir+") :", err)
		os.Exit(1)
	}

	srv := mcp.NewServer(serverName, ver, serverInstructions(readOnly), tools.MaxTextBytes)
	srv.Register(tools.ProjectTools(client)...)
	srv.Register(tools.GenericTools(client, cat, filesDir, readOnly)...)
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
