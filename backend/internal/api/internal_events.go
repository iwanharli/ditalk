package api

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

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
	// SelfName is the account's own WhatsApp display name.
	SelfName string `json:"self_name"`
	// Avatar is base64 image bytes downloaded by the connector, so the browser
	// never has to request anything from WhatsApp itself.
	Avatar     string `json:"avatar"`
	AvatarMime string `json:"avatar_mime"`
	Detail     string `json:"detail"`
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

	case "contacts.discovered":
		s.applyContactsEvent(ev)
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

	if p.SelfName != "" {
		s.wa.SetSelfName(p.SelfName)
	}

	if p.Avatar != "" {
		data, err := base64.StdEncoding.DecodeString(p.Avatar)
		if err != nil {
			s.log.Printf("connection event: avatar bukan base64 valid")
		} else if mime := sniffImageMIME(data); mime == "" {
			// Refuse anything that is not a plain image; the bytes get served back
			// to the browser, so an SVG or HTML payload here would be a scripting
			// vector.
			s.log.Printf("connection event: avatar bukan gambar yang didukung")
		} else {
			s.wa.SetAvatar(data, mime)
		}
	}

	// Status is applied last: switching to logged_out clears identity, and doing
	// that before the fields above would immediately discard them.
	if p.Status != "" {
		s.wa.SetStatus(wa.Status(p.Status), p.Detail)
	}
}

// sniffImageMIME verifies the bytes really are a raster image, ignoring whatever
// content type was declared. It returns "" when the format is not allowed.
func sniffImageMIME(data []byte) string {
	detected := http.DetectContentType(data)
	switch detected {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return detected
	default:
		return ""
	}
}

type contactsPayload struct {
	Contacts []struct {
		Phone         string `json:"phone"`
		Name          string `json:"name"`
		LastMessageAt string `json:"last_message_at"`
	} `json:"contacts"`
}

// applyContactsEvent stores the picker list in memory.
//
// These contacts are not allowlisted, so nothing about them is persisted. The
// list only lets the owner see what exists in order to choose (doc bab 19.1).
func (s *Server) applyContactsEvent(ev internalEvent) {
	var p contactsPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		s.log.Printf("contacts event: malformed payload")
		return
	}

	out := make([]wa.Candidate, 0, len(p.Contacts))
	for _, c := range p.Contacts {
		phone, err := waid.NormalizePhone(c.Phone)
		if err != nil {
			continue
		}

		cand := wa.Candidate{Phone: phone, Name: strings.TrimSpace(c.Name)}
		if c.LastMessageAt != "" {
			if t, err := time.Parse(time.RFC3339, c.LastMessageAt); err == nil {
				cand.LastMessageAt = &t
			}
		}
		out = append(out, cand)
	}

	s.wa.SetCandidates(out)
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
