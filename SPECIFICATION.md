# Digital Museum — Complete Rebuild Specification

## 1. Executive Summary

Digital Museum is a multi-tenant, AI-powered personal digital archive platform. Each registered
user owns a private archive containing their entire digital life — emails, messages, photos,
Facebook exports, iMessage/WhatsApp conversations, documents, contacts, and physical artefacts —
stored in PostgreSQL and queryable through an AI chat interface powered by Claude and/or Gemini.

The platform provides:
- Secure, isolated per-user data storage with PostgreSQL Row-Level Security
- Two AI providers (Anthropic Claude, Google Gemini) with full tool-calling access to archive data
- A memory companion mode (Pam Bot) for dementia support
- A structured interview system for life story capture
- Share tokens that grant visitors read access to an owner's archive
- A complete admin console for user management and billing
- LLM usage billing tracked in a separate database with PDF export

---

## 2. Technology Stack

### Required (mandated by specification)
- **Backend language:** Go (latest stable, currently 1.25)
- **Database:** PostgreSQL 14+, with `pg_trgm` and `pgcrypto` extensions
- **AI providers:** Anthropic Claude API, Google Gemini API

### Recommended additions (implementation team may choose alternatives)
- **HTTP router:** Chi v5 or similar (Fiber, Echo, Gorilla Mux)
- **DB driver:** pgx v5 (native PostgreSQL driver, strongly preferred for performance)
- **Frontend framework:** Any modern framework (React + TypeScript recommended for maintainability)
- **Frontend build:** Vite or similar
- **CSS:** Tailwind CSS or a design-system library (Radix UI, shadcn/ui)
- **Upload:** Tus resumable upload protocol for large file uploads (tus-go server library)
- **PDF generation:** Go PDF library (e.g., fpdf2, unipdf, or wkhtmltopdf wrapper)
- **File storage:** Local filesystem (default); S3-compatible (cloud deployments)

---

## 3. Deployment Modes

### Local (Single Machine)
- Single binary, `.env` file for configuration
- SQLite not supported — PostgreSQL required even locally
- Optional TLS via cert files
- Authentication required for all non-exempt routes

### Cloud / Web
- Multiple tenants per deployment; authentication required for the main app
- Serve behind reverse proxy (Nginx, Caddy) for TLS termination
- `SESSION_COOKIE_SECURE=true` when behind HTTPS
- Database user should be a non-owner role to enforce PostgreSQL RLS
- Separate `{DB_NAME}_billing` database must be reachable at same host/credentials

---

## 4. Configuration

All configuration is via environment variables (`.env` file loaded automatically from working
directory or any parent).

| Variable | Required | Default | Description |
|---|---|---|---|
| `DB_HOST` | Yes | — | PostgreSQL host |
| `DB_PORT` | No | `5432` | PostgreSQL port |
| `DB_NAME` | Yes | — | Database name (billing DB = `{DB_NAME}_billing`) |
| `DB_USER` | Yes | — | Database user |
| `DB_PASSWORD` | Yes | — | Database password |
| `ANTHROPIC_API_KEY` | One AI key required | — | Claude API key |
| `CLAUDE_MODEL_NAME` | No | `claude-sonnet-4-6` | Claude model ID |
| `GEMINI_API_KEY` | One AI key required | — | Gemini API key |
| `GEMINI_MODEL_NAME` | No | `gemini-2.5-flash` | Gemini model ID |
| `TAVILY_API_KEY` | No | — | Enables `search_tavily` web search tool |
| `KEYRING_PEPPER` | Yes | — | Application secret for Argon2 key derivation |
| `HOST_PORT` | No | `8080` | HTTP listen port |
| `TLS_CERT_FILE` | No | — | Path to TLS certificate |
| `TLS_KEY_FILE` | No | — | Path to TLS private key |
| `SESSION_COOKIE_SECURE` | No | `false` | Set `true` for HTTPS deployments |
| `ADMIN_EMAIL` | No | — | Seed admin account email |
| `ADMIN_PASSWORD` | No | — | Seed admin account password |
| `GMAIL_CLIENT_ID` | No | — | Gmail OAuth client ID |
| `GMAIL_CLIENT_SECRET` | No | — | Gmail OAuth client secret |
| `GMAIL_REDIRECT_URL` | No | — | Gmail OAuth callback URL |
| `TUS_CHUNK_SIZE_MB` | No | `10` | Resumable upload chunk size |
| `TUS_UPLOAD_DIR` | No | `tmp/tus_uploads` | Temp directory for uploads |
| `TUS_MAX_UPLOAD_GB` | No | `32` | Max upload size (capped at 512 GB) |
| `ATTACHMENT_ALLOWED_TYPES` | No | all | Comma-separated MIME allowlist |
| `ATTACHMENT_MIN_SIZE` | No | `0` | Minimum attachment size in bytes |
| `ENABLE_PPROF` | No | `false` | Expose Go pprof on `:6060` |

---

## 5. Database Schema

### 5.1 Identity & Session Tables

These tables do NOT have `user_id` columns — they are the identity layer itself.

```sql
CREATE TABLE users (
    id                  BIGSERIAL PRIMARY KEY,
    email               TEXT NOT NULL UNIQUE,
    password_hash       TEXT NOT NULL,             -- Argon2id hash
    display_name        TEXT NOT NULL,
    first_name          TEXT,
    family_name         TEXT,
    gender              TEXT,                      -- for AI pronoun substitution
    -- Per-user LLM API key overrides (stored encrypted)
    gemini_api_key      TEXT,
    gemini_model        TEXT,
    claude_api_key      TEXT,
    claude_model        TEXT,
    tavily_api_key      TEXT,
    allow_server_keys   BOOLEAN DEFAULT true,      -- can use server-level API keys
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
    session_id          TEXT PRIMARY KEY,          -- 32-byte random hex
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at          TIMESTAMPTZ NOT NULL,
    is_visitor          BOOLEAN NOT NULL DEFAULT false,
    visitor_llm_overrides JSONB,                  -- LLM key overrides for visitor sessions
    share_link_session  BOOLEAN NOT NULL DEFAULT false,
    visitor_key_hint_id BIGINT REFERENCES visitor_key_hints(id)
);

CREATE TABLE visitor_key_hints (
    id                      BIGSERIAL PRIMARY KEY,
    user_id                 BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hint                    TEXT,                 -- label shown to visitor
    email                   TEXT,                 -- visitor email to filter hints
    can_messages_chat       BOOLEAN DEFAULT true,
    can_emails              BOOLEAN DEFAULT true,
    can_contacts            BOOLEAN DEFAULT true,
    can_relationship_sensitive BOOLEAN DEFAULT false,
    llm_allow_owner_keys    BOOLEAN DEFAULT false,
    llm_allow_server_keys   BOOLEAN DEFAULT true,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE archive_shares (
    token               TEXT PRIMARY KEY,          -- 32-byte random hex
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label               TEXT,
    password_hash       TEXT,                      -- bcrypt or argon2id, nullable if no password
    expires_at          TIMESTAMPTZ,              -- nullable = no expiry
    tool_access_policy  JSONB,                    -- per-tool tier rules
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE audit_log (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    event_type          TEXT NOT NULL,
    ip_address          TEXT,
    user_agent          TEXT,
    details             JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 5.2 Archive Data Tables

All archive data tables include `user_id BIGINT REFERENCES users(id)` (nullable for legacy
single-tenant data). All queries MUST filter by `user_id`.

```sql
-- Binary media storage
CREATE TABLE media_blobs (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    image_data          BYTEA,
    thumbnail_data      BYTEA,
    content_type        TEXT,
    file_size           BIGINT
);

