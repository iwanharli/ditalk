// Package wa tracks the live WhatsApp link state reported by the connector.
//
// The QR string is held in memory only. It is a short-lived pairing credential:
// persisting it would leave a usable secret on disk long after it expired
// (doc bab 18.1).
package wa

import (
	"sync"
	"time"
)

type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusPairing      Status = "pairing"
	StatusConnected    Status = "connected"
	StatusLoggedOut    Status = "logged_out"
	StatusError        Status = "error"
)

// qrTTL matches WhatsApp's own pairing window; a stale code only produces a
// failed scan, so it is withheld once expired.
const qrTTL = 60 * time.Second

// connectorTTL is how long after its last poll the connector is still considered
// alive. It polls every 3s by default, so this tolerates a few missed ticks.
const connectorTTL = 15 * time.Second

type Snapshot struct {
	Status Status `json:"status"`
	// QR is the raw pairing payload. The frontend renders it as an image; the
	// backend never stores it.
	QR          string     `json:"qr,omitempty"`
	QRExpiresAt *time.Time `json:"qr_expires_at,omitempty"`
	// SelfPhone is the account that scanned the code, echoed back so the user can
	// confirm they linked the intended number.
	SelfPhone     string     `json:"self_phone,omitempty"`
	LastConnected *time.Time `json:"last_connected_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// ConnectorOnline distinguishes "the connector process is not running" from
	// "WhatsApp is not linked". Only the connector can produce a QR, so without
	// this the UI would show "not connected" and leave the user waiting for a
	// code that nothing is generating.
	ConnectorOnline   bool       `json:"connector_online"`
	ConnectorLastSeen *time.Time `json:"connector_last_seen,omitempty"`
}

type State struct {
	mu sync.RWMutex

	status        Status
	qr            string
	qrSetAt       time.Time
	selfPhone     string
	lastConnected *time.Time
	lastError     string
	updatedAt     time.Time

	// Commands the connector picks up on its next poll. The backend cannot call
	// into the connector, so control flows the same direction as events.
	logoutRequested  bool
	pairingRequested bool
	// allowlistVersion increments on every allowlist change so the connector can
	// refresh its filter without polling the full list each time.
	allowlistVersion int64

	connectorSeenAt time.Time
}

func NewState() *State {
	return &State{status: StatusDisconnected, updatedAt: time.Now()}
}

// Command is what the connector receives when it polls for instructions.
type Command struct {
	Logout           bool  `json:"logout"`
	Pair             bool  `json:"pair"`
	AllowlistVersion int64 `json:"allowlist_version"`
}

func (s *State) RequestLogout() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logoutRequested = true
	s.pairingRequested = false
	s.updatedAt = time.Now()
}

func (s *State) RequestPairing() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairingRequested = true
	s.logoutRequested = false
	s.updatedAt = time.Now()
}

func (s *State) BumpAllowlistVersion() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowlistVersion++
	s.updatedAt = time.Now()
}

// TakeCommand returns pending instructions and clears the one-shot flags, so a
// logout is acted on exactly once. Polling doubles as the connector's heartbeat.
func (s *State) TakeCommand() Command {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.connectorSeenAt = time.Now()

	cmd := Command{
		Logout:           s.logoutRequested,
		Pair:             s.pairingRequested,
		AllowlistVersion: s.allowlistVersion,
	}
	s.logoutRequested = false
	s.pairingRequested = false
	return cmd
}

func (s *State) AllowlistVersion() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allowlistVersion
}

func (s *State) SetQR(qr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.qr = qr
	s.qrSetAt = time.Now()
	s.status = StatusPairing
	s.lastError = ""
	s.updatedAt = time.Now()
}

func (s *State) SetStatus(status Status, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = status
	s.updatedAt = time.Now()

	switch status {
	case StatusConnected:
		now := time.Now()
		s.lastConnected = &now
		// A scanned code must not linger; it is spent.
		s.qr = ""
		s.qrSetAt = time.Time{}
		s.lastError = ""
	case StatusLoggedOut, StatusDisconnected:
		s.qr = ""
		s.qrSetAt = time.Time{}
		s.selfPhone = ""
	case StatusError:
		s.lastError = detail
	}
}

func (s *State) SetSelfPhone(phone string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selfPhone = phone
	s.updatedAt = time.Now()
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := Snapshot{
		Status:        s.status,
		SelfPhone:     s.selfPhone,
		LastConnected: s.lastConnected,
		LastError:     s.lastError,
		UpdatedAt:     s.updatedAt,
	}

	if s.qr != "" && time.Since(s.qrSetAt) < qrTTL {
		expires := s.qrSetAt.Add(qrTTL)
		snap.QR = s.qr
		snap.QRExpiresAt = &expires
	}

	if !s.connectorSeenAt.IsZero() {
		seen := s.connectorSeenAt
		snap.ConnectorLastSeen = &seen
		snap.ConnectorOnline = time.Since(seen) < connectorTTL
	}
	return snap
}
