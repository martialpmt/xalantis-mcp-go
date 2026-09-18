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
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL est utilisée quand Config.BaseURL est vide.
const DefaultBaseURL = "https://xalantis.com"

// MaxBodyBytes borne la taille d'une réponse lue en mémoire.
// ponytail: réponse entière en mémoire ; passer au streaming si des fichiers > 100 Mo apparaissent.
const MaxBodyBytes = 100 << 20

const (
	// headerTimeout borne l'attente des en-têtes de réponse : une API muette
	// échoue vite. requestTimeout borne l'ensemble, corps compris, pour
	// laisser passer un téléchargement volumineux.
	headerTimeout  = 30 * time.Second
	requestTimeout = 5 * time.Minute
	// maxRetryAfter borne l'attente acceptée avant de réessayer un 429.
	maxRetryAfter = 20 * time.Second
)

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
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = headerTimeout
	return &Client{
		baseURL:   base,
		apiKey:    cfg.APIKey,
		userAgent: cfg.UserAgent,
		http:      &http.Client{Timeout: requestTimeout, Transport: tr},
	}
}

// Response est une réponse HTTP 2xx lue en entier.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Do envoie une requête. path est relatif à /api/v1 et déjà échappé.
// body peut être nil. Un 429 est réessayé une fois si l'attente demandée
// reste courte. Toute réponse hors 2xx devient une erreur.
func (c *Client) Do(method, path string, query url.Values, headers map[string]string, body io.Reader, contentType string) (*Response, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("XALANTIS_API_KEY manquante : ajoutez-la dans la configuration du serveur MCP")
	}
	u := c.baseURL + "/api/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	// Le corps est relu à l'identique si la requête est rejouée ; il tient
	// déjà en mémoire (JSON ou multipart préparé par l’appelant).
	var payload []byte
	if body != nil {
		var err error
		if payload, err = io.ReadAll(body); err != nil {
			return nil, err
		}
	}

	resp, data, err := c.send(method, u, headers, payload, contentType)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		if wait, ok := retryWait(resp.Header.Get("Retry-After")); ok {
			time.Sleep(wait)
			if resp, data, err = c.send(method, u, headers, payload, contentType); err != nil {
				return nil, err
			}
		}
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

// send exécute une tentative et lit le corps de la réponse. Le *http.Response
// renvoyé n'est utilisé que pour son statut et ses en-têtes : son corps est
// déjà lu et fermé.
func (c *Client) send(method, u string, headers map[string]string, payload []byte, contentType string) (*http.Response, []byte, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, fmt.Errorf("appel API impossible : %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes+1))
	if err != nil {
		return nil, nil, err
	}
	return resp, data, nil
}

// retryWait renvoie l'attente avant de rejouer un 429 : la valeur de
// Retry-After en secondes, 1 s si l'en-tête est absent. false (pas de
// nouvelle tentative) si l'attente dépasse maxRetryAfter ou est illisible.
func retryWait(header string) (time.Duration, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return time.Second, true
	}
	seconds, err := strconv.Atoi(header)
	if err != nil || seconds < 0 {
		return 0, false
	}
	wait := time.Duration(seconds) * time.Second
	return wait, wait <= maxRetryAfter
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
