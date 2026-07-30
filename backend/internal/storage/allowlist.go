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

var ErrNotFound = errors.New("not found")

type AllowedContact struct {
	ID          string    `json:"id"`
	Phone       string    `json:"phone"`
	Label       string    `json:"label,omitempty"`
	IsActive    bool      `json:"is_active"`
	ConsentNote string    `json:"consent_note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (db *DB) ListAllowedContacts(ctx context.Context, userID string) ([]AllowedContact, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, phone, coalesce(label,''), is_active, coalesce(consent_note,''), created_at
		FROM allowed_contacts
		WHERE user_id = $1
		ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list allowlist: %w", err)
	}
	defer rows.Close()

	out := []AllowedContact{}
	for rows.Next() {
		var c AllowedContact
		if err := rows.Scan(&c.ID, &c.Phone, &c.Label, &c.IsActive, &c.ConsentNote, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan allowlist: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddAllowedContact registers a number. The phone is normalized first so the
// same contact cannot be added twice in different formats.
func (db *DB) AddAllowedContact(
	ctx context.Context, c *crypto.Cipher, userID, rawPhone, label, consentNote string,
) (*AllowedContact, error) {
	phone, err := waid.NormalizePhone(rawPhone)
	if err != nil {
		return nil, err
	}

	var out AllowedContact
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO allowed_contacts (user_id, phone, phone_hash, label, consent_note)
		VALUES ($1, $2, $3, nullif($4,''), nullif($5,''))
		ON CONFLICT (user_id, phone) DO UPDATE
		  SET label = coalesce(nullif($4,''), allowed_contacts.label),
		      consent_note = coalesce(nullif($5,''), allowed_contacts.consent_note),
		      is_active = true,
		      updated_at = now()
		RETURNING id, phone, coalesce(label,''), is_active, coalesce(consent_note,''), created_at`,
		userID, phone, c.Hash(phone), label, consentNote,
	).Scan(&out.ID, &out.Phone, &out.Label, &out.IsActive, &out.ConsentNote, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add allowlist: %w", err)
	}
	return &out, nil
}

// SetAllowedContactActive pauses or resumes reading a contact without losing the
// entry, so the user can stop ingestion without deleting their configuration.
func (db *DB) SetAllowedContactActive(ctx context.Context, userID, id string, active bool) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE allowed_contacts SET is_active = $3, updated_at = now()
		WHERE user_id = $1 AND id = $2`,
		userID, id, active,
	)
	if err != nil {
		return fmt.Errorf("update allowlist: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAllowedContact removes the entry. Messages already stored are untouched;
// removing consent to read future messages is a separate action from deleting
// history, which the user does from the privacy center (doc bab 19.1).
func (db *DB) DeleteAllowedContact(ctx context.Context, userID, id string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM allowed_contacts WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
	if err != nil {
		return fmt.Errorf("delete allowlist: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IsAllowed reports whether a JID may be read. It fails closed: any JID that is
// not an active, allowlisted one-to-one contact is rejected.
func (db *DB) IsAllowed(ctx context.Context, c *crypto.Cipher, userID, jid string) (bool, string) {
	if !waid.IsSupported(jid) {
		if waid.IsGroup(jid) {
			return false, "group_chat"
		}
		return false, "unsupported_jid"
	}

	phone, ok := waid.PhoneFromJID(jid)
	if !ok {
		return false, "unsupported_jid"
	}

	var isActive bool
	err := db.Pool.QueryRow(ctx,
		`SELECT is_active FROM allowed_contacts WHERE user_id = $1 AND phone_hash = $2`,
		userID, c.Hash(phone),
	).Scan(&isActive)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, "not_allowlisted"
	case err != nil:
		// A lookup failure must not become an accidental allow.
		return false, "not_allowlisted"
	case !isActive:
		return false, "inactive_contact"
	default:
		return true, ""
	}
}

// ActivePhones returns the numbers the connector filters on before forwarding
// anything.
//
// Normalized digits are sent rather than hashes: the connector has no access to
// the encryption key, and it already holds the WhatsApp session credentials,
// which are far more sensitive than the numbers themselves.
func (db *DB) ActivePhones(ctx context.Context, userID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT phone FROM allowed_contacts WHERE user_id = $1 AND is_active ORDER BY phone`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("active phones: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan phone: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RecordRejection increments a daily counter. Only the reason is stored, never
// the rejected number or content (doc bab 24.2).
func (db *DB) RecordRejection(ctx context.Context, userID, reason string) {
	_, _ = db.Pool.Exec(ctx, `
		INSERT INTO ingest_rejections (user_id, reason)
		VALUES ($1, $2)
		ON CONFLICT (user_id, reason, occurred_on)
		DO UPDATE SET count = ingest_rejections.count + 1`,
		userID, reason,
	)
}

type RejectionStat struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func (db *DB) RejectionStats(ctx context.Context, userID string) ([]RejectionStat, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT reason, sum(count)::int
		FROM ingest_rejections
		WHERE user_id = $1
		GROUP BY reason
		ORDER BY 2 DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("rejection stats: %w", err)
	}
	defer rows.Close()

	out := []RejectionStat{}
	for rows.Next() {
		var s RejectionStat
		if err := rows.Scan(&s.Reason, &s.Count); err != nil {
			return nil, fmt.Errorf("scan stat: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
