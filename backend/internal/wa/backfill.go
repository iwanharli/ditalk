package wa

import (
	"sync"
	"time"
)

// Backfill walks a conversation's history backwards by repeatedly asking
// WhatsApp for messages older than the oldest one already stored.
//
// There is no "no more history" reply to look for: WhatsApp simply stops
// returning older messages. Completion is therefore inferred from the cursor no
// longer moving after a request had time to land.
type Backfill struct {
	mu    sync.Mutex
	state map[string]*backfillEntry
}

type backfillEntry struct {
	// lastCursor is the oldest stored timestamp at the time of the last request.
	lastCursor  time.Time
	requestedAt time.Time
	stalls      int
	done        bool
	// fetched counts messages gained since backfill started, for the UI.
	startCount int
}

const (
	// Time to allow between requests for a conversation. A request travels to the
	// phone and back and the messages arrive asynchronously, so asking again on
	// the next three-second poll would just pile up duplicates.
	backfillInterval = 15 * time.Second
	// How many consecutive requests may leave the cursor unmoved before the
	// history is considered exhausted. Two guards against a single slow round.
	maxStalls = 2
	// Messages requested per round.
	BackfillBatch = 50
)

func NewBackfill() *Backfill {
	return &Backfill{state: map[string]*backfillEntry{}}
}

// Progress is what the dashboard shows for one conversation.
type Progress struct {
	Done    bool `json:"done"`
	Running bool `json:"running"`
	Fetched int  `json:"fetched"`
}

// ShouldRequest decides whether to ask for another page for this conversation,
// and records the decision. storedCount and oldest describe the current state of
// the conversation in the database.
func (b *Backfill) ShouldRequest(conversationID string, oldest time.Time, storedCount int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	e, ok := b.state[conversationID]
	if !ok {
		e = &backfillEntry{startCount: storedCount}
		b.state[conversationID] = e
	}

	if e.done {
		return false
	}
	if !e.requestedAt.IsZero() && time.Since(e.requestedAt) < backfillInterval {
		return false
	}

	// A first request always goes out; afterwards, an unmoved cursor means the
	// previous round brought nothing older.
	if !e.lastCursor.IsZero() {
		if !oldest.Before(e.lastCursor) {
			e.stalls++
			if e.stalls >= maxStalls {
				e.done = true
				return false
			}
		} else {
			e.stalls = 0
		}
	}

	e.lastCursor = oldest
	e.requestedAt = time.Now()
	return true
}

// Reset clears progress so a conversation can be walked again, for instance
// after the user re-links the device.
func (b *Backfill) Reset(conversationID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.state, conversationID)
}

func (b *Backfill) ResetAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = map[string]*backfillEntry{}
}

// Progress reports how far a conversation has got.
func (b *Backfill) Progress(conversationID string, storedCount int) Progress {
	b.mu.Lock()
	defer b.mu.Unlock()

	e, ok := b.state[conversationID]
	if !ok {
		return Progress{}
	}
	return Progress{
		Done:    e.done,
		Running: !e.done,
		Fetched: storedCount - e.startCount,
	}
}
