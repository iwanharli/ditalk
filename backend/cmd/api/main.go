package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ditalk/backend/internal/ai"
	"ditalk/backend/internal/api"
	"ditalk/backend/internal/config"
	"ditalk/backend/internal/crypto"
	"ditalk/backend/internal/queue"
	"ditalk/backend/internal/storage"
	"ditalk/backend/internal/wa"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	if err := storage.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	db, err := storage.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	q := queue.NewClient(cfg.RedisAddr, cfg.RedisDB)
	defer q.Close()

	// Without a key the server still starts, but endpoints that persist chat
	// content refuse to run rather than writing plaintext to disk.
	var cipher *crypto.Cipher
	if cipher, err = crypto.New(cfg.EncryptionKey); err != nil {
		log.Printf("WARNING: encryption disabled (%v); import and sync will be rejected", err)
		cipher = nil
	}

	if cfg.InternalToken == "" {
		log.Printf("WARNING: INTERNAL_TOKEN kosong; connector tidak dapat mengirim event")
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)
	waState := wa.NewState()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewServer(cfg, db, q, ai.NewClient(cfg), cipher, waState, logger).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("ditalk backend listening on :%s (%s)", cfg.Port, cfg.Environment)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
