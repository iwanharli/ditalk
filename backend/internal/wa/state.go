// Package wa tracks the live WhatsApp link state reported by the connector.
//
// The QR string is held in memory only. It is a short-lived pairing credential:
// persisting it would leave a usable secret on disk long after it expired
// (doc bab 18.1).
package wa

import (
	"crypto/sha256"
	"encoding/hex"
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
	SelfPhone string `json:"self_phone,omitempty"`
	// SelfName is the account's own WhatsApp display name.
	SelfName string `json:"self_name,omitempty"`
	// HasAvatar tells the UI whether GET /wa/avatar will return an image. The
	// bytes are not inlined here because the status endpoint is polled every few
	// seconds and would re-send the picture each time.
	HasAvatar bool `json:"has_avatar"`
	// AvatarVersion changes whenever the picture does, so the browser can cache
	// the image and still pick up a change.
	AvatarVersion string     `json:"avatar_version,omitempty"`
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
	selfName      string
	lastConnected *time.Time
	lastError     string
	updatedAt     time.Time

	// Avatar bytes live in memory only. A profile picture is personal data that
	// the app does not need to retain, so it is discarded on logout and lost on
	// restart rather than written to disk.
	avatar        []byte
	avatarMime    string
	avatarVersion string

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
	case StatusLoggedOut:
		// Unlinking must not leave the account's name or picture behind.
		s.qr = ""
		s.qrSetAt = time.Time{}
		s.selfPhone = ""
		s.selfName = ""
		s.avatar, s.avatarMime, s.avatarVersion = nil, "", ""
	case StatusDisconnected:
		// A dropped socket usually reconnects to the same account, so identity is
		// kept to avoid the profile flickering away and back.
		s.qr = ""
		s.qrSetAt = time.Time{}
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

func (s *State) SetSelfName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selfName = name
	s.updatedAt = time.Now()
}

// SetAvatar stores the picture and derives a version from its content, so an
// unchanged picture keeps the same cache key across reconnects.
func (s *State) SetAvatar(data []byte, mime string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(data) == 0 {
		s.avatar, s.avatarMime, s.avatarVersion = nil, "", ""
		return
	}

	sum := sha256.Sum256(data)
	s.avatar = data
	s.avatarMime = mime
	s.avatarVersion = hex.EncodeToString(sum[:8])
	s.updatedAt = time.Now()
}

// Avatar returns the stored picture. ok is false when none is available.
func (s *State) Avatar() (data []byte, mime, version string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.avatar) == 0 {
		return nil, "", "", false
	}
	return s.avatar, s.avatarMime, s.avatarVersion, true
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := Snapshot{
		Status:        s.status,
		SelfPhone:     s.selfPhone,
		SelfName:      s.selfName,
		HasAvatar:     len(s.avatar) > 0,
		AvatarVersion: s.avatarVersion,
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
