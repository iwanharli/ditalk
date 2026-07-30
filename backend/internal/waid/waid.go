// Package waid normalizes phone numbers and WhatsApp JIDs so the allowlist can
// be compared reliably.
//
// The same contact appears in different shapes depending on where the value came
// from: a user typing "0812-3456-7890", a JID like "6281234567890@s.whatsapp.net",
// or a LID/device-suffixed JID. Comparing raw strings would silently let
// non-allowlisted chats through.
package waid

import (
	"errors"
	"strings"
)

var (
	ErrEmpty     = errors.New("nomor kosong")
	ErrTooShort  = errors.New("nomor terlalu pendek")
	ErrNotDigits = errors.New("nomor mengandung karakter yang tidak valid")
)

// DefaultCountryCode is used when a local number starting with 0 is given.
// Indonesian numbers are the primary case (doc bab 8.3).
const DefaultCountryCode = "62"

// Suffixes WhatsApp appends to identify the kind of address.
const (
	suffixUser       = "@s.whatsapp.net"
	suffixGroup      = "@g.us"
	suffixBroadcast  = "@broadcast"
	suffixNewsletter = "@newsletter"
	suffixLID        = "@lid"
)

// NormalizePhone converts user input into digits in international form without
// a leading plus: "0812 3456 7890" and "+62 812-3456-7890" both become
// "6281234567890".
func NormalizePhone(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", ErrEmpty
	}

	// Drop the separators people naturally type.
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' || r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
			// Separator, ignore.
		default:
			return "", ErrNotDigits
		}
	}

	digits := b.String()
	if digits == "" {
		return "", ErrEmpty
	}

	switch {
	case strings.HasPrefix(digits, "00"):
		// International prefix dialled as 00, e.g. 006281234567890.
		digits = strings.TrimPrefix(digits, "00")
	case strings.HasPrefix(digits, "0"):
		// Local form: 0812... -> 62812...
		digits = DefaultCountryCode + strings.TrimPrefix(digits, "0")
	}

	// Shortest plausible international number; guards against typos becoming a
	// valid-looking allowlist entry.
	if len(digits) < 8 {
		return "", ErrTooShort
	}
	return digits, nil
}

// IsGroup reports whether a JID addresses a group rather than a single contact.
func IsGroup(jid string) bool {
	return strings.HasSuffix(jid, suffixGroup)
}

// IsSupported reports whether a JID is a one-to-one contact chat. Groups,
// broadcasts, newsletters, and status updates are out of scope: the allowlist
// grants access to a specific person, and a group would pull in third parties
// who never consented (doc bab 19.1).
func IsSupported(jid string) bool {
	switch {
	case strings.HasSuffix(jid, suffixGroup),
		strings.HasSuffix(jid, suffixBroadcast),
		strings.HasSuffix(jid, suffixNewsletter):
		return false
	case strings.HasSuffix(jid, suffixUser), strings.HasSuffix(jid, suffixLID):
		return true
	default:
		return false
	}
}

// PhoneFromJID extracts the bare phone digits from a JID.
//
// It returns ok=false for a @lid JID: those carry an opaque linked identifier,
// not a phone number, so treating the digits as a phone number would produce a
// wrong allowlist match.
func PhoneFromJID(jid string) (string, bool) {
	s := strings.TrimSpace(jid)
	if s == "" {
		return "", false
	}

	at := strings.LastIndex(s, "@")
	if at < 0 {
		// Bare digits were passed in.
		phone, err := NormalizePhone(s)
		if err != nil {
			return "", false
		}
		return phone, true
	}

	domain := s[at:]
	if domain == suffixLID {
		return "", false
	}
	if domain != suffixUser {
		return "", false
	}

	local := s[:at]
	// Strip a device or agent suffix such as "6281234567890:12".
	if colon := strings.Index(local, ":"); colon >= 0 {
		local = local[:colon]
	}
	// Strip the "_1" style agent marker some clients append.
	if us := strings.Index(local, "_"); us >= 0 {
		local = local[:us]
	}

	for _, r := range local {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	if len(local) < 8 {
		return "", false
	}
	return local, true
}

// JIDFromPhone builds the canonical contact JID for a normalized phone number.
func JIDFromPhone(phone string) string {
	return phone + suffixUser
}
