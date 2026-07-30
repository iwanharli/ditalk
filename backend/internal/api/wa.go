package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"ditalk/backend/internal/storage"
	"ditalk/backend/internal/wa"
	"ditalk/backend/internal/waid"
)

// handleWAStatus reports the link state and, while pairing, the QR payload the
// frontend renders. Polled by the pairing screen.
func (s *Server) handleWAStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	contacts, err := s.db.ListAllowedContacts(r.Context(), userID)
	if err != nil {
		s.log.Printf("list allowlist: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("allowlist_unavailable"))
		return
	}

	active := 0
	for _, c := range contacts {
		if c.IsActive {
			active++
		}
	}

	snap := s.wa.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"connection":             snap,
		"allowlist_total":        len(contacts),
		"allowlist_active":       active,
		"reads_only_allowlisted": true,
	})
}

// handleWALogout clears the link. The connector observes the flag on its next
// poll and tears down its socket and auth state.
func (s *Server) handleWALogout(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}

	s.wa.RequestLogout()
	writeJSON(w, http.StatusOK, map[string]string{"status": "logout_requested"})
}

// handleWAPair asks the connector to (re)start pairing so a fresh QR is emitted.
//
// Only the connector can produce a QR. If it is not running the request cannot be
// honoured, and reporting success would leave the user waiting for a code that
// nothing is generating.
func (s *Server) handleWAPair(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}

	if !s.wa.Snapshot().ConnectorOnline {
		writeJSON(w, http.StatusServiceUnavailable, errBody("connector_offline"))
		return
	}

	s.wa.RequestPairing()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "pairing_requested"})
}

// handleWAAvatar serves the linked account's profile picture from memory.
//
// Proxying it here means the browser never contacts WhatsApp's CDN, and the bytes
// are never written to disk.
func (s *Server) handleWAAvatar(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}

	data, mime, version, ok := s.wa.Avatar()
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("no_avatar"))
		return
	}
	serveImage(w, r, data, mime, version)
}

// handleContactAvatar serves a selected contact's profile picture. Pictures exist
// only for allowlisted contacts; see the connector's avatars.js for why they are
// not fetched for every discovered chat.
func (s *Server) handleContactAvatar(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}

	phone, err := waid.NormalizePhone(r.PathValue("phone"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_phone"))
		return
	}

	data, mime, version, ok := s.wa.ContactAvatar(phone)
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("no_avatar"))
		return
	}
	serveImage(w, r, data, mime, version)
}

