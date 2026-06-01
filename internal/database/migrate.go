package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// ReservedUserSlotID is the fixed users.id reserved so the first real archive owner gets id=2.
const ReservedUserSlotID int64 = 1

// ReservedUserEmail is the sentinel email for the id=1 placeholder row (not a login account).
const ReservedUserEmail = "__dm_reserved__@digitalmuseum.invalid"

// schemaDDL returns the complete CREATE TABLE and CREATE INDEX statements for a
// fresh database (single source of truth for new installs). Incremental ALTERs
// and data backfills run in MigrateSQLite() and dedicated helpers below.
func schemaDDL() []string {
	return []string{

		// ── users (identity — no user_id FK) ─────────────────────────────────
		`CREATE TABLE IF NOT EXISTS users (
			id                     BIGSERIAL    PRIMARY KEY,
			email                  VARCHAR(255) NOT NULL UNIQUE,
			password_hash          VARCHAR(255) NOT NULL,
			display_name           VARCHAR(255),
			first_name             VARCHAR(255),
			family_name            VARCHAR(255),
			user_gemini_api_key    TEXT,
			user_anthropic_api_key TEXT,
			user_gemini_model      TEXT,
			user_claude_model      TEXT,
			user_tavily_api_key    TEXT,
			user_deepseek_api_key  TEXT,
			user_deepseek_model    TEXT,
			user_runpod_api_key    TEXT,
			user_elevenlabs_api_key TEXT,
			allow_server_llm_keys  BOOLEAN      NOT NULL DEFAULT TRUE,
			created_at             TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_login_at          TIMESTAMPTZ,
			is_active              BOOLEAN      NOT NULL DEFAULT TRUE,
			is_admin               BOOLEAN      NOT NULL DEFAULT FALSE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users (email)`,

		// ── sensitive_keyring ─────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS sensitive_keyring (
			id                   SERIAL  PRIMARY KEY,
			encrypted_dek        BYTEA   NOT NULL,
			encrypted_master_dek BYTEA,
			is_master            BOOLEAN NOT NULL DEFAULT FALSE,
			created_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id              BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sensitive_keyring_user_id ON sensitive_keyring (user_id)`,

		// ── visitor_key_hints ─────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS visitor_key_hints (
			id                         SERIAL  PRIMARY KEY,
			keyring_id                 INTEGER NOT NULL REFERENCES sensitive_keyring(id) ON DELETE CASCADE,
			hint                       TEXT    NOT NULL,
			can_messages_chat          BOOLEAN NOT NULL DEFAULT FALSE,
			can_emails                 BOOLEAN NOT NULL DEFAULT FALSE,
			can_contacts               BOOLEAN NOT NULL DEFAULT FALSE,
			can_relationship_sensitive BOOLEAN NOT NULL DEFAULT FALSE,
			can_sensitive_private      BOOLEAN NOT NULL DEFAULT FALSE,
			llm_allow_owner_keys       BOOLEAN NOT NULL DEFAULT TRUE,
			llm_allow_server_keys      BOOLEAN NOT NULL DEFAULT TRUE,
			created_at                 TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id                    BIGINT REFERENCES users(id) ON DELETE CASCADE,
			UNIQUE (keyring_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visitor_key_hints_keyring_id ON visitor_key_hints (keyring_id)`,
		`CREATE INDEX IF NOT EXISTS idx_visitor_key_hints_user_id    ON visitor_key_hints (user_id)`,

		// ── sessions (identity — no per-row user_id scoping needed) ──────────
		`CREATE TABLE IF NOT EXISTS sessions (
			id                    VARCHAR(64)  PRIMARY KEY,
			user_id               BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			expires_at            TIMESTAMPTZ  NOT NULL,
			created_at            TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
			is_visitor            BOOLEAN      NOT NULL DEFAULT FALSE,
			visitor_llm_overrides JSONB,
			share_link_session    BOOLEAN      NOT NULL DEFAULT FALSE,
			visitor_key_hint_id   BIGINT REFERENCES visitor_key_hints(id) ON DELETE SET NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id    ON sessions (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions (expires_at)`,

		// ── archive_shares (identity — no per-row user_id scoping needed) ─────
		`CREATE TABLE IF NOT EXISTS archive_shares (
			id                 VARCHAR(64)  PRIMARY KEY,
			user_id            BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			label              VARCHAR(255),
			password_hash      VARCHAR(255),
			expires_at         TIMESTAMPTZ,
			tool_access_policy JSONB,
			created_at         TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_archive_shares_user_id ON archive_shares (user_id)`,

		// ── audit_log (identity — no per-row user_id scoping needed) ─────────
		`CREATE TABLE IF NOT EXISTS audit_log (
			id          BIGSERIAL   PRIMARY KEY,
			user_id     BIGINT      REFERENCES users(id) ON DELETE SET NULL,
			event_type  VARCHAR(50) NOT NULL,
			ip_address  VARCHAR(45),
			user_agent  TEXT,
			details     JSONB,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_user_id    ON audit_log (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_event_type ON audit_log (event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log (created_at)`,

		// ── media_blobs ───────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS media_blobs (
			id             SERIAL PRIMARY KEY,
			image_data     BYTEA,
			thumbnail_data BYTEA,
			created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id        BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_media_blobs_user_id ON media_blobs (user_id)`,

		// ── media_items ───────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS media_items (
			id                 SERIAL PRIMARY KEY,
			media_blob_id      INTEGER NOT NULL REFERENCES media_blobs(id) ON DELETE RESTRICT,
			description        TEXT,
			title              VARCHAR(1000),
			author             VARCHAR(500),
			tags               TEXT,
			categories         TEXT,
			notes              TEXT,
			available_for_task BOOLEAN NOT NULL DEFAULT FALSE,
			media_type         VARCHAR(255),
			processed          BOOLEAN NOT NULL DEFAULT FALSE,
			created_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			embedding          TEXT,
			year               INTEGER,
			month              INTEGER,
			day                INTEGER,
			latitude           DOUBLE PRECISION,
			longitude          DOUBLE PRECISION,
			altitude           DOUBLE PRECISION,
			rating             INTEGER NOT NULL DEFAULT 5,
			has_gps            BOOLEAN NOT NULL DEFAULT FALSE,
			google_maps_url    VARCHAR(500),
			region             VARCHAR(255),
			is_personal        BOOLEAN NOT NULL DEFAULT FALSE,
			is_business        BOOLEAN NOT NULL DEFAULT FALSE,
			is_social          BOOLEAN NOT NULL DEFAULT FALSE,
			is_promotional     BOOLEAN NOT NULL DEFAULT FALSE,
			is_spam            BOOLEAN NOT NULL DEFAULT FALSE,
			is_important       BOOLEAN NOT NULL DEFAULT FALSE,
			use_by_ai          BOOLEAN DEFAULT FALSE,
			require_classification BOOLEAN NOT NULL DEFAULT TRUE,
			is_referenced      BOOLEAN NOT NULL DEFAULT FALSE,
			source             VARCHAR(255),
			source_reference   TEXT,
			user_id            BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_processed   ON media_items (processed)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_source      ON media_items (source)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_source_reference ON media_items (source, source_reference)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_media_type  ON media_items (media_type)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_year_month  ON media_items (year, month)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_use_by_ai   ON media_items (use_by_ai)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_user_id     ON media_items (user_id)`,

		// ── emails ────────────────────────────────────────────────────────────
		// Unique constraint scoped per user so different users can hold the same message.
		`CREATE TABLE IF NOT EXISTS emails (
			id              SERIAL PRIMARY KEY,
			uid             VARCHAR(255)  NOT NULL,
			folder          VARCHAR(255)  NOT NULL,
			subject         VARCHAR(1000),
			from_address    VARCHAR(500),
			to_addresses    TEXT,
			cc_addresses    TEXT,
			bcc_addresses   TEXT,
			date            TIMESTAMP,
			raw_message     TEXT,
			plain_text      TEXT,
			snippet         TEXT,
			embedding       TEXT,
			has_attachments BOOLEAN NOT NULL DEFAULT FALSE,
			user_deleted    BOOLEAN NOT NULL DEFAULT FALSE,
			is_personal     BOOLEAN NOT NULL DEFAULT FALSE,
			is_business     BOOLEAN NOT NULL DEFAULT FALSE,
			is_social       BOOLEAN NOT NULL DEFAULT FALSE,
			is_promotional  BOOLEAN NOT NULL DEFAULT FALSE,
			is_spam         BOOLEAN NOT NULL DEFAULT FALSE,
			is_important    BOOLEAN NOT NULL DEFAULT FALSE,
			use_by_ai       BOOLEAN NOT NULL DEFAULT TRUE,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id         BIGINT REFERENCES users(id) ON DELETE CASCADE,
			source          VARCHAR(255),
			CONSTRAINT uq_email_uid_folder_user UNIQUE (uid, folder, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_uid_folder ON emails (uid, folder)`,
		`CREATE INDEX IF NOT EXISTS idx_emails_date      ON emails (date)`,
		`CREATE INDEX IF NOT EXISTS idx_emails_user_id   ON emails (user_id)`,

		// ── attachments ───────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS attachments (
			id              SERIAL PRIMARY KEY,
			email_id        INTEGER NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
			filename        VARCHAR(500),
			content_type    VARCHAR(255),
			size            INTEGER,
			data            BYTEA,
			image_thumbnail BYTEA,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id         BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_email_id ON attachments (email_id)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_user_id  ON attachments (user_id)`,

		// ── messages ──────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS messages (
			id             SERIAL PRIMARY KEY,
			chat_session   VARCHAR(500),
			message_date   TIMESTAMP,
			is_group_chat  BOOLEAN NOT NULL DEFAULT FALSE,
			delivered_date TIMESTAMP,
			read_date      TIMESTAMP,
			edited_date    TIMESTAMP,
			service        VARCHAR(100),
			type           VARCHAR(50),
			sender_id      VARCHAR(255),
			sender_name    VARCHAR(500),
			status         VARCHAR(100),
			replying_to    VARCHAR(500),
			subject        VARCHAR(1000),
			text           TEXT,
			processed      BOOLEAN NOT NULL DEFAULT FALSE,
			created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id        BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chat_session ON messages (chat_session)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_sender_name  ON messages (sender_name)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_service      ON messages (service)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_processed    ON messages (processed)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_user_id      ON messages (user_id)`,

		// ── message_attachments ───────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS message_attachments (
			id            SERIAL PRIMARY KEY,
			message_id    INTEGER NOT NULL REFERENCES messages(id)    ON DELETE CASCADE,
			media_item_id INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
			created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id       BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_message_attachments_message_id    ON message_attachments (message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_attachments_media_item_id ON message_attachments (media_item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_attachments_user_id       ON message_attachments (user_id)`,

		// ── contacts ──────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS contacts (
			id                SERIAL PRIMARY KEY,
			name              TEXT    NOT NULL,
			alternative_names TEXT,
			rel_type          TEXT    DEFAULT 'unknown',
			use_by_ai         BOOLEAN DEFAULT FALSE,
			source            VARCHAR(255),
			source_reference  VARCHAR(500),
			is_subject        BOOLEAN DEFAULT FALSE,
			is_group          BOOLEAN DEFAULT FALSE,
			email             TEXT,
			numemails         INTEGER DEFAULT 0,
			facebookid        TEXT,
			numfacebook       INTEGER DEFAULT 0,
			whatsappid        TEXT,
			numwhatsapp       INTEGER DEFAULT 0,
			imessageid        TEXT,
			numimessages      INTEGER DEFAULT 0,
			smsid             TEXT,
			numsms            INTEGER DEFAULT 0,
			instagramid       TEXT,
			numinstagram      INTEGER DEFAULT 0,
			description       TEXT,
			total             INTEGER DEFAULT 0,
			created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id           BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_email    ON contacts (email)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_name     ON contacts (name)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_rel_type ON contacts (rel_type)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_user_id  ON contacts (user_id)`,

		// ── relationships ─────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS relationships (
			id             SERIAL PRIMARY KEY,
			source_id      INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
			target_id      INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
			type           TEXT    NOT NULL,
			description    TEXT,
			ai_description TEXT,
			strength       INTEGER,
			is_active      BOOLEAN NOT NULL DEFAULT TRUE,
			is_personal    BOOLEAN NOT NULL DEFAULT FALSE,
			is_deleted     BOOLEAN NOT NULL DEFAULT FALSE,
			created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id        BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_relationship_source  ON relationships (source_id)`,
		`CREATE INDEX IF NOT EXISTS idx_relationship_target  ON relationships (target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_relationship_type    ON relationships (type)`,
		`CREATE INDEX IF NOT EXISTS idx_relationships_user_id ON relationships (user_id)`,

		// ── facebook_albums ───────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS facebook_albums (
			id                      SERIAL PRIMARY KEY,
			name                    VARCHAR(500) NOT NULL,
			description             TEXT,
			cover_photo_uri         VARCHAR(500),
			last_modified_timestamp TIMESTAMP,
			created_at              TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at              TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id                 BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_facebook_albums_user_id ON facebook_albums (user_id)`,

		// ── album_media ───────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS album_media (
			id            SERIAL PRIMARY KEY,
			album_id      INTEGER NOT NULL REFERENCES facebook_albums(id) ON DELETE CASCADE,
			media_item_id INTEGER NOT NULL REFERENCES media_items(id)     ON DELETE CASCADE,
			created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id       BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_album_media_user_id ON album_media (user_id)`,

		// ── facebook_posts ────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS facebook_posts (
			id           SERIAL PRIMARY KEY,
			timestamp    TIMESTAMP,
			title        VARCHAR(500),
			post_text    TEXT,
			external_url VARCHAR(2000),
			post_type    VARCHAR(50),
			created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id      BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_facebook_posts_user_id ON facebook_posts (user_id)`,

		// ── post_media ────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS post_media (
			id            SERIAL PRIMARY KEY,
			post_id       INTEGER NOT NULL REFERENCES facebook_posts(id) ON DELETE CASCADE,
			media_item_id INTEGER NOT NULL REFERENCES media_items(id)    ON DELETE CASCADE,
			created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id       BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_post_media_user_id ON post_media (user_id)`,

		// ── artefacts ─────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS artefacts (
			id          SERIAL PRIMARY KEY,
			name        VARCHAR(1000) NOT NULL,
			description TEXT,
			tags        TEXT,
			story       TEXT,
			created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id     BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_artefacts_user_id ON artefacts (user_id)`,

		// ── artefact_media ────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS artefact_media (
			id            SERIAL PRIMARY KEY,
			artefact_id   INTEGER NOT NULL REFERENCES artefacts(id)   ON DELETE CASCADE,
			media_item_id INTEGER NOT NULL REFERENCES media_items(id) ON DELETE RESTRICT,
			sort_order    INTEGER NOT NULL DEFAULT 0,
			created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id       BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_artefact_media_artefact_id   ON artefact_media (artefact_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artefact_media_media_item_id ON artefact_media (media_item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artefact_media_user_id       ON artefact_media (user_id)`,

		// ── reference_documents ───────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS reference_documents (
			id                  SERIAL PRIMARY KEY,
			filename            VARCHAR(500) NOT NULL,
			title               VARCHAR(1000),
			description         TEXT,
			author              VARCHAR(500),
			content_type        VARCHAR(255) NOT NULL,
			size                INTEGER      NOT NULL,
			data                BYTEA        NOT NULL,
			tags                TEXT,
			categories          TEXT,
			notes               TEXT,
			ai_detailed_summary TEXT,
			ai_quick_summary    TEXT,
			available_for_task  BOOLEAN NOT NULL DEFAULT FALSE,
			include_in_system_prompt BOOLEAN NOT NULL DEFAULT FALSE,
			is_private          BOOLEAN NOT NULL DEFAULT FALSE,
			is_sensitive        BOOLEAN NOT NULL DEFAULT FALSE,
			is_encrypted        BOOLEAN NOT NULL DEFAULT FALSE,
			created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id             BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_refdocs_available_for_task ON reference_documents (available_for_task)`,
		`CREATE INDEX IF NOT EXISTS idx_refdocs_sensitive          ON reference_documents (is_sensitive)`,
		`CREATE INDEX IF NOT EXISTS idx_refdocs_private            ON reference_documents (is_private)`,
		`CREATE INDEX IF NOT EXISTS idx_refdocs_user_id            ON reference_documents (user_id)`,

		// ── visitor_key_hint_reference_documents (per–visitor-key LLM reference doc allowlist) ──
		`CREATE TABLE IF NOT EXISTS visitor_key_hint_reference_documents (
			visitor_key_hint_id   BIGINT NOT NULL REFERENCES visitor_key_hints(id) ON DELETE CASCADE,
			reference_document_id BIGINT NOT NULL REFERENCES reference_documents(id) ON DELETE CASCADE,
			user_id               BIGINT REFERENCES users(id) ON DELETE CASCADE,
			PRIMARY KEY (visitor_key_hint_id, reference_document_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vkh_refdocs_hint_id ON visitor_key_hint_reference_documents (visitor_key_hint_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vkh_refdocs_user_id ON visitor_key_hint_reference_documents (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vkh_refdocs_doc_id  ON visitor_key_hint_reference_documents (reference_document_id)`,

		// ── visitor_key_hint_sensitive_reference_documents (per–visitor-key LLM sensitive/private ref-doc allowlist) ──
		`CREATE TABLE IF NOT EXISTS visitor_key_hint_sensitive_reference_documents (
			visitor_key_hint_id   BIGINT NOT NULL REFERENCES visitor_key_hints(id) ON DELETE CASCADE,
			reference_document_id BIGINT NOT NULL REFERENCES reference_documents(id) ON DELETE CASCADE,
			user_id               BIGINT REFERENCES users(id) ON DELETE CASCADE,
			PRIMARY KEY (visitor_key_hint_id, reference_document_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vkh_sens_refdocs_hint_id ON visitor_key_hint_sensitive_reference_documents (visitor_key_hint_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vkh_sens_refdocs_user_id ON visitor_key_hint_sensitive_reference_documents (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vkh_sens_refdocs_doc_id  ON visitor_key_hint_sensitive_reference_documents (reference_document_id)`,

		// ── gemini_files ──────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS gemini_files (
			id                    SERIAL PRIMARY KEY,
			reference_document_id INTEGER      NOT NULL UNIQUE REFERENCES reference_documents(id) ON DELETE CASCADE,
			gemini_file_name      VARCHAR(500) NOT NULL,
			gemini_file_uri       VARCHAR(1000),
			filename              VARCHAR(500) NOT NULL,
			state                 VARCHAR(50)  NOT NULL DEFAULT 'ACTIVE',
			verified_at           TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
			created_at            TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
			updated_at            TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
			user_id               BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gemini_file_reference_doc ON gemini_files (reference_document_id)`,
		`CREATE INDEX IF NOT EXISTS idx_gemini_file_name          ON gemini_files (gemini_file_name)`,
		`CREATE INDEX IF NOT EXISTS idx_gemini_files_user_id      ON gemini_files (user_id)`,

		// ── places ────────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS places (
			id               SERIAL PRIMARY KEY,
			name             VARCHAR(500) NOT NULL,
			description      TEXT,
			latitude         DOUBLE PRECISION,
			longitude        DOUBLE PRECISION,
			altitude         DOUBLE PRECISION,
			has_gps          BOOLEAN NOT NULL DEFAULT FALSE,
			google_maps_url  VARCHAR(500),
			region           VARCHAR(255),
			source           VARCHAR(255),
			source_reference VARCHAR(500),
			created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id          BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_places_user_id ON places (user_id)`,

		// ── locations ─────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS locations (
			id               SERIAL PRIMARY KEY,
			name             VARCHAR(500) NOT NULL,
			description      TEXT,
			address          TEXT,
			latitude         DOUBLE PRECISION,
			longitude        DOUBLE PRECISION,
			region           VARCHAR(255),
			altitude         DOUBLE PRECISION,
			source           VARCHAR(255),
			source_reference VARCHAR(500),
			created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id          BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_locations_user_id ON locations (user_id)`,

		// ── interests ─────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS interests (
			id         SERIAL PRIMARY KEY,
			name       VARCHAR(500) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_interests_user_id ON interests (user_id)`,

		// ── subject_configuration ─────────────────────────────────────────────
		// One row per user enforced by UNIQUE (user_id).
		`CREATE TABLE IF NOT EXISTS subject_configuration (
			id                       SERIAL PRIMARY KEY,
			subject_name             VARCHAR(500) NOT NULL,
			gender                   VARCHAR(20)  NOT NULL DEFAULT 'Male',
			family_name              VARCHAR(500),
			other_names              TEXT,
			email_addresses          TEXT,
			phone_numbers            TEXT,
			whatsapp_handle          VARCHAR(255),
			instagram_handle         VARCHAR(255),
			subject_contact_id       BIGINT REFERENCES contacts(id) ON DELETE SET NULL,
			writing_style_ai         TEXT,
			psychological_profile_ai TEXT,
			personality_profile_ai   TEXT,
			interests_ai             TEXT,
			created_at               TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at               TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id                  BIGINT REFERENCES users(id) ON DELETE CASCADE,
			CONSTRAINT uq_subject_configuration_user UNIQUE (user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_subject_configuration_user_id ON subject_configuration (user_id)`,

		// ── app_system_instructions (singleton: id = 1, universal prompts) ──
		`CREATE TABLE IF NOT EXISTS app_system_instructions (
			id                      SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			chat_instructions       TEXT NOT NULL DEFAULT '',
			core_instructions       TEXT NOT NULL DEFAULT '',
			question_instructions   TEXT NOT NULL DEFAULT '',
			pam_bot_instructions    TEXT,
			user_id                 BIGINT REFERENCES users(id) ON DELETE SET NULL,
			updated_at              TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		// ── pam_bot (dementia companion — same multi-tenant rules as other archive tables) ──
		`CREATE TABLE IF NOT EXISTS pam_bot_sessions (
			id                    SERIAL PRIMARY KEY,
			user_id               BIGINT REFERENCES users(id) ON DELETE CASCADE,
			started_at            TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_interaction_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			interaction_count     INTEGER NOT NULL DEFAULT 0,
			latest_summary        TEXT,
			latest_analysis       TEXT,
			latest_summary_at     TIMESTAMPTZ,
			last_facebook_post_id   BIGINT,
			last_facebook_album_id  BIGINT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pam_bot_sessions_user ON pam_bot_sessions (user_id, started_at DESC)`,
		`CREATE TABLE IF NOT EXISTS pam_bot_turns (
			id               SERIAL PRIMARY KEY,
			user_id          BIGINT REFERENCES users(id) ON DELETE CASCADE,
			session_id       INTEGER NOT NULL REFERENCES pam_bot_sessions(id) ON DELETE CASCADE,
			turn_number      INTEGER NOT NULL,
			subject_tag      VARCHAR(200),
			subject_category VARCHAR(100),
			bot_message      TEXT NOT NULL,
			user_action      VARCHAR(50),
			created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pbt_session_turn ON pam_bot_turns (session_id, turn_number)`,
		`CREATE TABLE IF NOT EXISTS pam_bot_subjects (
			id                SERIAL PRIMARY KEY,
			user_id           BIGINT REFERENCES users(id) ON DELETE CASCADE,
			subject_tag       VARCHAR(200) NOT NULL,
			subject_category  VARCHAR(100),
			last_discussed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			discuss_count     INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_pbs_user_tag ON pam_bot_subjects (user_id, subject_tag)`,

		// ── custom_voices ─────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS custom_voices (
			id           SERIAL PRIMARY KEY,
			key          VARCHAR(100) NOT NULL UNIQUE,
			name         VARCHAR(200) NOT NULL,
			description  VARCHAR(500),
			instructions TEXT         NOT NULL,
			creativity   DOUBLE PRECISION NOT NULL DEFAULT 0.5,
			created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id      BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_voices_user_id ON custom_voices (user_id)`,

		// ── chat_conversations ────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS chat_conversations (
			id              SERIAL PRIMARY KEY,
			title           VARCHAR(500) NOT NULL,
			voice           VARCHAR(100) NOT NULL,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_message_at TIMESTAMP,
			user_id         BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conv_last_message    ON chat_conversations (last_message_at)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conversations_user_id ON chat_conversations (user_id)`,

		// ── chat_turns ────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS chat_turns (
			id              SERIAL PRIMARY KEY,
			conversation_id INTEGER NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
			user_input      TEXT    NOT NULL,
			response_text   TEXT    NOT NULL,
			voice           VARCHAR(100),
			temperature     DOUBLE PRECISION,
			turn_number     INTEGER NOT NULL,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id         BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_turn_conv_turn    ON chat_turns (conversation_id, turn_number)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_turn_conv_created ON chat_turns (conversation_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_turns_user_id     ON chat_turns (user_id)`,

		// ── complete_profiles ─────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS complete_profiles (
			id                   SERIAL PRIMARY KEY,
			name                 VARCHAR(500) NOT NULL,
			profile              TEXT,
			generation_pending   BOOLEAN NOT NULL DEFAULT FALSE,
			created_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id              BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_complete_profiles_user_id ON complete_profiles (user_id)`,

		// ── saved_responses ───────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS saved_responses (
			id           SERIAL PRIMARY KEY,
			title        VARCHAR(500) NOT NULL,
			content      TEXT         NOT NULL,
			voice        VARCHAR(100),
			llm_provider VARCHAR(100),
			created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id      BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_saved_responses_user_id ON saved_responses (user_id)`,

		// ── import_control_last_run ───────────────────────────────────────────
		// Partial unique indexes replace the single-column UNIQUE so global rows
		// (user_id IS NULL) and per-user rows are each unique within their scope.
		`CREATE TABLE IF NOT EXISTS import_control_last_run (
			id             SERIAL PRIMARY KEY,
			import_type    VARCHAR(100) NOT NULL,
			last_run_at    TIMESTAMP    NOT NULL,
			result         VARCHAR(50)  NOT NULL,
			result_message TEXT,
			created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id        BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_import_control_last_run_type    ON import_control_last_run (import_type)`,
		`CREATE INDEX IF NOT EXISTS idx_import_control_last_run_user_id ON import_control_last_run (user_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_import_control_global
			ON import_control_last_run (import_type)
			WHERE user_id IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_import_control_user
			ON import_control_last_run (import_type, user_id)
			WHERE user_id IS NOT NULL`,

		// ── email_classifications ─────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS email_classifications (
			id             SERIAL PRIMARY KEY,
			name           VARCHAR(500) NOT NULL,
			classification VARCHAR(20)  NOT NULL,
			created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id        BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_classifications_user_id ON email_classifications (user_id)`,

		// ── email_matches ─────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS email_matches (
			id           SERIAL PRIMARY KEY,
			primary_name VARCHAR(500) NOT NULL,
			email        VARCHAR(300) NOT NULL,
			created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id      BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_matches_user_id ON email_matches (user_id)`,

		// ── email_exclusions ──────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS email_exclusions (
			id         SERIAL PRIMARY KEY,
			email      VARCHAR(300) NOT NULL,
			name       VARCHAR(500) NOT NULL,
			name_email BOOLEAN      NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_exclusions_user_id ON email_exclusions (user_id)`,

		// ── private_store ─────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS private_store (
			id              SERIAL PRIMARY KEY,
			key             TEXT  NOT NULL UNIQUE,
			encrypted_value BYTEA NOT NULL,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id         BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_private_store_user_id ON private_store (user_id)`,

		// ── master_keys ───────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS master_keys (
			id         SERIAL PRIMARY KEY,
			comment    VARCHAR(500) NOT NULL DEFAULT 'Don''t think I''m stupid enough to use this as a master key',
			public_key TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_master_keys_user_id ON master_keys (user_id)`,

		// ── app_configuration ─────────────────────────────────────────────────
		// user_id is ON DELETE SET NULL so global (NULL) rows survive user deletion.
		// Partial unique indexes allow the same key once globally and once per user.
		`CREATE TABLE IF NOT EXISTS app_configuration (
			id           SERIAL PRIMARY KEY,
			key          VARCHAR(255) NOT NULL,
			value        TEXT,
			is_mandatory BOOLEAN NOT NULL DEFAULT FALSE,
			description  TEXT,
			created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id      BIGINT REFERENCES users(id) ON DELETE SET NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_configuration_key     ON app_configuration (key)`,
		`CREATE INDEX IF NOT EXISTS idx_app_configuration_user_id ON app_configuration (user_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_app_config_global
			ON app_configuration (key)
			WHERE user_id IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_app_config_user
			ON app_configuration (key, user_id)
			WHERE user_id IS NOT NULL`,

		// ── interviews ───────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS interviews (
			id             SERIAL PRIMARY KEY,
			title          VARCHAR(500) NOT NULL,
			style          VARCHAR(50)  NOT NULL,
			purpose        VARCHAR(50)  NOT NULL,
			purpose_detail TEXT,
			state          VARCHAR(20)  NOT NULL DEFAULT 'active',
			provider       VARCHAR(20),
			writeup        TEXT,
			created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_turn_at   TIMESTAMP,
			user_id        BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_interviews_user_id      ON interviews (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_interviews_state        ON interviews (state)`,
		`CREATE INDEX IF NOT EXISTS idx_interviews_last_turn_at ON interviews (last_turn_at)`,

		// ── interview_turns ──────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS interview_turns (
			id            SERIAL PRIMARY KEY,
			interview_id  INTEGER NOT NULL REFERENCES interviews(id) ON DELETE CASCADE,
			question      TEXT    NOT NULL,
			answer        TEXT,
			turn_number   INTEGER NOT NULL,
			created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id       BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_interview_turns_interview_turn ON interview_turns (interview_id, turn_number)`,
		`CREATE INDEX IF NOT EXISTS idx_interview_turns_user_id        ON interview_turns (user_id)`,

		// ── have_a_chat_sessions ─────────────────────────────────────────────
		// Saved two-voice Have a Chat transcripts when the user stops a run.
		`CREATE TABLE IF NOT EXISTS have_a_chat_sessions (
			id              SERIAL PRIMARY KEY,
			user_id         BIGINT REFERENCES users(id) ON DELETE CASCADE,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			stopped_at      TIMESTAMPTZ,
			topic           TEXT,
			voice_a         TEXT NOT NULL,
			voice_b         TEXT NOT NULL,
			provider_a      TEXT NOT NULL,
			provider_b      TEXT NOT NULL,
			banter_mode     BOOLEAN NOT NULL DEFAULT FALSE,
			temperature     DOUBLE PRECISION NOT NULL DEFAULT 0.7,
			allow_explicit  BOOLEAN NOT NULL DEFAULT FALSE,
			turn_count      INTEGER NOT NULL DEFAULT 0,
			history_json    JSONB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_have_a_chat_sessions_user_id    ON have_a_chat_sessions (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_have_a_chat_sessions_stopped_at ON have_a_chat_sessions (stopped_at)`,

		// ── message_embedding_meta ───────────────────────────────────────────
		// Context-window metadata for message_embeddings vec rows (SQLite + PG).
		`CREATE TABLE IF NOT EXISTS message_embedding_meta (
			message_id      BIGINT PRIMARY KEY,
			model           TEXT NOT NULL,
			window_back     INTEGER NOT NULL,
			window_forward  INTEGER NOT NULL,
			content_hash    TEXT NOT NULL,
			updated_at      TIMESTAMPTZ NOT NULL
		)`,

		// ── background_jobs ──────────────────────────────────────────────────
		// Per-user scheduling state for the maintenance jobs exposed in the
		// Configuration > Background Jobs panel. One row per (user_id, job_name).
		`CREATE TABLE IF NOT EXISTS background_jobs (
			id                  SERIAL PRIMARY KEY,
			job_name            VARCHAR(100) NOT NULL,
			auto_start          BOOLEAN      NOT NULL DEFAULT FALSE,
			restart_on_complete BOOLEAN      NOT NULL DEFAULT FALSE,
			interval_seconds    INTEGER      NOT NULL DEFAULT 3600,
			last_run_at         TIMESTAMP,
			last_run_result     VARCHAR(50),
			last_run_message    TEXT,
			next_due_at         TIMESTAMP,
			created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			user_id             BIGINT REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_background_jobs_user_id     ON background_jobs (user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_background_jobs_next_due_at ON background_jobs (next_due_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_background_jobs_user_name
			ON background_jobs (user_id, job_name)
			WHERE user_id IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_background_jobs_global_name
			ON background_jobs (job_name)
			WHERE user_id IS NULL`,
		`CREATE TABLE IF NOT EXISTS regions (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			key         TEXT NOT NULL,
			sort_order  INTEGER NOT NULL DEFAULT 0,
			text        TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_regions_key ON regions (key)`,
		`CREATE INDEX IF NOT EXISTS idx_regions_sort ON regions (sort_order, id)`,
		`CREATE TABLE IF NOT EXISTS suggestions (
			id    INTEGER PRIMARY KEY AUTOINCREMENT,
			key   TEXT NOT NULL,
			text  TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_suggestions_key ON suggestions (key)`,

		// ── Guide topics (deployment-wide interactive help) ───────────────────
		`CREATE TABLE IF NOT EXISTS guide_topics (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			key  TEXT NOT NULL,
			text TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_guide_topics_key ON guide_topics (key)`,
	}
}

// rlsDDL returns ENABLE ROW LEVEL SECURITY and CREATE POLICY statements for
// all data tables. The policy allows a row when:
//   - user_id IS NULL          — legacy / global rows visible to all
//   - no auth context is set   — migrations, seeds, admin connections
//   - user_id matches session  — normal authenticated request
//
// RLS uses ENABLE (not FORCE), so the DB owner role bypasses it.
func rlsDDL() []string {
	tables := []string{
		"emails", "attachments",
		"media_blobs", "media_items",
		"messages", "message_attachments",
		"facebook_albums", "album_media",
		"facebook_posts", "post_media",
		"artefacts", "artefact_media",
		"reference_documents", "gemini_files",
		"contacts", "relationships",
		"places", "locations", "interests",
		"subject_configuration", "app_system_instructions", "custom_voices",
		"chat_conversations", "chat_turns",
		"complete_profiles", "saved_responses",
		"interviews", "interview_turns",
		"have_a_chat_sessions",
		"pam_bot_sessions", "pam_bot_turns", "pam_bot_subjects",
		"import_control_last_run",
		"email_classifications", "email_matches", "email_exclusions",
		"sensitive_keyring", "visitor_key_hints", "visitor_key_hint_reference_documents", "visitor_key_hint_sensitive_reference_documents",
		"private_store", "master_keys",
		"app_configuration",
		"background_jobs",
	}

	var stmts []string
	for _, t := range tables {
		policy := t + "_user_isolation"
		stmts = append(stmts,
			fmt.Sprintf(`ALTER TABLE %s ENABLE ROW LEVEL SECURITY`, t),
			fmt.Sprintf(`DROP POLICY IF EXISTS %s ON %s`, policy, t),
			fmt.Sprintf(`CREATE POLICY %s ON %s
    USING (
        user_id IS NULL
        OR current_setting('app.current_user_id', TRUE) = ''
        OR user_id = NULLIF(current_setting('app.current_user_id', TRUE), '')::BIGINT
    )
    WITH CHECK (
        user_id IS NULL
        OR current_setting('app.current_user_id', TRUE) = ''
        OR user_id = NULLIF(current_setting('app.current_user_id', TRUE), '')::BIGINT
    )`, policy, t),
		)
	}
	return stmts
}

// regionFunctionsDDL returns the two PL/pgSQL geographic region functions.
func regionFunctionsDDL() []string {
	caseBody := `
        IF    loc.latitude BETWEEN -44 AND -10  AND loc.longitude BETWEEN  110 AND  152 THEN region_text := 'aus';
        ELSIF loc.latitude BETWEEN  24 AND  26  AND loc.longitude BETWEEN   54 AND   56 THEN region_text := 'dxb';
        ELSIF loc.latitude BETWEEN  35 AND  70  AND loc.longitude BETWEEN  -10 AND   30 THEN region_text := 'eur';
        ELSIF loc.latitude BETWEEN  20 AND  50  AND loc.longitude BETWEEN -128 AND  -65 THEN region_text := 'usa';
        ELSIF loc.latitude BETWEEN -40 AND  35  AND loc.longitude BETWEEN  -20 AND   50 THEN region_text := 'af';
        ELSIF loc.latitude BETWEEN  10 AND  40  AND loc.longitude BETWEEN   30 AND   60 THEN region_text := 'me';
        ELSIF loc.latitude BETWEEN -12 AND  54  AND loc.longitude BETWEEN   68 AND  152 THEN region_text := 'asia';
        ELSIF loc.latitude BETWEEN   9 AND  26  AND loc.longitude BETWEEN -116 AND  -76 THEN region_text := 'central_america';
        ELSIF loc.latitude BETWEEN  12 AND  25  AND loc.longitude BETWEEN  -85 AND  -58 THEN region_text := 'carribean';
        ELSIF loc.latitude BETWEEN -47 AND -34  AND loc.longitude BETWEEN  163 AND  179 THEN region_text := 'nz';
        ELSIF loc.latitude BETWEEN -56 AND  12  AND loc.longitude BETWEEN  -99 AND  -26 THEN region_text := 'south_america';
        ELSE region_text := 'oth';
        END IF;`

	return []string{
		`CREATE OR REPLACE FUNCTION update_location_regions()
		RETURNS void AS $$
		DECLARE
		    loc RECORD;
		    region_text TEXT;
		BEGIN
		    FOR loc IN SELECT id, latitude, longitude FROM locations LOOP
		` + caseBody + `
		        UPDATE locations SET region = region_text WHERE id = loc.id;
		    END LOOP;
		END;
		$$ LANGUAGE plpgsql`,

		`CREATE OR REPLACE FUNCTION update_image_location_regions()
		RETURNS void AS $$
		DECLARE
		    loc RECORD;
		    region_text TEXT;
		BEGIN
		    FOR loc IN SELECT id, latitude, longitude FROM media_items
		               WHERE latitude IS NOT NULL AND longitude IS NOT NULL LOOP
		` + caseBody + `
		        UPDATE media_items SET region = region_text WHERE id = loc.id;
		    END LOOP;
		END;
		$$ LANGUAGE plpgsql`,
	}
}

// ── SQLite DDL conversion (from schemaDDL) ───────────────────────────────────

var (
	reSkipTable = regexp.MustCompile(`(?i)CREATE TABLE IF NOT EXISTS\s+(\w+)`)
	// Match PostgreSQL TIMESTAMPTZ/TIMESTAMP with flexible whitespace (schema uses 1–2 spaces).
	reTSNotNullDefCur = regexp.MustCompile(`(?i)TIMESTAMPTZ\s+NOT NULL\s+DEFAULT\s+CURRENT_TIMESTAMP`)
	reTSNotNullDefNow = regexp.MustCompile(`(?i)TIMESTAMPTZ\s+NOT NULL\s+DEFAULT\s+NOW\s*\(\s*\)`)
	reTNotNullDefCur  = regexp.MustCompile(`(?i)TIMESTAMP\s+NOT NULL\s+DEFAULT\s+CURRENT_TIMESTAMP`)
	reTDefCur         = regexp.MustCompile(`(?i)TIMESTAMP\s+DEFAULT\s+CURRENT_TIMESTAMP`)
	reTSNotNull       = regexp.MustCompile(`(?i)TIMESTAMPTZ\s+NOT NULL\b`)
	reTSPlain         = regexp.MustCompile(`(?i)\bTIMESTAMPTZ\b`)
	// Case-sensitive: (?i) would match the column name `timestamp` in DDL (e.g. facebook_posts)
	// and corrupt it to `TEXT`. Schema DDL uses uppercase TIMESTAMP for the type.
	reTPlain = regexp.MustCompile(`\bTIMESTAMP\b`)
)

// sqliteStatements returns DDL strings suitable for SQLite, derived from schemaDDL().
func sqliteStatements() []string {
	var out []string
	for _, s := range schemaDDL() {
		if skipSQLiteTable(s) || skipSQLiteIndexOnOmittedTable(s) {
			continue
		}
		out = append(out, pgDDLToSQLite(s))
	}
	// Email plain_text index (btree; no pg_trgm).
	out = append(out,
		`CREATE INDEX IF NOT EXISTS idx_plain_text_btree ON emails (plain_text)`,
	)
	return out
}

// encryptionSQLiteSkipTables can exclude schemaDDL tables from the SQLite build.
var encryptionSQLiteSkipTables = []string{}

func skipSQLiteTable(s string) bool {
	m := reSkipTable.FindStringSubmatch(s)
	if len(m) < 2 {
		return false
	}
	t := strings.ToLower(m[1])
	for _, skip := range encryptionSQLiteSkipTables {
		if t == skip {
			return true
		}
	}
	return false
}

func skipSQLiteIndexOnOmittedTable(s string) bool {
	u := strings.ToLower(strings.TrimSpace(s))
	if len(u) < 12 || !strings.HasPrefix(u, "create index") {
		return false
	}
	for _, tbl := range encryptionSQLiteSkipTables {
		if i := strings.Index(u, " on "+tbl); i >= 0 {
			rest := u[i+len(" on "+tbl):]
			if len(rest) == 0 || rest[0] == ' ' || rest[0] == '(' {
				return true
			}
		}
	}
	return false
}

func pgDDLToSQLite(s string) string {
	s = strings.ReplaceAll(s, "visitor_key_hint_id   BIGINT REFERENCES visitor_key_hints(id) ON DELETE SET NULL", "visitor_key_hint_id BIGINT")
	s = strings.ReplaceAll(s, "visitor_llm_overrides JSONB", "visitor_llm_overrides TEXT")
	s = strings.ReplaceAll(s, "tool_access_policy JSONB", "tool_access_policy TEXT")
	s = strings.ReplaceAll(s, "details     JSONB", "details     TEXT")

	s = strings.ReplaceAll(s, "BIGSERIAL", "INTEGER")
	s = strings.ReplaceAll(s, "SERIAL  PRIMARY KEY", "INTEGER PRIMARY KEY AUTOINCREMENT")
	s = strings.ReplaceAll(s, "SERIAL PRIMARY KEY", "INTEGER PRIMARY KEY AUTOINCREMENT")
	s = strings.ReplaceAll(s, "BOOLEAN NOT NULL", "INTEGER NOT NULL")
	s = strings.ReplaceAll(s, "BOOLEAN DEFAULT", "INTEGER DEFAULT")
	s = strings.ReplaceAll(s, "BOOLEAN", "INTEGER")
	s = reTSNotNullDefCur.ReplaceAllString(s, "TEXT NOT NULL DEFAULT (datetime('now'))")
	s = reTSNotNullDefNow.ReplaceAllString(s, "TEXT NOT NULL DEFAULT (datetime('now'))")
	s = reTNotNullDefCur.ReplaceAllString(s, "TEXT NOT NULL DEFAULT (datetime('now'))")
	s = reTDefCur.ReplaceAllString(s, "TEXT DEFAULT (datetime('now'))")
	s = reTSNotNull.ReplaceAllString(s, "TEXT NOT NULL")
	s = reTSPlain.ReplaceAllString(s, "TEXT")
	s = strings.ReplaceAll(s, "CURRENT_TIMESTAMP", "__DM_SQLITE_CURRENT_TS__")
	s = reTPlain.ReplaceAllString(s, "TEXT")
	s = strings.ReplaceAll(s, "__DM_SQLITE_CURRENT_TS__", "CURRENT_TIMESTAMP")
	s = strings.ReplaceAll(s, "JSONB", "TEXT")
	s = strings.ReplaceAll(s, "BYTEA", "BLOB")
	s = strings.ReplaceAll(s, "DOUBLE PRECISION", "REAL")
	s = strings.ReplaceAll(s, "INTEGER    PRIMARY KEY", "INTEGER PRIMARY KEY AUTOINCREMENT")

	return s
}

// ── SQLite migration ───────────────────────────────────────────────────────────

// repairSQLiteCorruptTimestampDefaults fixes rows where an older pgDDLToSQLite bug
// turned CURRENT_TIMESTAMP into the literal string "CURRENT_TEXT" (or related) in TEXT columns.
func repairSQLiteCorruptTimestampDefaults(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		UPDATE users SET created_at = (datetime('now'))
		WHERE typeof(created_at) = 'text' AND created_at IN ('CURRENT_TEXT', 'TEXTTZ')`); err != nil {
		return fmt.Errorf("repair users.created_at: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE users SET last_login_at = NULL
		WHERE typeof(last_login_at) = 'text' AND last_login_at IN ('CURRENT_TEXT', 'TEXTTZ')`); err != nil {
		return fmt.Errorf("repair users.last_login_at: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE typeof(expires_at) = 'text' AND expires_at IN ('CURRENT_TEXT', 'TEXTTZ')`); err != nil {
		return fmt.Errorf("repair sessions: %w", err)
	}
	return nil
}

// repairSQLiteFacebookPostsTimestampColumn adds facebook_posts.timestamp when missing.
func repairSQLiteFacebookPostsTimestampColumn(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'facebook_posts'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master facebook_posts: %w", err)
	}
	if n == 0 {
		return nil
	}
	var has int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('facebook_posts') WHERE name = 'timestamp'`,
	).Scan(&has); err != nil {
		return fmt.Errorf("pragma_table_info facebook_posts: %w", err)
	}
	if has > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE facebook_posts ADD COLUMN timestamp TEXT`); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "duplicate column") {
			return nil
		}
		return fmt.Errorf("add facebook_posts.timestamp: %w", err)
	}
	slog.Info("repaired facebook_posts: added missing timestamp column (SQLite)")
	return nil
}

// MigrateSQLite applies schema for the main application database.
func MigrateSQLite(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("pragma foreign_keys: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("pragma busy_timeout: %w", err)
	}

	for _, stmt := range sqliteStatements() {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			preview := stmt
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			return fmt.Errorf("sqlite migration failed (%s): %w", preview, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO app_system_instructions (id, chat_instructions, core_instructions, question_instructions, user_id)
		VALUES (1, '', '', '', NULL)
		ON CONFLICT(id) DO NOTHING`); err != nil {
		return fmt.Errorf("ensure app_system_instructions: %w", err)
	}

	if err := repairSQLiteCorruptTimestampDefaults(ctx, db); err != nil {
		return err
	}

	if err := repairSQLiteFacebookPostsTimestampColumn(ctx, db); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, `ALTER TABLE sessions ADD COLUMN visitor_key_hint_id INTEGER`); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
			return fmt.Errorf("add sessions.visitor_key_hint_id: %w", err)
		}
	}

	if err := addReferenceDocumentsIncludeInSystemPromptColumn(ctx, db); err != nil {
		return err
	}
	if err := addMediaItemsRequireClassificationColumn(ctx, db); err != nil {
		return err
	}
	if err := addUserDeepSeekLLMColumns(ctx, db); err != nil {
		return err
	}
	if err := addUserRunpodElevenlabsAPIColumns(ctx, db); err != nil {
		return err
	}
	if err := addSubjectConfigurationSubjectContactIDColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureSQLiteVecEmbeddingTables(ctx, db); err != nil {
		return err
	}
	if err := addUserRunpodEndpointColumns(ctx, db); err != nil {
		return err
	}
	if err := ensureMediaItemsSourceReferenceIndex(ctx, db); err != nil {
		return err
	}
	if err := addMediaItemsDayColumn(ctx, db); err != nil {
		return err
	}
	if err := seedReservedUserSlot(ctx, db); err != nil {
		return err
	}

	slog.Info("sqlite database migration complete")
	return nil
}

// seedReservedUserSlot inserts an inactive placeholder at users.id=1 on fresh archives so the
// first real owner account receives id=2. Skipped when id=1 already exists (legacy admin rows).
func seedReservedUserSlot(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'users'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master users: %w", err)
	}
	if n == 0 {
		return nil
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, is_active, is_admin)
		SELECT ?1, ?2, 'unused', '', 0, 0
		WHERE NOT EXISTS (SELECT 1 FROM users WHERE id = ?1)`,
		ReservedUserSlotID, ReservedUserEmail,
	)
	if err != nil {
		return fmt.Errorf("seed reserved user slot: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		slog.Info("sqlite migration: seeded reserved user slot", "id", ReservedUserSlotID)
	}
	return nil
}

func ensureMediaItemsSourceReferenceIndex(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'media_items'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master media_items: %w", err)
	}
	if n == 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_media_items_source_reference ON media_items (source, source_reference)`,
	); err != nil {
		return fmt.Errorf("create idx_media_items_source_reference: %w", err)
	}
	slog.Info("sqlite migration: ensured idx_media_items_source_reference")
	return nil
}

func addUserRunpodEndpointColumns(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'users'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master users: %w", err)
	}
	if n == 0 {
		return nil
	}
	for _, col := range []struct {
		name string
		stmt string
	}{
		{"user_runpod_endpoint_id", `ALTER TABLE users ADD COLUMN user_runpod_endpoint_id TEXT`},
		{"user_runpod_workers", `ALTER TABLE users ADD COLUMN user_runpod_workers INTEGER`},
	} {
		var has int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = ?`, col.name,
		).Scan(&has); err != nil {
			return fmt.Errorf("pragma_table_info users.%s: %w", col.name, err)
		}
		if has > 0 {
			continue
		}
		if _, err := db.ExecContext(ctx, col.stmt); err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
				return fmt.Errorf("add users.%s: %w", col.name, err)
			}
		}
		slog.Info("sqlite migration: added users." + col.name)
	}
	return nil
}

// ── Legacy column upgrades (existing SQLite archives) ──────────────────────────

func addMediaItemsRequireClassificationColumn(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'media_items'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master media_items: %w", err)
	}
	if n == 0 {
		return nil
	}
	var has int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('media_items') WHERE name = 'require_classification'`,
	).Scan(&has); err != nil {
		return fmt.Errorf("pragma_table_info media_items.require_classification: %w", err)
	}
	if has > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE media_items ADD COLUMN require_classification INTEGER NOT NULL DEFAULT 1`); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
			return fmt.Errorf("add media_items.require_classification: %w", err)
		}
	}
	slog.Info("sqlite migration: added media_items.require_classification")
	return nil
}

func addMediaItemsDayColumn(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'media_items'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master media_items: %w", err)
	}
	if n == 0 {
		return nil
	}
	var has int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('media_items') WHERE name = 'day'`,
	).Scan(&has); err != nil {
		return fmt.Errorf("pragma_table_info media_items.day: %w", err)
	}
	if has > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE media_items ADD COLUMN day INTEGER`); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
			return fmt.Errorf("add media_items.day: %w", err)
		}
	}
	slog.Info("sqlite migration: added media_items.day")
	return nil
}

func addUserDeepSeekLLMColumns(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'users'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master users: %w", err)
	}
	if n == 0 {
		return nil
	}
	for _, col := range []struct {
		name string
		stmt string
	}{
		{"user_deepseek_api_key", `ALTER TABLE users ADD COLUMN user_deepseek_api_key TEXT`},
		{"user_deepseek_model", `ALTER TABLE users ADD COLUMN user_deepseek_model TEXT`},
	} {
		var has int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = ?`, col.name,
		).Scan(&has); err != nil {
			return fmt.Errorf("pragma_table_info users.%s: %w", col.name, err)
		}
		if has > 0 {
			continue
		}
		if _, err := db.ExecContext(ctx, col.stmt); err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
				return fmt.Errorf("add users.%s: %w", col.name, err)
			}
		}
		slog.Info("sqlite migration: added users." + col.name)
	}
	return nil
}

func addSubjectConfigurationSubjectContactIDColumn(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'subject_configuration'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master subject_configuration: %w", err)
	}
	if n == 0 {
		return nil
	}
	var has int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('subject_configuration') WHERE name = 'subject_contact_id'`,
	).Scan(&has); err != nil {
		return fmt.Errorf("pragma_table_info subject_configuration.subject_contact_id: %w", err)
	}
	if has > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE subject_configuration ADD COLUMN subject_contact_id INTEGER REFERENCES contacts(id) ON DELETE SET NULL`); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
			return fmt.Errorf("add subject_configuration.subject_contact_id: %w", err)
		}
	}
	slog.Info("sqlite migration: added subject_configuration.subject_contact_id")
	return nil
}

func addUserRunpodElevenlabsAPIColumns(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'users'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master users: %w", err)
	}
	if n == 0 {
		return nil
	}
	for _, col := range []struct {
		name string
		stmt string
	}{
		{"user_runpod_api_key", `ALTER TABLE users ADD COLUMN user_runpod_api_key TEXT`},
		{"user_elevenlabs_api_key", `ALTER TABLE users ADD COLUMN user_elevenlabs_api_key TEXT`},
	} {
		var has int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = ?`, col.name,
		).Scan(&has); err != nil {
			return fmt.Errorf("pragma_table_info users.%s: %w", col.name, err)
		}
		if has > 0 {
			continue
		}
		if _, err := db.ExecContext(ctx, col.stmt); err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
				return fmt.Errorf("add users.%s: %w", col.name, err)
			}
		}
		slog.Info("sqlite migration: added users." + col.name)
	}
	return nil
}

