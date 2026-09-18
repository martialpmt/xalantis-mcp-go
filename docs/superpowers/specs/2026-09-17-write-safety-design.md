# Write safety — design

Date: 2026-09-17
Status: approved

## Goal

Let MCP clients tell reads from writes, let users turn writes off entirely,
and stop failing writes that lack an `Idempotency-Key`.

## Context

- `xalantis_call_operation` runs every catalog operation, GET or not. The
  client cannot tell a read from a delete, so it must ask for approval on
  both.
- Tools carry no MCP annotations (`readOnlyHint`, `destructiveHint`), which
  protocol `2025-06-18` supports.
- Blocking writes relies only on the API key's scopes.
- 80 operations declare `Idempotency-Key` as a required header ("clé
  opaque", 16 to 200 characters, one operation allows up to 255).
  `buildHeaders` returns an error when it is missing, so Claude has to
  retry.

## Changes

### 1. Tool annotations

`mcp.Tool` gains `Annotations map[string]any` with JSON tag
`annotations,omitempty`.

`readOnlyHint: true` on the 7 project tools, `xalantis_search_operations`,
`xalantis_describe_operation` and `xalantis_read_operation`.
`destructiveHint: true` on `xalantis_call_operation`.

### 2. Read/write split

- New tool `xalantis_read_operation`. Schema: `operation_id` (required),
  `path_params`, `query`, `headers`, `save_to`; no `body`, no `files`.
  Rejects any operation whose method is not GET:
  - normal mode: `opération d'écriture : utilisez xalantis_call_operation`;
  - read-only mode: `écriture désactivée (XALANTIS_READ_ONLY)`.
- `xalantis_call_operation` keeps its name and schema and becomes write-only.
  Rejects GET operations with `lecture : utilisez xalantis_read_operation`.
- Both handlers check the method, then call the existing `callOperation`.
  The unknown-operation error keeps precedence over the method check.
- Tool descriptions and the server `instructions` in `main.go` name the
  right tool for each step: search → describe → read or call.

### 3. Read-only mode

- `main.go` reads `XALANTIS_READ_ONLY`. Empty means off; otherwise the value
  is parsed with `strconv.ParseBool`, and an invalid value stops the server
  at startup with an error on stderr.
- `GenericTools(c, cat, filesDir, readOnly bool)`. When `readOnly`:
  - `xalantis_call_operation` is not registered;
  - `xalantis_search_operations` returns only GET operations (a `method`
    filter other than GET returns no results), and the no-filter area list
    counts only GET operations (`Catalog.AreaCounts` takes a method filter,
    empty = all);
  - `instructions` states that writes are disabled.
- `xalantis_describe_operation` is unchanged: describing a write is harmless.

### 4. Automatic `Idempotency-Key`

- In `buildHeaders`, when the operation declares an `Idempotency-Key`
  header (required or optional) and the caller gave none (or only
  whitespace), generate a UUID v4 with `crypto/rand` (no dependency). A
  caller-supplied key is kept as is.
- When the call fails, the error message includes the generated key and
  tells Claude to reuse it to resend the same request (the API refuses a
  reused key with a different body, HTTP 409), and to omit it if the body
  changes: `… (Idempotency-Key générée : <uuid> — réutilisez-la pour
  relancer la même requête ; si le corps change, omettez-la)`.
- The "en-tête requis manquant" error no longer fires for
  `Idempotency-Key`; it still fires for other required headers
  (e.g. `If-Match`).

## Testing

Go unit tests:

- `tools/list` includes `annotations` with the expected hints, and omits
  the field for a tool without annotations.
- Read-only mode: `xalantis_call_operation` absent; search returns only GET;
  area counts only count GET operations.
- `xalantis_read_operation` rejects a POST (both messages);
  `xalantis_call_operation` rejects a GET. Neither sends an HTTP request.
- A missing `Idempotency-Key` is generated (UUID v4 format, sent to the API);
  a supplied key is sent unchanged; the generated key appears in the error
  when the API fails.
- `XALANTIS_READ_ONLY` parsing: empty, `1`, `true`, `0`, invalid.
  The parsing lives in a small function in `main` so it is testable.

`test_mcp.py`: GET calls move to `xalantis_read_operation`; one POST goes
through `xalantis_call_operation` without `Idempotency-Key` and the mock
checks the header is present.

## Documentation

README: tools table (11 tools, split into reads and writes), new
`XALANTIS_READ_ONLY` variable, the write warning mentions read-only mode,
and the `Idempotency-Key` paragraph says the key is optional and generated
when missing.

## Out of scope

- Per-operation annotations (MCP annotations are per tool).
- UUID v7 / ULID: the key only needs to be unique; ordering would only
  benefit Xalantis's storage, at ≤ 60 requests per minute.
