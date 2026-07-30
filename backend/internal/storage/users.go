package storage

import (
	"context"
	"fmt"

	"ditalk/backend/internal/crypto"
)

// localUserIdentity is the fixed identity for single-user local-first mode. Once
// real authentication exists (doc bab 18.1), the user comes from the session
// instead and this helper is retired.
const localUserIdentity = "local-owner"

// EnsureLocalUser returns the id of the single local user, creating it on first
// use. The identity is hashed so no raw identifier is stored.
func (db *DB) EnsureLocalUser(ctx context.Context, c *crypto.Cipher) (string, error) {
	hash := c.Hash(localUserIdentity)

	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (email_hash)
		VALUES ($1)
		ON CONFLICT (email_hash) DO UPDATE SET updated_at = now()
		RETURNING id`,
		hash,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("ensure local user: %w", err)
	}
	return id, nil
}
