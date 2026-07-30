package api

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ditalk/backend/internal/storage"
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

	case "contact.avatar":
		s.applyContactAvatarEvent(ev)
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

type contactAvatarPayload struct {
	Phone      string `json:"phone"`
	Avatar     string `json:"avatar"`
	AvatarMime string `json:"avatar_mime"`
}

// applyContactAvatarEvent stores a contact's profile picture in memory.
//
// Only contacts the user selected reach here; the connector does not fetch
// pictures for the wider discovered list. Nothing is written to disk.
func (s *Server) applyContactAvatarEvent(ev internalEvent) {
	var p contactAvatarPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		s.log.Printf("contact avatar: malformed payload")
		return
	}

	phone, err := waid.NormalizePhone(p.Phone)
	if err != nil {
		return
	}

	if p.Avatar == "" {
		s.wa.SetContactAvatar(phone, nil, "")
		return
	}

	data, err := base64.StdEncoding.DecodeString(p.Avatar)
	if err != nil {
		s.log.Printf("contact avatar: bukan base64 valid")
		return
	}

	// These bytes are served back to the browser, so an SVG or HTML payload here
	// would be a scripting vector.
	mime := sniffImageMIME(data)
	if mime == "" {
		s.log.Printf("contact avatar: bukan gambar yang didukung")
		return
	}

	s.wa.SetContactAvatar(phone, data, mime)
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

	switch ev.Event {
	case "message.ingested":
		s.storeMessage(w, r, userID, ev)
	case "message.updated":
		s.applyMessageUpdate(w, r, userID, ev)
	case "message.deleted":
		s.applyMessageDelete(w, r, userID, p.ConversationID, ev)
	case "message.reaction":
		s.applyMessageReaction(w, r, userID, ev)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
	}
}

// canonicalMessage is the object the connector's normalizer produces (doc 6.1).
type canonicalMessage struct {
	MessageID       string `json:"message_id"`
	ConversationID  string `json:"conversation_id"`
	SenderRole      string `json:"sender_role"`
	Timestamp       string `json:"timestamp"`
	MessageType     string `json:"message_type"`
	Text            string `json:"text"`
	QuotedMessageID string `json:"quoted_message_id"`
	IsViewOnce      bool   `json:"is_view_once"`
	IsEphemeral     bool   `json:"is_ephemeral"`
}

func (s *Server) storeMessage(w http.ResponseWriter, r *http.Request, userID string, ev internalEvent) {
	var m canonicalMessage
	if err := json.Unmarshal(ev.Payload, &m); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_payload"))
		return
	}

	ts, err := time.Parse(time.RFC3339, m.Timestamp)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_timestamp"))
		return
	}

	outcome, err := s.db.SaveLiveMessage(r.Context(), s.cipher, userID, storage.LiveMessage{
		MessageID:       m.MessageID,
		ConversationID:  m.ConversationID,
		SenderRole:      m.SenderRole,
		Timestamp:       ts,
		MessageType:     m.MessageType,
		Text:            m.Text,
		QuotedMessageID: m.QuotedMessageID,
		IsViewOnce:      m.IsViewOnce,
		IsEphemeral:     m.IsEphemeral,
	})
	if err != nil {
		// The error is logged without the message body; content must never reach
		// logs (doc bab 24.2).
		s.log.Printf("save live message: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("save_failed"))
		return
	}

	if outcome == storage.SaveSkipped {
		s.db.RecordRejection(r.Context(), userID, "view_once")
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": string(outcome)})
}

type updatePayload struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Update         struct {
		// Present when the message was edited; absent for receipt-only updates.
		Message *struct {
			Conversation    string `json:"conversation"`
			ExtendedTextMsg *struct {
				Text string `json:"text"`
			} `json:"extendedTextMessage"`
		} `json:"message"`
		// Baileys reports a revoke as an update on some platforms.
		MessageStubType int `json:"messageStubType"`
	} `json:"update"`
}

