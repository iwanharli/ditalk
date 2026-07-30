package storage

import (
	"context"
	"fmt"
	"time"

	"ditalk/backend/internal/crypto"
)

// BackfillContentHashes fills content_hash for rows stored before the column
// existed.
//
// It runs in Go rather than SQL because the text is encrypted at rest. Rows that
// cannot be decrypted, or whose text is empty, keep a NULL hash: there is no
// content to identify them by, so they take part in no deduplication.
func (db *DB) BackfillContentHashes(ctx context.Context, c *crypto.Cipher) (int, error) {
	type pending struct {
		id   string
		hash string
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT id, timestamp, sender_role, text_cipher
		FROM messages
		WHERE content_hash IS NULL AND text_cipher IS NOT NULL`)
	if err != nil {
		return 0, fmt.Errorf("read messages: %w", err)
	}

	var todo []pending
	for rows.Next() {
		var (
			id     string
			ts     time.Time
			role   string
			cipher []byte
		)
		if err := rows.Scan(&id, &ts, &role, &cipher); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan: %w", err)
		}

		plain, err := c.DecryptString(cipher)
		if err != nil {
			// A row that cannot be decrypted keeps a NULL hash rather than a wrong one.
			continue
		}

		if h := ContentHash(ts, role, plain); h != "" {
			todo = append(todo, pending{id: id, hash: h})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate: %w", err)
	}

	updated := 0
	for _, p := range todo {
		// Two genuinely identical messages in one conversation collide on the unique
		// index. The guard leaves the later row unhashed instead of aborting the
		// whole pass over one duplicate.
		tag, err := db.Pool.Exec(ctx, `
			UPDATE messages m SET content_hash = $2
			WHERE m.id = $1
			  AND NOT EXISTS (
			    SELECT 1 FROM messages o
			    WHERE o.conversation_id = m.conversation_id
			      AND o.content_hash = $2)`,
			p.id, p.hash)
		if err != nil {
			return updated, fmt.Errorf("update: %w", err)
		}
		updated += int(tag.RowsAffected())
	}
	return updated, nil
}
