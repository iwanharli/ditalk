package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ditalk/backend/internal/crypto"
	"ditalk/backend/internal/waid"
)

// SessionGap mirrors waimport.DefaultGap: an idle stretch this long ends a
// session (doc bab 6.3).
const SessionGap = 30 * time.Minute

// LiveMessage is the canonical message the connector produces (doc bab 6.1).
type LiveMessage struct {
	MessageID       string
	ConversationID  string // WhatsApp JID
	SenderRole      string // SELF or OTHER
	Timestamp       time.Time
	MessageType     string
	Text            string
	QuotedMessageID string
	IsViewOnce      bool
	IsEphemeral     bool
}

type SaveOutcome string

const (
	SaveStored    SaveOutcome = "stored"
	SaveDuplicate SaveOutcome = "duplicate"
	SaveSkipped   SaveOutcome = "skipped_view_once"
)

// SaveLiveMessage stores one message from the live connector.
//
// Idempotent on (conversation_id, wa_message_id): WhatsApp redelivers messages on
// reconnect and history sync, so a repeat must not create a second row.
func (db *DB) SaveLiveMessage(
	ctx context.Context, c *crypto.Cipher, userID string, m LiveMessage,
) (SaveOutcome, error) {
	// The sender chose a message that disappears. Persisting it would override
	// that choice, so it is counted and dropped (doc Ringkasan Eksekutif).
	if m.IsViewOnce {
		return SaveSkipped, nil
	}

	if m.MessageID == "" || m.ConversationID == "" {
		return "", errors.New("message_id dan conversation_id wajib ada")
	}

	phone, ok := waid.PhoneFromJID(m.ConversationID)
	if !ok {
		return "", fmt.Errorf("jid tidak didukung")
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	conversationID, err := upsertConversation(ctx, tx, c, userID, phone)
	if err != nil {
		return "", err
	}

	textCipher, err := c.EncryptString(m.Text)
	if err != nil {
		return "", fmt.Errorf("encrypt text: %w", err)
	}

	role := m.SenderRole
	if role != "SELF" && role != "OTHER" {
		// Baileys reports fromMe; anything else means the connector sent us a
		// value we do not understand, and guessing would corrupt per-side stats.
		return "", fmt.Errorf("sender_role tidak valid: %q", m.SenderRole)
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO messages (
		  conversation_id, wa_message_id, sender_role, timestamp,
		  message_type, text_cipher, quoted_message_id, is_view_once, is_ephemeral
		) VALUES ($1, $2, $3, $4, $5, $6, nullif($7,''), false, $8)
		ON CONFLICT (conversation_id, wa_message_id) DO NOTHING`,
		conversationID, m.MessageID, role, m.Timestamp,
		normalizeType(m.MessageType), textCipher, m.QuotedMessageID, m.IsEphemeral,
	)
	if err != nil {
		return "", fmt.Errorf("insert message: %w", err)
	}

	if tag.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit: %w", err)
		}
		return SaveDuplicate, nil
	}

	if err := attachToSession(ctx, tx, conversationID, m.Timestamp); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return SaveStored, nil
}

// upsertConversation keys on the hashed phone number rather than an arbitrary
// label, so live sync and an Export Chat import of the same person converge on
// one conversation row.
func upsertConversation(
	ctx context.Context, tx pgx.Tx, c *crypto.Cipher, userID, phone string,
) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO conversations (user_id, chat_id_hash, is_selected)
		VALUES ($1, $2, true)
		ON CONFLICT (user_id, chat_id_hash) DO UPDATE SET updated_at = now()
		RETURNING id`,
		userID, c.Hash(phone),
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert conversation: %w", err)
	}
	return id, nil
}

// attachToSession extends the session this message belongs to, or opens a new one.
//
// It matches on a window around each existing session rather than only the latest,
// because history sync delivers messages out of order. Two sessions that become
// adjacent after an extension are not merged here; the periodic recompute in
// RecomputeSessions handles that.
func attachToSession(ctx context.Context, tx pgx.Tx, conversationID string, ts time.Time) error {
	var sessionID string
	err := tx.QueryRow(ctx, `
		SELECT id FROM conversation_sessions
		WHERE conversation_id = $1
		  AND $2::timestamptz BETWEEN start_at - $3::interval AND end_at + $3::interval
		  AND date_trunc('day', start_at) = date_trunc('day', $2::timestamptz)
		ORDER BY start_at DESC
		LIMIT 1`,
		conversationID, ts, SessionGap,
	).Scan(&sessionID)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		_, err = tx.Exec(ctx, `
			INSERT INTO conversation_sessions (conversation_id, start_at, end_at, message_count)
			VALUES ($1, $2, $2, 1)`,
			conversationID, ts,
		)
		if err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
		return nil

	case err != nil:
		return fmt.Errorf("find session: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE conversation_sessions
		SET start_at = least(start_at, $2),
		    end_at = greatest(end_at, $2),
		    message_count = message_count + 1
		WHERE id = $1`,
		sessionID, ts,
	)
	if err != nil {
		return fmt.Errorf("extend session: %w", err)
	}
	return nil
}

// RecomputeSessions rebuilds every session for a conversation from message
// timestamps alone.
//
// Sessions are derived data, so rebuilding is safe and does not require
// decrypting anything. Run it after a history sync burst, where out-of-order
// delivery can leave sessions that should have been merged.
func (db *DB) RecomputeSessions(ctx context.Context, conversationID string, gap time.Duration) (int, error) {
	if gap <= 0 {
		gap = SessionGap
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT timestamp FROM messages
		WHERE conversation_id = $1 AND NOT is_deleted
		ORDER BY timestamp`,
		conversationID,
	)
	if err != nil {
		return 0, fmt.Errorf("read timestamps: %w", err)
	}

	var stamps []time.Time
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan timestamp: %w", err)
		}
		stamps = append(stamps, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate timestamps: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM conversation_sessions WHERE conversation_id = $1`, conversationID,
	); err != nil {
		return 0, fmt.Errorf("clear sessions: %w", err)
	}

	count := 0
	for i := 0; i < len(stamps); {
		start := stamps[i]
		end := start
		n := 1
		j := i + 1
		for ; j < len(stamps); j++ {
			sameDay := stamps[j].YearDay() == end.YearDay() && stamps[j].Year() == end.Year()
			if !sameDay || stamps[j].Sub(end) > gap {
				break
			}
			end = stamps[j]
			n++
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO conversation_sessions (conversation_id, start_at, end_at, message_count)
			VALUES ($1, $2, $3, $4)`,
			conversationID, start, end, n,
		); err != nil {
			return 0, fmt.Errorf("insert session: %w", err)
		}
		count++
		i = j
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return count, nil
}

