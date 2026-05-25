# Digital Museum — CLAUDE.md

## Project Overview

Digital Museum is an AI-powered digital archive platform packaged as an
**Electron desktop app**. Each registered user owns one archive containing their entire
digital life (emails, messages, photos, Facebook, iMessage, WhatsApp, documents),
imported into a SQLite database and queryable through an AI chat interface. Claude,
Gemini, and a local Ollama/Gemma4 model can access the data via a tool-calling layer
and answer questions, adopt personas, and explore the archive conversationally.

Additional features beyond the core chat interface include:
- **Pam Bot** — dementia companion chat mode with a restricted, focussed tool set
- **Have-a-Chat** — two-voice conversation sessions (two AI personas talking to each other)
- **Interviews** — structured Q&A sessions driven by AI, saved for review
- **Identity Profile Wizard** — AI-guided wizard that builds a textual profile of the archive subject
- **Background Jobs** — per-user scheduled maintenance tasks (thumbnail generation, embedding, etc.)
- **Vector Similarity Search** — embedding-based similarity search over messages, emails, and media

## Tech Stack

- **Desktop shell:** Electron (Node.js) — `electron/main.js` manages the Go server process, Ollama, system tray, and IPC
- **Backend:** Go 1.25, Chi v5 router, `database/sql` with `github.com/mattn/go-sqlite3` (CGO)
- **Frontend:** Vanilla JavaScript (no framework), marked.js, highlight.js, Font Awesome
- **Database:** SQLite (two files — main app DB and billing DB); vector fields use `sqlite-vec`
- **AI Providers:** Anthropic Claude (`claude-sonnet-4-6`), Google Gemini (`gemini-2.5-flash`), DeepSeek (`deepseek-chat`) via Anthropic-compatible API, and local Ollama (`gemma4`) via native Ollama API
- **Module:** `github.com/daveontour/aimuseum`

## Project Layout

```
electron/
  main.js           ← Electron main process: spawns Go server + Ollama, IPC handlers, tray
  preload.js        ← IPC bridge (contextBridge) exposed to renderer pages
  loading.html      ← Splash screen shown while Go server starts
bin/
  digitalmuseum.exe ← Compiled Go server
  Ollama/           ← Bundled Ollama executable
  ImageMagick/      ← Bundled ImageMagick for thumbnail generation
cmd/
  server/           ← HTTP server entry point (main.go)
  launcher/         ← Windows GUI launcher (legacy)
internal/
  ai/               ← Claude, Gemini, DeepSeek & LocalAI (Ollama) providers, tool definitions & executor
  api/router/       ← Route wiring (router.go)
  appctx/           ← Shared context key (ContextKeyUserID / UserIDFromCtx)
  config/           ← Env-var config loading
  crypto/           ← Encryption / key derivation (keyring scoped by user_id)
  database/         ← Connection pool, migrations
  handler/          ← HTTP request handlers (~47 files)
  keystore/         ← RAM master key (unlocks encrypted data per session)
  middleware/        ← Logger, Recoverer, AuthMiddleware
  model/            ← Shared data types / DTOs
  repository/       ← Database access via database/sql (~32 repos, all user-scoped)
  service/          ← Business logic (~36 services)
  service/background_jobs/ ← Job definitions, registry, and scheduler
  sqlutil/          ← SQLite dialect helpers: IsSQLite(), ParseSQLiteDatetime(), InClause()
static/
  css/              ← museum_of.css (all styles, ~8000 lines)
  data/             ← voice_instructions.json, seed JSON files
  images/           ← Voice persona images
  js/museum/        ← Frontend modules (~22 JS files)
templates/          ← index.template.html (SPA), login.html, share.html, plus 3 others
sqlc/               ← schema.sql (full DB schema reference), sqlc.yaml
```

## Build & Run

```bash
# Run dev server (reads .env automatically)
make run                          # go run ./cmd/server

# Build binaries
make build-exe                    # bin/digitalmuseum.exe

# Tests / lint
make test
make lint                         # requires golangci-lint
make tidy                         # go mod tidy
```

To run the full Electron app in dev mode, open a terminal in `electron/` and run
`npx electron .` (requires Node.js). The Go server is spawned automatically.

## Configuration (`.env`)

The server reads `.env` from the working directory (set by Electron to the project root
in dev mode, or the install root in packaged mode). User-editable settings live in
`%APPDATA%\Digital Museum\.env` and are layered on top.

