package storage

import (
	"context"
	"fmt"
	"time"
)

// BackfillCursor points at the oldest message stored for a conversation.
//
// WhatsApp's on-demand history request needs a real message to anchor on: it
// returns messages older than the one given. Walking this cursor backwards is
// how a chat gets filled in from the newest message down to the first one.
type BackfillCursor struct {
	ConversationID string
	Phone          string
	MessageID      string
	FromMe         bool
	Timestamp      time.Time
	StoredCount    int
}

// BackfillCursors returns one cursor per active allowlisted conversation that
// already has at least one message.
//
// A conversation with nothing stored is skipped: there is no anchor to request
// history from, so it has to wait for a live message or a history sync first.
func (db *DB) BackfillCursors(ctx context.Context, userID string) ([]BackfillCursor, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT ON (c.id)
		       c.id, a.phone, m.wa_message_id, m.sender_role = 'SELF', m.timestamp,
		       count(*) OVER (PARTITION BY c.id)
		FROM conversations c
		JOIN allowed_contacts a
		  ON a.user_id = c.user_id AND c.chat_id_hash = a.phone_hash
		JOIN messages m ON m.conversation_id = c.id
		WHERE c.user_id = $1 AND a.is_active
		ORDER BY c.id, m.timestamp ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("backfill cursors: %w", err)
	}
	defer rows.Close()

	out := []BackfillCursor{}
	for rows.Next() {
		var c BackfillCursor
		if err := rows.Scan(&c.ConversationID, &c.Phone, &c.MessageID,
			&c.FromMe, &c.Timestamp, &c.StoredCount); err != nil {
			return nil, fmt.Errorf("scan cursor: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