func (s *Server) applyMessageUpdate(w http.ResponseWriter, r *http.Request, userID string, ev internalEvent) {
	var p updatePayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_payload"))
		return
	}

	// Most updates are delivery and read receipts, which carry no analysable
	// signal and would otherwise rewrite edited_at on every ack.
	if p.Update.Message == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	newText := p.Update.Message.Conversation
	if newText == "" && p.Update.Message.ExtendedTextMsg != nil {
		newText = p.Update.Message.ExtendedTextMsg.Text
	}

	found, err := s.db.MarkMessageEdited(
		r.Context(), s.cipher, userID, p.ConversationID, p.MessageID, newText,
	)
	if err != nil {
		s.log.Printf("mark edited: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("update_failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": outcomeLabel(found, "edited")})
}

type deletePayload struct {
	ConversationID string `json:"conversation_id"`
	Keys           []struct {
		ID        string `json:"id"`
		RemoteJID string `json:"remoteJid"`
	} `json:"keys"`
}

func (s *Server) applyMessageDelete(
	w http.ResponseWriter, r *http.Request, userID, jid string, ev internalEvent,
) {
	var p deletePayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_payload"))
		return
	}

	marked := 0
	for _, k := range p.Keys {
		target := k.RemoteJID
		if target == "" {
			target = jid
		}
		found, err := s.db.MarkMessageDeleted(r.Context(), s.cipher, userID, target, k.ID)
		if err != nil {
			s.log.Printf("mark deleted: %v", err)
			continue
		}
		if found {
			marked++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "marked": marked})
}

type reactionPayload struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Reaction       struct {
		Text string `json:"text"`
		Key  struct {
			FromMe bool `json:"fromMe"`
		} `json:"key"`
	} `json:"reaction"`
}

func (s *Server) applyMessageReaction(w http.ResponseWriter, r *http.Request, userID string, ev internalEvent) {
	var p reactionPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_payload"))
		return
	}

	role := "OTHER"
	if p.Reaction.Key.FromMe {
		role = "SELF"
	}

	// An empty text means the person removed their reaction.
	found, err := s.db.ApplyReaction(
		r.Context(), s.cipher, userID, p.ConversationID, p.MessageID, p.Reaction.Text, role,
	)
	if err != nil {
		s.log.Printf("apply reaction: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("reaction_failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": outcomeLabel(found, "reacted")})
}

type backfillRequest struct {
	ChatJID    string `json:"chat_jid"`
	OldestID   string `json:"oldest_id"`
	OldestFrom bool   `json:"oldest_from_me"`
	OldestTsMs int64  `json:"oldest_ts_ms"`
	Count      int    `json:"count"`
}

// pendingBackfill lists conversations that should be walked further back.
//
// The engine drives this on its own: as long as a selected contact still yields
// older messages, another page is requested. The user does not have to export a
// chat by hand.
func (s *Server) pendingBackfill(r *http.Request, userID string) []backfillRequest {
	cursors, err := s.db.BackfillCursors(r.Context(), userID)
	if err != nil {
		s.log.Printf("backfill cursors: %v", err)
		return nil
	}

	out := []backfillRequest{}
	for _, c := range cursors {
		if !s.wa.Backfill.ShouldRequest(c.ConversationID, c.Timestamp, c.StoredCount) {
			continue
		}
		out = append(out, backfillRequest{
			ChatJID:    waid.JIDFromPhone(c.Phone),
			OldestID:   c.MessageID,
			OldestFrom: c.FromMe,
			OldestTsMs: c.Timestamp.UnixMilli(),
			Count:      wa.BackfillBatch,
		})
	}
	return out
}

// outcomeLabel distinguishes "applied" from "the message is not stored here",
// which happens when an event arrives for a chat added to the allowlist later.
func outcomeLabel(found bool, applied string) string {
	if found {
		return applied
	}
	return "message_not_found"
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
		"backfill":          s.pendingBackfill(r, userID),
	})
}
