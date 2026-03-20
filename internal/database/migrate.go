package database

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate creates all tables, extensions, indexes, and PL/pgSQL functions.
// It is safe to call on an already-initialised database (all statements use IF NOT EXISTS).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for migration: %w", err)
	}
	defer conn.Release()

	// ── Extensions ────────────────────────────────────────────────────────────

	if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pgcrypto"); err != nil {
		return fmt.Errorf("pgcrypto extension required for encryption: %w", err)
	}

	pgTrgmAvailable := true
	if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_trgm"); err != nil {
		slog.Warn("pg_trgm extension unavailable — full-text search index will use btree fallback", "err", err)
		pgTrgmAvailable = false
	}

	// ── Tables and indexes ────────────────────────────────────────────────────

	for _, stmt := range schemaDDL() {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			preview := stmt
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			return fmt.Errorf("migration statement failed (%s): %w", preview, err)
		}
	}

	// ── reference_documents encryption columns ───────────────────────────────
	if _, err := conn.Exec(ctx, `DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='reference_documents' AND column_name='is_encrypted') THEN
    ALTER TABLE reference_documents ADD COLUMN is_encrypted BOOLEAN NOT NULL DEFAULT FALSE;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='reference_documents' AND column_name='is_sensitive') THEN
    ALTER TABLE reference_documents ADD COLUMN is_sensitive BOOLEAN NOT NULL DEFAULT FALSE;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='reference_documents' AND column_name='is_private') THEN
    ALTER TABLE reference_documents ADD COLUMN is_private BOOLEAN NOT NULL DEFAULT FALSE;
  END IF;
END$$`); err != nil {
		slog.Warn("could not add encryption columns to reference_documents", "err", err)
	}

	// ── sensitive_keyring master private DEK column ───────────────────────────
	if _, err := conn.Exec(ctx, `DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                 WHERE table_name='sensitive_keyring' AND column_name='encrypted_master_dek') THEN
    ALTER TABLE sensitive_keyring ADD COLUMN encrypted_master_dek BYTEA;
  END IF;
END$$`); err != nil {
		slog.Warn("could not add encrypted_master_dek column to sensitive_keyring", "err", err)
	}

	// ── Full-text search index for emails.plain_text ───────────────────────────
	var ftsSql string
	if pgTrgmAvailable {
		ftsSql = "CREATE INDEX IF NOT EXISTS idx_plain_text_fts ON emails USING gin (plain_text gin_trgm_ops)"
	} else {
		ftsSql = "CREATE INDEX IF NOT EXISTS idx_plain_text_btree ON emails (plain_text)"
	}
	if _, err := conn.Exec(ctx, ftsSql); err != nil {
		slog.Warn("could not create plain_text index — searches will work but may be slower", "err", err)
	}

	// ── PL/pgSQL region functions ─────────────────────────────────────────────
	for _, fnSQL := range regionFunctionsDDL() {
		if _, err := conn.Exec(ctx, fnSQL); err != nil {
			slog.Warn("could not create region function", "err", err)
		}
	}

	slog.Info("database migration complete")
	return nil
}

