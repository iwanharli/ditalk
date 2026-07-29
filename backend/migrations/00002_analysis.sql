-- +goose Up
-- AI analysis output and semantic index.
-- Reference: doc bab 12 (fusion), 13 (structured output), 16 (semantic search).

-- One row per analyzed unit. Either message_id or session_id is set, not both.
CREATE TABLE analyses (
    id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id                uuid REFERENCES messages(id) ON DELETE CASCADE,
    session_id                uuid REFERENCES conversation_sessions(id) ON DELETE CASCADE,
    primary_emotion           text,
    secondary_emotions        jsonb NOT NULL DEFAULT '[]'::jsonb,
    valence                   real CHECK (valence BETWEEN -1 AND 1),
    arousal                   real CHECK (arousal BETWEEN 0 AND 1),
    intensity                 real CHECK (intensity BETWEEN 0 AND 1),
    confidence                real CHECK (confidence BETWEEN 0 AND 1),
    communication_tone        jsonb NOT NULL DEFAULT '[]'::jsonb,
    topics                    jsonb NOT NULL DEFAULT '[]'::jsonb,
    intent                    text,
    context_sufficiency       text CHECK (context_sufficiency IN ('low','medium','high')),
    modality_agreement        text CHECK (modality_agreement IN ('conflict','partial','agree')),
    evidence                  jsonb NOT NULL DEFAULT '[]'::jsonb,
    alternative_interpretations jsonb NOT NULL DEFAULT '[]'::jsonb,
    limitations               jsonb NOT NULL DEFAULT '[]'::jsonb,
    status                    text NOT NULL DEFAULT 'complete'
                              CHECK (status IN ('complete','insufficient_data','not_applicable','failed')),
    model_version             text NOT NULL,
    prompt_version            text NOT NULL,
    superseded_by             uuid REFERENCES analyses(id) ON DELETE SET NULL,
    created_at                timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT analyses_target_exactly_one CHECK (
        (message_id IS NOT NULL AND session_id IS NULL)
        OR (message_id IS NULL AND session_id IS NOT NULL)
    )
);
CREATE INDEX idx_analyses_message ON analyses(message_id);
CREATE INDEX idx_analyses_session ON analyses(session_id);
-- Low-confidence results feed the "Moment Review" queue (doc bab 14).
CREATE INDEX idx_analyses_low_confidence ON analyses(confidence) WHERE confidence < 0.6;

-- User corrections keep an audit trail rather than overwriting AI output (doc 2).
CREATE TABLE analysis_corrections (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    analysis_id uuid NOT NULL REFERENCES analyses(id) ON DELETE CASCADE,
    field       text NOT NULL,
    old_value   jsonb,
    user_value  jsonb NOT NULL,
    reason      text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_analysis_corrections_analysis ON analysis_corrections(analysis_id);

-- Semantic index. 1536 dims matches text-embedding-3-small; change together
-- with the model and reindex (doc bab 16).
CREATE TABLE embeddings (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_type text NOT NULL
                CHECK (source_type IN ('message','session','transcript','ocr','memory','journal','insight')),
    source_id   uuid NOT NULL,
    conversation_id uuid REFERENCES conversations(id) ON DELETE CASCADE,
    content_hash text NOT NULL,
    vector      vector(1536) NOT NULL,
    model       text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_type, source_id, model)
);
CREATE INDEX idx_embeddings_source ON embeddings(source_type, source_id);
CREATE INDEX idx_embeddings_conversation ON embeddings(conversation_id);
CREATE INDEX idx_embeddings_vector ON embeddings
    USING hnsw (vector vector_cosine_ops);

-- +goose Down
DROP TABLE IF EXISTS embeddings;
DROP TABLE IF EXISTS analysis_corrections;
DROP TABLE IF EXISTS analyses;
