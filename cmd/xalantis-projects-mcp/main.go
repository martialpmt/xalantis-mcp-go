// xalantis-projects-mcp — serveur MCP (stdio) pour l'API Xalantis (api/v1) :
// projets et tâches, service desk et catalogue de services.
//
// Configuration par variables d'environnement :
//
//	XALANTIS_API_KEY  (obligatoire) — clé API tenant ; les scopes décident des opérations permises
//	XALANTIS_BASE_URL (optionnel)   — défaut : https://xalantis.com
//
// Compilation : go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"paymetrust/xalantis-projects-mcp/internal/mcp"
	"paymetrust/xalantis-projects-mcp/internal/openapi"
	"paymetrust/xalantis-projects-mcp/internal/tools"
	"paymetrust/xalantis-projects-mcp/internal/xalantis"
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
	home, _ := os.UserHomeDir()

	srv := mcp.NewServer(serverName, serverVersion, instructions)
	srv.Register(tools.ProjectTools(client)...)
	srv.Register(tools.GenericTools(client, cat, filepath.Join(home, "Downloads"))...)
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