// schemaDDL returns all CREATE TABLE and CREATE INDEX statements.
func schemaDDL() []string {
	return []string{
		// ── emails ──────────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS emails (
			id                SERIAL PRIMARY KEY,
			uid               VARCHAR(255)  NOT NULL,
			folder            VARCHAR(255)  NOT NULL,
			subject           VARCHAR(1000),
			from_address      VARCHAR(500),
			to_addresses      TEXT,
			cc_addresses      TEXT,
			bcc_addresses     TEXT,
			date              TIMESTAMP,
			raw_message       TEXT,
			plain_text        TEXT,
			snippet           TEXT,
			embedding         TEXT,
			has_attachments   BOOLEAN NOT NULL DEFAULT FALSE,
			user_deleted      BOOLEAN NOT NULL DEFAULT FALSE,
			is_personal       BOOLEAN NOT NULL DEFAULT FALSE,
			is_business       BOOLEAN NOT NULL DEFAULT FALSE,
			is_social         BOOLEAN NOT NULL DEFAULT FALSE,
			is_promotional    BOOLEAN NOT NULL DEFAULT FALSE,
			is_spam           BOOLEAN NOT NULL DEFAULT FALSE,
			is_important      BOOLEAN NOT NULL DEFAULT FALSE,
			use_by_ai         BOOLEAN NOT NULL DEFAULT TRUE,
			created_at        TIMESTAMP DEFAULT NOW(),
			updated_at        TIMESTAMP DEFAULT NOW(),
			CONSTRAINT uq_email_uid_folder UNIQUE (uid, folder)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_uid_folder ON emails (uid, folder)`,

		// ── attachments ─────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS attachments (
			id               SERIAL PRIMARY KEY,
			email_id         INTEGER NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
			filename         VARCHAR(500),
			content_type     VARCHAR(255),
			size             INTEGER,
			data             BYTEA,
			image_thumbnail  BYTEA,
			created_at       TIMESTAMP DEFAULT NOW()
		)`,

		// ── media_blobs ─────────────────────────────────────────────────────────
		// Binary blob storage referenced by media_items.
		`CREATE TABLE IF NOT EXISTS media_blobs (
			id              SERIAL PRIMARY KEY,
			image_data      BYTEA,
			thumbnail_data  BYTEA,
			created_at      TIMESTAMP DEFAULT NOW(),
			updated_at      TIMESTAMP DEFAULT NOW()
		)`,

		// ── media_items ─────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS media_items (
			id                  SERIAL PRIMARY KEY,
			media_blob_id       INTEGER NOT NULL REFERENCES media_blobs(id) ON DELETE RESTRICT,
			description         TEXT,
			title               VARCHAR(1000),
			author              VARCHAR(500),
			tags                TEXT,
			categories          TEXT,
			notes               TEXT,
			available_for_task  BOOLEAN NOT NULL DEFAULT FALSE,
			media_type          VARCHAR(255),
			processed           BOOLEAN NOT NULL DEFAULT FALSE,
			created_at          TIMESTAMP DEFAULT NOW(),
			updated_at          TIMESTAMP DEFAULT NOW(),
			embedding           TEXT,
			year                INTEGER,
			month               INTEGER,
			latitude            DOUBLE PRECISION,
			longitude           DOUBLE PRECISION,
			altitude            DOUBLE PRECISION,
			rating              INTEGER NOT NULL DEFAULT 5,
			has_gps             BOOLEAN NOT NULL DEFAULT FALSE,
			google_maps_url     VARCHAR(500),
			region              VARCHAR(255),
			is_personal         BOOLEAN NOT NULL DEFAULT FALSE,
			is_business         BOOLEAN NOT NULL DEFAULT FALSE,
			is_social           BOOLEAN NOT NULL DEFAULT FALSE,
			is_promotional      BOOLEAN NOT NULL DEFAULT FALSE,
			is_spam             BOOLEAN NOT NULL DEFAULT FALSE,
			is_important        BOOLEAN NOT NULL DEFAULT FALSE,
			use_by_ai           BOOLEAN DEFAULT FALSE,
			is_referenced       BOOLEAN NOT NULL DEFAULT FALSE,
			source              VARCHAR(255),
			source_reference    VARCHAR(500)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_processed   ON media_items (processed)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_source      ON media_items (source)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_media_type  ON media_items (media_type)`,
		`CREATE INDEX IF NOT EXISTS idx_media_items_year_month  ON media_items (year, month)`,

		// ── messages ────────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS messages (
			id               SERIAL PRIMARY KEY,
			chat_session     VARCHAR(500),
			message_date     TIMESTAMP,
			is_group_chat    BOOLEAN NOT NULL DEFAULT FALSE,
			delivered_date   TIMESTAMP,
			read_date        TIMESTAMP,
			edited_date      TIMESTAMP,
			service          VARCHAR(100),
			type             VARCHAR(50),
			sender_id        VARCHAR(255),
			sender_name      VARCHAR(500),
			status           VARCHAR(100),
			replying_to      VARCHAR(500),
			subject          VARCHAR(1000),
			text             TEXT,
			processed        BOOLEAN NOT NULL DEFAULT FALSE,
			created_at       TIMESTAMP DEFAULT NOW(),
			updated_at       TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chat_session ON messages (chat_session)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_sender_name  ON messages (sender_name)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_service      ON messages (service)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_processed    ON messages (processed)`,

		// ── message_attachments ─────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS message_attachments (
			id             SERIAL PRIMARY KEY,
			message_id     INTEGER NOT NULL REFERENCES messages(id)    ON DELETE CASCADE,
			media_item_id  INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
			created_at     TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_message_attachments_message_id    ON message_attachments (message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_attachments_media_item_id ON message_attachments (media_item_id)`,

		// ── facebook_albums ─────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS facebook_albums (
			id                        SERIAL PRIMARY KEY,
			name                      VARCHAR(500) NOT NULL,
			description               TEXT,
			cover_photo_uri           VARCHAR(500),
			last_modified_timestamp   TIMESTAMP,
			created_at                TIMESTAMP DEFAULT NOW(),
			updated_at                TIMESTAMP DEFAULT NOW()
		)`,

		// ── album_media ─────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS album_media (
			id             SERIAL PRIMARY KEY,
			album_id       INTEGER NOT NULL REFERENCES facebook_albums(id) ON DELETE CASCADE,
			media_item_id  INTEGER NOT NULL REFERENCES media_items(id)     ON DELETE CASCADE,
			created_at     TIMESTAMP DEFAULT NOW()
		)`,

		// ── facebook_posts ──────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS facebook_posts (
			id            SERIAL PRIMARY KEY,
			timestamp     TIMESTAMP,
			title         VARCHAR(500),
			post_text     TEXT,
			external_url  VARCHAR(2000),
			post_type     VARCHAR(50),
			created_at    TIMESTAMP DEFAULT NOW(),
			updated_at    TIMESTAMP DEFAULT NOW()
		)`,

		// ── post_media ──────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS post_media (
			id             SERIAL PRIMARY KEY,
			post_id        INTEGER NOT NULL REFERENCES facebook_posts(id) ON DELETE CASCADE,
			media_item_id  INTEGER NOT NULL REFERENCES media_items(id)    ON DELETE CASCADE,
			created_at     TIMESTAMP DEFAULT NOW()
		)`,

		// ── artefacts ───────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS artefacts (
			id           SERIAL PRIMARY KEY,
			name         VARCHAR(1000) NOT NULL,
			description  TEXT,
			tags         TEXT,
			story        TEXT,
			created_at   TIMESTAMP DEFAULT NOW(),
			updated_at   TIMESTAMP DEFAULT NOW()
		)`,

		// ── artefact_media ──────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS artefact_media (
			id             SERIAL PRIMARY KEY,
			artefact_id    INTEGER NOT NULL REFERENCES artefacts(id)    ON DELETE CASCADE,
			media_item_id  INTEGER NOT NULL REFERENCES media_items(id)  ON DELETE RESTRICT,
			sort_order     INTEGER NOT NULL DEFAULT 0,
			created_at     TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_artefact_media_artefact_id   ON artefact_media (artefact_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artefact_media_media_item_id ON artefact_media (media_item_id)`,

		// ── reference_documents ─────────────────────────────────────────────────
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
			created_at          TIMESTAMP DEFAULT NOW(),
			updated_at          TIMESTAMP DEFAULT NOW()
		)`,

		// ── places ──────────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS places (
			id                SERIAL PRIMARY KEY,
			name              VARCHAR(500) NOT NULL,
			description       TEXT,
			latitude          DOUBLE PRECISION,
			longitude         DOUBLE PRECISION,
			altitude          DOUBLE PRECISION,
			has_gps           BOOLEAN NOT NULL DEFAULT FALSE,
			google_maps_url   VARCHAR(500),
			region            VARCHAR(255),
			source            VARCHAR(255),
			source_reference  VARCHAR(500),
			created_at        TIMESTAMP DEFAULT NOW(),
			updated_at        TIMESTAMP DEFAULT NOW()
		)`,

		// ── contacts ────────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS contacts (
			id                  SERIAL PRIMARY KEY,
			name                TEXT      NOT NULL,
			alternative_names   TEXT,
			rel_type            TEXT      DEFAULT 'unknown',
			use_by_ai           BOOLEAN   DEFAULT FALSE,
			source              VARCHAR(255),
			source_reference    VARCHAR(500),
			is_subject          BOOLEAN   DEFAULT FALSE,
			is_group            BOOLEAN   DEFAULT FALSE,
			email               TEXT,
			numemails           INTEGER   DEFAULT 0,
			facebookid          TEXT,
			numfacebook         INTEGER   DEFAULT 0,
			whatsappid          TEXT,
			numwhatsapp         INTEGER   DEFAULT 0,
			imessageid          TEXT,
			numimessages        INTEGER   DEFAULT 0,
			smsid               TEXT,
			numsms              INTEGER   DEFAULT 0,
			instagramid         TEXT,
			numinstagram        INTEGER   DEFAULT 0,
			description         TEXT,
			total               INTEGER   DEFAULT 0,
			created_at          TIMESTAMP DEFAULT NOW(),
			updated_at          TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_email    ON contacts (email)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_name     ON contacts (name)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_rel_type ON contacts (rel_type)`,

		// ── relationships ───────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS relationships (
			id              SERIAL PRIMARY KEY,
			source_id       INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
			target_id       INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
			type            TEXT    NOT NULL,
			description     TEXT,
			ai_description  TEXT,
			strength        INTEGER,
			is_active       BOOLEAN NOT NULL DEFAULT TRUE,
			is_personal     BOOLEAN NOT NULL DEFAULT FALSE,
			is_deleted      BOOLEAN NOT NULL DEFAULT FALSE,
			created_at      TIMESTAMP DEFAULT NOW(),
			updated_at      TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_relationship_source ON relationships (source_id)`,
		`CREATE INDEX IF NOT EXISTS idx_relationship_target ON relationships (target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_relationship_type   ON relationships (type)`,

		// ── gemini_files ────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS gemini_files (
			id                     SERIAL PRIMARY KEY,
			reference_document_id  INTEGER      NOT NULL UNIQUE REFERENCES reference_documents(id) ON DELETE CASCADE,
			gemini_file_name       VARCHAR(500) NOT NULL,
			gemini_file_uri        VARCHAR(1000),
			filename               VARCHAR(500) NOT NULL,
			state                  VARCHAR(50)  NOT NULL DEFAULT 'ACTIVE',
			verified_at            TIMESTAMP    DEFAULT NOW(),
			created_at             TIMESTAMP    DEFAULT NOW(),
			updated_at             TIMESTAMP    DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gemini_file_reference_doc ON gemini_files (reference_document_id)`,
		`CREATE INDEX IF NOT EXISTS idx_gemini_file_name          ON gemini_files (gemini_file_name)`,

		// ── chat_conversations ──────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS chat_conversations (
			id               SERIAL PRIMARY KEY,
			title            VARCHAR(500) NOT NULL,
			voice            VARCHAR(100) NOT NULL,
			created_at       TIMESTAMP DEFAULT NOW(),
			updated_at       TIMESTAMP DEFAULT NOW(),
			last_message_at  TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_conv_last_message ON chat_conversations (last_message_at)`,

		// ── chat_turns ──────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS chat_turns (
			id               SERIAL PRIMARY KEY,
			conversation_id  INTEGER NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
			user_input       TEXT    NOT NULL,
			response_text    TEXT    NOT NULL,
			voice            VARCHAR(100),
			temperature      DOUBLE PRECISION,
			turn_number      INTEGER NOT NULL,
			created_at       TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_turn_conv_turn    ON chat_turns (conversation_id, turn_number)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_turn_conv_created ON chat_turns (conversation_id, created_at)`,

		// ── subject_configuration ───────────────────────────────────────────────
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
			writing_style_ai         TEXT,
			psychological_profile_ai TEXT,
			personality_profile_ai   TEXT,
			interests_ai             TEXT,
			system_instructions      TEXT NOT NULL,
			core_system_instructions TEXT NOT NULL,
			created_at               TIMESTAMP DEFAULT NOW(),
			updated_at               TIMESTAMP DEFAULT NOW()
		)`,

		// ── import_control_last_run ─────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS import_control_last_run (
			id              SERIAL PRIMARY KEY,
			import_type     VARCHAR(100) NOT NULL UNIQUE,
			last_run_at     TIMESTAMP    NOT NULL,
			result          VARCHAR(50)  NOT NULL,
			result_message  TEXT,
			created_at      TIMESTAMP DEFAULT NOW(),
			updated_at      TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_import_control_last_run_type ON import_control_last_run (import_type)`,

		// ── locations ───────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS locations (
			id                SERIAL PRIMARY KEY,
			name              VARCHAR(500) NOT NULL,
			description       TEXT,
			address           TEXT,
			latitude          DOUBLE PRECISION,
			longitude         DOUBLE PRECISION,
			region            VARCHAR(255),
			altitude          DOUBLE PRECISION,
			source            VARCHAR(255),
			source_reference  VARCHAR(500),
			created_at        TIMESTAMP DEFAULT NOW(),
			updated_at        TIMESTAMP DEFAULT NOW()
		)`,

		// ── complete_profiles ───────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS complete_profiles (
			id          SERIAL PRIMARY KEY,
			name        VARCHAR(500) NOT NULL,
			profile     TEXT,
			created_at  TIMESTAMP DEFAULT NOW(),
			updated_at  TIMESTAMP DEFAULT NOW()
		)`,

		// ── master_keys ─────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS master_keys (
			id          SERIAL PRIMARY KEY,
			comment     VARCHAR(500) NOT NULL DEFAULT 'Don''t think I''m stupid enough to use this as a master key',
			public_key  TEXT NOT NULL,
			created_at  TIMESTAMP DEFAULT NOW(),
			updated_at  TIMESTAMP DEFAULT NOW()
		)`,

		// ── sensitive_keyring ────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS sensitive_keyring (
			id                   SERIAL PRIMARY KEY,
			encrypted_dek        BYTEA   NOT NULL,
			encrypted_master_dek BYTEA,
			is_master            BOOLEAN NOT NULL DEFAULT FALSE,
			created_at           TIMESTAMP DEFAULT NOW()
		)`,

		// ── visitor_key_hints (plain-text hints for non-master / visitor seats) ───
		`CREATE TABLE IF NOT EXISTS visitor_key_hints (
			id           SERIAL PRIMARY KEY,
			keyring_id   INTEGER NOT NULL REFERENCES sensitive_keyring(id) ON DELETE CASCADE,
			hint         TEXT    NOT NULL,
			created_at   TIMESTAMP DEFAULT NOW(),
			UNIQUE(keyring_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visitor_key_hints_keyring_id ON visitor_key_hints (keyring_id)`,

		// ── private_store ────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS private_store (
			id               SERIAL PRIMARY KEY,
			key              TEXT    NOT NULL UNIQUE,
			encrypted_value  BYTEA   NOT NULL,
			created_at       TIMESTAMP DEFAULT NOW(),
			updated_at       TIMESTAMP DEFAULT NOW()
		)`,

		// ── email_classifications ───────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS email_classifications (
			id              SERIAL PRIMARY KEY,
			name            VARCHAR(500) NOT NULL,
			classification  VARCHAR(20)  NOT NULL,
			created_at      TIMESTAMP DEFAULT NOW(),
			updated_at      TIMESTAMP DEFAULT NOW()
		)`,

		// ── email_matches ───────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS email_matches (
			id            SERIAL PRIMARY KEY,
			primary_name  VARCHAR(500) NOT NULL,
			email         VARCHAR(300) NOT NULL,
			created_at    TIMESTAMP DEFAULT NOW(),
			updated_at    TIMESTAMP DEFAULT NOW()
		)`,

		// ── email_exclusions ────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS email_exclusions (
			id           SERIAL PRIMARY KEY,
			email        VARCHAR(300) NOT NULL,
			name         VARCHAR(500) NOT NULL,
			name_email   BOOLEAN      NOT NULL DEFAULT FALSE,
			created_at   TIMESTAMP DEFAULT NOW(),
			updated_at   TIMESTAMP DEFAULT NOW()
		)`,

		// ── saved_responses ─────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS saved_responses (
			id            SERIAL PRIMARY KEY,
			title         VARCHAR(500) NOT NULL,
			content       TEXT         NOT NULL,
			voice         VARCHAR(100),
			llm_provider  VARCHAR(100),
			created_at    TIMESTAMP DEFAULT NOW()
		)`,

		// ── app_configuration ───────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS app_configuration (
			id            SERIAL PRIMARY KEY,
			key           VARCHAR(255) NOT NULL UNIQUE,
			value         TEXT,
			is_mandatory  BOOLEAN NOT NULL DEFAULT FALSE,
			description   TEXT,
			created_at    TIMESTAMP DEFAULT NOW(),
			updated_at    TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_configuration_key ON app_configuration (key)`,

		// ── interests ───────────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS interests (
			id          SERIAL PRIMARY KEY,
			name        VARCHAR(500) NOT NULL,
			created_at  TIMESTAMP DEFAULT NOW(),
			updated_at  TIMESTAMP DEFAULT NOW()
		)`,

		// ── custom_voices ───────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS custom_voices (
			id           SERIAL PRIMARY KEY,
			key          VARCHAR(100) NOT NULL UNIQUE,
			name         VARCHAR(200) NOT NULL,
			description  VARCHAR(500),
			instructions TEXT         NOT NULL,
			creativity   DOUBLE PRECISION NOT NULL DEFAULT 0.5,
			created_at   TIMESTAMP DEFAULT NOW(),
			updated_at   TIMESTAMP DEFAULT NOW()
		)`,

		// Legacy tables removed (replaced by reference_documents + sensitive_keyring / media_items).
		`DROP TABLE IF EXISTS media_metadata`,
		`DROP TABLE IF EXISTS sensitive_data`,
		`DROP TABLE IF EXISTS trusted_keys`,
	}
}

// regionFunctionsDDL returns the two PL/pgSQL geographic region functions.
// These match the Python update_location_regions() / update_image_location_regions() functions
// in src/database/connection.py exactly, including region code strings and bounding boxes.
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
