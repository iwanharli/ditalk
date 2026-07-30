package waimport

import (
	"strings"
	"testing"
	"time"
)

func parse(t *testing.T, input string) *Result {
	t.Helper()
	res, err := Parse(strings.NewReader(input), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return res
}

func TestParseAndroidIndonesian(t *testing.T) {
	input := "21/07/2026 03.15 - Andi: Halo, sudah sampai?\n" +
		"21/07/2026 03.16 - Budi: Baru otw nih\n"

	res := parse(t, input)

	if len(res.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(res.Messages))
	}
	m := res.Messages[0]
	if m.Sender != "Andi" {
		t.Errorf("sender = %q, want Andi", m.Sender)
	}
	if m.Text != "Halo, sudah sampai?" {
		t.Errorf("text = %q", m.Text)
	}
	want := time.Date(2026, 7, 21, 3, 15, 0, 0, time.Local)
	if !m.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, want %v", m.Timestamp, want)
	}
	if res.DateOrder != "day-first" {
		t.Errorf("DateOrder = %q, want day-first", res.DateOrder)
	}
	if res.Senders["Budi"] != 1 {
		t.Errorf("Senders[Budi] = %d, want 1", res.Senders["Budi"])
	}
}

func TestParseIOSBracketsWithSeconds(t *testing.T) {
	input := "[21/07/2026, 03.15.42] Andi: Pesan iOS\n"

	res := parse(t, input)

	if len(res.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(res.Messages))
	}
	m := res.Messages[0]
	if m.Sender != "Andi" || m.Text != "Pesan iOS" {
		t.Errorf("sender=%q text=%q", m.Sender, m.Text)
	}
	if m.Timestamp.Second() != 42 {
		t.Errorf("second = %d, want 42", m.Timestamp.Second())
	}
}

func TestParseUS12HourMonthFirst(t *testing.T) {
	// 7/21/26 proves month-first because 21 cannot be a month.
	input := "7/21/26, 3:15 PM - Andi: Afternoon\n" +
		"7/21/26, 11:30 AM - Budi: Morning\n"

	res := parse(t, input)

	if res.DateOrder != "month-first" {
		t.Fatalf("DateOrder = %q, want month-first", res.DateOrder)
	}
	if got := res.Messages[0].Timestamp; got.Hour() != 15 || got.Day() != 21 || got.Month() != time.July {
		t.Errorf("PM message = %v, want 2026-07-21 15:00", got)
	}
	if got := res.Messages[1].Timestamp.Hour(); got != 11 {
		t.Errorf("AM hour = %d, want 11", got)
	}
}

func TestMidnightNoonMeridiem(t *testing.T) {
	input := "7/21/26, 12:05 AM - Andi: tengah malam\n" +
		"7/21/26, 12:05 PM - Andi: tengah hari\n"

	res := parse(t, input)

	if got := res.Messages[0].Timestamp.Hour(); got != 0 {
		t.Errorf("12:05 AM hour = %d, want 0", got)
	}
	if got := res.Messages[1].Timestamp.Hour(); got != 12 {
		t.Errorf("12:05 PM hour = %d, want 12", got)
	}
}

func TestMultilineMessageIsJoined(t *testing.T) {
	input := "21/07/2026 03.15 - Andi: Baris pertama\n" +
		"baris kedua\n" +
		"baris ketiga\n" +
		"21/07/2026 03.16 - Budi: Pesan lain\n"

	res := parse(t, input)

	if len(res.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(res.Messages))
	}
	want := "Baris pertama\nbaris kedua\nbaris ketiga"
	if res.Messages[0].Text != want {
		t.Errorf("text = %q, want %q", res.Messages[0].Text, want)
	}
}

func TestSystemMessagesHaveNoSender(t *testing.T) {
	input := "21/07/2026 03.10 - Pesan dan panggilan terenkripsi secara end-to-end.\n" +
		"21/07/2026 03.15 - Andi: Halo\n"

	res := parse(t, input)

	if len(res.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(res.Messages))
	}
	if !res.Messages[0].IsSystem {
		t.Error("encryption notice should be flagged as system")
	}
	if res.Messages[0].Sender != "" {
		t.Errorf("system sender = %q, want empty", res.Messages[0].Sender)
	}
	if res.Senders["Andi"] != 1 || len(res.Senders) != 1 {
		t.Errorf("Senders = %v, want only Andi", res.Senders)
	}
}

