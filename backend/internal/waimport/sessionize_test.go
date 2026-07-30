package waimport

import (
	"testing"
	"time"
)

func at(day, hour, minute int) time.Time {
	return time.Date(2026, 7, day, hour, minute, 0, 0, time.Local)
}

func TestSessionizeSplitsOnGap(t *testing.T) {
	msgs := []Message{
		{Timestamp: at(21, 3, 0), Sender: "Andi"},
		{Timestamp: at(21, 3, 10), Sender: "Budi"},
		// 45 minutes later exceeds the 30-minute gap.
		{Timestamp: at(21, 3, 55), Sender: "Andi"},
	}

	got := Sessionize(msgs, DefaultGap)

	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	if len(got[0].Indexes) != 2 {
		t.Errorf("session 0 has %d messages, want 2", len(got[0].Indexes))
	}
	if !got[0].EndAt.Equal(at(21, 3, 10)) {
		t.Errorf("session 0 EndAt = %v, want 03:10", got[0].EndAt)
	}
}

func TestSessionizeSplitsOnDayChange(t *testing.T) {
	// Only 20 minutes apart, but across midnight.
	msgs := []Message{
		{Timestamp: at(21, 23, 50), Sender: "Andi"},
		{Timestamp: at(22, 0, 10), Sender: "Budi"},
	}

	got := Sessionize(msgs, DefaultGap)

	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2 (day boundary)", len(got))
	}
}

func TestSessionizeKeepsRunTogether(t *testing.T) {
	msgs := []Message{
		{Timestamp: at(21, 3, 0), Sender: "Andi"},
		{Timestamp: at(21, 3, 5), Sender: "Budi"},
		{Timestamp: at(21, 3, 20), Sender: "Andi"},
	}

	got := Sessionize(msgs, DefaultGap)

	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if len(got[0].Indexes) != 3 {
		t.Errorf("got %d messages in session, want 3", len(got[0].Indexes))
	}
	if !got[0].StartAt.Equal(at(21, 3, 0)) || !got[0].EndAt.Equal(at(21, 3, 20)) {
		t.Errorf("session span = %v..%v", got[0].StartAt, got[0].EndAt)
	}
}

func TestSessionizeSkipsLeadingSystemMessage(t *testing.T) {
	msgs := []Message{
		{Timestamp: at(21, 2, 0), IsSystem: true, Text: "terenkripsi end-to-end"},
		{Timestamp: at(21, 3, 0), Sender: "Andi"},
	}

	got := Sessionize(msgs, DefaultGap)

	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if !got[0].StartAt.Equal(at(21, 3, 0)) {
		t.Errorf("StartAt = %v, want the first real message at 03:00", got[0].StartAt)
	}
}

func TestSessionizeSystemMessageDoesNotOpenSession(t *testing.T) {
	msgs := []Message{
		{Timestamp: at(21, 3, 0), Sender: "Andi"},
		// Far outside the gap, but a system line alone should not start one.
		{Timestamp: at(21, 8, 0), IsSystem: true, Text: "panggilan suara tak terjawab"},
	}

	got := Sessionize(msgs, DefaultGap)

	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
}

func TestSessionizeEmptyAndZeroGap(t *testing.T) {
	if got := Sessionize(nil, DefaultGap); got != nil {
		t.Errorf("nil input returned %v, want nil", got)
	}

	msgs := []Message{
		{Timestamp: at(21, 3, 0), Sender: "Andi"},
		{Timestamp: at(21, 3, 10), Sender: "Budi"},
	}
	// A zero gap must fall back to the default, not split every message.
	if got := Sessionize(msgs, 0); len(got) != 1 {
		t.Errorf("zero gap produced %d sessions, want 1", len(got))
	}
}
