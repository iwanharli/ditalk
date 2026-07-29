package queue

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// Task names mirror the internal event domain in doc bab 22.2.
const (
	TaskMessageIngested   = "message:ingested"
	TaskMediaDownload     = "media:download"
	TaskMediaTranscode    = "media:transcode"
	TaskTranscribe        = "media:transcribe"
	TaskVision            = "media:vision"
	TaskTextAnalysis      = "analysis:text"
	TaskEmotionAnalysis   = "analysis:emotion"
	TaskSessionAnalysis   = "analysis:session"
	TaskMemoryCandidate   = "memory:candidate"
	TaskRelationshipScore = "analysis:relationship"
	TaskAggregateScore    = "score:aggregate"
	TaskEmbedding         = "embedding:create"
)

// Separate queues let heavy media work run at lower concurrency than text.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueMedia    = "media"
	QueueLow      = "low"
)

type MessagePayload struct {
	MessageID      string `json:"message_id"`
	ConversationID string `json:"conversation_id"`
}

type MediaPayload struct {
	MediaAssetID string `json:"media_asset_id"`
	MessageID    string `json:"message_id"`
	MimeType     string `json:"mime_type"`
}

type SessionPayload struct {
	SessionID string `json:"session_id"`
}

type AggregatePayload struct {
	ConversationID string `json:"conversation_id"`
	Level          string `json:"level"`
	PeriodStart    string `json:"period_start"`
	PeriodEnd      string `json:"period_end"`
}

// NewTask builds a task whose ID is derived from the payload, so a retry or a
// duplicate event never causes double processing (doc bab 22.3).
func NewTask(name string, payload any, opts ...asynq.Option) (*asynq.Task, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", name, err)
	}
	return asynq.NewTask(name, body, opts...), nil
}
