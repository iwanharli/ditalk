-- +goose Up
-- Personal Knowledge Vault / Relationship Memory.
-- Reference: doc bab 16A. Nothing here is treated as fact until the user
-- confirms it; every row links back to its source messages.

CREATE TABLE people (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    alias          text NOT NULL,
    name_cipher    bytea,
    conversation_id uuid REFERENCES conversations(id) ON DELETE SET NULL,
    notes          text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, alias)
);

CREATE TABLE relationships (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    person_a     uuid NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    person_b     uuid REFERENCES people(id) ON DELETE CASCADE,
    relation_type text NOT NULL,
    is_confirmed boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_relationships_person ON relationships(person_a);

CREATE TABLE places (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        text NOT NULL,
    category    text,
    -- Precise coordinates are opt-in only; general locality is the default (doc 16A.1).
    locality    text,
    latitude    double precision,
    longitude   double precision,
    precise_opt_in boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT coords_require_opt_in CHECK (
        precise_opt_in OR (latitude IS NULL AND longitude IS NULL)
    )
);

-- Central memory record. Status follows the lifecycle in doc 16A.2.
CREATE TABLE memories (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    person_id         uuid REFERENCES people(id) ON DELETE CASCADE,
    category          text NOT NULL CHECK (category IN (
                        'event','important_date','place','preference','person',
                        'commitment','wishlist','boundary','decision',
                        'shared_experience','routine','journal')),
    title             text NOT NULL,
    body              text,
    status            text NOT NULL DEFAULT 'candidate'
                      CHECK (status IN ('candidate','confirmed','superseded','archived','outdated')),
    confidence        integer NOT NULL CHECK (confidence BETWEEN 0 AND 100),
    preference_strength integer CHECK (preference_strength BETWEEN 0 AND 100),
    importance        integer CHECK (importance BETWEEN 0 AND 100),
    recency           integer CHECK (recency BETWEEN 0 AND 100),
    frequency         integer NOT NULL DEFAULT 1,
    stability         integer CHECK (stability BETWEEN 0 AND 100),
    sensitivity       text NOT NULL DEFAULT 'low' CHECK (sensitivity IN ('low','medium','high')),
    valid_from        timestamptz,
    valid_until       timestamptz,
    last_confirmed_at timestamptz,
    supersedes        uuid REFERENCES memories(id) ON DELETE SET NULL,
    model_version     text,
    prompt_version    text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    -- Below 60 confidence a memory is never stored as confirmed (doc 16A.3).
    CONSTRAINT low_confidence_stays_candidate CHECK (
        confidence >= 60 OR status <> 'confirmed'
    )
);
CREATE INDEX idx_memories_user_status ON memories(user_id, status);
CREATE INDEX idx_memories_person ON memories(person_id, category);
CREATE INDEX idx_memories_inbox ON memories(user_id, confidence) WHERE status = 'candidate';

CREATE TABLE memory_evidence (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id  uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    message_id uuid REFERENCES messages(id) ON DELETE CASCADE,
    session_id uuid REFERENCES conversation_sessions(id) ON DELETE CASCADE,
    quote      text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT memory_evidence_target CHECK (message_id IS NOT NULL OR session_id IS NOT NULL)
);
CREATE INDEX idx_memory_evidence_memory ON memory_evidence(memory_id);

CREATE TABLE memory_versions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id    uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    snapshot     jsonb NOT NULL,
    change_reason text,
    valid_from   timestamptz NOT NULL,
    valid_until  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_memory_versions_memory ON memory_versions(memory_id, valid_from DESC);

CREATE TABLE memory_confirmations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id  uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    action     text NOT NULL CHECK (action IN ('confirmed','edited','ignored','marked_sensitive','expired','deleted')),
    note       text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_memory_confirmations_memory ON memory_confirmations(memory_id);

CREATE TABLE events (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id  uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    place_id   uuid REFERENCES places(id) ON DELETE SET NULL,
    starts_at  timestamptz,
    ends_at    timestamptz,
    date_precision text NOT NULL DEFAULT 'exact'
               CHECK (date_precision IN ('exact','approximate','range','recurring','unconfirmed')),
    participants jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE important_dates (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id      uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    occurs_on      date,
    is_recurring   boolean NOT NULL DEFAULT false,
    date_precision text NOT NULL DEFAULT 'exact'
                   CHECK (date_precision IN ('exact','approximate','range','recurring','unconfirmed')),
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_important_dates_occurs ON important_dates(occurs_on);

CREATE TABLE preferences (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id  uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    item       text NOT NULL,
    item_category text,
    polarity   text NOT NULL CHECK (polarity IN ('like','dislike','neutral')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE commitments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id    uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    owner_role   text NOT NULL CHECK (owner_role IN ('SELF','OTHER','BOTH')),
    description  text NOT NULL,
    due_at       timestamptz,
    status       text NOT NULL DEFAULT 'proposed' CHECK (status IN (
                   'proposed','accepted','scheduled','fulfilled','delayed','cancelled','missed','unknown')),
    topic        text,
    resolved_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_commitments_open ON commitments(status, due_at)
    WHERE status IN ('proposed','accepted','scheduled','delayed');

CREATE TABLE wishlists (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id  uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    item       text NOT NULL,
    status     text NOT NULL DEFAULT 'idea' CHECK (status IN (
                 'idea','discussed','agreed','scheduled','completed','cancelled','dormant')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE boundaries (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id    uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    -- Framed as respectful boundaries, never as a list of weaknesses (doc 16A.1).
    description  text NOT NULL,
    stated_by    text NOT NULL CHECK (stated_by IN ('SELF','OTHER')),
    acknowledged boolean NOT NULL DEFAULT false,
    needs_human_review boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE decisions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id    uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    description  text NOT NULL,
    agreed_by    jsonb NOT NULL DEFAULT '[]'::jsonb,
    decided_at   timestamptz,
    status       text NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','superseded','cancelled')),
    version      integer NOT NULL DEFAULT 1,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shared_experiences (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id     uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    place_id      uuid REFERENCES places(id) ON DELETE SET NULL,
    occurred_at   timestamptz,
    emotion_before text,
    emotion_after  text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE routines (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id         uuid NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    description       text NOT NULL,
    cadence           text,
    -- Habits change, so this is mandatory for re-confirmation (doc 16A.1).
    last_confirmed_at timestamptz NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);

-- Reminders never fire externally without explicit opt-in (doc bab 17).
CREATE TABLE reminders (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    memory_id    uuid REFERENCES memories(id) ON DELETE CASCADE,
    remind_at    timestamptz NOT NULL,
    channel      text NOT NULL DEFAULT 'in_app' CHECK (channel IN ('in_app','email','none')),
    opted_in     boolean NOT NULL DEFAULT false,
    status       text NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','sent','dismissed','cancelled')),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_reminders_due ON reminders(remind_at) WHERE status = 'pending';

-- Manual user notes about things that happened outside WhatsApp. Never sent to
-- an AI provider by default (doc 16A.1).
CREATE TABLE journal_entries (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    person_id      uuid REFERENCES people(id) ON DELETE SET NULL,
    occurred_at    timestamptz NOT NULL,
    body_cipher    bytea NOT NULL,
    share_with_ai  boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_journal_user_time ON journal_entries(user_id, occurred_at DESC);

CREATE TABLE relationship_context (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    relation_type   text,
    known_since     date,
    timezone        text,
    other_channels  jsonb NOT NULL DEFAULT '[]'::jsonb,
    major_events    jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (conversation_id)
);

-- +goose Down
DROP TABLE IF EXISTS relationship_context;
DROP TABLE IF EXISTS journal_entries;
DROP TABLE IF EXISTS reminders;
DROP TABLE IF EXISTS routines;
DROP TABLE IF EXISTS shared_experiences;
DROP TABLE IF EXISTS decisions;
DROP TABLE IF EXISTS boundaries;
DROP TABLE IF EXISTS wishlists;
DROP TABLE IF EXISTS commitments;
DROP TABLE IF EXISTS preferences;
DROP TABLE IF EXISTS important_dates;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS memory_confirmations;
DROP TABLE IF EXISTS memory_versions;
DROP TABLE IF EXISTS memory_evidence;
DROP TABLE IF EXISTS memories;
DROP TABLE IF EXISTS places;
DROP TABLE IF EXISTS relationships;
DROP TABLE IF EXISTS people;
