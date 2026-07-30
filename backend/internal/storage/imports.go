package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ditalk/backend/internal/crypto"
	"ditalk/backend/internal/waimport"
)

type ImportResult struct {
	ConversationID string
	Inserted       int
	Skipped        int
	Sessions       int
}

// ImportExport writes a parsed export into the database inside one transaction.
//
// Export files carry no message IDs, so a synthetic one is derived from the
// message content. Re-importing an overlapping export therefore skips rows that
// already exist rather than duplicating them (doc bab 6.2 idempotency).
func (db *DB) ImportExport(
	ctx context.Context,
	c *crypto.Cipher,
	userID string,
	chatKey string,
	displayName string,
	selfName string,
	res *waimport.Result,
	gap time.Duration,
) (*ImportResult, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	nameCipher, err := c.EncryptString(displayName)
	if err != nil {
		return nil, fmt.Errorf("encrypt display name: %w", err)
	}

	// alias is deliberately left unset. Doc bab 6.2 says the display name is
	// stored encrypted OR replaced by an alias; writing the real name into a
	// plaintext alias column as well would defeat the encryption. The user
	// chooses an alias explicitly if they want one.
	var conversationID string
	err = tx.QueryRow(ctx, `
		INSERT INTO conversations (user_id, chat_id_hash, display_name_cipher, is_selected)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (user_id, chat_id_hash) DO UPDATE
		  SET display_name_cipher = EXCLUDED.display_name_cipher,
		      updated_at = now()
		RETURNING id`,
		userID, c.Hash(chatKey), nameCipher,
	).Scan(&conversationID)
	if err != nil {
		return nil, fmt.Errorf("upsert conversation: %w", err)
	}

	out := &ImportResult{ConversationID: conversationID}

	for _, m := range res.Messages {
		// System notices are context, not participant messages; they carry no
		// emotion and would distort per-sender statistics.
		if m.IsSystem {
			out.Skipped++
			continue
		}

		textCipher, err := c.EncryptString(m.Text)
		if err != nil {
			return nil, fmt.Errorf("encrypt text: %w", err)
		}

		role := "OTHER"
		if selfName != "" && m.Sender == selfName {
			role = "SELF"
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO messages (
			  conversation_id, wa_message_id, sender_role, timestamp,
			  message_type, text_cipher, is_deleted, edited_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (conversation_id, wa_message_id) DO NOTHING`,
			conversationID,
			syntheticID(m),
			role,
			m.Timestamp,
			string(m.Type),
			textCipher,
			m.IsDeleted,
			editedAt(m),
		)
		if err != nil {
			return nil, fmt.Errorf("insert message line %d: %w", m.LineNumber, err)
		}

		if tag.RowsAffected() == 0 {
			out.Skipped++
			continue
		}
		out.Inserted++
	}

	sessions := waimport.Sessionize(res.Messages, gap)
	if err := insertSessions(ctx, tx, conversationID, sessions); err != nil {
		return nil, err
	}
	out.Sessions = len(sessions)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}

func insertSessions(ctx context.Context, tx pgx.Tx, conversationID string, sessions []waimport.Session) error {
	// Sessions are derived data: recompute cleanly instead of merging, so a
	// re-import with a different gap setting cannot leave stale boundaries.
	if _, err := tx.Exec(ctx,
		`DELETE FROM conversation_sessions WHERE conversation_id = $1`,
		conversationID,
	); err != nil {
		return fmt.Errorf("clear sessions: %w", err)
	}

	for _, s := range sessions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_sessions (conversation_id, start_at, end_at, message_count)
			VALUES ($1, $2, $3, $4)`,
			conversationID, s.StartAt, s.EndAt, len(s.Indexes),
		); err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
	}
	return nil
}

// syntheticID derives a stable identifier from the fields that make a message
// unique in an export. Two genuinely identical messages at the same second from
// the same sender collapse into one, which is the safer failure mode than
// duplicating on every re-import.
func syntheticID(m waimport.Message) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%s|%s",
		m.Timestamp.Unix(), m.Sender, m.Type, m.Text)
	return "import:" + hex.EncodeToString(h.Sum(nil))[:32]
}

func editedAt(m waimport.Message) *time.Time {
	if !m.IsEdited {
		return nil
	}
	// The export does not record when the edit happened, only that it did.
	t := m.Timestamp
	return &t
}