| Variable | Required | Description |
|----------|----------|-------------|
| `SQLITE_PATH` | Yes | Absolute path to the main SQLite database file |
| `ADMIN_SQLITE_PATH` | No | Billing/admin SQLite file; default `<exeDir>/data/admin.sqlite`. Optional override: absolute as-is, relative resolved against exe dir |
| `HOST_PORT` | No | HTTP listen port (default: 8000; Electron overrides to 8081) |
| `PAGE_TITLE` | No | Browser page title (default: `Digital Museum of SUBJECT_NAME`) |
| `ANTHROPIC_API_KEY` | At least one AI key | Claude API |
| `CLAUDE_MODEL_NAME` | No | Default: `claude-sonnet-4-6` |
| `DEEPSEEK_API_KEY` | No | DeepSeek API key (Anthropic-compatible Messages API at `api.deepseek.com/anthropic`) |
| `DEEPSEEK_MODEL_NAME` | No | Default: `deepseek-chat` |
| `GEMINI_API_KEY` | At least one AI key | Gemini API |
| `GEMINI_MODEL_NAME` | No | Default: `gemini-2.5-flash` |
| `LOCALAI_BASE_URL` | No | Ollama base URL, e.g. `http://localhost:11434` |
| `LOCALAI_MODEL_NAME` | No | Default: `local-model` (use `gemma4` for Ollama) |
| `LOCALAI_API_KEY` | No | Not required by Ollama; kept for compatibility |
| `LOCALAI_EMBEDDING_MODEL` | No | Falls back to `LOCALAI_MODEL_NAME` if empty |
| `LOCALAI_NUM_CTX` | No | Per-request `num_ctx` sent to Ollama under `options`. Empty / 0 / non-positive omits the field so Ollama uses its server-side default (set via `OLLAMA_NUM_CTX` when Electron starts the daemon). |
| `TAVILY_API_KEY` | No | Enables `search_tavily` web-search tool |
| `GMAIL_CLIENT_ID` | No | Google OAuth 2.0 Desktop App client ID |
| `GMAIL_CLIENT_SECRET` | No | Google OAuth 2.0 client secret |
| `GMAIL_REDIRECT_URL` | No | Set automatically by Electron at startup |
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
   `GMAIL_REDIRECT_URL` (dynamic), and `LOG_LEVEL` into the Go server's environment
3. Waits for `GET /health` to return 200 before showing the main `BrowserWindow`
4. Manages the system tray, developer tools shortcut, and single-instance lock
5. Manages the Ollama process (`ollama serve`) including health checks and shutdown

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

When `SQLITE_PATH` is not set or the file is absent, the router receives a `nil` pool.
In this mode the server serves only a minimal set of routes: `/health`,
`/api/resolved-main-sqlite-path`, `/api/profiles`, `/login`, `/`, and `/static/*`. All
other routes return `503` with a JSON error. This allows the Electron shell to let the
user create or select an archive before the full application starts.

## Multi-Archive Profile Management

The billing DB (`admin.sqlite`) stores a `profiles` table — one row per archive. Each
profile holds a display name and the absolute path to its main SQLite file. The
`ProfileHandler` (`internal/handler/profile_handler.go`) exposes CRUD routes under
`/api/profiles` and a `/profiles` SPA page (served from `templates/non_user_init.template.html`).

From the login screen users can switch between archives or create a new one; switching
writes the new `SQLITE_PATH` to `%APPDATA%\Digital Museum\.env` and restarts the Go
server (via the `select-db` IPC channel). The Electron `get-profiles` / `create-profile`
/ `update-profile` IPC handlers read/write the billing DB directly.

## Local AI — Ollama / Gemma4

`internal/ai/localai.go` implements `ChatProvider` using the **native Ollama API**:

- Chat: `POST {LOCALAI_BASE_URL}/api/chat` with `stream: false`
- Embeddings: `POST {LOCALAI_BASE_URL}/api/embed`
- Tool arguments arrive as `map[string]any` (not a JSON string — unlike OpenAI compat)
- Token counts read from `prompt_eval_count` / `eval_count` (not `usage.prompt_tokens`)
- No `Authorization` header required
- Model options (temperature, num_ctx) sent under `"options"` key

**Context size (`num_ctx`):**