-- Photo/image metadata
CREATE TABLE media_items (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    blob_id             BIGINT REFERENCES media_blobs(id),
    title               TEXT,
    description         TEXT,
    author              TEXT,
    tags                TEXT[],
    categories          TEXT[],
    year                INT,
    month               INT,
    day                 INT,
    latitude            DOUBLE PRECISION,
    longitude           DOUBLE PRECISION,
    rating              INT,
    classifications     TEXT[],
    use_by_ai           BOOLEAN DEFAULT true,
    is_referenced       BOOLEAN DEFAULT false,
    source              TEXT,                     -- 'google_photos', 'facebook', 'filesystem', etc.
    original_filename   TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

-- Email messages
CREATE TABLE emails (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    message_id          TEXT,                     -- RFC 2822 Message-ID
    thread_id           TEXT,
    labels              TEXT[],                   -- IMAP folders / Gmail labels
    from_address        TEXT,
    to_addresses        TEXT[],
    cc_addresses        TEXT[],
    bcc_addresses       TEXT[],
    subject             TEXT,
    date                TIMESTAMPTZ,
    raw_message         BYTEA,
    plain_text          TEXT,
    html_body           TEXT,
    snippet             TEXT,
    has_attachments     BOOLEAN DEFAULT false,
    classifications     TEXT[],
    created_at          TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX emails_labels_gin ON emails USING GIN(labels);
CREATE INDEX emails_fts ON emails USING GIN(to_tsvector('english', COALESCE(plain_text,'') || ' ' || COALESCE(subject,'')));

-- Email attachments
CREATE TABLE attachments (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    email_id            BIGINT REFERENCES emails(id) ON DELETE CASCADE,
    filename            TEXT,
    content_type        TEXT,
    size                BIGINT,
    data                BYTEA,
    image_thumbnail     BYTEA,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

-- Chat/messaging (WhatsApp, iMessage, SMS, Instagram, Facebook Messenger)
CREATE TABLE messages (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    chat_session        TEXT NOT NULL,            -- conversation identifier
    message_date        TIMESTAMPTZ,
    is_group_chat       BOOLEAN DEFAULT false,
    service             TEXT,                     -- 'whatsapp', 'imessage', 'sms', 'instagram', 'facebook'
    sender_id           TEXT,
    sender_name         TEXT,
    text                TEXT,
    status              TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX messages_fts ON messages USING GIN(to_tsvector('english', COALESCE(text,'')));
CREATE INDEX messages_chat_session ON messages(chat_session, user_id);

CREATE TABLE message_attachments (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    message_id          BIGINT REFERENCES messages(id) ON DELETE CASCADE,
    blob_id             BIGINT REFERENCES media_blobs(id),
    filename            TEXT,
    content_type        TEXT
);

-- People / Contacts
CREATE TABLE contacts (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    name                TEXT NOT NULL,
    alternative_names   TEXT[],
    rel_type            TEXT,                     -- 'family', 'friend', 'colleague', etc.
    email               TEXT[],
    facebook_id         TEXT,
    whatsapp_id         TEXT,
    imessage_id         TEXT,
    sms_id              TEXT,
    instagram_id        TEXT,
    description         TEXT,
    use_by_ai           BOOLEAN DEFAULT true,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE relationships (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    source_id           BIGINT REFERENCES contacts(id),
    target_id           BIGINT REFERENCES contacts(id),
    type                TEXT,
    strength            INT,
    is_active           BOOLEAN DEFAULT true,
    is_personal         BOOLEAN DEFAULT true,
    is_deleted          BOOLEAN DEFAULT false
);

-- Facebook albums
CREATE TABLE facebook_albums (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    name                TEXT,
    description         TEXT,
    cover_photo_uri     TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE album_media (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    album_id            BIGINT REFERENCES facebook_albums(id) ON DELETE CASCADE,
    media_item_id       BIGINT REFERENCES media_items(id),
    caption             TEXT,
    position            INT
);

-- Facebook posts
CREATE TABLE facebook_posts (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    post_timestamp      TIMESTAMPTZ,
    title               TEXT,
    post_text           TEXT,
    external_url        TEXT,
    post_type           TEXT
);

CREATE TABLE post_media (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    post_id             BIGINT REFERENCES facebook_posts(id) ON DELETE CASCADE,
    media_item_id       BIGINT REFERENCES media_items(id),
    caption             TEXT
);

-- Artefacts (physical objects)
CREATE TABLE artefacts (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    name                TEXT NOT NULL,
    description         TEXT,
    tags                TEXT[],
    story               TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE artefact_media (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    artefact_id         BIGINT REFERENCES artefacts(id) ON DELETE CASCADE,
    media_item_id       BIGINT REFERENCES media_items(id),
    position            INT
);

-- Reference documents (uploaded PDFs, docs, etc.)
CREATE TABLE reference_documents (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    filename            TEXT,
    title               TEXT,
    description         TEXT,
    content_type        TEXT,
    size                BIGINT,
    data                BYTEA,
    tags                TEXT[],
    category            TEXT,
    available_for_task  TEXT[],                  -- which AI tasks can access this doc
    is_private          BOOLEAN DEFAULT false,
    is_sensitive        BOOLEAN DEFAULT false,
    is_encrypted        BOOLEAN DEFAULT false,
    ai_detailed_summary TEXT,
    ai_quick_summary    TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

-- Gemini File API references
CREATE TABLE gemini_files (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    document_id         BIGINT REFERENCES reference_documents(id) ON DELETE CASCADE,
    gemini_file_name    TEXT,
    gemini_file_uri     TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

-- Location data
CREATE TABLE places (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    name                TEXT,
    description         TEXT,
    latitude            DOUBLE PRECISION,
    longitude           DOUBLE PRECISION,
    google_maps_url     TEXT
);

CREATE TABLE locations (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    address             TEXT,
    latitude            DOUBLE PRECISION,
    longitude           DOUBLE PRECISION,
    country             TEXT,
    city                TEXT
);
```

### 5.3 Configuration & Preferences Tables

```sql
CREATE TABLE interests (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    name                TEXT NOT NULL,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE subject_configuration (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    subject_name        TEXT,
    gender              TEXT,
    family_name         TEXT,
    other_names         TEXT[],
    writing_style_ai    TEXT,                    -- AI-generated writing style summary
    psychological_profile_ai TEXT,               -- AI-generated personality profile
    personality_profile_ai   TEXT,
    interests_ai        TEXT
);

CREATE TABLE app_system_instructions (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    chat_instructions   TEXT,
    core_instructions   TEXT,
    question_instructions TEXT,
    pambot_instructions TEXT
);

CREATE TABLE custom_voices (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    key                 TEXT NOT NULL,
    name                TEXT NOT NULL,
    description         TEXT,
    instructions        TEXT,
    creativity          FLOAT DEFAULT 0.7,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE chat_conversations (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    title               TEXT,
    voice               TEXT,
    last_message_at     TIMESTAMPTZ,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE chat_turns (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    conversation_id     BIGINT REFERENCES chat_conversations(id) ON DELETE CASCADE,
    user_input          TEXT,
    response_text       TEXT,
    voice               TEXT,
    temperature         FLOAT,
    turn_number         INT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE complete_profiles (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    name                TEXT,
    profile_text        TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE saved_responses (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    title               TEXT,
    content             TEXT,
    voice               TEXT,
    llm_provider        TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE app_configuration (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    key                 TEXT NOT NULL,
    value               TEXT,
    UNIQUE(user_id, key)
);

-- Encrypted key-value store (AES-256 encrypted values at rest)
CREATE TABLE private_store (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    key                 TEXT NOT NULL,
    encrypted_value     BYTEA,
    UNIQUE(user_id, key)
);

-- Encrypted sensitive data
CREATE TABLE sensitive_keyring (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    encrypted_dek       BYTEA,                  -- data encryption key, encrypted with master key
    encrypted_master_dek BYTEA,                 -- master DEK encrypted with derived key
    is_master           BOOLEAN DEFAULT false
);

-- Email-to-contact mapping helpers
CREATE TABLE email_matches (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    primary_name        TEXT,
    email               TEXT
);

CREATE TABLE email_exclusions (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    email               TEXT,
    name                TEXT,
    name_email          TEXT
);

CREATE TABLE email_classifications (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    name                TEXT,
    classification      TEXT
);

-- Master RSA keys (admin-only)
CREATE TABLE master_keys (
    id                  BIGSERIAL PRIMARY KEY,
    comment             TEXT,
    public_key          TEXT
);

-- Import job tracking
CREATE TABLE import_control_last_run (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    import_type         TEXT NOT NULL,
    last_run_at         TIMESTAMPTZ,
    UNIQUE(user_id, import_type)
);
```

### 5.4 Interviews Table

```sql
CREATE TABLE interviews (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    title               TEXT,
    style               TEXT,                    -- 'formal', 'casual', 'therapeutic', etc.
    purpose             TEXT,
    purpose_detail      TEXT,
    state               TEXT DEFAULT 'active',   -- 'active', 'paused', 'finished'
    provider            TEXT,                    -- 'claude' or 'gemini'
    writeup             TEXT,                    -- AI-generated narrative from interview
    last_turn_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE interview_turns (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    interview_id        BIGINT REFERENCES interviews(id) ON DELETE CASCADE,
    question            TEXT,
    answer              TEXT,
    turn_number         INT,
    created_at          TIMESTAMPTZ DEFAULT NOW()
);
```

### 5.5 Pam Bot Tables

```sql
CREATE TABLE pambot_sessions (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    session_id          TEXT NOT NULL,
    turns_count         INT DEFAULT 0,
    latest_analysis     TEXT,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, session_id)
);

CREATE TABLE pambot_turns (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT REFERENCES users(id),
    session_id          TEXT NOT NULL,
    question            TEXT,
    response            TEXT,
    subject_tag         TEXT,                   -- topic classification
    photo_url           TEXT,                   -- URL of photo shown in this turn
    created_at          TIMESTAMPTZ DEFAULT NOW()
);
```

### 5.6 Billing Database (`{DB_NAME}_billing`)

Separate database. Contains one table:

```sql
CREATE TABLE llm_usage_events (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT,                 -- nullable for legacy data
    provider            TEXT,                   -- 'claude' or 'gemini'
    model_name          TEXT,
    input_tokens        BIGINT DEFAULT 0,
    output_tokens       BIGINT DEFAULT 0,
    is_visitor          BOOLEAN DEFAULT false,
    used_server_llm_key BOOLEAN,               -- true = server key, false = user key, null = unknown
    succeeded           BOOLEAN DEFAULT true,
    error_message       TEXT,
    user_email          TEXT,                  -- snapshot at time of insert
    user_first_name     TEXT,
    user_family_name    TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 5.7 Row-Level Security

Enable RLS on all archive data tables:

```sql
-- Example for media_items (repeat for all archive tables)
ALTER TABLE media_items ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_isolation ON media_items
    USING (user_id IS NULL OR user_id = current_setting('app.current_user_id', true)::bigint);
```

> RLS uses `ENABLE` (not `FORCE`). The database owner role bypasses it. For full enforcement
> in production, the application DB role must NOT be the owner. This is defence-in-depth
> alongside the application-layer `AND user_id = $N` filter on every query.

---

## 6. Authentication & Authorization

### 6.1 Registration

`POST /auth/register`
```json
{ "email": "user@example.com", "password": "...", "display_name": "Alice", "family_name": "Smith", "gender": "female" }
```
- Email must be unique
- Password minimum 12 characters
- Password hashed with Argon2id (time=3, memory=65536 KB, threads=4, salt=16 random bytes)
- Returns `201 Created` with `{ "id": 1, "email": "...", "display_name": "..." }`

### 6.2 Login

`POST /auth/login`
```json
{ "email": "user@example.com", "password": "..." }
```
- Rate limited: 10 requests/minute per IP
- On success: creates 32-byte random session ID, stores in `sessions` table with 24-hour TTL
- Sets `dm_session` cookie: `HttpOnly; SameSite=Strict; Secure` (Secure flag per config)
- Returns `200 OK` with user profile JSON

### 6.3 Session Validation (Middleware)

Every request goes through auth middleware:
1. Read `dm_session` cookie
2. Look up session in DB; reject if expired
3. Slide TTL by 24 hours (keep-alive)
4. Inject into request context: `user_id`, `is_visitor`, `visitor_access_flags`
5. If no valid session on a non-exempt path: API calls get `401 JSON`, browser navigation gets `302 → /login`

**Exempt paths** (never require auth):
```
GET  /health
GET  /static/*
GET  /login
POST /auth/login
POST /auth/register
GET  /share/*
POST /share/*
GET  /s/*
GET  /visitor/*
```

### 6.4 Context Access Pattern

All layers read the current user via a shared context key:
```go
uid := appctx.UserIDFromCtx(ctx)  // returns 0 if unauthenticated
```

### 6.5 Visitor Sessions

When an archive owner creates a share token and a visitor joins it:
1. Visitor requests `GET /s/{token}` — receives the share visitor HTML page
2. Page calls `GET /share/{token}` for public metadata (label, has_password, expiry, owner name)
3. If password required: visitor submits via `POST /share/{token}`
4. Server validates, calls `authSvc.CreateShareSession(ctx, ownerUserID)` to create a `dm_session`
5. Session is tagged `share_link_session=true`, `is_visitor=true`
6. Visitor's browser receives the cookie and is redirected to `/`
7. All repository queries run under the **owner's** `user_id` — visitor sees exactly the owner's archive

The share token can also carry a `tool_access_policy` (see §8) that limits which AI tools are available in visitor sessions.

### 6.6 Visitor Key Hints

An alternative visitor access mechanism for trusted visitors:
1. Owner creates a `visitor_key_hints` record with granular feature flags and an optional email filter
2. Visitor visits `/visitor/hints?email=visitor@example.com` to discover available hints
3. Visitor calls `POST /visitor/join/{hintID}` to create a key-scoped session
4. Session respects the hint's flags: `can_messages_chat`, `can_emails`, `can_contacts`, `can_relationship_sensitive`

---

## 7. Keyring & Encryption

### 7.1 Overview

Two separate security concepts:
- `dm_session` cookie — identifies the user (DB-backed sessions)
- `dm_keyring_sid` cookie — carries the keyring unlock password in a RAM store

The keyring must be unlocked separately from login. Users set a keyring master password that
encrypts sensitive data entries and encrypted reference documents.

### 7.2 Key Derivation

```
Argon2id(
    password = user_master_password,
    salt     = static 16-byte seed,
    pepper   = KEYRING_PEPPER env var,
    time     = 1,
    memory   = 65536 KB,
    threads  = 4
) → 32-byte AES-256 key
```

### 7.3 Encrypted Storage

- `private_store` table: values encrypted with AES-256-GCM using derived key
- `sensitive_keyring` table: holds an encrypted data-encryption-key (DEK) structure
  - `encrypted_dek`: data encryption key encrypted with the session key
  - `encrypted_master_dek`: master DEK encrypted with the Argon2-derived key
- `reference_documents`: documents with `is_encrypted=true` have their `data` field encrypted

### 7.4 Keyring Endpoints

```
POST /session/unlock   { "password": "..." }   → unlock keyring (stores in RAM)
POST /session/lock                              → clear keyring from RAM
GET  /session/status                            → { "locked": true/false }
```

---

## 8. AI Tool System

### 8.1 Tool Access Control

Three tiers:
- `TierNone` — tools not available (keyring locked or no access)
- `TierVisitor` — limited tool set for visitor sessions
- `TierMaster` — full tool access for authenticated owners

Each tool has a `ToolAccessRule`:
```go
type ToolAccessRule struct {
    NoKey   bool // allow when no keyring (public data only)
    Visitor bool // allow for visitor sessions
    Master  bool // allow for owner sessions
}
```

Tool access policy is stored as JSONB in `archive_shares` and `private_store`. Owners can
configure per-tool visibility via `PUT /api/llm-tools-access`.

### 8.2 Tool Definitions

All tools accept a JSON schema `input` and return a text result. The tool executor runs SQL
queries scoped by `user_id`.

| Tool Name | Description | Default Tier |
|---|---|---|
| `get_current_time` | Return current UTC timestamp | NoKey |
| `get_imessages_by_chat_session` | Get all messages in a named chat session | Visitor |
| `get_messages_around_in_chat` | Get 20 messages before/after anchor message ID | Visitor |
| `list_available_chat_sessions` | List all message conversation names and services | Visitor |
| `search_chat_messages_globally` | Full-text search across all message conversations | Visitor |
| `search_chat_messages_in_session` | Full-text search within a single chat session | Visitor |
| `get_emails_by_contact` | Get emails where contact is sender or recipient | Visitor |
| `get_all_messages_by_contact` | Get all communications (email + messages) with contact | Visitor |
| `get_subject_writing_examples` | Retrieve writing samples for voice mimicry | Master |
| `search_tavily` | Web search via Tavily API (requires TAVILY_API_KEY) | Master |
| `search_facebook_albums` | Search albums by name/description | Visitor |
| `get_album_images` | Get first 5 images from a Facebook album | Visitor |
| `search_facebook_posts` | Search posts by text | Visitor |
| `get_unique_tags_count` | Get tag statistics across the archive | Master |
| `get_user_interests` | Get archive owner's interest list | Visitor |
| `get_reference_document` | Retrieve reference document content by ID | Master |
| `get_available_reference_documents` | List all reference documents with metadata | Master |
| `list_interviews` | List interviews with optional state filter | Master |
| `get_interview` | Get full interview details and turns | Master |

### 8.3 Pam Bot Tool Subset

Pam Bot uses a smaller, focused tool set:
- `search_facebook_albums`, `search_facebook_posts`, `get_album_images`
- `get_reference_document`, `get_available_reference_documents`
- `search_chat_messages_globally`, `search_chat_messages_in_session`
- `get_emails_by_contact`, `list_available_chat_sessions`
- `get_imessages_by_chat_session`, `get_messages_around_in_chat`

### 8.4 Tool Execution Safety

- All SQL in tools MUST include `AND user_id = $N` (or use a helper that appends it)
- Tool results are returned as plain text (not JSON) to the LLM
- Max 15 tool-call iterations per request to prevent runaway loops
- Tool access policy is checked before each tool invocation

---

## 9. AI Providers

### 9.1 Claude (Anthropic)

- API endpoint: `https://api.anthropic.com/v1/messages`
- Authentication: `x-api-key` header
- Default model: `claude-sonnet-4-6`
- Supports prompt caching (ephemeral cache control)
- Tool calling: native `tools` parameter
- Max tool iterations: 15

Request to `/chat/generate` with `provider: "claude"` routes through:
1. Build tool declarations from `GetToolDefinitions()` filtered by access tier
2. Build system prompt (subject config + voice instructions with pronoun substitution)
3. Load last 30 turns from `chat_turns` as conversation history
4. Execute tool loop: send message → if tool_use response → call executor → append result → repeat
5. Return final text response, save turn to DB
6. Record LLM usage event to billing DB (best-effort, never fails the request)

### 9.2 Gemini (Google)

- API endpoint: `https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent`
- Authentication: `key` query parameter
- Default model: `gemini-2.5-flash`
- Function calling: native `tools` parameter with function declarations
- Max tool iterations: 15
- Supports Gemini File API for document uploads

### 9.3 Provider Selection per Request

Every chat request includes a `provider` field (`"claude"` or `"gemini"`). The system checks
availability:
- A provider is available if its API key is set in config, OR if the user has their own key stored,
  OR if a visitor session has LLM key overrides

### 9.4 Per-User API Keys

Users can store their own Gemini and Claude API keys (stored encrypted). When present, the user's
key is used instead of the server key. The `allow_server_keys` flag controls whether the server
key is a fallback.

---

## 10. Chat System

### 10.1 Conversations

Chat is organized into named conversations, each with a voice setting.

```
POST   /chat/conversations         { "title": "...", "voice": "default" }
GET    /chat/conversations         ?limit=20
GET    /chat/conversations/{id}
PUT    /chat/conversations/{id}    { "title": "...", "voice": "..." }
DELETE /chat/conversations/{id}
GET    /chat/conversations/{id}/turns
```

### 10.2 Message Generation

```
POST /chat/generate
{
    "prompt":       "Tell me about Alice",
    "provider":     "claude",
    "conversation_id": 42,
    "voice":        "historical",
    "temperature":  0.7
}
```

Response:
```json
{
    "response": "Based on your archive...",
    "turn_id":  99
}
```

System prompt construction:
1. Load `subject_configuration` for current user
2. Load voice instructions from `custom_voices` or built-in voices JSON
3. Substitute `{SUBJECT_NAME}`, `{he}`, `{him}`, `{his}` with gender-appropriate pronouns
4. Append `app_system_instructions.chat_instructions` if set

### 10.3 Built-in Voices

Defined in `static/data/voice_instructions.json` (or equivalent config file). Each voice has:
- `key` — internal identifier
- `name` — display name
- `description` — shown in UI
- `instructions` — appended to system prompt
- `creativity` — default temperature (0.0–1.0)

Custom voices are stored in the `custom_voices` DB table and served via `GET /api/voices`.

### 10.4 Random Question Generation

```
POST /chat/generate-random-question
{ "provider": "claude" }
```

Returns a random contextually-aware question to spark conversation.

---

## 11. Complete Profile Feature

The system can build a complete narrative profile of the archive subject by analysing all data.

```
GET    /chat/complete-profile/names   → list of profiles
GET    /chat/complete-profile         → current profile text
POST   /chat/complete-profile         → trigger AI profile build
PUT    /chat/complete-profile         { "name": "...", "profile_text": "..." }
DELETE /chat/complete-profile
```

---

## 12. Email System

### 12.1 Viewing Emails

```
GET /emails/folders                               → list of distinct labels/folders
GET /emails/label?labels=INBOX&labels=SENT        → emails in given folders
GET /emails/{id}/html                             → HTML body
GET /emails/{id}/text                             → plain text body
GET /emails/{id}/snippet                          → brief excerpt
GET /emails/{id}/metadata                         → from, to, cc, bcc, date, subject
```

### 12.2 Searching Emails

```
GET /emails/search?from_address=alice&subject=birthday&from=2020-01-01&to=2023-12-31
```

Full-text search using PostgreSQL `tsvector` on `plain_text` and `subject`.

### 12.3 Email Management

```
PUT    /emails/{id}           { "classifications": ["personal", "important"] }
DELETE /emails/{id}
DELETE /emails/bulk-delete    { "ids": [1, 2, 3] }
```

### 12.4 Thread Summarization

```
POST /emails/thread/{participant}/summarize
```

Uses Gemini `SimpleGenerate` to produce a prose summary of the email thread with the given contact.

---

## 13. Messages System

Covers WhatsApp, iMessage, SMS, Instagram DMs, Facebook Messenger — all stored in the same
`messages` table with a `service` discriminator.

```
GET    /imessages/chat-sessions                    → all conversation identifiers
GET    /imessages/conversation/{chat_session}      → all messages in session
DELETE /imessages/conversation/{chat_session}      → delete entire conversation
POST   /imessages/conversation/{chat_session}/summarize
GET    /imessages/{message_id}/metadata
GET    /imessages/{message_id}/attachment          → binary attachment image
```

---

## 14. Images & Media

### 14.1 Image Retrieval

```
GET /images/{id}                → metadata
GET /images/{id}/image          ?preview=true|false   → binary JPEG/PNG
GET /images/timeline            → images grouped by year/month
GET /images/filter              ?year=2022&month=6&tags=birthday&rating=4
GET /images/years               → list of distinct years
GET /images/tags                → list of all tags
GET /images/locations           → images with GPS coordinates
GET /images/places              → Facebook places with GPS
```

### 14.2 Image Update / Delete

```
PUT    /images/{id}   { "description": "...", "tags": ["birthday"], "rating": 5 }
DELETE /images/{id}
```

### 14.3 Facebook Albums

```
GET /facebook/albums
GET /facebook/albums/{album_id}/images
GET /facebook/albums/{album_id}/images/{image_id}   → binary image
```

### 14.4 Facebook Posts

```
GET /facebook/posts              ?page=1&limit=20
GET /facebook/posts/{post_id}/media
GET /facebook/posts/{post_id}/media/{media_id}      → binary media
```

---

## 15. Contacts & Relationships

### 15.1 Contact Management

```
GET    /contacts             ?search=alice&page=1&limit=50
GET    /contacts/{id}
DELETE /contacts/{id}
PATCH  /contacts/{id}        { "rel_type": "family" }
GET    /contacts/names       → compact list of names only
```

### 15.2 Relationship Graph

```
GET /contacts/relationship-graph?types=family,friend&sources=facebook&maxNodes=100
```

Returns graph data (nodes + edges) for rendering in a force-directed graph visualisation.

### 15.3 Email-Contact Mappings

Manual mappings connecting email addresses to contact names, used by the AI to resolve identities.

```
GET    /contacts/email-matches
POST   /contacts/email-matches    { "primary_name": "Alice Smith", "email": "alice@example.com" }
PUT    /contacts/email-matches/{id}
DELETE /contacts/email-matches/{id}
```

### 15.4 Email Exclusions

Email addresses to exclude from contact discovery.

```
GET    /contacts/email-exclusions
POST   /contacts/email-exclusions   { "email": "noreply@example.com" }
PUT    /contacts/email-exclusions/{id}
DELETE /contacts/email-exclusions/{id}
```

### 15.5 Email Classifications

Rules for classifying emails by contact name and type.

```
GET    /contacts/email-classifications
POST   /contacts/email-classifications   { "name": "Alice Smith", "classification": "family" }
PUT    /contacts/email-classifications/{id}
DELETE /contacts/email-classifications/{id}
```

---

## 16. Artefacts

Physical objects documented in the archive.

```
GET    /artefacts             ?search=watch&tags=heirloom
GET    /artefacts/{id}
GET    /artefacts/{id}/thumbnail    → binary image
POST   /artefacts             { "name": "Grandfather's Watch", "description": "...", "tags": [], "story": "..." }
PUT    /artefacts/{id}
DELETE /artefacts/{id}
```

---

## 17. Reference Documents

Documents uploaded for AI reference (PDFs, Word docs, text files).

```
GET    /documents             ?search=...&category=...&tag=...&content_type=...
GET    /documents/{id}
GET    /documents/{id}/data         → raw file download
GET    /documents/summary/{id}      → AI-generated summary
POST   /documents             multipart/form-data (file + metadata)
PUT    /documents/{id}
DELETE /documents/{id}
```

Documents can be marked:
- `is_private` — not shown in general listings
- `is_sensitive` — encrypted at rest
- `available_for_task` — list of AI task types that can access this document
- `is_encrypted` — stored encrypted, requires keyring unlock to download

---

## 18. Sensitive Data

Encrypted key-value entries requiring keyring unlock.

```
GET    /sensitive          (requires keyring unlock via session)
GET    /sensitive/{id}
POST   /sensitive          { "description": "...", "details": "...", "is_private": true }
PUT    /sensitive/{id}
DELETE /sensitive/{id}
```

Values are encrypted using the keyring-derived AES-256 key. The keyring must be unlocked
(via `POST /session/unlock`) before these endpoints function.

---

## 19. Private Store

Encrypted key-value configuration store.

```
GET    /private-store
GET    /private-store/{key}
POST   /private-store        { "key": "...", "value": "..." }
PUT    /private-store/{key}  { "value": "..." }
DELETE /private-store/{key}
```

---

## 20. Custom Voices

```
GET    /api/voices            → all voices (built-in + custom)
GET    /api/voices/custom     → custom voices only
POST   /api/voices            { "key": "aunt_mary", "name": "Aunt Mary", "instructions": "...", "creativity": 0.8 }
PUT    /api/voices/{id}
DELETE /api/voices/{id}
```

---

## 21. Interests

Simple list of archive owner's interests, used as AI context.

```
GET    /interests
POST   /interests    { "name": "Photography" }
PUT    /interests/{id}
DELETE /interests/{id}
```

---

## 22. Saved Responses

Saved AI response templates.

```
GET    /saved-responses
POST   /saved-responses    { "title": "...", "content": "...", "voice": "...", "llm_provider": "claude" }
PUT    /saved-responses/{id}
DELETE /saved-responses/{id}
```

---

## 23. Attachments

Email attachment browser.

```
GET /attachments            ?page=1&limit=20&order=size&direction=desc
GET /attachments/{id}/info
GET /attachments/{id}/data  → raw attachment download
DELETE /attachments/{id}
```

---

## 24. Interview System

Structured life-story interviews where the AI asks questions and saves answers.

```
POST   /interviews           { "title": "...", "style": "casual", "purpose": "memoir", "purpose_detail": "...", "provider": "claude" }
GET    /interviews
GET    /interviews/{id}
PUT    /interviews/{id}      { "state": "paused" }   or   { "state": "finished" }
DELETE /interviews/{id}
POST   /interviews/{id}/turns   { "answer": "..." }  → AI generates next question, saves Q&A
GET    /interviews/{id}/turns
```

When an interview is marked `finished`, the AI generates a prose writeup from all turns and saves
it to `interviews.writeup`.

Interview styles: `formal`, `casual`, `therapeutic`, `journalistic`
Interview states: `active`, `paused`, `finished`

---

## 25. Pam Bot (Memory Companion)

A specialised AI assistant for people with dementia or memory challenges. It asks gentle questions
about photos, memories, and familiar people, maintaining continuity across sessions.

```
POST /api/pambot/message
{
    "action": "next",      // "next" | "repeat" | "back" | "custom"
    "typed_text": "..."    // only for action: "custom"
}
→ { "message": "...", "photo_url": "...", "session_id": "..." }

GET /api/pambot/session
→ { "session_id": "...", "interaction_count": 42, "latest_analysis": "..." }
```

**Behaviour:**
- Maintains a dedicated session per user (or per-day session, implementation choice)
- After every N turns (configurable, e.g. 10), generates an `analysis` summarising topics discussed
- Uses the Pam Bot tool subset to find relevant photos and memories
- Always responds warmly and reassuringly; never contradicts or corrects
- System prompt loaded from `app_system_instructions.pambot_instructions`

---

## 26. Share Token System

```
POST   /share           { "label": "Family Visit", "password": "opt", "expires_at": "2025-12-31T00:00:00Z", "tool_access_policy": {...} }
GET    /share           → list of active share tokens
DELETE /share/{token}
GET    /share/{token}   → public info (label, has_password, owner display name, expiry)
POST   /share/{token}   { "password": "..." }  → validate and create visitor session
GET    /s/{token}       → serve visitor HTML page (no auth required)
```

---

## 27. Data Import Pipeline

### 27.1 Import Tiers

| Tier | Method | Endpoint |
|---|---|---|
| A | IMAP credentials (server-to-server) | `POST /imap/process` |
| B | ZIP file upload (Facebook/Instagram/WhatsApp/iMessage exports) | `POST /import/upload` |
| C1 | Browser folder picker (photos) | `POST /import/photo-batch` |
| D | Server-triggered jobs | Various `/import/*` endpoints |

### 27.2 IMAP Email Import

```
POST /imap/test       { "host": "imap.gmail.com", "port": 993, "user": "...", "password": "...", "use_tls": true }
POST /imap/folders    { ...same... }   → list available folders
POST /imap/process    { ...+ "folders": ["INBOX", "SENT"], "new_only": false }
GET  /imap/status     → { "running": true, "progress": "...", "emails_imported": 1234 }
```

### 27.3 Gmail OAuth Import

```
GET  /gmail/auth/authorize   → redirect to Google OAuth
GET  /gmail/auth/callback    → OAuth callback, exchanges code for token
POST /gmail/process          → start Gmail import with stored token
GET  /gmail/status
```

### 27.4 ZIP Upload Import (Tier B)

Supports resumable uploads via Tus protocol:

```
POST /import/upload              multipart with type + file
GET  /import/upload/stream       SSE stream of import progress
GET  /import/upload/status       → job status
POST /import/photo-batch         multipart with multiple image files
GET  /import/jobs                → list of all import jobs with status
GET  /api/upload-config          → { "chunk_size_mb": 10, "max_gb": 32 }
```

Tus endpoints at `/files/`:
```
POST   /files/             → create upload
HEAD   /files/{id}         → check upload status
PATCH  /files/{id}         → upload chunk
```

Supported ZIP formats:
- `facebook` — Facebook data export (albums, posts, messenger, places)
- `instagram` — Instagram data export (messages, photos)
- `whatsapp` — WhatsApp chat export (conversations, media)
- `imessage` — iMessage backup export (conversations, attachments)

### 27.5 Server-Side Import Jobs (Tier D)

```
POST /import/thumbnails          → generate thumbnails for all media without them
POST /import/facebook-albums     → re-process Facebook albums
POST /import/facebook-posts      → re-process Facebook posts
GET  /import/status              → current job status
```

### 27.6 Import Safety

- Each import job runs in a background goroutine
- `user_id` is captured from HTTP context BEFORE goroutine launch and threaded explicitly:
  ```go
  uid := appctx.UserIDFromCtx(r.Context())
  go func() {
      ctx := context.WithValue(context.Background(), appctx.ContextKeyUserID, uid)
      // ... run import
  }()
  ```
- Only one import job may run at a time per user (singleton job guard)
- Progress is reported via Server-Sent Events (SSE)

---

## 28. Dashboard

```
GET /dashboard
→ {
    "email_count": 12345,
    "message_count": 8901,
    "image_count": 4567,
    "contact_count": 234,
    "artefact_count": 45,
    "document_count": 12,
    "facebook_post_count": 678,
    "facebook_album_count": 34,
    "interview_count": 3
}
```

---

## 29. Admin Console

### 29.1 User Management

Admin routes require the requesting user to be an admin (flagged in `users.is_admin` or via
separate admin session mechanism — implementation choice).

```
GET    /admin                → admin dashboard HTML
GET    /admin/users          → list all users
POST   /admin/users          { "email": "...", "password": "...", "display_name": "..." }
GET    /admin/users/{id}
PUT    /admin/users/{id}     { "display_name": "...", "is_admin": true }
DELETE /admin/users/{id}     → delete user and all their data
```

### 29.2 LLM Usage (Billing)

```
GET /admin/users/{id}/billing?from=2025-01-01T00:00:00Z&to=2025-02-01T00:00:00Z
    → { "events": [...], "summary": { "total_input_tokens": ..., "total_output_tokens": ..., "event_count": ... } }

GET /admin/llm-usage/users/{id}/bill.pdf?from=...&to=...
    → PDF binary
```

### 29.3 Import Control

```
GET /api/import-control-last-run   → { "whatsapp": "2025-01-15T...", "facebook": "..." }
GET /api/control-defaults          → pre-filled default paths for import forms
```

### 29.4 Data Cleanup

```
DELETE /admin/empty-media-tables   → delete media_blobs not referenced by any media_items
```

### 29.5 AI Profile Generation (Admin-triggered)

```
POST /writing-style/summarize        → AI generates writing style summary from emails
POST /psychological-profile/summarize → AI generates personality profile
```

---

## 30. LLM Tools Access Policy

Owners can configure which AI tools are accessible in visitor sessions.

```
GET /api/llm-tools-access
→ {
    "tools": [
        { "name": "get_emails_by_contact", "description": "...", "visitor_allowed": true, "master_only": false },
        ...
    ],
    "policy": { "get_emails_by_contact": { "visitor": true }, ... }
}

PUT /api/llm-tools-access
{ "policy": { "get_emails_by_contact": { "visitor": false } } }
```

---

## 31. LLM Usage & Billing (User-facing)

Archive owners can view and download their own usage statements.

```
GET /api/llm-usage/me/bill.pdf?period=current    → PDF for current UTC calendar month
GET /api/llm-usage/me/bill.pdf?period=previous   → PDF for previous UTC calendar month
```

PDF includes:
- Summary (total input/output tokens, event count)
- Breakdown by provider (Claude/Gemini)
- Breakdown by session type (owner/visitor)
- Event detail table (truncated at 50k rows)
- 5-minute bucket timeseries chart data

---

## 32. Session & Keyring Endpoints

```
POST /session/unlock    { "password": "keyring_password" }
POST /session/lock
GET  /session/status    → { "locked": false }
```

---

## 33. Configuration Endpoints

```
GET    /config              → list app_configuration entries
DELETE /config/{key}        → delete configuration entry
```

---

## 34. Health Check

```
GET /health   → 200 OK   { "status": "ok" }
```

---

## 35. Data Isolation Guarantee

Every archive data table enforces isolation at three independent layers:

1. **Application layer** — every repository method appends `AND user_id = $N` to all
   SELECT/UPDATE/DELETE queries, and includes `user_id = $N` in INSERT statements.

2. **PostgreSQL RLS** — Row-Level Security policies on all archive tables reject rows
   where `user_id != current_setting('app.current_user_id')`.

3. **Session binding** — the `user_id` is extracted from the validated session in middleware
   and injected into the request context. It cannot be overridden by client input.

When `user_id = 0` (unauthenticated single-tenant mode), queries use `user_id IS NULL` to
match legacy rows without a user FK, preserving backward compatibility.

---

## 36. Security Requirements (Non-Negotiable)

- Password hashing: Argon2id only (never bcrypt, SHA-*, MD5)
- Session IDs: 32 cryptographically random bytes (never sequential IDs)
- Session storage: server-side in DB (never JWT for session management)
- Cookies: `HttpOnly; SameSite=Strict`; `Secure` flag when `SESSION_COOKIE_SECURE=true`
- Rate limiting on `/auth/login` and `/auth/register`: 10 req/min per IP
- SQL injection: all queries use parameterised placeholders (never string concatenation)
- File uploads: validate content type and enforce size limits
- Import goroutines: never inherit HTTP request context (use `context.WithValue` with explicit `user_id`)
- Billing inserts: best-effort, must NEVER fail the user-facing request
- Keyring master key: NEVER persisted to disk, stored only in RAM
- `.env` / secrets: never committed, never logged
- Admin routes: separate auth check; admin status is NOT inferrable from a regular session cookie

---

## 37. Frontend Application

### 37.1 Architecture

The frontend is a Single Page Application (SPA) served from `GET /`. All data fetching uses the
REST API described above. Recommended: React + TypeScript + Vite + Tailwind CSS + shadcn/ui.

### 37.2 Key UI Sections

**Top Navigation Bar**
- Application title
- Voice selector (built-in + custom voices)
- Mood/style selector (per voice)
- Creativity slider (maps to LLM temperature 0.0–1.0)
- "Companion mode" toggle (simpler Pam Bot interface vs full chat)
- "Who's asking?" toggle (owner vs visitor perspective)
- Account menu: billing PDF download, change password, sign out

**Left Sidebar / Navigation Panel**
- Messages & Chats (WhatsApp, iMessage, SMS, Instagram, Messenger)
- Emails
- Images (gallery + timeline + map view)
- Facebook Albums
- Facebook Posts
- Locations (GPS map)
- Artefacts
- Memory Companion (Pam Bot)
- Import Data
- Configuration / Settings

**Main Chat Area**
- Chat history with message threading per conversation
- Conversation list/switcher
- Markdown rendering for AI responses
- Copy-to-clipboard on AI responses
- Provider selector (Claude / Gemini) per message

**Modal Panels (full-screen or side-drawer)**
- Email gallery with full viewer, search, thread view
- Image gallery: grid, timeline, map with GPS pins
- Message conversation viewer with attachments
- Facebook albums + posts
- Artefacts collection with photo viewer
- Contact graph: force-directed relationship visualisation
- Reference documents browser
- Sensitive data viewer (requires keyring unlock)
- Settings panel (see §37.3)
- Data import controls

### 37.3 Settings Panel (Tabs)

1. **Subject** — edit subject name, gender, family name, pronouns
2. **Voices** — manage custom voices, preview
3. **System Prompt** — edit chat/core/question/pambot instructions
4. **API Keys** — per-user Gemini/Claude/Tavily keys + allow-server-keys toggle
5. **Tool Access** — configure which tools visitors can use
6. **Share Links** — create/manage share tokens
7. **Visitor Keys** — manage visitor key hints with feature flags
8. **Interviews** — browse/manage interview sessions
9. **Profile** — complete profile builder
10. **Writing Style** — trigger AI writing style analysis
11. **Admin** — user management, billing (admin only)

### 37.4 Pam Bot UI

Dedicated companion interface:
- Large text display showing current AI message
- Photo display (when AI references a photo)
- Three large buttons: "Next", "Repeat", "Back"
- Optional text input for typed responses
- Session counter display

### 37.5 Import UI

Step-by-step wizard:
1. Select import type (Facebook, WhatsApp, iMessage, Instagram, Gmail, IMAP, Photos)
2. For ZIP imports: drag-and-drop or file picker with Tus resumable upload progress bar
3. For IMAP: credentials form with test connection button
4. For Gmail: OAuth flow button
5. For photo batch: folder picker (browser File API)
6. Progress display (SSE stream)
7. Import history list

### 37.6 Relationship Graph

Force-directed graph (e.g., D3.js or React Flow):
- Nodes represent contacts
- Edges represent relationships with type and strength
- Filter by relationship type, source, max nodes
- Click node to open contact details

### 37.7 Image Gallery

- Grid view with thumbnails
- Timeline view grouped by year/month
- Map view with GPS pins (Leaflet or similar)
- Filter bar: year, month, tags, rating, source
- Click to open full-size with metadata edit

---

## 38. Migrations

Database migrations are applied automatically at server startup. Rules:
- Never modify existing migration logic — always add a new migration function
- Migration functions must be idempotent (`CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`)
- Migrations run in order; completed migrations are tracked in a `schema_migrations` table
- The billing database is migrated separately at startup

---

## 39. Error Response Format

All API error responses use consistent JSON:

```json
{
    "error": "human-readable error message",
    "code":  "MACHINE_READABLE_CODE"
}
```

HTTP status codes:
- `400` — invalid request body or parameters
- `401` — not authenticated
- `403` — authenticated but not authorised (wrong user, wrong tier)
- `404` — resource not found
- `409` — conflict (e.g., duplicate email on register)
- `429` — rate limited
- `500` — internal server error (never expose stack traces)

---

## 40. Rate Limiting

| Endpoint | Limit |
|---|---|
| `POST /auth/login` | 10 req/min per IP |
| `POST /auth/register` | 10 req/min per IP |
| All others | No default limit (implementation may add) |

Rate limiting implementation: in-process token bucket (no external dependency required).

---

## 41. Logging

- Structured log line per HTTP request: method, path, status, duration, user_id (if authenticated)
- No request body or response body logging (PII/security)
- No stack traces in HTTP responses (log internally only)
- Optional pprof profiling server on `:6060` when `ENABLE_PPROF=true`

---

## 42. Non-functional Requirements

- **Startup time:** Full server ready (including DB migration) in under 10 seconds on typical hardware
- **Archive scale:** Support archives with 100k+ emails, 50k+ photos, 500k+ messages without pagination issues
- **Concurrent users:** Support at least 20 concurrent active users per deployment
- **AI tool timeout:** Each tool call must complete within 30 seconds
- **LLM request timeout:** Full chat generation (including tool loops) must complete within 120 seconds
- **Upload size:** Support resumable uploads up to 512 GB (Tus protocol)
- **Photo batch:** Support batch of 1000+ photos in a single request
- **Billing inserts:** Must be non-blocking and never delay the chat response

---

## 43. Feature Flags / Per-User Overrides

The following behaviours are configurable per-user via `private_store` or `users` columns:
- LLM provider API keys (override server keys)
- Whether server-level API keys are permitted as fallback
- LLM tool access policy (which tools visitors can use)
- Keyring pepper (do not allow per-user override — this is always from env)

---

## 44. Glossary

| Term | Meaning |
|---|---|
| **Archive** | The complete digital data collection belonging to one user |
| **Subject** | The person whose digital life the archive represents (may differ from the account owner) |
| **Voice** | A persona/style the AI adopts when generating responses |
| **Turn** | A single user prompt + AI response pair in a conversation |
| **Tool** | A function the AI can call to query the archive database |
| **Tier** | Tool access level: None / Visitor / Master |
| **Keyring** | Per-user encrypted key store unlocked by a separate master password |
| **Share token** | A URL token granting a visitor read access to an owner's archive |
| **Visitor key hint** | A configured access rule for a specific trusted visitor |
| **Pam Bot** | Memory companion AI for dementia support |
| **DEK** | Data Encryption Key — symmetric key encrypted by the keyring master key |
| **Billing DB** | Separate PostgreSQL database (`{DB_NAME}_billing`) tracking LLM token usage |
