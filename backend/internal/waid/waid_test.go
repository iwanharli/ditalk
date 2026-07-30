package waid

import (
	"errors"
	"testing"
)

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"6281234567890", "6281234567890"},
		{"081234567890", "6281234567890"},
		{"+6281234567890", "6281234567890"},
		{"+62 812-3456-7890", "6281234567890"},
		{"0812 3456 7890", "6281234567890"},
		{"(0812) 3456-7890", "6281234567890"},
		{"0812.3456.7890", "6281234567890"},
		{"006281234567890", "6281234567890"},
		{"  6281234567890  ", "6281234567890"},
		// Non-Indonesian numbers must pass through untouched.
		{"+14155552671", "14155552671"},
		{"60123456789", "60123456789"},
	}

	for _, tc := range tests {
		got, err := NormalizePhone(tc.in)
		if err != nil {
			t.Errorf("NormalizePhone(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizePhoneRejects(t *testing.T) {
	tests := []struct {
		in      string
		wantErr error
	}{
		{"", ErrEmpty},
		{"   ", ErrEmpty},
		{"0812abc", ErrNotDigits},
		{"08/12", ErrNotDigits},
		{"0812", ErrTooShort},
		{"12345", ErrTooShort},
	}

	for _, tc := range tests {
		_, err := NormalizePhone(tc.in)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("NormalizePhone(%q) err = %v, want %v", tc.in, err, tc.wantErr)
		}
	}
}

func TestLocalAndInternationalCollapseToSameValue(t *testing.T) {
	// The whole point of normalization: the allowlist must match regardless of
	// which form the number arrived in.
	local, err := NormalizePhone("081234567890")
	if err != nil {
		t.Fatalf("local: %v", err)
	}
	intl, err := NormalizePhone("+62 812 3456 7890")
	if err != nil {
		t.Fatalf("intl: %v", err)
	}
	if local != intl {
		t.Errorf("local %q != intl %q", local, intl)
	}
}

func TestPhoneFromJID(t *testing.T) {
	tests := []struct {
		jid  string
		want string
		ok   bool
	}{
		{"6281234567890@s.whatsapp.net", "6281234567890", true},
		// Device suffix must be stripped.
		{"6281234567890:12@s.whatsapp.net", "6281234567890", true},
		{"6281234567890_1@s.whatsapp.net", "6281234567890", true},
		{"  6281234567890@s.whatsapp.net ", "6281234567890", true},
		// Bare digits accepted and normalized.
		{"081234567890", "6281234567890", true},

		// A @lid carries an opaque identifier, not a phone number. Treating its
		// digits as one would match the wrong contact.
		{"123456789012345@lid", "", false},
		{"6281234567890@g.us", "", false},
		{"status@broadcast", "", false},
		{"abc@s.whatsapp.net", "", false},
		{"123@s.whatsapp.net", "", false},
		{"", "", false},
		{"@s.whatsapp.net", "", false},
	}

	for _, tc := range tests {
		got, ok := PhoneFromJID(tc.jid)
		if ok != tc.ok {
			t.Errorf("PhoneFromJID(%q) ok = %v, want %v", tc.jid, ok, tc.ok)
			continue
		}
		if got != tc.want {
			t.Errorf("PhoneFromJID(%q) = %q, want %q", tc.jid, got, tc.want)
		}
	}
}

func TestIsSupported(t *testing.T) {
	supported := []string{
		"6281234567890@s.whatsapp.net",
		"123456789012345@lid",
	}
	for _, jid := range supported {
		if !IsSupported(jid) {
			t.Errorf("IsSupported(%q) = false, want true", jid)
		}
	}

	// Groups and broadcasts would pull in third parties who never consented.
	unsupported := []string{
		"6281234567890-1234567890@g.us",
		"status@broadcast",
		"abc@newsletter",
		"nonsense",
		"",
	}
	for _, jid := range unsupported {
		if IsSupported(jid) {
			t.Errorf("IsSupported(%q) = true, want false", jid)
		}
	}
}

func TestIsGroup(t *testing.T) {
	if !IsGroup("6281234567890-1234@g.us") {
		t.Error("group JID not detected")
	}
	if IsGroup("6281234567890@s.whatsapp.net") {
		t.Error("contact JID misdetected as group")
	}
}

func TestJIDFromPhoneRoundTrip(t *testing.T) {
	phone, err := NormalizePhone("081234567890")
	if err != nil {
		t.Fatalf("NormalizePhone: %v", err)
	}

	jid := JIDFromPhone(phone)
	got, ok := PhoneFromJID(jid)
	if !ok || got != phone {
		t.Errorf("round trip: %q -> %q -> %q (ok=%v)", phone, jid, got, ok)
	}
}
