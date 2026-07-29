package main

import (
	"context"
	"log"

	"github.com/hibiken/asynq"

	"ditalk/backend/internal/ai"
	"ditalk/backend/internal/config"
	"ditalk/backend/internal/queue"
	"ditalk/backend/internal/storage"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	db, err := storage.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	aiClient := ai.NewClient(cfg)

	mux := asynq.NewServeMux()
	// Handlers land here as the pipeline is built, one task type at a time.
	mux.HandleFunc(queue.TaskTextAnalysis, func(ctx context.Context, t *asynq.Task) error {
		log.Printf("todo: %s with model %s", t.Type(), aiClient.Model(ai.Tier1))
		return nil
	})

	if err := queue.NewServer(cfg.RedisAddr, cfg.RedisDB).Run(mux); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
