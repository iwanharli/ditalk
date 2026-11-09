package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ditalk/backend/internal/crypto"
)

type ConversationSummary struct {
	ID              string     `json:"id"`
	DisplayName     string     `json:"display_name"`
	Phone           string     `json:"phone,omitempty"`
	LastMessageText string     `json:"last_message_text,omitempty"`
	LastMessageAt   *time.Time `json:"last_message_at,omitempty"`
	LastSenderRole  string     `json:"last_sender_role,omitempty"`
	LastMessageType string     `json:"last_message_type,omitempty"`
	MessageCount    int        `json:"message_count"`
}

type ConversationMessage struct {
	ID              string          `json:"id"`
	WAMessageID     string          `json:"wa_message_id"`
	SenderRole      string          `json:"sender_role"`
	Timestamp       time.Time       `json:"timestamp"`
	MessageType     string          `json:"message_type"`
	Text            string          `json:"text,omitempty"`
	QuotedMessageID string          `json:"quoted_message_id,omitempty"`
	Reactions       json.RawMessage `json:"reactions"`
	IsViewOnce      bool            `json:"is_view_once"`
	IsEphemeral     bool            `json:"is_ephemeral"`
	IsDeleted       bool            `json:"is_deleted"`
	EditedAt        *time.Time      `json:"edited_at,omitempty"`
}

// ListConversations returns selected conversations with enough metadata for the
// chat list. Text and names are decrypted here; ciphertext never leaves Go.
func (db *DB) ListConversations(
	ctx context.Context, c *crypto.Cipher, userID string,
) ([]ConversationSummary, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT c.id::text,
		       c.display_name_cipher,
		       coalesce(c.alias, ''),
		       coalesce(a.phone, ''),
		       coalesce(a.label, ''),
		       lm.text_cipher,
		       coalesce(lm.sender_role, ''),
		       lm.timestamp,
		       coalesce(lm.message_type, ''),
		       coalesce(lm.is_deleted, false),
		       coalesce(mc.message_count, 0)::int
		FROM conversations c
		LEFT JOIN allowed_contacts a
		  ON a.user_id = c.user_id AND a.phone_hash = c.chat_id_hash
		LEFT JOIN LATERAL (
			SELECT text_cipher, sender_role, timestamp, message_type, is_deleted
			FROM messages
			WHERE conversation_id = c.id
			ORDER BY timestamp DESC
			LIMIT 1
		) lm ON true
		LEFT JOIN LATERAL (
			SELECT count(*) AS message_count
			FROM messages
			WHERE conversation_id = c.id
		) mc ON true
		WHERE c.user_id = $1 AND c.is_selected
		ORDER BY lm.timestamp DESC NULLS LAST, c.updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	out := []ConversationSummary{}
	for rows.Next() {
		var row ConversationSummary
		var displayCipher []byte
		var alias, label string
		var lastTextCipher []byte
		var lastAt sql.NullTime
		var isDeleted bool

		if err := rows.Scan(
			&row.ID, &displayCipher, &alias, &row.Phone, &label,
			&lastTextCipher, &row.LastSenderRole, &lastAt,
			&row.LastMessageType, &isDeleted, &row.MessageCount,
		); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}

		displayName, err := c.DecryptString(displayCipher)
		if err != nil {
			return nil, fmt.Errorf("decrypt display name: %w", err)
		}
		row.DisplayName = firstNonEmpty(label, alias, displayName, row.Phone, "Percakapan")

		if lastAt.Valid {
			t := lastAt.Time
			row.LastMessageAt = &t
		}
		row.LastMessageText, err = previewText(c, lastTextCipher, row.LastMessageType, isDeleted)
		if err != nil {
			return nil, err
		}

		out = append(out, row)
	}
	return out, rows.Err()
}