// serveImage writes image bytes with caching and a policy that stops the browser
// from treating the response as anything other than an image.
func serveImage(w http.ResponseWriter, r *http.Request, data []byte, mime, version string) {
	etag := `"` + version + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", mime)
	w.Header().Set("ETag", etag)
	// private: these are personal photographs, never a shared cache entry.
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

// handleBackfillRetry lets the user ask for another attempt at pulling older
// history for a contact.
//
// Backfill stops once WhatsApp returns nothing older, which is usually right.
// But "nothing older" can also mean the request went out at a bad moment, and
// without a retry the conversation would stay stuck at whatever it had.
func (s *Server) handleBackfillRetry(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	phone, err := waid.NormalizePhone(r.PathValue("phone"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_phone"))
		return
	}

	conversationID, err := s.db.ConversationIDForJID(
		r.Context(), s.cipher, userID, waid.JIDFromPhone(phone))
	if errors.Is(err, storage.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errBody("conversation_not_found"))
		return
	}
	if err != nil {
		s.log.Printf("resolve conversation: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("retry_failed"))
		return
	}

	s.wa.Backfill.Reset(conversationID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "backfill_restarted"})
}

// contactRow is one line in the picker: a chat that exists on the device, an
// already-registered number, or both.
type contactRow struct {
	Phone string `json:"phone"`
	// Name from WhatsApp; empty when the contact is not saved on the phone.
	Name string `json:"name,omitempty"`
	// Label is the user's own note, which wins over the WhatsApp name.
	Label         string     `json:"label,omitempty"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	Registered    bool       `json:"registered"`
	IsActive      bool       `json:"is_active"`
	// ContactID is the allowlist row id, present only when registered.
	ContactID string `json:"contact_id,omitempty"`
	// FromDevice marks rows discovered from WhatsApp rather than typed manually.
	FromDevice bool `json:"from_device"`
	// AvatarVersion is set only for selected contacts, whose picture the
	// connector fetches; empty means the row falls back to initials.
	AvatarVersion string `json:"avatar_version,omitempty"`
	// Stored and Backfill describe how much of this chat's history is saved.
	Stored   int          `json:"stored"`
	Backfill *wa.Progress `json:"backfill,omitempty"`
}

// handleWAContacts merges the chats discovered on the device with the allowlist,
// so the picker can show registration state inline instead of making the user
// compare two separate lists.
func (s *Server) handleWAContacts(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	registered, err := s.db.ListAllowedContacts(r.Context(), userID)
	if err != nil {
		s.log.Printf("list allowlist: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("allowlist_unavailable"))
		return
	}

	byPhone := make(map[string]*contactRow, len(registered))
	rows := make([]*contactRow, 0, len(registered))

	for _, c := range registered {
		row := &contactRow{
			Phone:      c.Phone,
			Label:      c.Label,
			Registered: true,
			IsActive:   c.IsActive,
			ContactID:  c.ID,
		}
		byPhone[c.Phone] = row
		rows = append(rows, row)
	}

	for _, cand := range s.wa.Candidates() {
		if existing, seen := byPhone[cand.Phone]; seen {
			existing.Name = cand.Name
			existing.LastMessageAt = cand.LastMessageAt
			existing.FromDevice = true
			continue
		}

		row := &contactRow{
			Phone:         cand.Phone,
			Name:          cand.Name,
			LastMessageAt: cand.LastMessageAt,
			FromDevice:    true,
		}
		byPhone[cand.Phone] = row
		rows = append(rows, row)
	}

	// Registered first so the user's own choices stay at the top, then by recent
	// activity, then by name so the order is stable between polls.
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Registered != b.Registered {
			return a.Registered
		}
		at, bt := a.LastMessageAt, b.LastMessageAt
		switch {
		case at != nil && bt != nil && !at.Equal(*bt):
			return at.After(*bt)
		case at != nil && bt == nil:
			return true
		case at == nil && bt != nil:
			return false
		}
		return displayName(a) < displayName(b)
	})

	for phone, version := range s.wa.ContactAvatarVersions() {
		if row, ok := byPhone[phone]; ok {
			row.AvatarVersion = version
		}
	}

	// How much history is stored, and whether the engine is still walking back.
	if cursors, err := s.db.BackfillCursors(r.Context(), userID); err == nil {
		for _, c := range cursors {
			row, ok := byPhone[c.Phone]
			if !ok {
				continue
			}
			row.Stored = c.StoredCount
			p := s.wa.Backfill.Progress(c.ConversationID, c.StoredCount)
			row.Backfill = &p
		}
	} else {
		s.log.Printf("backfill progress: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{"contacts": rows})
}

func displayName(c *contactRow) string {
	if c.Label != "" {
		return strings.ToLower(c.Label)
	}
	if c.Name != "" {
		return strings.ToLower(c.Name)
	}
	return c.Phone
}

// ---------------------------------------------------------------- allowlist API

func (s *Server) handleAllowlistList(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	contacts, err := s.db.ListAllowedContacts(r.Context(), userID)
	if err != nil {
		s.log.Printf("list allowlist: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("allowlist_unavailable"))
		return
	}

	stats, err := s.db.RejectionStats(r.Context(), userID)
	if err != nil {
		s.log.Printf("rejection stats: %v", err)
		stats = nil
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"contacts":   contacts,
		"rejections": stats,
	})
}

type addContactRequest struct {
	Phone       string `json:"phone"`
	Label       string `json:"label"`
	ConsentNote string `json:"consent_note"`
}

func (s *Server) handleAllowlistAdd(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	var req addContactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_json"))
		return
	}

	contact, err := s.db.AddAllowedContact(r.Context(), s.cipher, userID, req.Phone, req.Label, req.ConsentNote)
	switch {
	case errors.Is(err, waid.ErrEmpty):
		writeJSON(w, http.StatusBadRequest, errBody("phone_required"))
		return
	case errors.Is(err, waid.ErrTooShort):
		writeJSON(w, http.StatusBadRequest, errBody("phone_too_short"))
		return
	case errors.Is(err, waid.ErrNotDigits):
		writeJSON(w, http.StatusBadRequest, errBody("phone_invalid_characters"))
		return
	case err != nil:
		s.log.Printf("add allowlist: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("add_failed"))
		return
	}

	// The connector caches the allowlist; bump the version so it refreshes.
	s.wa.BumpAllowlistVersion()
	writeJSON(w, http.StatusCreated, contact)
}

type setActiveRequest struct {
	IsActive bool `json:"is_active"`
}

func (s *Server) handleAllowlistSetActive(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	var req setActiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_json"))
		return
	}

	err := s.db.SetAllowedContactActive(r.Context(), userID, r.PathValue("id"), req.IsActive)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errBody("contact_not_found"))
		return
	case err != nil:
		s.log.Printf("set active: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("update_failed"))
		return
	}

	s.wa.BumpAllowlistVersion()
	writeJSON(w, http.StatusOK, map[string]bool{"is_active": req.IsActive})
}

func (s *Server) handleAllowlistDelete(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	err := s.db.DeleteAllowedContact(r.Context(), userID, r.PathValue("id"))
	switch {
	case errors.Is(err, storage.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errBody("contact_not_found"))
		return
	case err != nil:
		s.log.Printf("delete allowlist: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("delete_failed"))
		return
	}

	s.wa.BumpAllowlistVersion()
	w.WriteHeader(http.StatusNoContent)
}

// requireUser resolves the owning user. Encryption is mandatory because every
// path below either reads or writes chat-derived data.
func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	if s.cipher == nil {
		writeJSON(w, http.StatusServiceUnavailable, errBody("encryption_key_not_configured"))
		return "", false
	}

	userID, err := s.db.EnsureLocalUser(r.Context(), s.cipher)
	if err != nil {
		s.log.Printf("resolve local user: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("user_unavailable"))
		return "", false
	}
	return userID, true
}

func errBody(code string) map[string]string {
	return map[string]string{"error": code}
}
