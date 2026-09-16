# Xalantis API coverage — design

Date: 2026-09-16
Status: approved

## Goal

Extend `xalantis-projects-mcp` from 7 read-only project tools to every
operation of the Xalantis API (OpenAPI 3.1, v2.11.0) in 12 selected areas,
and restructure the code into focused packages.

## Scope

206 operations, selected by their first OpenAPI tag:

| Tag | Operations |
|---|---|
| Projets et tâches | 108 |
| Tickets | 57 |
| Automatisations de tickets | 8 |
| Catégories de tickets | 4 |
| Tags de tickets | 5 |
| Politiques SLA | 4 |
| Violations SLA | 2 |
| Politiques d’escalade | 3 |
| Réponses prédéfinies | 4 |
| Disponibilité des agents | 2 |
| Catalogue de services | 3 |
| Administration du catalogue | 6 |

Note: the tag `Politiques d’escalade` uses a typographic apostrophe (U+2019).
The allowlist must match it exactly.

Out of scope: contracts, signatures, CRM, KYC, knowledge base, finance and
billing areas.

## Decisions

- **Writes are not gated in code.** Every operation is exposed. What succeeds
  depends only on the API key's scopes; the API returns 403 otherwise.
- **Base URL default stays `https://xalantis.com`** (the spec says
  `https://app.xalantis.com`; the user confirmed `xalantis.com`).
  `XALANTIS_BASE_URL` still overrides it. `/api/v1` is appended as today.
- **Approach: 3 generic spec-driven tools + the 7 existing tools unchanged.**
  One tool per operation (206 tools) was rejected: too much context, too many
  tools for clients such as Claude Desktop, and several operationIds exceed
  the 64-character tool name limit.
- **Files use local paths.** The server runs on the user's machine.
- **No dependencies.** Standard library only, as today.

## Architecture

```
cmd/xalantis-projects-mcp/main.go   wiring: read env config, build client and catalog, register tools, run server
internal/mcp/server.go              JSON-RPC stdio loop + tool registry; no Xalantis knowledge
internal/xalantis/client.go         HTTP client: Do(method, path, query, headers, body)
internal/openapi/catalog.go         embedded spec, tag filter, search, describe, $ref resolution
internal/openapi/xalantis-openapi.json   full spec, embedded with //go:embed
internal/tools/projects.go          the 7 existing tools
internal/tools/generic.go           search / describe / call tools, file upload and download
```

Dependency direction: `cmd` → `tools`, `mcp`, `xalantis`, `openapi`;
`tools` → `xalantis`, `openapi`. `mcp`, `xalantis` and `openapi` do not
import each other.

Rules:

- No interface with a single implementation, no DI framework, no
  domain/use-case/repository layers.
- `main` reads environment variables once and passes a `Config` struct to the
  client. The client never calls `os.Getenv`.
- The root `main.go` is removed. Build command:
  `go build -o xalantis-projects-mcp ./cmd/xalantis-projects-mcp`.
- Module path stays `paymetrust/xalantis-projects-mcp`.
- The spec file is the full, unmodified OpenAPI document. Filtering happens at
  startup. Updating the API = replace the file and rebuild.

### `internal/mcp`

- `Tool` = definition (name, description, input schema) + handler
  `func(args map[string]any) (string, error)`.
- `Server` handles `initialize`, `ping`, `tools/list`, `tools/call`, and the
  known notifications, with the current behavior:
  - no reply to any message without an `id`;
  - `-32601` for unknown methods, `-32602` for unparsable `tools/call` params;
  - a handler error becomes a result with `isError: true` and text
    `Erreur : <message>`.
- `initialize` returns an `instructions` string:
  use the 7 dedicated tools for common project reads; for anything else,
  `xalantis_search_operations` → `xalantis_describe_operation` →
  `xalantis_call_operation`.
- `serverVersion` becomes `2.0.0`. Protocol version stays `2025-06-18`.

### `internal/xalantis`

- `Client{BaseURL, APIKey, HTTP *http.Client, UserAgent}`.
- `Do(method, path, query, headers, body, contentType)`: path is already
  built and relative to `/api/v1`; query is `url.Values`; body is an
  `io.Reader` (nil for none). Returns status, headers and body bytes
  (10 MB limit).
- Keeps current behavior: `Authorization: Bearer`, `Accept: application/json`,
  `User-Agent`, 30 s timeout, missing-key error message.
- Error mapping:
  - 429 → rate-limit message with `Retry-After`;
  - other non-2xx → `API Xalantis HTTP <code> : <body truncated to 500 chars>`.
- Helper for multipart bodies built with `mime/multipart`.

### `internal/openapi`

- Parses the embedded spec at startup. A parse failure makes the program exit
  with a clear message on stderr.
