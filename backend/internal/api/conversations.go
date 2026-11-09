package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ditalk/backend/internal/storage"
)

func (s *Server) handleConversationsList(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	conversations, err := s.db.ListConversations(r.Context(), s.cipher, userID)
	if err != nil {
		s.log.Printf("list conversations: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("conversations_unavailable"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"conversations": conversations})
}

func (s *Server) handleConversationMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	messages, err := s.db.ConversationMessages(
		r.Context(), s.cipher, userID, r.PathValue("id"),
	)
	if err != nil {
		s.log.Printf("read conversation messages: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("messages_unavailable"))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

type sendConversationMessageRequest struct {
	Text string `json:"text"`
}

func (s *Server) handleConversationSend(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	var req sendConversationMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid_json"))
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, errBody("message_text_required"))
		return
	}
	if len([]rune(req.Text)) > 4000 {
		writeJSON(w, http.StatusBadRequest, errBody("message_text_too_long"))
		return
	}

	message, err := s.db.AddLocalMessage(
		r.Context(), s.cipher, userID, r.PathValue("id"), req.Text,
	)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errBody("conversation_not_found"))
		return
	case err != nil:
		s.log.Printf("send local conversation message: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("send_failed"))
		return
	}

	writeJSON(w, http.StatusCreated, message)
}