- The Ollama daemon spawned by Electron is started with `OLLAMA_NUM_CTX=32768` (configured in
  the `start-ollama` IPC handler in `electron/main.js`). This becomes the **server-side default**
  for any request that omits `num_ctx`.
- The Go provider also sends `num_ctx` per-request when `LOCALAI_NUM_CTX` is set to a positive
  integer. When unset / 0, the field is omitted and Ollama applies the server-side default.
- Note: the env-var default only applies to the daemon Electron starts. If users connect to a
  pre-existing Ollama daemon they started themselves, that daemon's own configuration wins.

## DeepSeek Provider

`internal/ai/deepseek.go` implements `ChatProvider` using DeepSeek's **Anthropic-compatible Messages API**:

- Endpoint: `POST https://api.deepseek.com/anthropic/v1/messages`
- Authentication: `x-api-key` header (same field name as Anthropic)
- Protocol: identical to Claude's Messages API — same tool-calling format, same `stop_reason: "tool_use"` loop
- Uses `anthropic-version: 2023-06-01` header required by DeepSeek's endpoint
- Token counts read from standard `usage.input_tokens` / `usage.output_tokens`
- Returns `nil` from constructor when `DEEPSEEK_API_KEY` is empty

Select via `"provider": "deepseek"` in `POST /chat/generate` and `Have-a-Chat` requests.

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
  - `GET/PUT /admin/pambot-instructions` — Pam Bot companion persona

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