- `Operation{ID, Method, Path, Tag, Summary, Scope, Params, Body}`, keeping
  only the 12 allowed tags. Expected count: 206.
- `Search(query, area, method)`:
  - query matched against operationId, path and summary, case- and
    accent-insensitive;
  - at most 50 results;
  - no filters at all → the 12 areas with operation counts.
- `Describe(id)`: parameters (name, location, required, type, description),
  request body schema with `$ref` resolved up to depth 3 (deeper refs left as
  their name), list of binary (file) fields, required scope. Response schemas
  are omitted.

### `internal/tools`

`projects.go`: the 7 existing tools, same names, schemas, query mapping and
validation as today, rewritten on top of `xalantis.Client`.

`generic.go`:

**`xalantis_search_operations`** — inputs `query`, `area`, `method` (all
optional). Output: JSON list of `{operation_id, method, path, summary, scope}`.

**`xalantis_describe_operation`** — input `operation_id` (required). Output:
compact JSON from `Describe`.

**`xalantis_call_operation`** — inputs:

- `operation_id` (required);
- `path_params` object;
- `query` object; array values become repeated `key[]` parameters (existing
  convention); booleans → `1`/`0`; numbers formatted without decimals when
  integral;
- `headers` object; only `Idempotency-Key` and `If-Match` are accepted;
- `body` object, sent as JSON; for multipart operations sent as form fields;
- `files` object, field name → path or list of paths; multipart operations
  only;
- `save_to` optional path for binary responses.

Validation before any HTTP call (failures → tool error, no API call):

- operation exists in the filtered catalog;
- every required path parameter is present; each path value passes the
  existing `requireUUID` rule (non-empty, no `/?#&%`, not `.` or `..`) and is
  path-escaped;
- required query parameters are present;
- `files` only on multipart operations; every path exists and is a regular
  file;
- unknown header names are rejected.

The body is not validated against the schema; the API returns 422 with
details.

Response handling:

- `Content-Type` JSON or `text/*` → returned as text.
- Anything else → written to `save_to`, or by default
  `~/Downloads/<filename>`, where the filename comes from
  `Content-Disposition` reduced with `filepath.Base` (fallback:
  `<operation_id>.bin`). An existing file is never overwritten: a suffix
  ` (1)`, ` (2)`… is added. The tool returns path, size and content type.

Not included: automatic retries, automatic `Idempotency-Key`, body schema
validation.

## Testing

Go unit tests (`go test ./...`, standard library only):

- `mcp`: handshake, `instructions` present, `tools/list`, handler error →
  `isError`, no reply to notifications, unknown method.
- `xalantis`: auth header, 429 message with `Retry-After`, error truncation,
  multipart request, missing key (with `httptest`).
- `openapi`: 206 operations after filtering, the typographic-apostrophe tag
  included, search (accent-insensitive, area and method filters, empty query
  lists areas), describe resolves `$ref`, binary fields detected.
- `tools`: path/query/header building, `.`/`..` and `/` rejection in path
  params, unknown header rejected, files on non-multipart rejected, download
  written to a temp dir, malicious `Content-Disposition` stays inside the
  target dir, no overwrite.

End-to-end `test_mcp.py`: keeps the current 14 checks (count of API calls
adjusted for new cases) and adds search, describe, a JSON `POST` call (method,
body and `Idempotency-Key` checked by the mock), and a binary download to a
temp dir.

## Documentation

`README.md` rewritten in French: the 10 tools and the search → describe → call
flow, the 12 areas and their scopes, file handling, the new build command,
a note that write access depends on the API key's scopes, and the base URL
default `https://xalantis.com`.

## Amendments (implementation planning, 2026-09-16)

Facts found in the OpenAPI file changed these details:

1. `tools` imports `mcp` for the `mcp.Tool` type; `mcp` stays a leaf package.
2. Required header parameters are validated like required query parameters.
   70 operations require `Idempotency-Key`.
3. Query parameter names are used as declared (`status[]`, `type[]`, …).
   `query` also accepts the name without `[]` and `base[sub]` keys for object
   parameters (`custom_fields[clé]`). Unknown names are rejected.
4. The client reads responses up to 100 MB (documents reach 50 MB). Text
   returned to Claude is limited to 10 MB; larger text is an error.
5. Multipart array file fields are sent as `name[]` (e.g. `files[]`).
6. `save_to` never overwrites either; it uses the same ` (1)` suffix rule.
7. "No filters → list areas" is handled by the search tool, not by
   `Catalog.Search`.
8. Unknown path parameter keys are rejected; operations without a request
   body reject `body`.
9. No `$ref` appears in request bodies or parameters of the 206 operations
   (only in responses). The resolver is kept as a safeguard.
