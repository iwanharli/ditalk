package ai

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"ditalk/backend/internal/config"
)

// Tier selects how much reasoning a unit of work deserves. Escalation happens
// only on low confidence, modality conflict, or high-impact insight (doc 23A.2).
type Tier int

const (
	Tier1 Tier = iota // bulk volume: extraction, candidates, simple classification
	Tier2             // session analysis, vision, relationship pillars
	Tier3             // ambiguous cases, period reports, GCI explanation
)

type Client struct {
	api *openai.Client
	cfg config.Config
}

func NewClient(cfg config.Config) *Client {
	c := openai.NewClient(option.WithAPIKey(cfg.OpenAIAPIKey))
	return &Client{api: &c, cfg: cfg}
}

// Model resolves a tier to a configured model id, so swapping providers or
// versions never requires touching analysis code (doc prinsip model-agnostic).
func (c *Client) Model(t Tier) string {
	switch t {
	case Tier3:
		return c.cfg.ModelTier3
	case Tier2:
		return c.cfg.ModelTier2
	default:
		return c.cfg.ModelTier1
	}
}

func (c *Client) TranscribeModel(highQuality bool) string {
	if highQuality {
		return c.cfg.ModelTranscribeHQ
	}
	return c.cfg.ModelTranscribeDefault
}

func (c *Client) EmbeddingModel() string {
	return c.cfg.EmbeddingModel
}

func (c *Client) API() *openai.Client {
	return c.api
}