// MarkMessageDeleted records that a message was deleted for everyone. The row is
// kept with the flag set rather than removed, so analysis that already cited it
// keeps a target to point at (doc bab 5.2).
func (db *DB) MarkMessageDeleted(ctx context.Context, c *crypto.Cipher, userID, jid, messageID string) (bool, error) {
	phone, ok := waid.PhoneFromJID(jid)
	if !ok {
		return false, fmt.Errorf("jid tidak didukung")
	}

	tag, err := db.Pool.Exec(ctx, `
		UPDATE messages m
		SET is_deleted = true, text_cipher = NULL, caption_cipher = NULL
		FROM conversations c
		WHERE m.conversation_id = c.id
		  AND c.user_id = $1 AND c.chat_id_hash = $2
		  AND m.wa_message_id = $3`,
		userID, c.Hash(phone), messageID,
	)
	if err != nil {
		return false, fmt.Errorf("mark deleted: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// MarkMessageEdited stores the new text and records when the edit was seen.
func (db *DB) MarkMessageEdited(
	ctx context.Context, c *crypto.Cipher, userID, jid, messageID, newText string,
) (bool, error) {
	phone, ok := waid.PhoneFromJID(jid)
	if !ok {
		return false, fmt.Errorf("jid tidak didukung")
	}

	cipherText, err := c.EncryptString(newText)
	if err != nil {
		return false, fmt.Errorf("encrypt text: %w", err)
	}

	tag, err := db.Pool.Exec(ctx, `
		UPDATE messages m
		SET text_cipher = coalesce($4, m.text_cipher), edited_at = now()
		FROM conversations c
		WHERE m.conversation_id = c.id
		  AND c.user_id = $1 AND c.chat_id_hash = $2
		  AND m.wa_message_id = $3`,
		userID, c.Hash(phone), messageID, cipherText,
	)
	if err != nil {
		return false, fmt.Errorf("mark edited: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ApplyReaction replaces the reaction from one sender, since WhatsApp treats a
// reaction as a single mutable slot per person rather than an append-only list.
// An empty emoji means the reaction was removed.
func (db *DB) ApplyReaction(
	ctx context.Context, c *crypto.Cipher, userID, jid, messageID, emoji, fromRole string,
) (bool, error) {
	phone, ok := waid.PhoneFromJID(jid)
	if !ok {
		return false, fmt.Errorf("jid tidak didukung")
	}

	tag, err := db.Pool.Exec(ctx, `
		UPDATE messages m
		SET reactions = (
		      SELECT coalesce(jsonb_agg(r), '[]'::jsonb)
		      FROM jsonb_array_elements(m.reactions) r
		      WHERE r->>'from' IS DISTINCT FROM $4
		    ) || CASE WHEN $5 = '' THEN '[]'::jsonb
		              ELSE jsonb_build_array(jsonb_build_object('from', $4, 'emoji', $5))
		         END
		FROM conversations c
		WHERE m.conversation_id = c.id
		  AND c.user_id = $1 AND c.chat_id_hash = $2
		  AND m.wa_message_id = $3`,
		userID, c.Hash(phone), messageID, fromRole, emoji,
	)
	if err != nil {
		return false, fmt.Errorf("apply reaction: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ConversationIDForJID resolves the stored conversation for a JID.
func (db *DB) ConversationIDForJID(ctx context.Context, c *crypto.Cipher, userID, jid string) (string, error) {
	phone, ok := waid.PhoneFromJID(jid)
	if !ok {
		return "", fmt.Errorf("jid tidak didukung")
	}

	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM conversations WHERE user_id = $1 AND chat_id_hash = $2`,
		userID, c.Hash(phone),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find conversation: %w", err)
	}
	return id, nil
}

// normalizeType keeps unknown types from violating the message_type constraint.
func normalizeType(t string) string {
	switch t {
	case "text", "image", "audio", "video", "sticker", "document":
		return t
	default:
		return "unknown"
	}
}
