# Digital Museum — CLAUDE.md

## Project Overview

Digital Museum is an AI-powered digital archive platform packaged as an
**Electron desktop app**. Each registered user owns one archive containing their entire
digital life (emails, messages, photos, Facebook, iMessage, WhatsApp, documents),
imported into a SQLite database and queryable through an AI chat interface. Any
admin-configured AI model (routed through OpenRouter — Claude, Gemini, DeepSeek, or any
other OpenRouter-supported model) and a local Ollama/Gemma4 model can access the data
via a tool-calling layer and answer questions, adopt personas, and explore the archive
conversationally.

Additional features beyond the core chat interface include:
- **Have-a-Chat** — two-voice conversation sessions (two AI personas talking to each other)
- **Interviews** — structured Q&A sessions driven by AI, saved for review
- **Identity Profile Wizard** — AI-guided wizard that builds a textual profile of the archive subject
- **Background Jobs** — per-user scheduled maintenance tasks (thumbnail generation, embedding, etc.)
- **Vector Similarity Search** — embedding-based similarity search over messages, emails, and media

## Tech Stack

- **Desktop shell:** Electron (Node.js) — `electron/main.js` manages the Go server, MCP tools server, and Ollama processes, system tray, and IPC
- **Backend:** Go 1.25, Chi v5 router, `database/sql` with `github.com/mattn/go-sqlite3` (CGO)
- **Frontend:** Vanilla JavaScript (no framework), marked.js, highlight.js, Font Awesome
- **Database:** SQLite (two files — main app DB and billing DB); vector fields use `sqlite-vec`
- **AI Providers:** A single OpenRouter adapter (`internal/ai/openrouter.go`) backs every admin-configured AI model (managed in Configuration → AI Models, seeded by default with Claude, Gemini, DeepSeek, and ChatGPT presets — any OpenRouter-supported model can be added), plus local Ollama (`gemma4`) via native Ollama API for fully offline use and embeddings
- **AI Tooling:** every tool the LLM can call is discovered at runtime from one or more MCP
  (Model Context Protocol) servers — the bundled `cmd/mcpserver` plus any owner-added additional
  ones — rather than defined statically in the main app; see [MCP Tools Server & AI Tool Discovery](#mcp-tools-server--ai-tool-discovery)
- **Module:** `github.com/daveontour/aimuseum`

## Project Layout

```
electron/
  main.js           ← Electron main process: spawns Go server + MCP server + Ollama, IPC handlers, tray
  preload.js        ← IPC bridge (contextBridge) exposed to renderer pages
  loading.html      ← Splash screen shown while Go server starts
bin/
  digitalmuseum.exe     ← Compiled Go server
  digitalmuseum-mcp.exe ← Compiled MCP tools server (every AI tool)
  Ollama/               ← Bundled Ollama executable
  ImageMagick/          ← Bundled ImageMagick for thumbnail generation
cmd/
  server/           ← HTTP server entry point (main.go)
  mcpserver/        ← MCP tools server — every AI tool lives here, see [MCP Tools Server & AI Tool Discovery](#mcp-tools-server--ai-tool-discovery)
  mcpquery/         ← Dev CLI for querying the MCP tools server directly (list tools / call one)
internal/
  ai/               ← OpenRouter adapter (backs every admin-configured model) & LocalAI (Ollama)
                       providers, MCP client/executor for tool-calling — tool schemas are
                       discovered from cmd/mcpserver at runtime, not defined here
  api/router/       ← Route wiring (router.go)
  appctx/           ← Shared context key (ContextKeyUserID / UserIDFromCtx)
  config/           ← Env-var config loading
  crypto/           ← Encryption / key derivation (keyring scoped by user_id)
  database/         ← Connection pool, migrations
  handler/          ← HTTP request handlers (~59 files)
  keystore/         ← RAM master key (unlocks encrypted data per session)
  middleware/        ← Logger, Recoverer, AuthMiddleware
  model/            ← Shared data types / DTOs
  repository/       ← Database access via database/sql (~34 repos; most user-scoped — a few
                       deployment-wide config tables, e.g. ai_models/guide_topics/suggestions/
                       mcp_servers, are the exceptions)
  service/          ← Business logic (~43 services)
  service/background_jobs/ ← Job definitions, registry, and scheduler
  sqlutil/          ← SQLite dialect helpers: IsSQLite(), ParseSQLiteDatetime(), Int64IN()
static/
  css/              ← museum_of.css (all styles, ~8000 lines)
  data/             ← voice_instructions.json, seed JSON files
  images/           ← Voice persona images
  js/museum/        ← Frontend modules (~33 JS files)
templates/          ← index.template.html (SPA), login.html, share.html, plus 3 others
sqlc/               ← schema.sql (full DB schema reference), sqlc.yaml
```

## Build & Run

```bash
# Run dev server (reads .env automatically)
make run                          # go run ./cmd/server

# Build binaries
make build-exe                    # bin/digitalmuseum.exe
make build-mcp-exe                # bin/digitalmuseum-mcp.exe — the MCP tools server; required for
                                   # tool-calling in dev (see MCP Tools Server & AI Tool Discovery)
# Do not use bare `go build ./...` — CGO needs -I./cgo-compat (see Makefile); plain go build fails on sqlite3.h
# Fix: `make build-exe`, `source scripts/cgo-env.sh`, or `./scripts/build-exe.sh` (see .cursor/rules/build-cgo.mdc)
# (build-mcp-exe/build-mcp-exe-electron and build-mcpquery need the same CGO setup — they also link go-sqlite3)

# Tests / lint
make test
make lint                         # golangci-lint v2.4+ (Go 1.25): go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
make tidy                         # go mod tidy
```

To run the full Electron app in dev mode: `make electron-dev` (builds `bin/digitalmuseum.exe`
and `bin/digitalmuseum-mcp.exe`, then `cd electron && npm install --prefer-offline && npx electron .`).
Requires Node.js. The Go server and MCP server are spawned automatically.

## Building the Installable Package (Electron Installer)

```bash
make electron-dist
```

Produces a Windows NSIS installer at `dist/electron/Digital Museum Setup *.exe`. This target
(`Makefile`):
1. Builds both Go executables in the console-hidden, Electron-packaging configuration:
   `build-exe-electron` → `bin/digitalmuseum.exe`, `build-mcp-exe-electron` →
   `bin/digitalmuseum-mcp.exe` (same CGO requirements as `build-exe`).
2. Verifies both `.exe` files exist (fails fast with a clear message otherwise).
3. Removes `dist/electron/` first — a stale/locked `*.nsis.7z` from a previous build otherwise
   makes NSIS fail with `failed creating mmap` (this can also be caused by antivirus real-time
   scanning; temporarily excluding the directory helps if the clean-first step alone doesn't).
4. Runs `cd electron && npm install --prefer-offline && npx electron-builder`, driven by
   `electron/electron-builder.yml`.

**Prerequisite — bundled third-party binaries (`bin/` is entirely gitignored — untracked, so
these must be placed there manually before packaging; they are not produced by any `make`
target):**

| Path | Binary | Used by |
|------|--------|---------|
| `bin/ImageMagick/` | Portable Windows ImageMagick build (needs `magick.exe`) | `internal/import/thumbnails` (thumbnail generation), invoked as the relative command `bin/ImageMagick/magick` from the app's working directory |
| `bin/Ollama/` | Standalone Windows Ollama build (`ollama.exe`) | The two Ollama daemons Electron spawns — see [Local AI — Ollama / Gemma4](#local-ai--ollama--gemma4) |
| `bin/exiftool/` | ExifTool's Windows "stand-alone executable" distribution (`exiftool.exe` + its `exiftool_files/` folder) | `internal/imagetagsync` (image tag read/write) |
| `bin/FaceRecognizer/` | `onnxruntime.dll` (from [microsoft/onnxruntime releases](https://github.com/microsoft/onnxruntime/releases), `onnxruntime-win-x64-*.zip`) + `models/detection.onnx` and `models/recognition.onnx` (from [huggingface.co/immich-app/buffalo_l](https://huggingface.co/immich-app/buffalo_l) — `detection/model.onnx` and `recognition/model.onnx`; these are InsightFace's `buffalo_l` weights, **non-commercial-research-use only** unless separately licensed from InsightFace) | `cmd/facerecognizer` (built by `make build-facerecognizer-exe`, see below) |

`electron/electron-builder.yml`'s `extraResources` bundles both compiled Go `.exe` files, the
four third-party binary folders above, `templates/`, `static/` (excluding a few local/private
data files — `email_classifications.json`, `email_matches.json`, `exclusions.json`,
`identity_and_health.txt`), and `electron/.env.defaults` (packaged as `.env` — this is the file
to edit for defaults shipped to end users, **not** the project-root `.env`, which is dev-only and
never bundled). NSIS options (installable directory choice, desktop/start-menu shortcuts,
installer/uninstaller icon) and the app icon (`electron/build/icon.ico`) are also set there.

## Configuration (`.env`)

The server reads `.env` from the working directory (set by Electron to the project root
in dev mode, or the install root in packaged mode). User-editable settings live in
`%APPDATA%\Digital Museum\.env` and are layered on top.

**OpenRouter, Tavily, and RunPod API keys are NOT read from `.env`.** They are configured
only from Configuration → API Keys in the running app (archive owner's saved key, stored in
the `users` table — see `internal/repository/user_llm.go`), with no server-wide fallback.
(ElevenLabs still supports an optional env-configured default via `ELEVENLABS_API_KEY`.)

| Variable | Required | Description |
|----------|----------|-------------|
| `SQLITE_PATH` | Yes | Absolute path to the main SQLite database file |
| `ADMIN_SQLITE_PATH` | No | Billing/admin SQLite file; default `<exeDir>/data/admin.sqlite`. Optional override: absolute as-is, relative resolved against exe dir |
| `HOST_PORT` | No | HTTP listen port (default: 8000; Electron overrides to 8081) |
| `PAGE_TITLE` | No | Browser page title (default: `Digital Museum of SUBJECT_NAME`) |
| `LOCALAI_BASE_URL` | No | Ollama chat base URL, e.g. `http://localhost:11434` |
| `LOCALAI_EMBEDDING_BASE_URL` | No | Ollama embedding base URL (default `http://127.0.0.1:11435`); separate daemon from chat |
| `LOCALAI_MODEL_NAME` | No | Default: `local-model` (use `gemma4` for Ollama) |
| `LOCALAI_API_KEY` | No | Not required by Ollama; kept for compatibility |
| `LOCALAI_EMBEDDING_MODEL` | No | Falls back to `LOCALAI_MODEL_NAME` if empty |
| `LOCALAI_NUM_CTX` | No | Per-request `num_ctx` sent to Ollama under `options`. Empty / 0 / non-positive omits the field so Ollama uses its server-side default (set via `OLLAMA_NUM_CTX` when Electron starts the daemon). |
| `GMAIL_CLIENT_ID` | No | Google OAuth 2.0 Desktop App client ID |
| `GMAIL_CLIENT_SECRET` | No | Google OAuth 2.0 client secret |
| `GMAIL_REDIRECT_URL` | No | Set automatically by Electron at startup |
| `MCP_SERVER_URL` | No | MCP tools server endpoint, e.g. `http://127.0.0.1:8082/mcp` (see [MCP Tools Server & AI Tool Discovery](#mcp-tools-server--ai-tool-discovery)). Set automatically by Electron at startup. Unset means no tools are available at all — every AI tool lives in `cmd/mcpserver`, so `go run ./cmd/server` outside Electron needs `cmd/mcpserver` running too if you want tool-calling in dev. |
| `MCP_AUTH_TOKEN` | No | Shared-secret bearer token required by the MCP server; must match between it and the Go server. Set automatically by Electron (a random token per launch); set manually only when running `cmd/mcpserver` standalone for development. |
| `KEYRING_PEPPER` | No | Secret for encryption key derivation (optional in current build) |
| `LOG_LEVEL` | No | Go server log level: `debug`, `info`, `warn` (default), `error` |
| `SESSION_COOKIE_SECURE` | No | Set `true` for HTTPS deployments |
| `DEPLOYMENT_NATURE` | No | `local` (Electron default) or `web` |
| `TEMPLATES_DIR` | No | Override templates directory path |
| `ASSET_STATIC_DIR` | No | Override static assets directory path |
| `ENABLE_PPROF` | No | Set `true` to expose `/debug/pprof` on `:6060` |
| `ADMIN_EMAIL` | No | Email for `/admin` login (not stored in archive SQLite) |
| `ADMIN_PASSWORD` | No | Password for `/admin` login (not stored in archive SQLite) |
| `TLS_CERT_FILE` | No | Path to TLS certificate file (both `TLS_CERT_FILE` and `TLS_KEY_FILE` required for HTTPS) |
| `TLS_KEY_FILE` | No | Path to TLS private key file |
| `TUS_CHUNK_SIZE_MB` | No | Chunk size (MB) for resumable tus uploads (default: 10) |
| `TUS_MAX_UPLOAD_GB` | No | Maximum ZIP/tus upload size in GiB (default: 32, max: 512) |
| `TUS_UPLOAD_DIR` | No | Directory for in-progress tus upload chunks (default: OS temp on Windows) |
| `ATTACHMENT_ALLOWED_TYPES` | No | Comma-separated MIME types to import (empty = all) |
| `ATTACHMENT_MIN_SIZE` | No | Minimum attachment size in bytes (default: 0) |
| `FILESYSTEM_IMPORT_EXCLUDE_PATTERNS` | No | Comma-separated glob patterns to skip during filesystem import |
| `GUIDE_TOPICS_CONFIG_PATH` | No | Override path to the guide topics seed JSON (default: `ASSET_STATIC_DIR/data/guide_topics.json`) |
| `AI_MODELS_CONFIG_PATH` | No | Override path to the AI models seed JSON (default: `ASSET_STATIC_DIR/data/ai_models.json`) |
| `GUIDE_TOPICS_RELOAD_FROM_FILE_ON_STARTUP` | No | Set `true` to delete all `guide_topics` rows on server startup and reload from the seed JSON file (default: insert-if-missing only) |
| `SUGGESTIONS_CONFIG_PATH` | No | Override path to the suggestions seed JSON (default: `ASSET_STATIC_DIR/data/suggestions.json`) |

### Import UI Defaults (`DEFAULT_*`)

These env vars pre-fill import dialog fields and are not required:

`DEFAULT_PROCESS_ALL_FOLDERS`, `DEFAULT_NEW_ONLY_OPTION`, `DEFAULT_WHATSAPP_IMPORT_DIRECTORY`,
`DEFAULT_FACEBOOK_IMPORT_DIRECTORY`, `DEFAULT_FACEBOOK_EXPORT_ROOT`, `DEFAULT_FACEBOOK_USER_NAME`,
`DEFAULT_INSTAGRAM_IMPORT_DIRECTORY`, `DEFAULT_INSTAGRAM_EXPORT_ROOT`, `DEFAULT_INSTAGRAM_USER_NAME`,
`DEFAULT_IMESSAGE_DIRECTORY_PATH`, `DEFAULT_FACEBOOK_ALBUMS_IMPORT_DIRECTORY`,
`DEFAULT_FACEBOOK_ALBUMS_EXPORT_ROOT`, `DEFAULT_FILESYSTEM_IMPORT_DIRECTORY`,
`DEFAULT_FILESYSTEM_IMPORT_MAX_IMAGES`, `DEFAULT_FILESYSTEM_IMPORT_CREATE_THUMBNAIL`,
`DEFAULT_IMAGE_EXPORT_DIRECTORY`, `DEFAULT_IMAP_HOST`, `DEFAULT_IMAP_PORT`, `DEFAULT_IMAP_USERNAME`.

## Electron Desktop Shell

`electron/main.js` is the Node.js main process. It:

1. Finds a free port (currently hard-coded to 8081) and spawns `bin/digitalmuseum.exe`
2. Injects `SQLITE_PATH`, `ADMIN_SQLITE_PATH`, `TEMPLATES_DIR`, `ASSET_STATIC_DIR`,
   `GMAIL_REDIRECT_URL` (dynamic), `LOG_LEVEL`, and `MCP_SERVER_URL`/`MCP_AUTH_TOKEN`
   (a random per-launch token, see below) into the Go server's environment
3. Waits for `GET /health` to return 200 before showing the main `BrowserWindow`
4. Manages the system tray, developer tools shortcut, and single-instance lock
5. Manages the Ollama process (`ollama serve`) including health checks and shutdown
6. Spawns and manages `bin/digitalmuseum-mcp.exe` (the MCP tools server, port 8082,
   hard-coded like the Go server's) the same way — see
   [MCP Tools Server & AI Tool Discovery](#mcp-tools-server--ai-tool-discovery)

**IPC channels** (renderer ↔ main via `electron/preload.js`):

| Channel | Direction | Purpose |
|---------|-----------|---------|
| `show-open-dialog` | renderer→main | Native file-open dialog |
| `show-save-dialog` | renderer→main | Native file-save dialog |
| `confirm-continue-without-ai` | renderer→main | Warning dialog when no AI models configured |
| `get-db-path` | renderer→main | Current SQLITE_PATH |
| `get-log-level` | renderer→main | Current LOG_LEVEL |
| `select-db` | renderer→main | Switch database + log level, restart Go server |
| `get-profiles` | renderer→main | List archive profiles from billing DB |
| `create-profile` | renderer→main | Create a new named archive profile |
| `update-profile` | renderer→main | Update profile display name / metadata |
| `get-profile-db-path` | renderer→main | Get SQLite path for a given profile ID |
| `get-admin-data-dir` | renderer→main | Return the admin/billing data directory |
| `suggest-archive-db-path` | renderer→main | Suggest a filesystem path for a new archive |
| `check-ollama-model` | renderer→main | Check if gemma4 is in `~/.ollama/models/` |
| `pull-ollama-model` | renderer→main | Run `ollama pull gemma4`, streams progress |
| `start-ollama` | renderer→main | Start `ollama serve`, wait for health |
| `get-auto-start-local-ai` | renderer→main | Read Ollama auto-start preference |
| `set-auto-start-local-ai` | renderer→main | Persist Ollama auto-start preference |
| `ollama-pull-progress` | main→renderer | Live pull progress lines |
| `status-update` | main→renderer | Loading screen status messages |

**Advanced login panel** (`templates/login.html`) allows users to:
- Switch or create a SQLite database before signing in
- Change the Go server log level
- Start the local Ollama AI (with auto-download of gemma4 if not present)

Selecting a new database writes `SQLITE_PATH` to `%APPDATA%\Digital Museum\.env` and
restarts the Go server via `restartGoServer()` in `electron/main.js`.

## No-Archive ("Minimal") Router Mode

When `SQLITE_PATH` is not set or the file is absent, the router receives a `nil` pool
(`internal/api/router/router.go`, `New()`). In this mode the server serves only: `/health`,
`/api/resolved-main-sqlite-path`, `/api/local-ai/status`, `/login`, `/` (redirects to `/login`),
`/static/*`, the full admin panel (`/admin`, `/admin/login`, `/admin/users`, …, still exempt from
`AuthMiddleware` — the billing DB is always available even with no archive open), and archive
profile management (`/profiles`, `/api/profiles/*`, see Multi-Archive Profile Management below).
All other routes return `503` with a JSON error. This allows the Electron shell to let the
user create or select an archive before the full application starts.

## Multi-Archive Profile Management

The billing DB (`admin.sqlite`) stores a `profiles` table — one row per archive. Each
profile holds a display name and the absolute path to its main SQLite file. The
`ProfileHandler` (`internal/handler/profile_handler.go`) exposes `GET /profiles` (public JSON
list of enabled profiles, no `db_path`), `POST /api/profiles` / `PATCH /api/profiles/{id}` /
`GET /api/profiles/{id}/dbpath` (localhost-only — Electron calls these directly), and
`GET/POST/PATCH/DELETE /admin/profiles*` (admin-panel CRUD). The profile-selection *page* itself
is `GET /` rendering `templates/non_user_init.template.html` (via `TemplateHandler`) rather than
a route on `ProfileHandler`.

From the login screen users can switch between archives or create a new one; switching
writes the new `SQLITE_PATH` to `%APPDATA%\Digital Museum\.env` and restarts the Go
server (via the `select-db` IPC channel). The Electron `get-profiles` / `create-profile`
/ `update-profile` IPC handlers read/write the billing DB directly.

## Local AI — Ollama / Gemma4

`internal/ai/localai.go` implements `ChatProvider` using the **native Ollama API**:

- Chat: `POST {LOCALAI_BASE_URL}/api/chat` with `stream: false`
- Embeddings: `POST {LOCALAI_EMBEDDING_BASE_URL}/api/embed` (separate Ollama daemon; default port 11435)
- Tool arguments arrive as `map[string]any` (not a JSON string — unlike OpenAI compat)
- Token counts read from `prompt_eval_count` / `eval_count` (not `usage.prompt_tokens`)
- No `Authorization` header required
- Model options (temperature, num_ctx) sent under `"options"` key

**Dual Ollama servers (Electron desktop):**

Digital Museum runs **two** Ollama `serve` processes when using the bundled desktop app:

| Daemon | Env URL | Default port | Model | Notes |
|--------|---------|--------------|-------|-------|
| Chat | `LOCALAI_BASE_URL` | 11434 | `LOCALAI_MODEL_NAME` (UI-selectable) | `OLLAMA_NUM_CTX=32768`; `CUDA_VISIBLE_DEVICES=-1` when CPU-only checkbox is set |
| Embedding | `LOCALAI_EMBEDDING_BASE_URL` | 11435 | `LOCALAI_EMBEDDING_MODEL` | GPU allowed; no `CUDA_VISIBLE_DEVICES` override from the UI |

Both daemons are started with `OLLAMA_KEEP_ALIVE=-1`, `OLLAMA_MAX_LOADED_MODELS=1`, and preload their configured model at startup (`POST /api/generate` for chat, `POST /api/embed` for embedding). Apply in **AI & Setup** restarts both servers.

**Context size (`num_ctx`):**

- The **chat** Ollama daemon spawned by Electron is started with `OLLAMA_NUM_CTX=32768`. The embedding daemon does not set `OLLAMA_NUM_CTX`.
- The Go provider also sends `num_ctx` per-request when `LOCALAI_NUM_CTX` is set to a positive integer. When unset / 0, the field is omitted and Ollama applies its server-side default.
- Note: the env-var default only applies to the daemon Electron starts. If users connect to a pre-existing Ollama daemon they started themselves, that daemon's own configuration wins.

**Local AI status API and chat toggle:**

- `GET /api/local-ai/status` — auth-exempt; probes chat and embedding Ollama URLs separately for reachability and configured models. Works on the login page before sign-in (infrastructure fields only). When authenticated, also returns `use_enabled_for_chat` and `chat_available`. Includes `embedding_base_url`, `embedding_server_reachable`, and `embedding_server_error`.
- Per-archive **`local_ai_use_enabled_v1`** in `app_configuration` (via `POST /api/configuration`) controls whether Local AI appears in provider menus and Auto routing; default enabled when unset. See `internal/service/local_ai_use.go`.
- Configuration → **AI & Setup** and the login **Local AI Setup** panel use [`static/js/museum/local-ai-setup.js`](static/js/museum/local-ai-setup.js) and the status API. Browser mode shows server Ollama status; Electron additionally offers start/download via IPC.

## MCP Tools Server & AI Tool Discovery

**Every AI tool runs in a separate standalone MCP server** (`cmd/mcpserver`), not in the main
app — this was originally built as a learning exercise in the Model Context Protocol using
[`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk), then
extended to the full tool set. `internal/ai/provider.go` and `internal/ai/tool_access.go` define
**no static tool list**; the main app discovers what tools exist by calling the MCP server's
`tools/list`.

**Architecture:** Electron spawns `bin/digitalmuseum-mcp.exe` as a fourth managed child process
(peer to the Go server and the two Ollama daemons — same spawn/health-poll/shutdown pattern),
listening on `127.0.0.1:8082` (the MCP Streamable HTTP transport) behind a shared-secret bearer
token Electron generates once per launch (`MCP_AUTH_TOKEN`) and injects into both processes. All
tool query/decrypt logic still lives in `internal/ai/tools.go` (unchanged) — `cmd/mcpserver/main.go`
calls it through exported wrapper functions (`ai.SearchChatMessagesGlobally`,
`ai.GetSensitiveReferenceDocumentsWithPassword`, …), one `server.AddTool(...)` per tool, so there
is exactly one copy of the SQL. The main Go server acts purely as the MCP *client*:
`internal/ai/mcpclient.go`'s `MCPClient` connects lazily on first use, and
`internal/ai/tools.go`'s `NewMCPToolExecutor` is the single `ToolExecutor` every provider's
tool-call loop uses (`openrouter.go` / `localai.go` needed no changes — they've always called
through the `ToolExecutor` interface).

**Tool catalog & discovery (`internal/ai/tool_catalog.go`):** `DefaultToolCatalog()` is a
process-wide, in-memory, refreshable cache of tool schemas — never persisted, since it should
always reflect what the MCP server currently reports. `AllToolMetas()` / `FilterToolDefinitionsForTier()`
(`tool_access.go`) and `GetToolDefinitions()` (`provider.go`) all read from it. It's populated:
- **At startup** — `cmd/server/main.go` kicks off `RefreshWithRetry` in a background goroutine
  (bounded retries, a few seconds apart) right after the background jobs scheduler starts. This
  must not block `/health`: Electron waits for the Go server's `/health` *before* it resolves the
  archive's SQLite path and starts the MCP server, so the MCP server provably isn't up yet the
  instant `cmd/server` boots — the retry is what makes discovery actually converge once it comes
  up moments later.
- **On demand** — the AI Tool Access config tab (below) has a **Refresh Tools** button
  (`POST /api/settings/llm-tools-access/refresh-catalog`).
- Chat generation itself never triggers a refresh; it uses whatever's cached, same as before.

**AI Tool Access tab (Configuration → AI Tool Access, `internal/handler/llm_tools_access_handler.go`,
`static/js/museum/modals-settings.js` `Modals.LLMToolsAccess`):** lists every tool the catalog
currently knows about with per-tier (master key / visitor key) enable checkboxes, saved as an
encrypted `ToolAccessPolicy` (`internal/ai/tool_access.go`). Each row shows a **Server** column
(the tool's name prefix — "Digital Museum" for the bundled server, or the additional server's
configured name — see Additional MCP Servers below) so an admin can see where a tool comes from.
**A tool with no saved policy row is denied by default** (`PolicyAllows` — unchanged) — so a
newly discovered tool is never offered to the LLM until an admin explicitly ticks a box and
saves; the UI marks such rows with a **New** badge (`GET`'s `configured` field, distinct from
`enabled`/`visitor_enabled`, both false either way). A related **Tool Test** tab
(`internal/handler/llm_tools_test_handler.go`, `static/js/museum/modals-tool-test.js`) lets an
owner run any enabled tool directly with arbitrary arguments to see its raw result, for debugging.

**Additional (user-added) MCP servers — `internal/ai/mcp_registry.go`, `internal/service/mcp_servers_service.go`,
`internal/repository/mcp_servers_repo.go`, `internal/handler/mcp_servers_handler.go`:** beyond the
bundled `cmd/mcpserver`, an owner can register arbitrary third-party MCP servers (any http(s)
Streamable-HTTP endpoint + optional bearer token) from Configuration → **MCP Servers**
(`static/js/museum/modals-mcp-servers-config.js`, owner-only — `POST/PATCH/DELETE /api/mcp-servers`,
`POST /api/mcp-servers/test`). Rows live in the deployment-wide `mcp_servers` table (one reserved
`is_builtin` row for the bundled server, seeded once by `database.SeedBuiltinMCPServerIfMissing`
and never deletable — only disable-able; its `endpoint_url`/`auth_token` columns are unused since
its real connection is Electron-managed). `MCPServersService` pushes the current row list into
`ai.DefaultMCPRegistry()` on every load/write, which resolves each tool call to the right server:
- The bundled server is **trusted** and its tools are unprefixed.
- Every additional server is **untrusted** and its tools are namespaced `"<server_name>:<tool_name>"`
  (`server_name` may not contain `:`) so the catalog, tool-access policy, and executor can tell
  them apart — `MCPRegistry.Resolve()` strips the prefix before the real `tools/call`.
- `NewMCPToolExecutor` (`internal/ai/tools.go`) attaches the secret hidden args
  (`ArgMasterPassword`, `ArgTavilyKey`) **only** when the resolved server is trusted — an
  additional server never receives them, regardless of what a tool there happens to be named.
- `ToolCatalog.Refresh` discovers tools from every registered server independently, so one
  unreachable additional server doesn't blank out the bundled server's tools (or vice versa).

`cmd/mcpquery` is a small dev CLI (`MCPClient`-based, same client the app uses) for listing or
calling tools directly against any MCP server — bundled or additional — without going through
the app or an LLM; see its `-h` for usage.

**Hidden per-call context (`internal/ai/mcp_args.go`):** since a `tools/call` crosses a process
boundary, `NewMCPToolExecutor` attaches the caller context that Go's request `ctx` used to carry
automatically, as extra `tools/call` arguments the LLM never sees, and `cmd/mcpserver`'s
`withCallerContext` reconstructs `appctx.UserID` / `appctx.VisitorAccess` from them before calling
into `internal/ai`. `ArgUID` and the `ArgVisitor*` flags are attached to **every** call (cheap, not
secret). `ArgMasterPassword` (get_reference_document / get_sensitive_reference_document) and
`ArgTavilyKey` (search_tavily) are attached **only** to the tool names that need them
(`mcpSecretArgTools` in `tools.go`) — both are session/visitor-resolved values
(`ChatService.effectiveOpenRouterConfig`, `keystore.SessionMasterStore`), not static env config,
so unlike `LOCALAI_*` they can't just be read from the MCP server's own environment.

**Security note (deliberate):** the sensitive-document tool decrypts data using the in-RAM session
master password. Since it runs in a separate process, that password is sent as a `tools/call`
argument over the local HTTP transport instead of staying purely in-process — mitigated by binding
the MCP server to `127.0.0.1` only and requiring `MCP_AUTH_TOKEN` on every request. Never logged.

**Multi-process SQLite:** the MCP server opens its own `*sql.DB` against the same `SQLITE_PATH`
and sets `_journal_mode=WAL` on its DSN (the main server's pool, `internal/database/db.go`, is a
single connection with no WAL pragma — safe on its own, but two processes touching the file
concurrently need WAL to avoid "database is locked" errors under the default rollback journal).
WAL is a property of the database file itself, so this needs no change on the main server's side.

**Dev / build:** `make build-mcp-exe` / `build-mcp-exe-electron` build `bin/digitalmuseum-mcp.exe`
the same way `build-exe` / `build-exe-electron` build the main server (same CGO requirements — it
links `go-sqlite3` too); `electron-dev` and `electron-dist` depend on the relevant target.
`electron/electron-builder.yml` bundles the binary via `extraResources`, same as
`digitalmuseum.exe`. Electron also passes `LOCALAI_*` env vars into the MCP server's spawn env
(needed by the two similarity-search tools' direct Ollama embedding calls).

## AI Models & the OpenRouter Adapter

`internal/ai/openrouter.go` implements `ChatProvider` (`OpenRouterProvider`) using
OpenRouter's OpenAI-compatible **Chat Completions API**:

- Endpoint: `POST https://openrouter.ai/api/v1/chat/completions`
- Authentication: `Authorization: Bearer` header
- Native `tools` / `tool_calls` loop (same max iterations as every provider, `maxToolCallIterations` in `internal/ai/provider.go`); tool-call arguments arrive as a JSON string requiring `json.Unmarshal` (same shape as OpenAI's API)
- Token counts from `usage.prompt_tokens` / `usage.completion_tokens`
- One `OpenRouterProvider` instance backs one admin-configured AI model; its `providerKey` field (the model's admin-defined key, e.g. `"claude"`) is stamped into `LLMUsage.Provider` so billing keeps a distinct bucket per model
- Returns `nil` from constructor when the resolved API key or model slug is empty

**AI Models are admin-managed, not hardcoded.** `internal/service/ai_models_service.go` +
`internal/repository/ai_models_repo.go` + `internal/handler/ai_models_handler.go` implement
a deployment-wide CRUD table (`ai_models`: `key`, `display_name`, `model_slug`, `enabled`,
`sort_order`) managed from Configuration → **AI Models**. Seeded on first run from
`static/data/ai_models.json` (insert-if-missing, same pattern as Guide Topics/Suggestions)
with four defaults — `claude`, `gemini`, `deepseek`, `openai` — each mapped to an OpenRouter
model slug (e.g. `anthropic/claude-sonnet-4.5`). Admins can add, edit, disable, delete, and
reorder any model; the `key` a chat request selects (`"provider"` in `POST /chat/generate`
and `Have-a-Chat` requests) is looked up against this table by
`ChatService.effectiveProviderByKey` — there is no fixed set of valid provider names beyond
`"auto"` and `"localai"`. `AIModelsService` caches reads in-process (invalidated on every
write) since a model is resolved by key on every chat generation.

Routes: `GET /api/ai-models` (admin, all rows), `GET /api/ai-models/available` (enabled-only,
`sort_order`, used by every provider picker in the app via the shared `AIModels` JS cache
module in `static/js/museum/ai-models.js`), `POST /api/ai-models`, `PATCH /api/ai-models/{id}`,
`DELETE /api/ai-models/{id}`.

The OpenRouter API key backing every model is configured only via Configuration → **API
Keys** — there is no `OPENROUTER_API_KEY` env var. One shared key (not per-model) is stored
per archive owner (`users.user_openrouter_api_key`) and optionally overridden per visitor
session; `ChatService.effectiveOpenRouterConfig` resolves owner → visitor (no server tier).
Tavily and RunPod keys follow the same owner → visitor precedence and are also
env-var-free; ElevenLabs is the only one of the four that still supports a server-side
`ELEVENLABS_API_KEY` fallback. The API Keys tab shades the required OpenRouter key field
red and the optional keys yellow until each is configured, green once set, with an eye-icon
toggle to reveal what's typed; actions that need a hosted AI provider before one is
configured show a warning dialog routing to this tab (`static/js/museum/app.js`'s
`blockIfChatProviderNotReady`), and the welcome dialog shows the same warning when no
OpenRouter key resolves for the owner.

Configuration → **Model Catalog** (`static/js/museum/model-catalog.js`,
`GET /api/ai-models/catalog`) lets an owner search/filter OpenRouter's full public model
list (name, cost per 1M tokens, context length, full spec on click) and hand a picked
model to the AI Models "Add model" form instead of typing a slug by hand.

## Admin User Management

`internal/handler/admin_user_handler.go` provides a web-based admin panel at `/admin`:

- **Separate session:** `dm_admin_sid` cookie, 2-hour TTL, RAM-only (not DB-backed)
- **Authentication:** `POST /admin/login` validates `ADMIN_EMAIL` / `ADMIN_PASSWORD` from server config only (not archive `users` rows)
- **Archive users:** first real owner is always `users.id = 2`; `users.id = 1` is a reserved inactive placeholder (see `database.seedReservedUserSlot`)
- **Routes:**
  - `GET /admin` — admin SPA page
  - `POST /admin/login` / `POST /admin/logout`
  - `GET/POST /admin/users` — list / create users
  - `PATCH/DELETE /admin/users/{id}` — update / delete user
  - `GET /admin/users/{id}/dashboard` — user archive dashboard
  - `GET /admin/llm-usage/users/{id}/summary|events|timeseries|bill.pdf`
  - `GET /admin/llm-usage/error-events`
  - `GET/PUT /admin/system-instructions` — app-wide LLM system prompts

The admin panel is intentionally **exempt from** `AuthMiddleware` — it uses its own session guard (`requireAdmin`).

## Authentication & Authorisation

### Overview

1. **Authentication** — who is the user? Handled by `AuthService` + `AuthMiddleware` using a DB-backed session cookie (`dm_session`).
2. **Data authorisation** — which rows can they see? Handled by the repository layer adding `AND user_id = $N` to every query.

### Authentication Flow

1. User registers via `POST /auth/register` or logs in via `POST /auth/login`.
2. Passwords are hashed with **argon2id**; `internal/crypto/` provides `HashPassword` / `VerifyPassword`.
3. On successful login, a 32-byte random session ID is stored in `sessions` with a 24-hour TTL.
4. The session ID is set as an `HttpOnly; SameSite=Strict` cookie named `dm_session`.
5. On every subsequent request, `AuthMiddleware` reads the cookie, looks up the session, and injects `user_id` into the request context via `appctx.ContextKeyUserID`.

### Auth Middleware (`internal/middleware/auth.go`)

Unauthenticated requests to non-exempt paths receive a `302` redirect to `/login`
(browser) or a `401 JSON` error (XHR/API calls detected via `Accept` header).

**Exempt routes** (`exemptPrefixes` / `exemptExact` in `internal/middleware/auth.go`):
```
# Prefix matches (any method)
/static/
/share/
/s/
/visitor/
/admin           # has its own session-based auth (dm_admin_sid) — see Admin User Management
/api/profiles    # localhost-only; isLocalhost() guards in the handler itself
/api/quiz/       # unauthenticated "Take the Quiz" login-page feature

# Exact matches
/health
/favicon.ico
/auth/login
/auth/register
/login
/quiz
/profiles
/api/resolved-main-sqlite-path
/api/local-ai/status
```

### Context Key (`internal/appctx/appctx.go`)

```go
uid := appctx.UserIDFromCtx(ctx)  // returns 0 if unauthenticated
```

### Data Scoping — Repository Layer

Every repository method calls `uidFromCtx(ctx)` and appends `AND user_id = $N` to
SELECT/UPDATE/DELETE queries, and includes `user_id = uidVal(uid)` on INSERT.

`uidVal(uid)` returns `nil` (SQL NULL) when `uid == 0`.

The helper `userscope.go` exists in both `internal/repository/` and `internal/importstorage/`.

### Share Token System (`internal/service/archive_share_service.go`)

Visitors access an archive via a share token: `GET /s/{token}` → password check via
`POST /share/{token}` → `authSvc.CreateShareSession()` issues a `dm_session` scoped to
the **owner's** `user_id`. The visitor sees the owner's data through normal repository
filters with no special code paths.

### Keyring (Encryption Layer)

- `dm_session` — authentication (DB-backed sessions table)
- `dm_keyring_sid` — keyring unlock password (RAM store, `SessionMasterStore`)

`internal/crypto/keys.go` scopes all keyring operations by `user_id`.

## SQLite Dialect Notes

The codebase targets SQLite exclusively (via `github.com/mattn/go-sqlite3`). Key rules:

- **`internal/sqlutil/dialect.go`** — `IsSQLite(ctx, db *sql.DB) bool` detects the driver. Always returns `true` currently, but keep dialect branches for forward compatibility.
- **`internal/sqlutil/dbtime.go`** — `ParseSQLiteDatetime()` handles multiple timestamp formats SQLite may store, including the non-standard `"2006-01-02 15:04:05 -0700 -0700"` format that Go's `time.String()` produces for timezones without an alphabetic name.
- **Partial unique indexes**: SQLite requires the `WHERE` clause in upsert conflict targets to match the index's `WHERE` clause. Use `ON CONFLICT (key) WHERE user_id IS NULL DO UPDATE SET …` not just `ON CONFLICT (key) DO UPDATE SET …`.
- **No `::type` casts**: SQLite does not support `$1::jsonb` or `$1::vector`; omit the cast.
- **No `unnest(string_to_array(…))`**: Use a Go-side split + deduplication loop instead.
- **`ON CONFLICT ON CONSTRAINT name`**: PostgreSQL-only syntax. Use `ON CONFLICT(col1, col2)` instead.

## Architecture Patterns

### Adding a New API Endpoint

1. Add the handler method in `internal/handler/<domain>_handler.go`
2. Register the route in `internal/api/router/router.go`
3. Add the service method in `internal/service/<domain>_service.go`
4. Add the repository method in `internal/repository/<domain>_repo.go` (raw `database/sql`)
5. **For data tables:** use `addUIDFilter(q, args, uidFromCtx(ctx))` on SELECT/UPDATE/DELETE, and `uidVal(uidFromCtx(ctx))` on INSERT.

### Adding a New AI Tool

Every AI tool lives in the MCP tools server (`cmd/mcpserver`), discovered by the main app rather
than hardcoded there — see [MCP Tools Server & AI Tool Discovery](#mcp-tools-server--ai-tool-discovery).

1. Add the query/decrypt implementation as a private function in `internal/ai/tools.go`, plus an
   exported MCP-facing wrapper for it (same pattern as `SearchChatMessagesGlobally`,
   `GetEmailsByContact`, …). All SQL must include `AND user_id = $N` via
   `toolsUIDFilter(ctx, q, args)`.
2. Register the tool in `cmd/mcpserver/main.go`'s `registerTools()` — one `server.AddTool(...)`
   call with its JSON-Schema `InputSchema`, calling the exported wrapper from step 1.
3. If the tool needs a secret (a master password, an API key) that's session/visitor-resolved
   rather than static env config, add a hidden arg constant in `internal/ai/mcp_args.go`, attach
   it in `NewMCPToolExecutor` only for that tool name (`mcpSecretArgTools`), and read it in the
   MCP server's tool handler — never log it.
4. Rebuild `cmd/mcpserver` (`make build-mcp-exe`) and restart it, then click **Refresh Tools** in
   Configuration → AI Tool Access (or restart the whole app) to pick it up. **The tool is denied
   to every session by default** until an admin ticks a box for it there and saves — no extra step
   needed to gate it off.

### Database Migrations

- Migration logic lives in `internal/database/` Go files
- Applied automatically at server startup via `database.Migrate()`
- **Never modify existing migration logic** — always add a new migration function

### Billing Database (LLM Usage)

A **second SQLite file** (billing/admin DB: default `<exeDir>/data/admin.sqlite`, overridable via `ADMIN_SQLITE_PATH`) holds:
- `llm_usage_events` — one row per completed LLM interaction with provider, model, token counts, user snapshot fields, and whether the server API key was used. Billing inserts are best-effort.
- `profiles` — one row per archive (display name + SQLite path), for multi-archive management.

Admin JSON/UI lives under `/admin/llm-usage/…`. Users can download their own PDF bill via
`GET /api/llm-usage/me/bill.pdf?period=current|previous`.

### Chat System

- **Backend:** `POST /chat/generate` → `ChatHandler` → `ChatService.GenerateResponse()`
- System prompt = subject config + voice instructions with `{SUBJECT_NAME}`, `{he}`, `{him}`, `{his}` substituted at runtime
- **Reference doc inlining:** `internal/service/reference_prompt_inline.go` appends any `reference_documents` rows with `include_in_system_prompt = true` directly into the system prompt (decrypts if needed; skips for restricted visitor sessions)
- History: last 30 turns from `chat_turns` table
- Tool loop: up to `maxToolCallIterations` per request in all providers
- **Provider selection:** `"auto"`, `"localai"`, or any `key` from the admin-managed AI Models table (default seed: `"claude"`, `"gemini"`, `"deepseek"`, `"openai"`) in request body
- All AI tool SQL is scoped by `user_id` via `toolsUIDFilter(ctx, q, args)` in `internal/ai/tools.go`

### Have-a-Chat (Two-Voice Conversations)

`HaveAChatHandler` (`internal/handler/have_a_chat_handler.go`) drives sessions where
two AI personas converse with each other about the archive. Sessions are persisted to
`have_a_chat_sessions`. The `ChatService` is reused for both turns; the handler sequences
the turns and passes each response back as the next prompt. Routes: `POST /chat/have-a-chat/turn`
(generate the next turn) and `/api/have-a-chat/sessions*` (save / list / get / export a session).

### Interviews

`InterviewHandler` (`internal/handler/interview_handler.go`) manages structured Q&A
sessions: the AI asks questions, the user answers, and the interview is saved for later
review. State is persisted to `interviews` / `interview_turns` tables via
`InterviewRepo`. Routes under `/interview/…` (not `/api/`-prefixed).

### Identity Profile Wizard

`IdentityProfileHandler` (`internal/handler/identity_profile_handler.go`) is a guided,
multi-step flow that uses `ChatService` to build a textual identity profile of the
archive subject. It writes finished profiles to `complete_profiles` via `CompleteProfileRepo`.
Routes under `/api/identity-profile/…`.

### Background Jobs Scheduler

`internal/service/background_jobs/` contains:
- `definitions.go` — job definitions (thumbnail generation, embedding, etc.)
- `registry.go` — job discovery and registration
- `scheduler.go` — `Scheduler` struct that ticks periodically and runs due per-user jobs

Jobs are persisted to the `background_jobs` table via `BackgroundJobRepo`. The
`BackgroundJobsRunner` handler (`internal/handler/background_jobs_runner.go`) executes
individual jobs on-demand; `BackgroundJobsHandler` exposes control routes under
`/api/background-jobs/…`. The scheduler is started by `cmd/server/main.go` after the
router is constructed.

### Vector Embeddings & Similarity Search

The `EmbeddingService` (`internal/service/`) wraps the Ollama embedding endpoint. A
`MediaTagEmbeddingHelper` pre-computes tag embeddings for `media_items`. The
`MessageSimilarityHandler` (`internal/handler/message_similarity_handler.go`) exposes
embedding-based similarity search. Embedding vectors are stored in `embedding_vector`
columns (type `vector(2560)`) on `emails`, `messages`, `facebook_albums`, and
`facebook_posts` tables using `sqlite-vec`. Metadata for context-window management is
tracked in `message_embedding_meta`.

### Facial Recognition

Detects faces in photos, clusters likely-same-person faces, and lets the archive owner link a
cluster to a `Contact` to name them — from Configuration → Background Jobs (detection/grouping)
and the **People in Photos** sidebar button (review/naming UI, gallery lightbox bounding-box
overlay, and the `find_photos_of_person` AI tool). Engine, background jobs, REST API, and
frontend are all implemented and verified end-to-end against real bundled model files.

**Review/naming REST API (`internal/handler/face_handler.go`, `FaceHandler`):**
`GET /api/faces/clusters` (optional `?named=true|false`), `GET`/`PATCH /api/faces/clusters/{id}`
(detail + link-to-contact, propagates to every member face via
`FaceService.LinkClusterToContact`), `GET /api/faces/clusters/{id}/thumbnail` and
`GET /api/faces/{id}/crop` (JPEG face crops generated on demand via
`service.CropFaceJPEG` — ImageMagick `-crop`, same bundled-binary/subprocess pattern as
`internal/import/thumbnails`, not a stored thumbnail), `PATCH /api/faces/{id}` (move to another
cluster, or detach — `face_cluster_id: null` — as "not this person"), and
`GET /api/media-items/{id}/faces` (per-photo face list consumed by the gallery lightbox
overlay). Gated by `requireVisitorContacts` — the same visitor-tier permission as the Contacts
API, since linking a face to a named Contact is equivalently sensitive identity information.

**Frontend (`static/js/museum/modals-faces.js`, `Modals.Faces`):** a cluster grid (All/Named/
Unnamed filter tabs) → cluster detail panel with a contact search-and-link picker (backed by
`GET /contacts/names`) and live "possible match" suggestion chips for unnamed clusters, plus a
member-face grid with a "not this person" (detach) control per face. The main photo gallery's
lightbox (`Modals.ImageDetailModal` in `modals-media.js`) overlays bounding boxes (and linked
contact names) on the displayed photo, positioned as CSS percentages of a wrapper div sized
exactly to the rendered `<img>` (`.new-image-gallery-detail-image-wrap`), so overlays track the
image responsively with no resize-recalculation JS needed.

**AI tool:** `find_photos_of_person` (`internal/ai/tools.go`'s `findPhotosOfPerson` /
`FindPhotosOfPerson`, registered in `cmd/mcpserver/main.go`) — matches a name against
`contacts.name`/`alternative_names` and returns photos via `media_item_faces.contact_id`. Like
every AI tool, denied to every session by default until an admin enables it in Configuration →
AI Tool Access — a person must actually be named via the review UI before this tool can find
anything for them.

**Engine (`cmd/facerecognizer`):** a separate bundled subprocess (not linked into
`digitalmuseum.exe`/`digitalmuseum-mcp.exe`), following the same "external bundled binary"
principle as ImageMagick/exiftool/Ollama, but as a **long-lived process** rather than
spawn-per-call: model load is not free like ImageMagick's near-instant startup, so
`facerecognizer.exe --serve` starts once per background-job batch and stays resident,
communicating over stdin/stdout via a 4-byte-length-prefixed framing protocol (`ReadFrame`/
`WriteFrame` in `internal/service/facerecognizer/client.go`, shared by both the Go client and
`cmd/facerecognizer`'s serve loop, so the wire format can't drift between the two sides).

- Uses [`github.com/yalue/onnxruntime_go`](https://github.com/yalue/onnxruntime_go) (dlopens
  the bundled `onnxruntime.dll` at runtime rather than static-linking) to run two ONNX models
  under `bin/FaceRecognizer/models/`: `detection.onnx` (SCRFD face detector, buffalo_l's
  `det_10g`) and `recognition.onnx` (ArcFace embedder, buffalo_l's `w600k_r50`, 512-d output —
  see [Building the Installable Package](#building-the-installable-package-electron-installer)
  for where to obtain these). Both models' actual input/output tensor names/shapes were
  confirmed by loading them with `GetInputOutputInfo` before writing the decode logic — see the
  comments atop `cmd/facerecognizer/scrfd.go` and `recognize.go` for the exact names depended on.
- `cmd/facerecognizer/scrfd.go` implements SCRFD's preprocessing (aspect-preserving letterbox
  resize to 640×640, top-left padding, bilinear resize), anchor decode (strides 8/16/32, 2
  anchors/location), and NMS — matching the reference `insightface` `scrfd.py` algorithm.
- `cmd/facerecognizer/align.go` aligns a detected face to ArcFace's fixed 112×112 landmark
  template via a closed-form complex-number least-squares similarity transform (mathematically
  equivalent to `skimage`'s Umeyama-based `SimilarityTransform.estimate` for the non-reflective
  case, which always holds for face landmarks) and inverse-mapped bilinear sampling — matching
  `cv2.warpAffine`'s behavior without depending on OpenCV.
- **Build gotcha (do not skip `-ldflags="-s -w"` for this binary):** an unstripped build was
  confirmed to fail to launch at all on Windows — `"...is not a valid Win32 application"` /
  `"not a valid application for this OS platform"` — from every invocation path tested (Bash,
  PowerShell, `os/exec`), *despite* a structurally valid PE header (correct machine type,
  correct PE32+ magic), while the exact same code ran fine under `go run`. This is the known
  Go 1.25 + CGO + Windows PE bug already documented by this Makefile's `WINDOWS_STRIP_LDF`
  variable (see its comment, and https://go.dev/issue/75121) — `make build-facerecognizer-exe`
  already applies it; never build this binary with a bare `go build` for a launchable artifact.

**Schema (`internal/database/migrate.go`):** `face_clusters` (one row per believed-same-person
group; `contact_id` set once a user links it), `media_item_faces` (one row per detected face —
fractional `bbox_x/y/w/h`, `landmarks` JSON, `embedding_model`, `face_cluster_id`, denormalized
`contact_id`), `media_items.faces_processed`, and a dedicated `face_embeddings` `sqlite-vec`
table (**512-d, not the 768-d used by every other embedding table** — added via its own
`ensureFaceEmbeddingsVecTable` function, deliberately not folded into
`ensureSQLiteVecEmbeddingTables`'s hardcoded-768-d table list). Embeddings are L2-normalized
before storage (`NormalizeFaceEmbedding` in `internal/service/face_embedding.go`) so vec0's
built-in L2 "MATCH" distance behaves like cosine similarity.

**Clustering (`internal/service/face_service.go`):** `decideClusterAssignment` is a pure,
unit-tested function choosing, per face, whether to auto-join an already-named cluster (tight
distance threshold), join any existing cluster (looser threshold), or start a new one — greedy/
streaming, not full pairwise clustering, which is adequate at personal-archive scale.

**Background jobs:** `JobFaceDetection` ("Detect faces in photos") and `JobFaceClustering`
("Group similar faces"), registered like every other job in
`internal/service/background_jobs/definitions.go` and dispatched from
`internal/handler/background_jobs_runner.go` — they appear in Configuration → Background Jobs
with no extra wiring needed. Face detection reports a clear "face recognition isn't set up yet"
status (not a crash) if `bin/FaceRecognizer/facerecognizer.exe` isn't staged.

### Suggestions library (chat sidebar)

- **`GET /api/suggestions`** — [`internal/handler/template_handler.go`](internal/handler/template_handler.go) assembles categories from the deployment-wide `suggestions` SQLite table via [`internal/service/suggestions_service.go`](internal/service/suggestions_service.go), then renders Jinja subject variables from `buildContext` (`owner`, `owners`, `full_name`, `he` / `him` / `his` / `himself`, `owner_gender`, `deployment_nature_local`, image tokens, etc.).
- Startup seeds missing rows from [`static/data/suggestions.json`](static/data/suggestions.json) (insert-if-missing only; key = `{category}::{title}`). Optional **`SUGGESTIONS_CONFIG_PATH`** overrides the seed file path.
- The JSON response includes **`_meta.deployment_nature_local`** (`True` / `False` strings) for client-side rules (e.g. `requires: ["local_only"]` on an item).
- Configuration UI: **Configuration → Suggestions** ([`static/js/museum/modals-suggestions-config.js`](static/js/museum/modals-suggestions-config.js)) — CRUD, export, import with per-key conflict resolution. Admin API under `/api/suggestions/*`.
- Chat sidebar UI: [`static/js/museum/modals-suggestions.js`](static/js/museum/modals-suggestions.js); optional per-item fields include `action_label`, `description`, `requires`, `sensitivity`, and `function` (must match a key on `AppActions` in [`static/js/museum/app.js`](static/js/museum/app.js)).

### Guide system (interactive help)

The guide provides step-by-step help topics accessible from the Guide button in the top bar. Topics and their steps are stored in the database and seeded from a JSON file at startup — the same insert-if-missing pattern as Suggestions.

**Data flow:**
- `guide_topics` table: `id`, `key` (unique string), `text` (JSON blob containing title, description, category, recommended, steps array)
- Startup seeds from [`static/data/guide_topics.json`](static/data/guide_topics.json) (insert-if-missing by default). Optional **`GUIDE_TOPICS_CONFIG_PATH`** env var overrides the seed file path. Set **`GUIDE_TOPICS_RELOAD_FROM_FILE_ON_STARTUP=true`** to delete all guide topic rows on each server startup and reload entirely from that file (overwrites any admin edits made in the database). When that flag is enabled, signed-in archive owners also see **Account → Reload Guide Topics**, which runs the same reload on demand via **`POST /api/guide-topics/reload-from-file`**.
- Runtime API: **`GET /api/guide-topics`** → returns `{ topics: { KeyName: { title, category, steps, … } } }` consumed by `guide.js`.
- Admin CRUD API under `/api/guide-topics/*` (list, create, update, delete, export, import/preview/apply, **`DELETE /api/guide-topics/all`** to clear all rows).
- Configuration UI: **Configuration → Guide Topics** ([`static/js/museum/modals-guide-topics-config.js`](static/js/museum/modals-guide-topics-config.js)) — per-topic step editor, export/import with conflict resolution, **Clear All** (with confirmation; on next restart, missing keys are re-seeded from the filesystem JSON).

**Guide topic JSON fields** (stored in `text` column):

| Field | Type | Notes |
|-------|------|-------|
| `key` | string | Unique identifier (e.g. `"GettingStarted"`) |
| `title` | string | Display name in the guide modal |
| `description` | string | Subtitle shown under title |
| `category` | string | Groups topics. Built-in order: `Getting Started`, `Daily Use`, `Setup & Import`, `Troubleshooting`. Custom category names are supported and appear alphabetically after the built-in four. |
| `recommended` | bool | Shown with "Recommended first" badge and sorted to top |
| `dismiss_navigate_action` | string | Optional. Same semicolon-separated NavAction syntax as step `navigate_action`. Runs when the topic session ends — Done on the last step, Close (×), Escape, or overlay click. Guide UI is torn down first, then the action chain runs. |
| `steps` | array | Ordered list of step objects (see below) |

**Step object fields:**

| Field | Type | Notes |
|-------|------|-------|
| `text` | string | Instruction text shown to the user |
| `glow` | string | CSS selector of element to highlight with a pulsing outline |
| `position` | string | Dialog position: `middle-center` (default), `top-left/center/right`, `middle-left/right`, `bottom-left/center/right` |
| `navigate_action` | string | One or more named action keys, separated by `;` — see Navigation actions below. Actions run in order; pause keys block until elapsed. |
| `click_selector` | string | Optional CSS selector of a control to click before the step dialog appears. Runs after `navigate_action` (if any). Retries briefly until the element exists. |
| `image_url` | string | Optional image URL displayed below the step instruction text |

**Navigation actions** — defined in the `NavActions` dictionary in [`static/js/museum/guide.js`](static/js/museum/guide.js). Steps call `Guide._runNavActions()`, which splits `navigate_action` on `;`, trims each part, and runs matching handlers **sequentially** (awaiting promises from pause actions). If `click_selector` is set, `Guide._clickSelector()` runs next (with short retries for DOM settle). After both complete, the step dialog appears following a short DOM settle delay.

**Multiple actions:** combine keys with semicolons, e.g. `"closeOpenDialog;pause0_5s;openImageGallery"`. Use pause actions between UI opens/closes when modals or tabs need time to render. The admin step editor dropdown selects one key at a time; for chains, edit the JSON directly, use the topic **Dismiss navigate action** text field, or type a semicolon-separated value into exported topic data before import.

| Key | What it does |
|-----|-------------|
| `showGettingStartedDialog` | Opens the Getting Started overlay dialog |
| `closeOpenDialog` | Closes the topmost visible modal/overlay (excluding guide UI) |
| `pause0_5s` | Waits 0.5 seconds before the next action |
| `pause1s` | Waits 1.0 second before the next action |
| `pause2s` | Waits 2.0 seconds before the next action |
| `pause5s` | Waits 5.0 seconds before the next action |
| `openSmsMessages` | Clicks the Messages sidebar button |
| `openEmailGallery` | Clicks the Emails sidebar button |
| `openImageGallery` | Clicks the Images sidebar button |
| `openFacebookAlbums` | Clicks the Facebook Albums sidebar button |
| `openFacebookPosts` | Clicks the Facebook Posts sidebar button |
| `openMultiSourceSearch` | Clicks the Multi-Source Search (similarity) sidebar button |
| `openLocations` | Clicks the Locations sidebar button |
| `openArtefacts` | Clicks the Artefacts sidebar button |
| `openIdentityProfile` | Clicks the Identity Profile Wizard sidebar button |
| `openDataImport` | Opens Data Maintenance on the Data Import tab |
| `openDataSourcesImport` | Alias of `openDataImport` |
| `openDataMaintenance` | Opens Data Maintenance on the Data Maintenance tab |
| `openDataImportImport` | Opens Data Maintenance on the Data Import tab |
| `openDataImportMaintenance` | Opens Data Maintenance on the Data Maintenance tab |
| `openDataImportBackgroundJobs` | Opens Data Maintenance on the Scheduled Jobs tab |
| `openConfiguration` | Clicks the Configuration sidebar button |
| `openPreviousResponses` | Clicks the Previous Responses sidebar button |
| `openSuggestions` | Clicks the Suggestions sidebar button |
| `openContacts` | Clicks the Contacts and Relationships sidebar button |
| `openContactsRelationships` | Opens Contacts and Relationships on the Relationships tab |
| `openProfiles` | Clicks the Profiles sidebar button |
| `openSensitiveData` | Clicks the Sensitive Data sidebar button |
| `openDashboard` | Clicks the Dashboard/Statistics sidebar button |
| `openHaveAChat` | Clicks the Have-a-Chat sidebar button |
| `openRandomQuestion` | Clicks the Random Question sidebar button |
| `openTodaysThing` | Clicks the Today's Thing sidebar button |
| `openInterviewer` | Clicks the Interviewer sidebar button |
| `openPersonalitySettings` | Opens the Personality Settings dialog from the top bar voice image |
| `openConfigAppearance` | Opens Configuration on the Appearance/Settings tab |
| `openConfigApiKeys` | Opens Configuration on the API Keys tab |
| `openConfigAiSetup` | Opens Configuration on the AI & Setup tab |
| `openConfigSubjectConfiguration` | Opens Configuration on the Subject Configuration tab |
| `openConfigRegions` | Opens Configuration on the Regions tab |
| `openConfigSuggestions` | Opens Configuration on the Suggestions tab |
| `openConfigGuideTopics` | Opens Configuration on the Guide Topics tab |
| `openConfigCustomVoices` | Opens Configuration on the Custom Voices tab |
| `openConfigManageVisitorKeys` | Opens Configuration on the Manage Visitor Keys tab |
| `openConfigMcpServers` | Opens Configuration on the MCP Servers tab |
| `openConfigToolsAccess` | Opens Configuration on the AI Tool Access tab |
| `openConfigToolTest` | Opens Configuration on the Tool Test tab |
| `openSettingsManageKeys` | Alias for `openConfigManageVisitorKeys` |
| `openReferenceDocuments` | Clicks the Ref Docs segment in the chat context bar, opening the Reference Documents manager |
| `openToolCallsDialog` | Opens the tool calls log dialog from the chat context bar (last request) |

To add a new navigation action: (1) add an entry to `Guide.NavActions` in `guide.js`; (2) add the key to the `NAV_ACTIONS` array in `modals-guide-topics-config.js` so it appears in the admin UI dropdown.

**Frontend module** — [`static/js/museum/guide.js`](static/js/museum/guide.js):
- `Guide.fetchTopics()` — fetches from `GET /api/guide-topics`, caches in `Guide._topics`. Call `Guide.invalidateCache()` after any admin write to force a fresh fetch.
- `Guide.openGuideModal()` — fetches topics then renders the topic list.
- `Guide.onTopicSelected(key)` — starts a step-through session for the given topic key.
- `Guide._runNavActions(navActionRaw)` — runs semicolon-separated `navigate_action` keys in order (used internally by `_showStep` and on topic dismiss).
- `Guide._getTopicDismissNavAction(topicKey)` — reads `dismiss_navigate_action` from the cached topic config.
- `Guide.topicAliases` — maps legacy string keys (e.g. `'Browsing images'`) to canonical DB keys for backward compatibility.

### Import Pipeline

Every import mechanism is desktop-native (Electron's file/folder picker hands the Go server a
local filesystem path — there is no browser-side byte upload), and every one follows the same
Start / `/stream` (SSE progress) / `/cancel` / `/status` quartet, backed by a per-mechanism
`importer.ImportJob` singleton (one job of that kind at a time):

| Mechanism | Trigger endpoint | Body |
|-----------|------------------|------|
| IMAP credentials | `POST /imap/process` (`internal/handler/imap_handler.go`) | host/user/password/folders |
| ZIP or extracted-folder path — Facebook, Instagram, WhatsApp, or iMessage | `POST /import/from-path` (`internal/handler/upload_import_handler.go`) | `{file_path, type}`, `type` ∈ `facebook`/`instagram`/`whatsapp`/`imessage`; progress under `/import/upload/*` |
| Filesystem/photo import by root directory | `POST /images/import` (`internal/handler/importer_handler.go` → `FilesystemStart`) | `{root_directory, …}` |
| Per-source dedicated importer (already-exported directory) | `POST /whatsapp/import`, `/imessages/import`, `/instagram/import`, `/facebook/all/import`, `/emails/process`, `/contacts/extract`, … (`importer_handler.go`) | `{directory_path}` or similar |
| Server-triggered maintenance (thumbnails, reference import, embedding backfills, region recalculation, tag QA, GPS-duplicate spread, exports) | various under `/images/*`, `/emails/embeddings/backfill`, `/messages/context-embeddings/backfill`, … (`importer_handler.go`) | job-specific |

All import handlers capture `uid` before launching background goroutines and pass it via
`context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)`.

### Gmail Import

Gmail OAuth uses a Desktop App client configured in Google Cloud Console:
- Register `http://localhost:8081/gmail/auth/callback` (and ports 8080–8085 for safety) as authorized redirect URIs
- `GMAIL_CLIENT_ID` and `GMAIL_CLIENT_SECRET` in `.env`
- `GMAIL_REDIRECT_URL` is set dynamically by Electron to match the actual running port

### Frontend Module Pattern

All frontend JS uses a revealing-module pattern (IIFE returning a public API):

```javascript
const MyModule = (() => {
    function init() { /* wire DOM event listeners */ }
    return { init };
})();
```

- Constants and DOM element cache: `foundation.js`
- Event listener wiring: `app.js` (calls `ModuleName.init()` from `Modals.initAll()`)
- Modules loaded after `app.js` must self-initialize at the bottom of their file
- All dates displayed should be in local format

### Typography (UI)

UI typography is centralised in `static/css/museum_of.css` under `:root` (same file as colour and spacing tokens).

**Font stack**

- **`--font-sans-ui`** — system UI stack (`ui-sans-serif`, `system-ui`, Segoe UI, Roboto, etc.). Applied on `body`, `.modal-content`, `.email-message`, `.form-control`, and `.modal-btn` so the shell, modals, and email cards share one family.
- **`login.html`** uses `font-family: var(--font-sans-ui)` on `html` (after `museum_of.css` loads).

**Type scale (rem; default UI sizing)**

| Token | Typical use |
|-------|----------------|
| `--text-2xs` | Dense uppercase labels (e.g. image gallery sidebar section titles) |
| `--text-xs` | Small UI copy (~12px) |
| `--text-sm` | Compact controls (~13px) |
| `--text-md` | Secondary body, small inputs (~14px) |
| `--text-caption` | Helper / caption lines (~0.85rem) |
| `--text-lead` | Slightly larger supporting copy (~15px) |
| `--text-base` | Default body / form text (1rem) |
| `--text-lg` … `--text-2xl` | Stepped emphasis and large controls |
| `--text-heading`, `--text-heading-lg` | Modal subheads |
| `--text-3xl` | Prominent in-modal or setup titles (1.8rem) |
| `--text-emphasis` | Short emphasised blocks |
| `--text-xl`, `--text-icon-xl`, `--text-icon-2xl`, `--text-screen-title` | Icons and hero-style titles where needed |

**Line height**

- **`--line-height-tight`**, **`--line-height-snug`**, **`--line-height-body`** — reuse for new blocks instead of ad-hoc numbers.

**Chat (separate from UI scale)**

- **`--message-font-size`** — chat bubble text size (default 16px). The Settings UI range control and `foundation.js` read/write this; do not fold chat rendering into the rem UI scale unless intentionally changing product behaviour.

**Intentional exceptions (do not "normalise" away)**

- User-selectable message fonts in the chat UI, VT323 / retro blocks, `monospace` / `ui-monospace` for code and technical fields, and Font Awesome icon font rules keep their own `font-family` values.

**Conventions for new work**

- Prefer **`font-size: var(--text-…)`** (and existing colour tokens) in HTML `style` attributes or CSS when `museum_of.css` is on the page.
- Use **`em`** only for sizes that must track a parent's font size (e.g. a hint span under a micro label).
- Pages that **do not** load `museum_of.css` should not reference these variables; use plain **`rem`** (or page-local CSS) instead.
- Every HTML control must have a unique and descriptive id attribute

## Key Files Quick Reference

| What | Where |
|------|-------|
| Electron main process | `electron/main.js` |
| Electron IPC bridge | `electron/preload.js` |
| Electron installer / packaging config | `electron/electron-builder.yml` (see Building the Installable Package) |
| Route wiring | `internal/api/router/router.go` |
| Auth middleware | `internal/middleware/auth.go` |
| Auth service | `internal/service/auth_service.go` |
| Auth handler | `internal/handler/auth_handler.go` |
| Share token service | `internal/service/archive_share_service.go` |
| Context key (user_id) | `internal/appctx/appctx.go` |
| Repository user scoping | `internal/repository/userscope.go` |
| Import storage user scoping | `internal/importstorage/userscope.go` |
| SQLite dialect detection | `internal/sqlutil/dialect.go` |
| SQLite datetime parsing | `internal/sqlutil/dbtime.go` |
| DB migrations (main) | `internal/database/migrate.go` |
| DB migrations (billing) | `internal/database/migrate_billing.go` |
| LLM usage repository | `internal/repository/billing_repo.go` |
| LLM usage PDF export | `internal/billingpdf/bill.go` |
| AI provider interface | `internal/ai/provider.go` |
| OpenRouter adapter (backs every admin-configured AI model) | `internal/ai/openrouter.go` |
| AI Models CRUD (admin-managed model list) | `internal/service/ai_models_service.go`, `internal/repository/ai_models_repo.go`, `internal/handler/ai_models_handler.go` |
| Local AI / Ollama provider | `internal/ai/localai.go` |
| MCP tools server (every AI tool) | `cmd/mcpserver/main.go` |
| Tool query/decrypt logic + MCP-facing wrappers | `internal/ai/tools.go` |
| MCP client + executor | `internal/ai/mcpclient.go`, `internal/ai/tools.go` → `NewMCPToolExecutor()` |
| Tool catalog (discovery cache) | `internal/ai/tool_catalog.go` → `DefaultToolCatalog()` |
| Hidden per-call arg constants | `internal/ai/mcp_args.go` |
| MCP server registry (multi-server tool routing) | `internal/ai/mcp_registry.go` → `DefaultMCPRegistry()` |
| Additional MCP servers CRUD (admin-managed) | `internal/service/mcp_servers_service.go`, `internal/repository/mcp_servers_repo.go`, `internal/handler/mcp_servers_handler.go`, `internal/model/mcp_server_row.go` |
| Builtin MCP server row seed | `internal/database/seed_mcp_servers.go` |
| Dev CLI for querying an MCP server directly | `cmd/mcpquery/main.go` |
| AI Tool Access handler (policy + catalog refresh) | `internal/handler/llm_tools_access_handler.go` |
| Tool access tiers | `internal/ai/tool_access.go` |
| Tool Test tab handler (owner diagnostics) | `internal/handler/llm_tools_test_handler.go` |
| Chat orchestration | `internal/service/chat_service.go` |
| Chat HTTP handler | `internal/handler/chat_handler.go` |
| Reference doc system-prompt inlining | `internal/service/reference_prompt_inline.go` |
| Have-a-Chat handler | `internal/handler/have_a_chat_handler.go` |
| Interview handler | `internal/handler/interview_handler.go` |
| Identity Profile Wizard handler | `internal/handler/identity_profile_handler.go` |
| Background jobs scheduler | `internal/service/background_jobs/scheduler.go` |
| Background jobs runner (handler) | `internal/handler/background_jobs_runner.go` |
| Embedding service | `internal/service/` (EmbeddingService) |
| Embedding handler | `internal/handler/embedding_handler.go` |
| Message similarity handler | `internal/handler/message_similarity_handler.go` |
| Face recognition subprocess (SCRFD detection + ArcFace embedding) | `cmd/facerecognizer/` |
| Face recognition Go client + wire protocol | `internal/service/facerecognizer/client.go` |
| Face detection/clustering data + business logic | `internal/repository/face_repo.go`, `internal/service/face_service.go`, `internal/service/face_embedding.go` |
| Face detection/clustering background jobs + review/naming REST API | `internal/handler/face_handler.go` |
| Face crop image generation | `internal/service/face_crop.go` |
| Frontend People in Photos UI (cluster review/naming) | `static/js/museum/modals-faces.js` |
| Archive profile handler | `internal/handler/profile_handler.go` |
| Archive provision service | `internal/service/archive_provision.go` |
| Config (key-value store) service | `internal/service/config_service.go` |
| Dashboard service | `internal/service/dashboard_service.go` |
| Voice service | `internal/service/voice_service.go` |
| Admin user management handler | `internal/handler/admin_user_handler.go` |
| Config loading | `internal/config/config.go` |
| DB schema reference | `sqlc/schema.sql` |
| Frontend main | `static/js/museum/app.js` |
| Frontend auth | `static/js/museum/auth.js` |
| Frontend chat renderer | `static/js/museum/chat.js` |
| Frontend Have-a-Chat UI | `static/js/museum/have-a-chat.js` |
| Frontend Interview UI | `static/js/museum/interviewer.js` |
| Frontend Identity Wizard UI | `static/js/museum/identity-profile-wizard.js` |
| Frontend upload/import UI | `static/js/museum/upload-import.js` |
| Constants / DOM cache | `static/js/museum/foundation.js` |
| All styles; `:root` tokens (colours, spacing, **typography**: `--font-sans-ui`, `--text-*`, `--line-height-*`) | `static/css/museum_of.css` |
| Main SPA template | `templates/index.template.html` |
| Login / register page | `templates/login.html` |
| Share visitor page | `templates/share.html` |
| Profile selection / first-run page | `templates/non_user_init.template.html` |
| Login-page quiz (unauthenticated) | `templates/quiz.html`, `internal/handler/quiz_handler.go`, `ChatService.GenerateQuiz` in `internal/service/quiz_service.go` |
| Email attachments grid (embedded in SPA modal) | `templates/index.template.html` (`#email-attachments-modal`) |
| Suggestions (DB + seed JSON) | `static/data/suggestions.json`, `internal/service/suggestions_service.go`, `static/js/museum/modals-suggestions-config.js` |
| Guide system (DB + seed JSON) | `static/data/guide_topics.json`, `internal/service/guide_topics_service.go`, `static/js/museum/guide.js`, `static/js/museum/modals-guide-topics-config.js` |
| Frontend MCP Servers config UI | `static/js/museum/modals-mcp-servers-config.js` |
| Frontend AI Tool Access / Tool Test UI | `static/js/museum/modals-settings.js` (`Modals.LLMToolsAccess`), `static/js/museum/modals-tool-test.js` |

## Security Notes

- `.env` contains API keys — **never commit it**
- `KEYRING_PEPPER` is used to derive encryption keys — rotating it requires re-encrypting all affected records
- The RAM master key unlocks `private_store` and encrypted documents per session; it is never persisted to disk
- Tool access is tiered: Visitor / Master — controlled via `PUT /api/settings/llm-tools-access`
- All archive data tables have `user_id` (nullable) — NULL means legacy/single-tenant data
- Share visitor sessions are `dm_session` cookies scoped to the **owner's** `user_id`
- The MCP tools server (`cmd/mcpserver`) binds to `127.0.0.1` only and requires the shared-secret
  `MCP_AUTH_TOKEN` bearer header on every request — see [MCP Tools Server & AI Tool Discovery](#mcp-tools-server--ai-tool-discovery)
- Additional (owner-added) MCP servers are treated as **untrusted third parties**: they never
  receive the master password or Tavily key, but a tool call there still carries the archive
  data the LLM chose to pass as arguments — registering one is a deliberate owner trust decision,
  same tier as API Keys / AI Tool Access (owner-only, not just any authenticated session)

## What NOT to Do

- Don't use PostgreSQL-specific SQL: no `::type` casts, no `ON CONFLICT ON CONSTRAINT name`, no `unnest(string_to_array(…))`, no partial-index conflict targets without the matching `WHERE` clause
- Don't add Node.js / npm tooling to the Go backend — the frontend is intentional vanilla JS
- Don't modify existing migration functions — always add a new one
- Don't commit `.env` or stray binary files
- Don't add `user_id` filtering to the `users`, `sessions`, or `archive_shares` tables — these are identity/auth tables
- Don't use `context.Background()` in import background goroutines — always thread the `user_id` via `context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)`
- Don't return nil slices from list handlers — always substitute an empty slice so JSON encodes as `[]` not `null`
- Don't attach secret hidden args (`ArgMasterPassword`, `ArgTavilyKey`) to a non-builtin MCP
  server call — `NewMCPToolExecutor` already gates this on `trusted`; never bypass that gate
  when adding a tool or wiring a new secret
- Don't write raw SQL with `pgx` — the codebase now uses `database/sql` with `github.com/mattn/go-sqlite3`
