package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"ditalk/backend/internal/crypto"
)

// These tests need a real PostgreSQL: the logic under test is largely SQL
// (idempotency via ON CONFLICT, session windowing, jsonb reaction replacement),
// so a fake would verify nothing that matters.
//
// Set DITALK_TEST_DSN to run them; they are skipped otherwise.
func testDB(t *testing.T) (*DB, *crypto.Cipher, string) {
	t.Helper()

	dsn := os.Getenv("DITALK_TEST_DSN")
	if dsn == "" {
		t.Skip("DITALK_TEST_DSN tidak diset; melewati test integrasi")
	}

	ctx := context.Background()
	db, err := NewDB(ctx, dsn)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(db.Close)

	c, err := crypto.New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)))
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}

	userID, err := db.EnsureLocalUser(ctx, c)
	if err != nil {
		t.Fatalf("EnsureLocalUser: %v", err)
	}

	// Each test starts from a clean slate for this user's conversations.
	if _, err := db.Pool.Exec(ctx, `DELETE FROM conversations WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(),
			`DELETE FROM conversations WHERE user_id = $1`, userID)
	})

	return db, c, userID
}

const testJID = "6281234567890@s.whatsapp.net"

func msg(id string, ts time.Time, role, text string) LiveMessage {
	return LiveMessage{
		MessageID:      id,
		ConversationID: testJID,
		SenderRole:     role,
		Timestamp:      ts,
		MessageType:    "text",
		Text:           text,
	}
}

func at(day, hour, minute int) time.Time {
	return time.Date(2026, 7, day, hour, minute, 0, 0, time.Local)
}

func TestSaveLiveMessageStoresEncrypted(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	got, err := db.SaveLiveMessage(ctx, c, userID, msg("M1", at(21, 10, 0), "OTHER", "halo apa kabar"))
	if err != nil {
		t.Fatalf("SaveLiveMessage: %v", err)
	}
	if got != SaveStored {
		t.Errorf("outcome = %q, want stored", got)
	}

	var cipherText []byte
	var role string
	err = db.Pool.QueryRow(ctx, `
		SELECT m.text_cipher, m.sender_role FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.wa_message_id = 'M1'`, userID,
	).Scan(&cipherText, &role)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	if role != "OTHER" {
		t.Errorf("sender_role = %q", role)
	}
	if bytes.Contains(cipherText, []byte("halo")) {
		t.Error("teks tersimpan sebagai plaintext")
	}
	plain, err := c.DecryptString(cipherText)
	if err != nil || plain != "halo apa kabar" {
		t.Errorf("decrypt = %q, %v", plain, err)
	}
}

func TestSaveLiveMessageIsIdempotent(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	m := msg("M1", at(21, 10, 0), "OTHER", "sama")
	if _, err := db.SaveLiveMessage(ctx, c, userID, m); err != nil {
		t.Fatalf("first: %v", err)
	}

	// WhatsApp redelivers on reconnect; a repeat must not duplicate.
	got, err := db.SaveLiveMessage(ctx, c, userID, m)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if got != SaveDuplicate {
		t.Errorf("outcome = %q, want duplicate", got)
	}

	var n int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*) FROM messages m JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("jumlah pesan = %d, want 1", n)
	}
}

func TestViewOnceIsNotStored(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	m := msg("M1", at(21, 10, 0), "OTHER", "rahasia sekali")
	m.IsViewOnce = true

	got, err := db.SaveLiveMessage(ctx, c, userID, m)
	if err != nil {
		t.Fatalf("SaveLiveMessage: %v", err)
	}
	if got != SaveSkipped {
		t.Errorf("outcome = %q, want skipped_view_once", got)
	}

	var n int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*) FROM messages m JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("view-once tersimpan (%d baris); harus dilewati", n)
	}
}

func TestInvalidSenderRoleRejected(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	m := msg("M1", at(21, 10, 0), "SIAPA_TAHU", "halo")
	if _, err := db.SaveLiveMessage(ctx, c, userID, m); err == nil {
		t.Error("sender_role tidak valid diterima; statistik per pihak akan rusak")
	}
}

func TestSessionsExtendAndSplit(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	// Two messages 10 minutes apart share a session; the third is 45 minutes
	// later and starts a new one; the fourth is the next day.
	for i, m := range []LiveMessage{
		msg("A", at(21, 10, 0), "OTHER", "satu"),
		msg("B", at(21, 10, 10), "SELF", "dua"),
		msg("C", at(21, 10, 55), "OTHER", "tiga"),
		msg("D", at(22, 9, 0), "SELF", "empat"),
	} {
		if _, err := db.SaveLiveMessage(ctx, c, userID, m); err != nil {
			t.Fatalf("pesan %d: %v", i, err)
		}
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT s.start_at, s.end_at, s.message_count
		FROM conversation_sessions s JOIN conversations c ON c.id = s.conversation_id
		WHERE c.user_id = $1 ORDER BY s.start_at`, userID)
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	defer rows.Close()

	type session struct {
		start, end time.Time
		count      int
	}
	var got []session
	for rows.Next() {
		var s session
		if err := rows.Scan(&s.start, &s.end, &s.count); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, s)
	}

	if len(got) != 3 {
		t.Fatalf("jumlah sesi = %d, want 3", len(got))
	}
	if got[0].count != 2 || !got[0].start.Equal(at(21, 10, 0)) || !got[0].end.Equal(at(21, 10, 10)) {
		t.Errorf("sesi 1 = %+v", got[0])
	}
	if got[1].count != 1 {
		t.Errorf("sesi 2 count = %d, want 1", got[1].count)
	}
	if got[2].count != 1 || got[2].start.Day() != 22 {
		t.Errorf("sesi 3 = %+v, want hari ke-22", got[2])
	}
}

