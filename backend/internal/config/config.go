package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	Environment string

	DatabaseURL  string
	RedisAddr    string
	RedisDB      int
	AllowOrigins []string

	OpenAIAPIKey string
	AIMediaURL   string

	// Model routing per doc bab 23A. Tier 1 handles bulk volume, tier 3 only
	// escalations, so the expensive model never sees every message.
	ModelTier1 string
	ModelTier2 string
	ModelTier3 string
	ModelTranscribeDefault string
	ModelTranscribeHQ      string
	EmbeddingModel         string

	// Field-level encryption key for chat text, JIDs, and session credentials.
	// Must come from KMS/Vault in production, never stored beside the database.
	EncryptionKey string
	JWTSecret     string
}

// Load reads .env if present, then environment variables take precedence.
func Load() Config {
	_ = godotenv.Load()

	return Config{
		Port:        getEnv("PORT", "8080"),
		Environment: getEnv("ENV", "development"),

		DatabaseURL:  getEnv("DATABASE_URL", "postgres://localhost:5432/db_ditalk?sslmode=disable"),
		RedisAddr:    getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisDB:      getEnvInt("REDIS_DB", 0),
		AllowOrigins: []string{getEnv("FRONTEND_ORIGIN", "http://localhost:5173")},

		OpenAIAPIKey: os.Getenv("OPENAI_API_KEY"),
		AIMediaURL:   getEnv("AI_MEDIA_URL", "http://127.0.0.1:8000"),

		ModelTier1:             getEnv("MODEL_TIER1", "gpt-5.6-luna"),
		ModelTier2:             getEnv("MODEL_TIER2", "gpt-5.6-terra"),
		ModelTier3:             getEnv("MODEL_TIER3", "gpt-5.6-sol"),
		ModelTranscribeDefault: getEnv("MODEL_TRANSCRIBE", "gpt-4o-mini-transcribe"),
		ModelTranscribeHQ:      getEnv("MODEL_TRANSCRIBE_HQ", "gpt-4o-transcribe"),
		EmbeddingModel:         getEnv("EMBEDDING_MODEL", "text-embedding-3-small"),

		EncryptionKey: os.Getenv("ENCRYPTION_KEY"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