**Exempt routes:**
```
GET  /health
GET  /static/*
GET  /login
POST /auth/login
POST /auth/register
GET  /share/*
POST /share/*
GET  /s/*
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

1. Add the tool definition (JSON schema) in `internal/ai/provider.go` — `GetToolDefinitions()`
2. Add the execution case in `internal/ai/tools.go` — `NewToolExecutor()` switch statement
3. All SQL in `tools.go` must include `AND user_id = $N` via `toolsUIDFilter(ctx, q, args)`
4. Optionally add access-tier controls in `internal/ai/tool_access.go`
5. For Pam Bot's restricted tool set, also update `internal/ai/pambot_tools.go`

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
- **Provider selection:** `"claude"`, `"gemini"`, `"deepseek"`, or `"localai"` in request body
- All AI tool SQL is scoped by `user_id` via `toolsUIDFilter(ctx, q, args)` in `internal/ai/tools.go`

### Pam Bot (Dementia Companion)

`PamBotService` (`internal/service/pambot_service.go`) runs a separate, simplified chat
loop with a restricted tool set defined in `internal/ai/pambot_tools.go`. Sessions and
turns are persisted to `pam_bot_sessions` / `pam_bot_turns` / `pam_bot_subjects` tables.
The handler (`internal/handler/pambot_handler.go`) exposes routes under `/api/pambot/…`.
App-wide Pam Bot instructions are stored alongside the main system instructions in
`app_system_instructions.pam_bot_instructions` and managed via `GET/PUT /admin/pambot-instructions`.

### Have-a-Chat (Two-Voice Conversations)

`HaveAChatHandler` (`internal/handler/have_a_chat_handler.go`) drives sessions where
two AI personas converse with each other about the archive. Sessions are persisted to
`have_a_chat_sessions`. The `ChatService` is reused for both turns; the handler sequences
the turns and passes each response back as the next prompt. Routes under `/api/have-a-chat/…`.

### Interviews

`InterviewHandler` (`internal/handler/interview_handler.go`) manages structured Q&A
sessions: the AI asks questions, the user answers, and the interview is saved for later
review. State is persisted to `interviews` / `interview_turns` tables via
`InterviewRepo`. Routes under `/api/interview/…`.

### Identity Profile Wizard

`IdentityProfileHandler` (`internal/handler/identity_profile_wizard.go`) is a guided,
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

### Suggestions library (chat sidebar)

- **`GET /api/suggestions`** — [`internal/handler/template_handler.go`](internal/handler/template_handler.go) renders [`static/data/suggestions.json`](static/data/suggestions.json) with Jinja subject variables from `buildContext` (`owner`, `owners`, `full_name`, `he` / `him` / `his` / `himself`, `owner_gender`, `deployment_nature_local`, image tokens, etc.).
- Optional **`static/data/suggestions.override.json`** (gitignored): same top-level shape as `suggestions.json`. Categories with the same `category` name **append** their `suggestions` arrays to the base file; new category names are appended whole. Lets operators add prompts without editing the stock file.
- The JSON response includes **`_meta.deployment_nature_local`** (`True` / `False` strings) for client-side rules (e.g. `requires: ["local_only"]` on an item).
- UI: [`static/js/museum/modals-suggestions.js`](static/js/museum/modals-suggestions.js); optional per-item fields include `action_label`, `description`, `requires`, `sensitivity`, and `function` (must match a key on `AppActions` in [`static/js/museum/app.js`](static/js/museum/app.js)).

### Import Pipeline

| Tier | Mechanism | Endpoint |
|------|-----------|----------|
| A | IMAP credentials | `POST /imap/process` |
| B | ZIP file upload (Facebook, Instagram, WhatsApp, iMessage) | `POST /import/upload` |
| C1 | Browser folder picker (photos) | `POST /import/photo-batch` |
| D | Server-triggered (contacts, thumbnails, reference import) | various |

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

## Key Files Quick Reference

| What | Where |
|------|-------|
| Electron main process | `electron/main.js` |
| Electron IPC bridge | `electron/preload.js` |
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
| Claude provider | `internal/ai/claude.go` |
| DeepSeek (Anthropic-compatible API) | `internal/ai/deepseek.go` |
| Gemini provider | `internal/ai/gemini.go` |
| Local AI / Ollama provider | `internal/ai/localai.go` |
| Tool definitions | `internal/ai/provider.go` → `GetToolDefinitions()` |
| Tool execution | `internal/ai/tools.go` → `NewToolExecutor()` |
| Pam Bot tool definitions | `internal/ai/pambot_tools.go` |
| Tool access tiers | `internal/ai/tool_access.go` |
| Chat orchestration | `internal/service/chat_service.go` |
| Chat HTTP handler | `internal/handler/chat_handler.go` |
| Reference doc system-prompt inlining | `internal/service/reference_prompt_inline.go` |
| Pam Bot service | `internal/service/pambot_service.go` |
| Pam Bot handler | `internal/handler/pambot_handler.go` |
| Have-a-Chat handler | `internal/handler/have_a_chat_handler.go` |
| Interview handler | `internal/handler/interview_handler.go` |
| Identity Profile Wizard handler | `internal/handler/identity_profile_wizard.go` |
| Background jobs scheduler | `internal/service/background_jobs/scheduler.go` |
| Background jobs runner (handler) | `internal/handler/background_jobs_runner.go` |
| Embedding service | `internal/service/` (EmbeddingService) |
| Embedding handler | `internal/handler/embedding_handler.go` |
| Message similarity handler | `internal/handler/message_similarity_handler.go` |
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
| Frontend Pam Bot UI | `static/js/museum/pam-bot.js` |
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
| Attachment viewer (standalone) | `templates/attachments_viewer.html` |
| Image grid (standalone) | `templates/images_grid.html` |
| Suggestions JSON + optional override | `static/data/suggestions.json`, `static/data/suggestions.override.json` (override gitignored) |

## Security Notes

- `.env` contains API keys — **never commit it**
- `KEYRING_PEPPER` is used to derive encryption keys — rotating it requires re-encrypting all affected records
- The RAM master key unlocks `private_store` and encrypted documents per session; it is never persisted to disk
- Tool access is tiered: Visitor / Master — controlled via `PUT /api/settings/llm-tools-access`
- All archive data tables have `user_id` (nullable) — NULL means legacy/single-tenant data
- Share visitor sessions are `dm_session` cookies scoped to the **owner's** `user_id`

## What NOT to Do

- Don't use PostgreSQL-specific SQL: no `::type` casts, no `ON CONFLICT ON CONSTRAINT name`, no `unnest(string_to_array(…))`, no partial-index conflict targets without the matching `WHERE` clause
- Don't add Node.js / npm tooling to the Go backend — the frontend is intentional vanilla JS
- Don't modify existing migration functions — always add a new one
- Don't commit `.env` or stray binary files
- Don't add `user_id` filtering to the `users`, `sessions`, or `archive_shares` tables — these are identity/auth tables
- Don't use `context.Background()` in import background goroutines — always thread the `user_id` via `context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)`
- Don't return nil slices from list handlers — always substitute an empty slice so JSON encodes as `[]` not `null`
- Don't write raw SQL with `pgx` — the codebase now uses `database/sql` with `github.com/mattn/go-sqlite3`
