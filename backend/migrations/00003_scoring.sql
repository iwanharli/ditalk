-- +goose Up
-- Aggregate scoring: session -> period -> global, with reliability and versioning.
-- Reference: doc bab 12A-12F.

-- Formula + weights are versioned so old reports stay reproducible (doc 12C).
CREATE TABLE score_versions (
    version      text PRIMARY KEY,
    formula_json jsonb NOT NULL,
    thresholds   jsonb NOT NULL DEFAULT '{}'::jsonb,
    notes        text,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE aggregate_scores (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id          uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    aggregation_level        text NOT NULL CHECK (aggregation_level IN ('session','day','week','month','custom')),
    period_start             timestamptz NOT NULL,
    period_end               timestamptz NOT NULL,
    global_communication_index integer CHECK (global_communication_index BETWEEN 0 AND 100),
    data_reliability         integer NOT NULL CHECK (data_reliability BETWEEN 0 AND 100),
    reliability_status       text NOT NULL
                             CHECK (reliability_status IN ('insufficient','provisional','sufficient','high','very_high')),
    scores                   jsonb NOT NULL DEFAULT '{}'::jsonb,
    coverage                 jsonb NOT NULL DEFAULT '{}'::jsonb,
    score_version            text NOT NULL REFERENCES score_versions(version),
    model_version            text NOT NULL,
    superseded_by            uuid REFERENCES aggregate_scores(id) ON DELETE SET NULL,
    calculated_at            timestamptz NOT NULL DEFAULT now(),
    -- GCI must be NULL whenever reliability is insufficient (doc 12E.2).
    CONSTRAINT gci_hidden_when_insufficient CHECK (
        reliability_status <> 'insufficient' OR global_communication_index IS NULL
    ),
    CONSTRAINT period_order CHECK (period_end >= period_start)
);
CREATE INDEX idx_aggregate_scores_lookup
    ON aggregate_scores(conversation_id, aggregation_level, period_start DESC);
CREATE UNIQUE INDEX idx_aggregate_scores_current
    ON aggregate_scores(conversation_id, aggregation_level, period_start, period_end, score_version)
    WHERE superseded_by IS NULL;

-- Every subscore must be openable down to its source evidence (doc 12A).
CREATE TABLE score_evidence_links (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_score_id uuid NOT NULL REFERENCES aggregate_scores(id) ON DELETE CASCADE,
    component          text NOT NULL,
    session_id         uuid REFERENCES conversation_sessions(id) ON DELETE CASCADE,
    message_id         uuid REFERENCES messages(id) ON DELETE CASCADE,
    contribution       real NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT evidence_target_present CHECK (session_id IS NOT NULL OR message_id IS NOT NULL)
);
CREATE INDEX idx_score_evidence_score ON score_evidence_links(aggregate_score_id, component);

CREATE TABLE score_corrections (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_score_id uuid NOT NULL REFERENCES aggregate_scores(id) ON DELETE CASCADE,
    component          text NOT NULL,
    user_value         real NOT NULL,
    reason             text,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_score_corrections_score ON score_corrections(aggregate_score_id);

-- Cached period-over-period deltas. Comparison is suppressed when either side
-- has reliability below 60 (doc 12E.4).
CREATE TABLE period_comparisons (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    period_a    uuid NOT NULL REFERENCES aggregate_scores(id) ON DELETE CASCADE,
    period_b    uuid NOT NULL REFERENCES aggregate_scores(id) ON DELETE CASCADE,
    delta_json  jsonb NOT NULL DEFAULT '{}'::jsonb,
    reliability integer NOT NULL CHECK (reliability BETWEEN 0 AND 100),
    is_conclusive boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (period_a, period_b)
);

-- +goose Down
DROP TABLE IF EXISTS period_comparisons;
DROP TABLE IF EXISTS score_corrections;
DROP TABLE IF EXISTS score_evidence_links;
DROP TABLE IF EXISTS aggregate_scores;
DROP TABLE IF EXISTS score_versions;
