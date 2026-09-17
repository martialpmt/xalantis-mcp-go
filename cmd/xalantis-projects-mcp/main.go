// xalantis-projects-mcp — serveur MCP (stdio) pour l'API Xalantis (api/v1) :
// projets et tâches, service desk et catalogue de services.
//
// Configuration par variables d'environnement :
//
//	XALANTIS_API_KEY   (obligatoire) — clé API tenant ; les scopes décident des opérations permises
//	XALANTIS_BASE_URL  (optionnel)   — défaut : https://xalantis.com
//	XALANTIS_FILES_DIR (optionnel)   — dossier autorisé pour les fichiers locaux (envoi, save_to,
//	                                    téléchargements) ; défaut : <dossier personnel>/Downloads/xalantis
//
// Compilation : go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martialpmt/xalantis-mcp-go/internal/mcp"
	"github.com/martialpmt/xalantis-mcp-go/internal/openapi"
	"github.com/martialpmt/xalantis-mcp-go/internal/tools"
	"github.com/martialpmt/xalantis-mcp-go/internal/xalantis"
)

const (
	serverName    = "xalantis-projects-mcp"
	serverVersion = "2.0.0"
	instructions  = "Pour les lectures courantes de projets, utilisez les 7 outils dédiés (xalantis_list_projects, xalantis_list_tasks…). " +
		"Pour toute autre opération Xalantis (tickets, SLA, catalogue, écriture sur les projets…) : " +
		"xalantis_search_operations, puis xalantis_describe_operation, puis xalantis_call_operation."
)

func main() {
	cat, err := openapi.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "spécification OpenAPI embarquée illisible :", err)
		os.Exit(1)
	}
	client := xalantis.NewClient(xalantis.Config{
		BaseURL:   os.Getenv("XALANTIS_BASE_URL"),
		APIKey:    os.Getenv("XALANTIS_API_KEY"),
		UserAgent: serverName + "/" + serverVersion,
	})

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

	srv := mcp.NewServer(serverName, serverVersion, instructions)
	srv.Register(tools.ProjectTools(client)...)
	srv.Register(tools.GenericTools(client, cat, filesDir)...)
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
