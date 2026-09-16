// Package xalantis est le client HTTP de l'API Xalantis (api/v1).
package xalantis

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultBaseURL est utilisée quand Config.BaseURL est vide.
const DefaultBaseURL = "https://xalantis.com"

// MaxBodyBytes borne la taille d'une réponse lue en mémoire.
// ponytail: réponse entière en mémoire ; passer au streaming si des fichiers > 100 Mo apparaissent.
const MaxBodyBytes = 100 << 20

// Config regroupe les paramètres du client, lus une seule fois par main.
type Config struct {
	BaseURL   string
	APIKey    string
	UserAgent string
}

// Client appelle l'API Xalantis.
type Client struct {
	baseURL, apiKey, userAgent string
	http                       *http.Client
}

// NewClient crée un client ; BaseURL vide = DefaultBaseURL.
func NewClient(cfg Config) *Client {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	return &Client{
		baseURL:   base,
		apiKey:    cfg.APIKey,
		userAgent: cfg.UserAgent,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// Response est une réponse HTTP 2xx lue en entier.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Do envoie une requête. path est relatif à /api/v1 et déjà échappé.
// body peut être nil. Toute réponse hors 2xx devient une erreur.
func (c *Client) Do(method, path string, query url.Values, headers map[string]string, body io.Reader, contentType string) (*Response, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("XALANTIS_API_KEY manquante : ajoutez-la dans la configuration du serveur MCP")
	}
	u := c.baseURL + "/api/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("appel API impossible : %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limit atteint (60 req/min) — réessayez dans %s s", resp.Header.Get("Retry-After"))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return nil, fmt.Errorf("API Xalantis HTTP %d : %s", resp.StatusCode, msg)
	}
	if len(data) > MaxBodyBytes {
		return nil, fmt.Errorf("réponse trop volumineuse (plus de %d Mo)", MaxBodyBytes>>20)
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

// Get est un raccourci pour un GET dont la réponse est du texte.
func (c *Client) Get(path string, query url.Values) (string, error) {
	resp, err := c.Do(http.MethodGet, path, query, nil, nil, "")
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}

// FilePart est un fichier local à joindre sous le champ Field.
type FilePart struct {
	Field string
	Path  string
}

// Multipart construit un corps multipart/form-data. Renvoie le corps et son Content-Type.
func Multipart(fields url.Values, files []FilePart) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, values := range fields {
		for _, v := range values {
			if err := w.WriteField(name, v); err != nil {
				return nil, "", err
			}
		}
	}
	for _, f := range files {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, "", fmt.Errorf("lecture de %s impossible : %v", f.Path, err)
		}
		part, err := w.CreateFormFile(f.Field, filepath.Base(f.Path))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}