func TestOutOfOrderMessageJoinsExistingSession(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	// History sync delivers out of order: the later message arrives first.
	if _, err := db.SaveLiveMessage(ctx, c, userID, msg("B", at(21, 10, 20), "SELF", "kedua")); err != nil {
		t.Fatalf("B: %v", err)
	}
	if _, err := db.SaveLiveMessage(ctx, c, userID, msg("A", at(21, 10, 0), "OTHER", "pertama")); err != nil {
		t.Fatalf("A: %v", err)
	}

	var n, count int
	var start time.Time
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*), min(s.start_at), sum(s.message_count)
		FROM conversation_sessions s JOIN conversations c ON c.id = s.conversation_id
		WHERE c.user_id = $1`, userID).Scan(&n, &start, &count); err != nil {
		t.Fatalf("query: %v", err)
	}

	if n != 1 {
		t.Errorf("jumlah sesi = %d, want 1 (pesan lebih lama harus bergabung)", n)
	}
	if !start.Equal(at(21, 10, 0)) {
		t.Errorf("start_at = %v, want mundur ke 10:00", start)
	}
	if count != 2 {
		t.Errorf("message_count = %d, want 2", count)
	}
}

func TestRecomputeSessionsMatchesIncremental(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	for _, m := range []LiveMessage{
		msg("A", at(21, 10, 0), "OTHER", "satu"),
		msg("B", at(21, 10, 10), "SELF", "dua"),
		msg("C", at(21, 10, 55), "OTHER", "tiga"),
	} {
		if _, err := db.SaveLiveMessage(ctx, c, userID, m); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	convID, err := db.ConversationIDForJID(ctx, c, userID, testJID)
	if err != nil {
		t.Fatalf("ConversationIDForJID: %v", err)
	}

	n, err := db.RecomputeSessions(ctx, convID, SessionGap)
	if err != nil {
		t.Fatalf("RecomputeSessions: %v", err)
	}
	if n != 2 {
		t.Errorf("recompute = %d sesi, want 2 (sama dengan hasil inkremental)", n)
	}
}

func TestMarkDeletedClearsText(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	if _, err := db.SaveLiveMessage(ctx, c, userID, msg("M1", at(21, 10, 0), "OTHER", "akan dihapus")); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := db.MarkMessageDeleted(ctx, c, userID, testJID, "M1")
	if err != nil || !found {
		t.Fatalf("MarkMessageDeleted = %v, %v", found, err)
	}

	var isDeleted bool
	var cipherText []byte
	if err := db.Pool.QueryRow(ctx, `
		SELECT m.is_deleted, m.text_cipher FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.wa_message_id = 'M1'`, userID,
	).Scan(&isDeleted, &cipherText); err != nil {
		t.Fatalf("read back: %v", err)
	}

	if !isDeleted {
		t.Error("is_deleted tidak diset")
	}
	// The sender revoked it, so the text must not remain readable.
	if cipherText != nil {
		t.Error("teks masih tersimpan setelah dihapus")
	}
}

func TestMarkEditedStoresNewText(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	if _, err := db.SaveLiveMessage(ctx, c, userID, msg("M1", at(21, 10, 0), "SELF", "jam 7 ya")); err != nil {
		t.Fatalf("save: %v", err)
	}

	found, err := db.MarkMessageEdited(ctx, c, userID, testJID, "M1", "jam 8 ya")
	if err != nil || !found {
		t.Fatalf("MarkMessageEdited = %v, %v", found, err)
	}

	var cipherText []byte
	var editedAt *time.Time
	if err := db.Pool.QueryRow(ctx, `
		SELECT m.text_cipher, m.edited_at FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.wa_message_id = 'M1'`, userID,
	).Scan(&cipherText, &editedAt); err != nil {
		t.Fatalf("read back: %v", err)
	}

	plain, err := c.DecryptString(cipherText)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plain != "jam 8 ya" {
		t.Errorf("teks = %q, want teks hasil edit", plain)
	}
	if editedAt == nil {
		t.Error("edited_at tidak diset")
	}
}

func TestEditWithoutNewTextKeepsOriginal(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	if _, err := db.SaveLiveMessage(ctx, c, userID, msg("M1", at(21, 10, 0), "SELF", "asli")); err != nil {
		t.Fatalf("save: %v", err)
	}

	// An update event without message content must not blank the text.
	if _, err := db.MarkMessageEdited(ctx, c, userID, testJID, "M1", ""); err != nil {
		t.Fatalf("MarkMessageEdited: %v", err)
	}

	var cipherText []byte
	if err := db.Pool.QueryRow(ctx, `
		SELECT m.text_cipher FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.wa_message_id = 'M1'`, userID).Scan(&cipherText); err != nil {
		t.Fatalf("read back: %v", err)
	}

	plain, err := c.DecryptString(cipherText)
	if err != nil || plain != "asli" {
		t.Errorf("teks = %q (%v), want tetap 'asli'", plain, err)
	}
}

func TestReactionReplacesPerSender(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	if _, err := db.SaveLiveMessage(ctx, c, userID, msg("M1", at(21, 10, 0), "OTHER", "halo")); err != nil {
		t.Fatalf("save: %v", err)
	}

	readReactions := func() string {
		t.Helper()
		var raw string
		if err := db.Pool.QueryRow(ctx, `
			SELECT m.reactions::text FROM messages m
			JOIN conversations c ON c.id = m.conversation_id
			WHERE c.user_id = $1 AND m.wa_message_id = 'M1'`, userID).Scan(&raw); err != nil {
			t.Fatalf("read reactions: %v", err)
		}
		return raw
	}

	if _, err := db.ApplyReaction(ctx, c, userID, testJID, "M1", "👍", "SELF"); err != nil {
		t.Fatalf("react: %v", err)
	}
	if got := readReactions(); got == "[]" {
		t.Fatal("reaksi tidak tersimpan")
	}

	// Changing your own reaction replaces it rather than adding a second entry.
	if _, err := db.ApplyReaction(ctx, c, userID, testJID, "M1", "❤️", "SELF"); err != nil {
		t.Fatalf("react again: %v", err)
	}
	var n int
	if err := db.Pool.QueryRow(ctx, `
		SELECT jsonb_array_length(m.reactions) FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.wa_message_id = 'M1'`, userID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("jumlah reaksi = %d, want 1 (satu slot per orang)", n)
	}

	// The other party reacting adds a separate entry.
	if _, err := db.ApplyReaction(ctx, c, userID, testJID, "M1", "😂", "OTHER"); err != nil {
		t.Fatalf("other react: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT jsonb_array_length(m.reactions) FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.wa_message_id = 'M1'`, userID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("jumlah reaksi = %d, want 2", n)
	}

	// Removing a reaction sends an empty emoji.
	if _, err := db.ApplyReaction(ctx, c, userID, testJID, "M1", "", "SELF"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT jsonb_array_length(m.reactions) FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.wa_message_id = 'M1'`, userID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("setelah dihapus jumlah reaksi = %d, want 1", n)
	}
}

func TestGroupJIDRejected(t *testing.T) {
	db, c, userID := testDB(t)
	ctx := context.Background()

	m := msg("M1", at(21, 10, 0), "OTHER", "halo")
	m.ConversationID = "6281234567890-1600000000@g.us"

	if _, err := db.SaveLiveMessage(ctx, c, userID, m); err == nil {
		t.Error("JID grup diterima; grup tidak boleh disimpan")
	}
}
