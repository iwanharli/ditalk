// Command hashfill computes content_hash for messages stored before the column
// existed.
//
// The hash cannot be produced in SQL because the text is encrypted at rest, so
// each row has to be decrypted, hashed, and written back. Without it, rows
// predating the column take part in no content-based deduplication at all.
package main

import (
	"context"
	"log"
	"os"

	"ditalk/backend/internal/config"
	"ditalk/backend/internal/crypto"
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

	c, err := crypto.New(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("encryption: %v", err)
	}

	n, err := db.BackfillContentHashes(ctx, c)
	if err != nil {
		log.Fatalf("hashfill: %v", err)
	}
	log.Printf("content_hash diisi untuk %d pesan", n)
	os.Exit(0)
}
