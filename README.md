# Digital Museum

A personal digital archive and AI-powered memory explorer. Digital Museum aggregates your emails, messages, photos, and social media into a single searchable archive, then lets you explore it through a conversational AI that has direct access to your data.

## Features

- **AI chat with tool access** — ask the AI to search your emails, messages, photos, Facebook posts, and more. Supports Google Gemini and Anthropic Claude.
- **Multi-source archiving** — import from Gmail/IMAP, WhatsApp, iMessage, Instagram, Facebook, and local filesystems.
- **Voice personalities** — choose how the AI responds: as an expert, a friend, or in the subject's own voice with a selectable mood.
- **Companion Mode** — ongoing conversational dialogue rather than one-off Q&A.
- **Relationship tracking** — contact profiles, interaction graphs, and relationship analysis.
- **Sensitive data vault** — encrypted key-value store protected by a master key.
- **Custom voices** — create AI personas with custom instructions and creativity levels.
- **Today's Thing of Interest** — daily AI-generated prompts based on the subject's interests.
- **Reference documents** — attach documents for the AI to consult when answering questions.
- **Statistics and visualisations** — email counts, contact maps, media timelines, and location maps.

## Tech Stack

- **Backend:** Go 1.25, [Chi](https://github.com/go-chi/chi) router
- **Database:** PostgreSQL (pgx v5), with `pg_trgm` for full-text search and `pgcrypto` for encryption
- **AI providers:** Google Gemini (`gemini-2.5-flash`), Anthropic Claude (`claude-sonnet-4-6`)
- **Email:** IMAP via `go-imap`, Gmail via OAuth2
- **Frontend:** Vanilla JS, Leaflet (maps), Cytoscape (relationship graphs), Marked (Markdown), Highlight.js

## Prerequisites

- Go 1.25+
- PostgreSQL 14+ with the `pgcrypto` and `pg_trgm` extensions available
- At least one AI provider API key (Gemini or Anthropic)

## Configuration

Copy `.env.example` to `.env` and fill in the values. The server searches for `.env` starting from the executable directory and walking upward.

### Required

| Variable | Description |
|---|---|
| `DB_HOST` | PostgreSQL host |
| `DB_PORT` | PostgreSQL port (default `5432`) |
| `DB_NAME` | Database name (default `musego`) |
| `DB_USER` | Database user |
| `DB_PASSWORD` | Database password |
| `KEYRING_PEPPER` | Secret string used to derive encryption keys — keep this safe and consistent |

### AI Providers (at least one required for chat)

| Variable | Description |
|---|---|
| `GEMINI_API_KEY` | Google Gemini API key |
| `GEMINI_MODEL_NAME` | Model override (default `gemini-2.5-flash`) |
| `ANTHROPIC_API_KEY` | Anthropic Claude API key |
| `CLAUDE_MODEL_NAME` | Model override (default `claude-sonnet-4-6`) |
| `TAVILY_API_KEY` | Tavily web search key (optional — enables AI web search) |

### Server

| Variable | Description |
|---|---|
| `HOST_PORT` | HTTP port (default `8001`) |
| `PAGE_TITLE` | Browser tab title |
| `SESSION_COOKIE_SECURE` | Set `true` when running behind HTTPS |
| `ENABLE_PPROF` | Set `true` to enable pprof on `:6060` |

### Gmail / OAuth2

| Variable | Description |
|---|---|
| `GMAIL_CLIENT_ID` | Google OAuth2 client ID |
| `GMAIL_CLIENT_SECRET` | Google OAuth2 client secret |
| `GMAIL_REDIRECT_URL` | OAuth2 callback URL (default `http://localhost:8001/gmail/auth/callback`) |

### Import Defaults (all optional)

| Variable | Description |
|---|---|
| `DEFAULT_IMAP_HOST` | Pre-fill IMAP host in the import UI |
| `DEFAULT_IMAP_PORT` | Pre-fill IMAP port (default `993`) |
| `DEFAULT_IMAP_USERNAME` | Pre-fill IMAP username |
| `DEFAULT_FACEBOOK_IMPORT_DIRECTORY` | Default path to Facebook archive |
| `DEFAULT_INSTAGRAM_IMPORT_DIRECTORY` | Default path to Instagram archive |
| `DEFAULT_WHATSAPP_IMPORT_DIRECTORY` | Default path to WhatsApp export |
| `DEFAULT_IMESSAGE_DIRECTORY_PATH` | Default path to iMessage database directory |
| `DEFAULT_NEW_ONLY_OPTION` | Import only new items by default (`true`/`false`) |
| `FILESYSTEM_IMPORT_EXCLUDE_PATTERNS` | Comma-separated glob patterns to exclude from filesystem imports |

## Running

```bash
# Install dependencies
go mod download

# Run the server
go run ./cmd/server

# The server starts at http://localhost:8001
```

On first run the server will create the database (if it doesn't exist), apply all migrations, and seed reference data from the `static/data/` directory.

To build a binary:

```bash
go build -o bin/server ./cmd/server
```

## Data Import

Imports are managed from **Settings → Manage Imported Data**. Supported sources:

| Source | Notes |
|---|---|
| **Facebook** | Requires a JSON archive downloaded from facebook.com/settings |
| **Instagram** | Requires a JSON archive downloaded from instagram.com |
| **WhatsApp** | Point at an exported chat directory |
| **iMessage** | Reads the macOS iMessage database directory |
| **Gmail** | OAuth2 flow — link your Google account in settings |
| **IMAP** | Any IMAP mailbox with folder selection |
| **Filesystem** | Import images from a local directory, with optional thumbnail generation |

All imports run as background jobs with real-time progress streamed to the UI.

## Architecture

```
cmd/server/          Entry point, server startup
internal/
  api/router/        Route definitions
  handler/           HTTP request handlers
  service/           Business logic
  repository/        Database access (pgx)
  model/             Shared data types
  ai/                Claude and Gemini provider implementations
  import/            Per-source import plugins
  crypto/            Encryption and key derivation
  config/            Environment-based configuration
  database/          Migrations and connection pool
static/
  js/museum/         Frontend JavaScript
  css/               Stylesheets
  data/              Seed data (voice instructions, email classifications)
templates/           HTML templates
```

## Security Notes

- Never commit `.env` or any file containing `KEYRING_PEPPER` or API keys — both are listed in `.gitignore`.
- `KEYRING_PEPPER` is used to derive encryption keys for the sensitive data vault. Changing it will make existing encrypted data unreadable.
- Set `SESSION_COOKIE_SECURE=true` when deploying behind HTTPS.
- The master key system controls access to subject configuration, imported data management, and sensitive data. Visitor access is read-only by default.