// ConversationMessages decrypts messages in timestamp order for one selected
// conversation owned by the user.
func (db *DB) ConversationMessages(
	ctx context.Context, c *crypto.Cipher, userID, conversationID string,
) ([]ConversationMessage, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT m.id::text, m.wa_message_id, m.sender_role, m.timestamp,
		       m.message_type, m.text_cipher, coalesce(m.quoted_message_id, ''),
		       m.reactions, m.is_view_once, m.is_ephemeral, m.is_deleted, m.edited_at
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND c.id = $2
		ORDER BY m.timestamp ASC`,
		userID, conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("read messages: %w", err)
	}
	defer rows.Close()

	out := []ConversationMessage{}
	for rows.Next() {
		var m ConversationMessage
		var textCipher []byte
		var reactions []byte
		var editedAt sql.NullTime
		if err := rows.Scan(
			&m.ID, &m.WAMessageID, &m.SenderRole, &m.Timestamp,
			&m.MessageType, &textCipher, &m.QuotedMessageID,
			&reactions, &m.IsViewOnce, &m.IsEphemeral, &m.IsDeleted, &editedAt,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}

		text, err := c.DecryptString(textCipher)
		if err != nil {
			return nil, fmt.Errorf("decrypt message: %w", err)
		}
		m.Text = text
		if len(reactions) == 0 {
			reactions = []byte("[]")
		}
		m.Reactions = json.RawMessage(reactions)
		if editedAt.Valid {
			t := editedAt.Time
			m.EditedAt = &t
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddLocalMessage stores a user-composed message in an existing conversation.
//
// This is deliberately local-only for now: it makes the chat surface writable
// without claiming a WhatsApp send happened. A later connector command can take
// over real delivery and reconcile the WhatsApp message id.
func (db *DB) AddLocalMessage(
	ctx context.Context, c *crypto.Cipher, userID, conversationID, text string,
) (*ConversationMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("message text required")
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM conversations
			WHERE user_id = $1 AND id = $2 AND is_selected
		)`,
		userID, conversationID,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check conversation: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	textCipher, err := c.EncryptString(text)
	if err != nil {
		return nil, fmt.Errorf("encrypt text: %w", err)
	}

	now := time.Now()
	waMessageID, err := localMessageID()
	if err != nil {
		return nil, err
	}

	var out ConversationMessage
	var reactions []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (
		  conversation_id, wa_message_id, sender_role, timestamp,
		  message_type, text_cipher
		) VALUES ($1, $2, 'SELF', $3, 'text', $4)
		RETURNING id::text, wa_message_id, sender_role, timestamp,
		          message_type, reactions, is_view_once, is_ephemeral, is_deleted`,
		conversationID, waMessageID, now, textCipher,
	).Scan(
		&out.ID, &out.WAMessageID, &out.SenderRole, &out.Timestamp,
		&out.MessageType, &reactions, &out.IsViewOnce, &out.IsEphemeral, &out.IsDeleted,
	)
	if errorsIsUnique(err) {
		return nil, fmt.Errorf("message id collision")
	}
	if err != nil {
		return nil, fmt.Errorf("insert local message: %w", err)
	}

	if err := attachToSession(ctx, tx, conversationID, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE conversations SET updated_at = now() WHERE id = $1`, conversationID,
	); err != nil {
		return nil, fmt.Errorf("touch conversation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	out.Text = text
	out.Reactions = json.RawMessage(reactions)
	if len(out.Reactions) == 0 {
		out.Reactions = json.RawMessage("[]")
	}
	return &out, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func previewText(c *crypto.Cipher, cipherText []byte, messageType string, deleted bool) (string, error) {
	if deleted {
		return "Pesan dihapus", nil
	}
	text, err := c.DecryptString(cipherText)
	if err != nil {
		return "", fmt.Errorf("decrypt preview: %w", err)
	}
	if strings.TrimSpace(text) != "" {
		return text, nil
	}

	switch messageType {
	case "image":
		return "Foto", nil
	case "audio":
		return "Voice message", nil
	case "video":
		return "Video", nil
	case "sticker":
		return "Sticker", nil
	case "document":
		return "Dokumen", nil
	default:
		return "", nil
	}
}

func localMessageID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read local message id: %w", err)
	}
	return "local:" + hex.EncodeToString(b[:]), nil
}

func errorsIsUnique(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate key value")
}
