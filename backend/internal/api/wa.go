package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"ditalk/backend/internal/storage"
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
func (s *Server) handleWAPair(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}

	s.wa.RequestPairing()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "pairing_requested"})
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
