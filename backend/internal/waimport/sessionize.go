package waimport

import "time"

// DefaultGap is the idle period that ends a session. Doc bab 6.3 suggests
// 30-60 minutes; 30 is the tighter default and can be tuned per user.
const DefaultGap = 30 * time.Minute

// Session groups consecutive messages that belong to one active exchange, so
// analysis never reads a lone message out of context (doc bab 6.3).
type Session struct {
	StartAt time.Time
	EndAt   time.Time
	// Indexes point back into the message slice passed to Sessionize.
	Indexes []int
}

// Sessionize splits messages into sessions on either an idle gap or a calendar
// day change. Messages must be sorted by timestamp ascending.
//
// System messages are kept inside sessions because they carry context (a missed
// call explains a silence), but they never start one on their own.
func Sessionize(messages []Message, gap time.Duration) []Session {
	if len(messages) == 0 {
		return nil
	}
	if gap <= 0 {
		gap = DefaultGap
	}

	var sessions []Session
	var current *Session

	for i, m := range messages {
		if current == nil {
			// Skip leading system messages so a session always opens with real
			// participant activity.
			if m.IsSystem {
				continue
			}
			sessions = append(sessions, Session{
				StartAt: m.Timestamp,
				EndAt:   m.Timestamp,
				Indexes: []int{i},
			})
			current = &sessions[len(sessions)-1]
			continue
		}

		sameDay := m.Timestamp.YearDay() == current.EndAt.YearDay() &&
			m.Timestamp.Year() == current.EndAt.Year()
		withinGap := m.Timestamp.Sub(current.EndAt) <= gap

		if sameDay && withinGap {
			current.Indexes = append(current.Indexes, i)
			current.EndAt = m.Timestamp
			continue
		}

		if m.IsSystem {
			// A system message alone does not justify a new session; hold it
			// until a participant speaks again.
			continue
		}

		sessions = append(sessions, Session{
			StartAt: m.Timestamp,
			EndAt:   m.Timestamp,
			Indexes: []int{i},
		})
		current = &sessions[len(sessions)-1]
	}

	return sessions
}
