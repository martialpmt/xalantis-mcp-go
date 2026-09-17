// Package mcp implémente un serveur MCP minimal (JSON-RPC 2.0 sur stdio)
// avec un registre d'outils. Il ne connaît rien de Xalantis.
package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// ProtocolVersion est la version du protocole MCP annoncée à l'initialisation.
const ProtocolVersion = "2025-06-18"

// Handler exécute un outil. Une erreur devient un résultat isError=true.
type Handler func(args map[string]any) (string, error)

// Tool décrit un outil exposé au client MCP. Annotations porte les indices
// MCP (readOnlyHint, destructiveHint…) ; nil = champ absent.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
	Handler     Handler        `json:"-"`
}

// Server est un serveur MCP stdio.
type Server struct {
	name, version, instructions string
	tools                       []Tool
	byName                      map[string]Tool
}

// NewServer crée un serveur sans outil.
func NewServer(name, version, instructions string) *Server {
	return &Server{name: name, version: version, instructions: instructions, byName: map[string]Tool{}}
}

// Register ajoute des outils, dans l'ordre donné.
func (s *Server) Register(tools ...Tool) {
	for _, t := range tools {
		s.tools = append(s.tools, t)
		s.byName[t.Name] = t
	}
}

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func textResult(text string, isErr bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	}
}

// Serve lit une requête JSON-RPC par ligne sur r et écrit les réponses sur w,
// jusqu'à la fin de r.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 0, 1<<20), 16<<20)
	out := bufio.NewWriter(w)
	enc := json.NewEncoder(out)
	send := func(resp response) error {
		resp.JSONRPC = "2.0"
		if err := enc.Encode(resp); err != nil {
			return err
		}
		return out.Flush()
	}

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue // ligne illisible : on ignore
		}
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue // notification : jamais de réponse
		}
		if err := send(s.handle(req)); err != nil {
			return err
		}
	}
	return in.Err()
}

func (s *Server) handle(req request) response {
	switch req.Method {
	case "initialize":
		result := map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		}
		if s.instructions != "" {
			result["instructions"] = s.instructions
		}
		return response{ID: req.ID, Result: result}

	case "ping":
		return response{ID: req.ID, Result: map[string]any{}}

	case "tools/list":
		return response{ID: req.ID, Result: map[string]any{"tools": s.tools}}

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return response{ID: req.ID, Error: &rpcError{Code: -32602, Message: "paramètres invalides"}}
		}
		tool, ok := s.byName[params.Name]
		if !ok {
			return response{ID: req.ID, Result: textResult("Erreur : outil inconnu : "+params.Name, true)}
		}
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		text, err := tool.Handler(params.Arguments)
		if err != nil {
			return response{ID: req.ID, Result: textResult("Erreur : "+err.Error(), true)}
		}
		return response{ID: req.ID, Result: textResult(text, false)}
	}
	return response{ID: req.ID, Error: &rpcError{Code: -32601, Message: "méthode non supportée : " + req.Method}}
}
