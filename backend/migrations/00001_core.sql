-- +goose Up
-- Core ingestion model: users, WhatsApp session, conversations, messages, media.
-- Reference: doc bab 17 "Desain Data dan Retensi".

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email_hash  text NOT NULL UNIQUE,
    settings    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Baileys credentials. encrypted_auth is ciphertext only; the key lives in
-- KMS/Vault, never alongside the database (doc 5.3).
CREATE TABLE wa_sessions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    encrypted_auth bytea,
    status         text NOT NULL DEFAULT 'disconnected'
                   CHECK (status IN ('disconnected','pairing','connected','logged_out','error')),
    last_synced_at timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_wa_sessions_user ON wa_sessions(user_id);

-- chat_id_hash stores a hash of the JID; display_name is encrypted or aliased
-- so raw phone numbers never sit in plaintext (doc 6.2).
CREATE TABLE conversations (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id_hash        text NOT NULL,
    display_name_cipher bytea,
    alias               text,
    is_group            boolean NOT NULL DEFAULT false,
    is_selected         boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, chat_id_hash)
);
CREATE INDEX idx_conversations_selected ON conversations(user_id, is_selected);

CREATE TABLE messages (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id   uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    wa_message_id     text NOT NULL,
    sender_role       text NOT NULL CHECK (sender_role IN ('SELF','OTHER')),
    timestamp         timestamptz NOT NULL,
    message_type      text NOT NULL
                      CHECK (message_type IN ('text','image','audio','video','sticker','document','unknown')),
    text_cipher       bytea,
    caption_cipher    bytea,
    quoted_message_id text,
    reactions         jsonb NOT NULL DEFAULT '[]'::jsonb,
    is_view_once      boolean NOT NULL DEFAULT false,
    is_ephemeral      boolean NOT NULL DEFAULT false,
    is_deleted        boolean NOT NULL DEFAULT false,
    edited_at         timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    -- Idempotency key so reconnect + history sync never duplicate (doc 6.2).
    UNIQUE (conversation_id, wa_message_id)
);
CREATE INDEX idx_messages_conv_time ON messages(conversation_id, timestamp DESC);
CREATE INDEX idx_messages_type ON messages(conversation_id, message_type);

-- Sessionization boundary: a run of messages within one active topic (doc 6.3).
CREATE TABLE conversation_sessions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    start_at        timestamptz NOT NULL,
    end_at          timestamptz NOT NULL,
    message_count   integer NOT NULL DEFAULT 0,
    summary_cipher  bytea,
    topics          jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_conv_time ON conversation_sessions(conversation_id, start_at DESC);

CREATE TABLE media_assets (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id     uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    mime_type      text NOT NULL,
    sha256         text,
    byte_size      bigint,
    duration_ms    integer,
    width          integer,
    height         integer,
    object_key     text,
    thumbnail_key  text,
    transcript     text,
    ocr_text       text,
    prosody        jsonb,
    scan_status    text NOT NULL DEFAULT 'pending'
                   CHECK (scan_status IN ('pending','clean','infected','skipped','failed')),
    expires_at     timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_media_message ON media_assets(message_id);
CREATE INDEX idx_media_sha256 ON media_assets(sha256);
CREATE INDEX idx_media_expiry ON media_assets(expires_at) WHERE expires_at IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS media_assets;
DROP TABLE IF EXISTS conversation_sessions;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS wa_sessions;
DROP TABLE IF EXISTS users;