func TestMediaOmittedAndAttachments(t *testing.T) {
	input := "21/07/2026 03.15 - Andi: <Media tidak disertakan>\n" +
		"21/07/2026 03.16 - Andi: IMG-20260721-WA0001.jpg (file terlampir)\n" +
		"21/07/2026 03.17 - Andi: PTT-20260721-WA0002.opus (file attached)\n" +
		"21/07/2026 03.18 - Budi: image omitted\n" +
		"21/07/2026 03.19 - Budi: laporan.pdf (file terlampir)\n" +
		"21/07/2026 03.20 - Budi: VID-20260721-WA0003.mp4 (file terlampir)\n"

	res := parse(t, input)

	tests := []struct {
		idx      int
		wantType MessageType
		omitted  bool
		attach   string
	}{
		{0, TypeUnknown, true, ""},
		{1, TypeImage, false, "IMG-20260721-WA0001.jpg"},
		{2, TypeAudio, false, "PTT-20260721-WA0002.opus"},
		{3, TypeImage, true, ""},
		{4, TypeDocument, false, "laporan.pdf"},
		{5, TypeVideo, false, "VID-20260721-WA0003.mp4"},
	}

	for _, tc := range tests {
		m := res.Messages[tc.idx]
		if m.Type != tc.wantType {
			t.Errorf("msg %d type = %q, want %q", tc.idx, m.Type, tc.wantType)
		}
		if m.MediaOmitted != tc.omitted {
			t.Errorf("msg %d MediaOmitted = %v, want %v", tc.idx, m.MediaOmitted, tc.omitted)
		}
		if m.AttachmentName != tc.attach {
			t.Errorf("msg %d attachment = %q, want %q", tc.idx, m.AttachmentName, tc.attach)
		}
	}
}

func TestDeletedAndEditedMarkers(t *testing.T) {
	input := "21/07/2026 03.15 - Andi: Pesan ini telah dihapus\n" +
		"21/07/2026 03.16 - Budi: Jadi jam 8 ya <Pesan ini telah diedit>\n"

	res := parse(t, input)

	if !res.Messages[0].IsDeleted {
		t.Error("expected IsDeleted")
	}
	edited := res.Messages[1]
	if !edited.IsEdited {
		t.Error("expected IsEdited")
	}
	if edited.Text != "Jadi jam 8 ya" {
		t.Errorf("edited text = %q, want marker stripped", edited.Text)
	}
}

func TestSanitizeStripsBidiAndNarrowSpace(t *testing.T) {
	// Real iOS exports prefix lines with U+200E and use U+202F before AM/PM.
	input := "\u200e[7/21/26, 3:15\u202fPM] Andi: \u200eHalo\n"

	res := parse(t, input)

	if len(res.Messages) != 1 {
		t.Fatalf("got %d messages, want 1; bidi marks likely broke the header", len(res.Messages))
	}
	m := res.Messages[0]
	if m.Sender != "Andi" {
		t.Errorf("sender = %q, want Andi", m.Sender)
	}
	if m.Text != "Halo" {
		t.Errorf("text = %q, want Halo (marks stripped)", m.Text)
	}
	if m.Timestamp.Hour() != 15 {
		t.Errorf("hour = %d, want 15", m.Timestamp.Hour())
	}
}

func TestMessageTextContainingColon(t *testing.T) {
	input := "21/07/2026 03.15 - Andi: jam: 08:00 ya\n"

	res := parse(t, input)

	m := res.Messages[0]
	if m.Sender != "Andi" {
		t.Errorf("sender = %q, want Andi", m.Sender)
	}
	if m.Text != "jam: 08:00 ya" {
		t.Errorf("text = %q, want full text preserved", m.Text)
	}
}

func TestAmbiguousDateWarns(t *testing.T) {
	// Both components under 12, so the layout cannot be determined.
	input := "05/07/2026 03.15 - Andi: Halo\n"

	res := parse(t, input)

	if res.DateOrder != "day-first" {
		t.Errorf("DateOrder = %q, want day-first fallback", res.DateOrder)
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning about ambiguous date format")
	}
	if got := res.Messages[0].Timestamp; got.Day() != 5 || got.Month() != time.July {
		t.Errorf("timestamp = %v, want 5 July", got)
	}
}

func TestImpossibleDateIsRejectedNotShifted(t *testing.T) {
	// 31 February must be dropped, never normalized to 3 March.
	input := "31/02/2026 03.15 - Andi: tanggal mustahil\n" +
		"21/07/2026 03.16 - Budi: valid\n"

	res := parse(t, input)

	if len(res.Messages) != 1 {
		t.Fatalf("got %d messages, want 1 (impossible date dropped)", len(res.Messages))
	}
	if res.Messages[0].Sender != "Budi" {
		t.Errorf("remaining sender = %q, want Budi", res.Messages[0].Sender)
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning for the dropped line")
	}
}

func TestEmptyInput(t *testing.T) {
	res := parse(t, "")

	if len(res.Messages) != 0 {
		t.Errorf("got %d messages, want 0", len(res.Messages))
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning for an unrecognized file")
	}
}

func TestSelfNameNotFoundWarns(t *testing.T) {
	res, err := Parse(strings.NewReader("21/07/2026 03.15 - Andi: Halo\n"), "Charlie")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "Charlie") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning naming the missing self name, got %v", res.Warnings)
	}
}

func TestTwoDigitYearExpands(t *testing.T) {
	res := parse(t, "21/07/26 03.15 - Andi: Halo\n")

	if got := res.Messages[0].Timestamp.Year(); got != 2026 {
		t.Errorf("year = %d, want 2026", got)
	}
}