func ensureSQLiteVecEmbeddingTables(ctx context.Context, db *sql.DB) error {
	var vecVersion string
	if err := db.QueryRowContext(ctx, `SELECT vec_version()`).Scan(&vecVersion); err != nil {
		return fmt.Errorf("sqlite-vec not available (vec_version): %w", err)
	}

	tables := []string{
		"email_embeddings",
		"message_embeddings",
		"media_tag_embeddings",
		"facebook_post_text_embeddings",
		"facebook_album_description_embeddings",
		"artefact_embeddings",
	}
	for _, table := range tables {
		var exists int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&exists); err != nil {
			return fmt.Errorf("sqlite_master %s: %w", table, err)
		}
		if exists > 0 {
			continue
		}
		stmt := fmt.Sprintf(
			`CREATE VIRTUAL TABLE %s USING vec0(embedding float[768], int_ids text)`,
			table,
		)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create vec0 table %s: %w", table, err)
		}
		slog.Info("sqlite migration: created sqlite-vec table", "table", table, "vec_version", vecVersion)
	}
	return nil
}

func addReferenceDocumentsIncludeInSystemPromptColumn(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'reference_documents'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("sqlite_master reference_documents: %w", err)
	}
	if n == 0 {
		return nil
	}
	var has int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('reference_documents') WHERE name = 'include_in_system_prompt'`,
	).Scan(&has); err != nil {
		return fmt.Errorf("pragma_table_info reference_documents.include_in_system_prompt: %w", err)
	}
	if has > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE reference_documents ADD COLUMN include_in_system_prompt INTEGER NOT NULL DEFAULT 0`); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
			return fmt.Errorf("add reference_documents.include_in_system_prompt: %w", err)
		}
	}
	slog.Info("sqlite migration: added reference_documents.include_in_system_prompt")
	return nil
}
