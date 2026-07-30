-- +goose Up
-- Explicit allowlist of contacts the machine is permitted to read.
--
-- This is the enforcement point for "only numbers I registered": ingestion drops
-- any chat whose contact is absent or inactive here. It also implements the
-- data-minimisation requirement in doc bab 19.1 — nothing is processed just
-- because it happens to arrive on the linked device.

CREATE TABLE allowed_contacts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Normalized international digits without a plus, e.g. 6281234567890.
    phone       text NOT NULL,
    -- Stable lookup value; the raw number is also kept because the owner needs
    -- to see and manage their own allowlist in the UI.
    phone_hash  text NOT NULL,
    label       text,
    is_active   boolean NOT NULL DEFAULT true,
    -- Set when the user confirms this contact may be analyzed (doc bab 19.2).
    consent_note text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT phone_is_digits CHECK (phone ~ '^[0-9]{8,20}$'),
    UNIQUE (user_id, phone)
);
CREATE INDEX idx_allowed_contacts_lookup ON allowed_contacts(user_id, phone_hash)
    WHERE is_active;

-- Counts rejected traffic without storing what was rejected, so the user can
-- see the filter is working without the machine retaining out-of-scope data
-- (doc bab 24.2: metadata only, never content).
CREATE TABLE ingest_rejections (
    id          bigserial PRIMARY KEY,
    user_id     uuid REFERENCES users(id) ON DELETE CASCADE,
    reason      text NOT NULL CHECK (reason IN (
                  'not_allowlisted','group_chat','unsupported_jid','inactive_contact')),
    occurred_on date NOT NULL DEFAULT current_date,
    count       integer NOT NULL DEFAULT 1,
    UNIQUE (user_id, reason, occurred_on)
);

-- +goose Down
DROP TABLE IF EXISTS ingest_rejections;
DROP TABLE IF EXISTS allowed_contacts;
