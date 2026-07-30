package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"ditalk/backend/internal/wa"
	"ditalk/backend/internal/waid"
)

type internalEvent struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

type connectionPayload struct {
	Status  string `json:"status"`
	QR      string `json:"qr"`
	SelfJID string `json:"self_jid"`
	Detail  string `json:"detail"`
}

type messagePayload struct {
	ConversationID string `json:"conversation_id"`
}

// requireInternalToken authenticates the connector. Without it, anything able to
// reach the port could inject fabricated messages or a fake QR code, and the
// event endpoint is the one path that writes chat data.
func (s *Server) requireInternalToken(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.InternalToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, errBody("internal_token_not_configured"))
		return false
	}

	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.InternalToken)) != 1 {
		writeJSON(w, http.StatusUnauthorized, errBody("unauthorized"))
		return false
	}
	return true
}

// handleInternalEvents receives connector events. Message events are dropped
// unless their conversation belongs to an active allowlisted contact; this is the
// second of two filters, the first being in the connector itself.
func (s *Server) handleInternalEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalToken(w, r) {
		return
	}

	var ev internalEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_json"))
		return
	}

	switch ev.Event {
	case "connection.qr", "connection.update":
		s.applyConnectionEvent(ev)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return

	case "message.ingested", "message.updated", "message.deleted", "message.reaction":
		s.applyMessageEvent(w, r, ev)
		return

	default:
		// Unknown events are acknowledged but ignored, so a connector upgrade
		// that emits new types does not fail loudly.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
	}
}

func (s *Server) applyConnectionEvent(ev internalEvent) {
	var p connectionPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		s.log.Printf("connection event: malformed payload")
		return
	}

	if p.QR != "" {
		s.wa.SetQR(p.QR)
	}

	if p.SelfJID != "" {
		if phone, ok := waid.PhoneFromJID(p.SelfJID); ok {
			s.wa.SetSelfPhone(phone)
		}
	}

	if p.Status != "" {
		s.wa.SetStatus(wa.Status(p.Status), p.Detail)
	}
}

func (s *Server) applyMessageEvent(w http.ResponseWriter, r *http.Request, ev internalEvent) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	var p messagePayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_payload"))
		return
	}

	if p.ConversationID == "" {
		writeJSON(w, http.StatusBadRequest, errBody("conversation_id_required"))
		return
	}

	allowed, reason := s.db.IsAllowed(r.Context(), s.cipher, userID, p.ConversationID)
	if !allowed {
		s.db.RecordRejection(r.Context(), userID, reason)
		// 200 with a rejected verdict: the connector did nothing wrong, and the
		// message is simply out of scope.
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "rejected",
			"reason": reason,
		})
		return
	}

	// Persisting allowlisted messages is wired up with the live sync pipeline.
	// Until then the filter decision is the meaningful result.
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

// handleInternalCommands lets the connector poll for instructions and the
// current allowlist filter.
func (s *Server) handleInternalCommands(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalToken(w, r) {
		return
	}

	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	phones, err := s.db.ActivePhones(r.Context(), userID)
	if err != nil {
		s.log.Printf("active phones: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("allowlist_unavailable"))
		return
	}

	cmd := s.wa.TakeCommand()
	writeJSON(w, http.StatusOK, map[string]any{
		"logout":            cmd.Logout,
		"pair":              cmd.Pair,
		"allowlist_version": cmd.AllowlistVersion,
		"allowed_phones":    phones,
	})
}
