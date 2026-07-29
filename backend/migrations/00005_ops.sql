-- +goose Up
-- Operational tables: audit, deletion ledger, job bookkeeping.
-- Reference: doc bab 17 (cascade deletion), 18 (audit), 24 (logging policy).

-- Audit records carry operational metadata only. Chat content, phone numbers,
-- credentials, and raw prompts must never land here (doc 24.2).
CREATE TABLE audit_log (
    id          bigserial PRIMARY KEY,
    user_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    action      text NOT NULL,
    target_type text,
    target_id   text,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    ip_hash     text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_user_time ON audit_log(user_id, created_at DESC);
CREATE INDEX idx_audit_action ON audit_log(action, created_at DESC);

-- Proves when data actually disappears from every copy, including backups
-- whose expiry runs on a separate clock (doc bab 17).
CREATE TABLE deletion_ledger (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           uuid REFERENCES users(id) ON DELETE SET NULL,
    target_type       text NOT NULL,
    target_id         text NOT NULL,
    requested_at      timestamptz NOT NULL DEFAULT now(),
    db_deleted_at     timestamptz,
    objects_deleted_at timestamptz,
    cache_purged_at   timestamptz,
    backup_expires_at timestamptz,
    completed_at      timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_deletion_pending ON deletion_ledger(requested_at) WHERE completed_at IS NULL;

-- Deterministic job IDs prevent double processing across retries (doc 22.3).
CREATE TABLE processing_jobs (
    id           text PRIMARY KEY,
    job_type     text NOT NULL,
    payload      jsonb NOT NULL DEFAULT '{}'::jsonb,
    status       text NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','running','succeeded','failed','dead_letter','cancelled')),
    attempts     integer NOT NULL DEFAULT 0,
    last_error   text,
    started_at   timestamptz,
    finished_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_jobs_status ON processing_jobs(status, created_at);
CREATE INDEX idx_jobs_type ON processing_jobs(job_type, status);

-- Per-provider spend tracking so the cost dashboard in doc 23B.6 has real data.
CREATE TABLE ai_usage (
    id            bigserial PRIMARY KEY,
    user_id       uuid REFERENCES users(id) ON DELETE SET NULL,
    provider      text NOT NULL,
    model         text NOT NULL,
    operation     text NOT NULL,
    input_tokens  integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    audio_seconds real NOT NULL DEFAULT 0,
    image_count   integer NOT NULL DEFAULT 0,
    cost_micros   bigint NOT NULL DEFAULT 0,
    conversation_id uuid REFERENCES conversations(id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_usage_time ON ai_usage(created_at DESC);
CREATE INDEX idx_ai_usage_model ON ai_usage(model, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS ai_usage;
DROP TABLE IF EXISTS processing_jobs;
DROP TABLE IF EXISTS deletion_ledger;
DROP TABLE IF EXISTS audit_log;
