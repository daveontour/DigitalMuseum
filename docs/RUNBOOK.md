# Digital Museum — Developer Runbook

Operational and development procedures for the Digital Museum codebase. The intended reader is a developer who has already cloned the repo and read the README.

---

## Table of Contents

1. [Environment Setup](#1-environment-setup)
2. [Daily Dev Workflow](#2-daily-dev-workflow)
3. [Building for Production](#3-building-for-production)
4. [Database Migrations](#4-database-migrations)
5. [Adding a New Data Importer](#5-adding-a-new-data-importer)
6. [Adding or Changing an AI Tool](#6-adding-or-changing-an-ai-tool)
7. [Adding a New AI Provider](#7-adding-a-new-ai-provider)
8. [Log Inspection](#8-log-inspection)
9. [Performance Profiling](#9-performance-profiling)
10. [Troubleshooting](#10-troubleshooting)
11. [Escalation](#11-escalation)

---

## 1. Environment Setup

### Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Go | 1.25+ | `go version` to verify |
| gcc / MinGW-w64 | any | Required for CGO; on Windows use MSYS2 |
| Node.js | 18+ | Only needed for the Electron shell |
| golangci-lint | v2.4+ | Only needed for linting |

### Windows (MSYS2)

```bash
# Install MSYS2 from https://www.msys2.org, then in a MINGW64 shell:
pacman -S mingw-w64-ucrt-x86_64-gcc

# Verify gcc is on PATH
gcc --version
```

Always build from a **MINGW64** shell (not MSYS or UCRT64) to avoid DLL resolution issues with CGO.

### Configuration file

```bash
cp .env.example .env
```

Minimum required fields in `.env`:

```env
SQLITE_PATH=./bin/data/myarchive.sqlite
ANTHROPIC_API_KEY=sk-ant-...         # at least one AI key required
```

The server reads `.env` from the executable directory or the working directory. In Electron, the file at `%APPDATA%\Digital Museum\.env` is layered on top.

---

## 2. Daily Dev Workflow

```bash
# Build + start the Go server (auto-reads .env)
make run

# Run tests
make test

# Lint
make lint

# Tidy go.mod after changing dependencies
make tidy
```

The server hot-restarts are **not** automatic — kill and `make run` again after Go source changes.

### Useful dev flags

Set in `.env` for development:

```env
LOG_LEVEL=debug
ENABLE_PPROF=true
```

- `LOG_LEVEL=debug` logs every SQL query and AI tool call.
- `ENABLE_PPROF=true` exposes Go's profiling endpoints on `:6060`.

---

## 3. Building for Production

### Go server only

```bash
# Windows .exe (standard console subsystem)
make build-exe

# Windows .exe (no console window — for Electron packaging)
make build-exe-electron

# Linux amd64 (must run on Linux; CGO cannot cross-compile)
make build-linux
```

Output: `bin/digitalmuseum.exe` (or `bin/digitalmuseum` on Linux).

### Full Electron installer

```bash
make electron-dist
```

Produces `dist/electron/Digital Museum Setup *.exe`. This target:

1. Calls `build-exe-electron` (windowsgui subsystem, stripped debug info).
2. Removes `dist/electron` to avoid stale NSIS archives.
3. Runs `electron-builder` inside `electron/`.

**Common failure:** NSIS `failed creating mmap` — remove `dist/electron/` manually and retry. Antivirus real-time scanning can also cause this; temporarily exclude the directory during the build.

The packaged app reads `electron/.env.defaults` for default settings — not your project-root `.env`. Edit `.env.defaults` for defaults that ship to end users.

---

## 4. Database Migrations

Migrations run automatically at server startup via `MigrateSQLite()` in `internal/database/migrate.go`.

There are two layers:

- **`schemaDDL()`** — full `CREATE TABLE IF NOT EXISTS` DDL for fresh installs. New tables go here.
- **`MigrateSQLite()`** — incremental helper functions for `ALTER TABLE` changes on existing installs. New columns go here.

### Adding a new table

Add a `CREATE TABLE IF NOT EXISTS` block to the `schemaDDL()` slice in `migrate.go`. Keep alphabetical order. The statement runs on every startup and is a no-op if the table already exists.

### Adding a new column

Add a helper function and call it from `MigrateSQLite()`:

```go
func addMyNewColumn(ctx context.Context, db *sql.DB) error {
    _, err := db.ExecContext(ctx, `ALTER TABLE contacts ADD COLUMN foo TEXT`)
    if err != nil {
        msg := strings.ToLower(err.Error())
        if strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists") {
            return nil // already applied
        }
        return fmt.Errorf("add contacts.foo: %w", err)
    }
    return nil
}
```

Then call it inside `MigrateSQLite()`:

```go
if err := addMyNewColumn(ctx, db); err != nil {
    return err
}
```

Never modify an existing helper function — always add a new one.

### Rollback

SQLite has no transactional DDL for `ALTER TABLE`. If a migration has bad data, write a corrective follow-up helper rather than reverting the schema. For the billing DB, follow the same pattern in `migrate_billing.go`.

---

## 5. Adding a New Data Importer

Each importer lives under `internal/import/<source>/`. Pattern to follow:

1. **Create the package** `internal/import/mysource/` with an `Importer` struct that accepts a `*sql.DB` and a `userID int64`.
2. **Implement** a `Run(ctx context.Context, opts Options, progress chan<- string) error` function that:
   - Reads source data (ZIP, directory, IMAP, etc.)
   - Inserts rows with explicit `user_id` on every `INSERT`.
   - Sends human-readable status strings to `progress` for SSE streaming.
   - Respects `ctx.Done()` for cancellation.
3. **Wire a handler** in `internal/handler/` with an HTTP endpoint (e.g. `POST /import/mysource`).
   - Capture `userID` from context **before** launching the goroutine:
     ```go
     uid := appctx.UserIDFromCtx(r.Context())
     go func() {
         ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
         importer.Run(ctx, opts, progress)
     }()
     ```
4. **Register the route** in `internal/api/router/`.
5. **Add the import type** to the `import_control_last_run` seed and the UI's import type selector.
6. Write at least a smoke test that imports a small fixture ZIP and checks row counts.

---

## 6. Adding or Changing an AI Tool

All tool definitions are in `internal/ai/`. Each tool is a struct implementing the tool interface with a JSON schema input and a text output.

### Add a new tool

1. Define the tool name constant and JSON schema in `internal/ai/tools.go` (or a new file in the package).
2. Implement the executor in `internal/ai/executor.go` — a `case` block that runs the SQL query and returns plain text.
3. Assign a `ToolAccessRule`:
   ```go
   // Enabled = owner (master-key) session; VisitorEnabled = visitor session
   "my_new_tool": {Enabled: true, VisitorEnabled: false},
   ```
4. Add the tool to `GetToolDefinitions()` so it is included when building requests to Claude/Gemini.
5. Update `SPECIFICATION.md` §8.2 table to document the new tool.

### Tool safety checklist

- All SQL **must** include `AND user_id = $N`.
- Return plain text, not JSON (the LLM sees the raw string).
- Pam Bot uses a subset of tools — if the new tool is appropriate for memory-companion sessions, add it to `GetPamBotToolDefinitions()` too.
- Max 15 tool-call iterations per request is enforced by the loop in each provider; do not change this without performance testing.

---

## 7. Adding a New AI Provider

1. Create `internal/ai/<provider>.go` implementing the `Provider` interface (same interface as `claude.go` and `gemini.go`).
2. Add config fields to `internal/config/config.go` (base URL, API key, model name).
3. Register the provider in the factory function that selects by the `provider` field on `POST /chat/generate`.
4. Add usage-event recording (`llm_usage_events` insert) in the same best-effort pattern used by Claude/Gemini — wrap in a goroutine and never let a billing failure propagate to the caller.
5. Update the UI provider selector and the `README.md` tech stack section.

---

## 8. Log Inspection

Logs are structured and written to stdout (and `bin/data/app.log` when running via Electron).

```bash
# Tail the log file (Electron mode)
tail -f bin/data/app.log

# Filter for errors only
grep '"level":"error"' bin/data/app.log

# Filter for a specific user's requests (replace 42 with user_id)
grep '"user_id":42' bin/data/app.log

# Filter for AI tool calls
grep 'tool_call' bin/data/app.log
```

Set `LOG_LEVEL=debug` in `.env` to see SQL queries and full request bodies (avoid in production — logs will contain PII).

Log fields of interest:

| Field | Meaning |
|---|---|
| `method` / `path` / `status` | HTTP request details |
| `duration_ms` | Request duration |
| `user_id` | Authenticated user (0 = unauthenticated) |
| `tool` | AI tool name when a tool is invoked |
| `provider` | `claude` or `gemini` |

---

## 9. Performance Profiling

Enable pprof in `.env`:

```env
ENABLE_PPROF=true
```

The profiling server starts on `:6060`. Access it while the server is running:

```bash
# CPU profile (30 seconds)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Heap snapshot
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine dump (useful for detecting goroutine leaks from import jobs)
curl http://localhost:6060/debug/pprof/goroutine?debug=1
```

Common hotspots:

- **Thumbnail generation** (ImageMagick subprocess) — check `internal/import/thumbnails/`.
- **Vector embedding** (sqlite-vec) — the `email_embeddings` and `message_embeddings` tables.
- **Large email imports** — IMAP import runs sequentially; each message is a DB insert. Use `LOG_LEVEL=debug` to see per-message timing.

---

## 10. Troubleshooting

### `cgo.exe: exit status 2` on Windows

This is a toolchain/PATH problem, not a code bug.

1. Confirm you are in a MINGW64 shell: `echo $MSYSTEM` should print `MINGW64`.
2. Check `gcc --version` returns the MinGW-w64 version, not MSVC or Cygwin.
3. Try `make build-exe` — the Makefile prepends gcc's directory to PATH automatically.
4. If `cc1.exe` errors about missing DLLs, add the compiler `bin/` directory to the **system** PATH (not just user PATH).
5. Update Git for Windows: `git update-git-for-windows` — old Git Bash has been reported to break CGO while cmd/PowerShell works fine.

See [golang/go#75838](https://github.com/golang/go/issues/75838) for further workarounds.

### `sqlite3.h` not found

The `cgo-compat/sqlite3.h` shim must be visible. Build via `make` targets — the Makefile sets `CGO_CFLAGS` correctly. Running bare `go build` without those flags will fail.

### Server starts but AI chat fails

1. Check `.env` has a valid API key for the selected provider.
2. Try a different provider (`"provider": "gemini"` vs `"provider": "claude"`).
3. Check the log for `"level":"error"` near the chat request.
4. Confirm network access to `api.anthropic.com` / `generativelanguage.googleapis.com`.
5. If using Ollama: confirm `ollama serve` is running and `LOCALAI_BASE_URL` matches.

### Import job gets stuck

1. Check `GET /import/status` — if `running: true` indefinitely, the goroutine may have panicked silently.
2. Restart the server — the singleton import job guard resets on restart.
3. Check the log for `panic` or `error` around the import path.
4. For IMAP: test credentials with `POST /imap/test` first.
5. For large ZIPs: ensure `TUS_MAX_UPLOAD_GB` is large enough and the OS temp directory has sufficient space.

### Electron app opens a blank window

1. Check whether the Go server started — look for `"msg":"server listening"` in the log.
2. The Electron app waits for the server to respond on the configured port. If Go fails to start (usually CGO build issue), the window stays blank.
3. Open Electron DevTools (`Ctrl+Shift+I`) and check the console for network errors.
4. Confirm `bin/digitalmuseum.exe` exists (run `make build-exe` if not).

### NSIS packaging fails with `failed creating mmap`

```bash
rm -rf dist/electron
make electron-dist
```

If it fails again, temporarily disable antivirus real-time scanning on the `dist/` directory during packaging.

### Keyring data inaccessible after restart

The keyring master key is stored only in RAM — it is cleared on server restart. Users must unlock the keyring again via the UI after each restart. This is by design.

Do **not** change `KEYRING_PEPPER` in `.env` after initial setup. Changing it makes all existing encrypted keyring entries permanently unreadable.

### pprof not reachable

Confirm `ENABLE_PPROF=true` is in `.env` and the server restarted after the change. The pprof server binds to `localhost:6060` only — it is not exposed by the main HTTP server.

---

## 11. Escalation

| Symptom | First check | Owner |
|---|---|---|
| Data loss / corruption | SQLite file integrity: `sqlite3 <db> "PRAGMA integrity_check;"` | Data layer |
| API key billing charges unexpected | `GET /admin/users/{id}/billing` or `/api/llm-usage/me/bill.pdf` | Admin console |
| CGO linker errors blocking CI | See §10 and [go#75838](https://github.com/golang/go/issues/75838) | Build toolchain |
| sqlite-vec query panic | Check `internal/ai/` for vec query and update sqlite-vec bindings | AI / DB layer |
| Electron packaging regression | Clean `dist/electron`, update `electron-builder`, check Node version | Electron / infra |
