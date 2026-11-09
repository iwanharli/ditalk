package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"ditalk/backend/internal/waimport"
)

// maxExportSize caps the upload. A very long chat history exports to a few MB of
// text, so 64 MB is generous while still bounding memory.
const maxExportSize = 64 << 20

type importPreview struct {
	DateOrder string         `json:"date_order"`
	Total     int            `json:"total_messages"`
	Senders   map[string]int `json:"senders"`
	FirstAt   *time.Time     `json:"first_at"`
	LastAt    *time.Time     `json:"last_at"`
	Warnings  []string       `json:"warnings"`
}

// handleImportPreview parses an export without writing anything, so the user can
// confirm which sender is themselves before any data is stored.
func (s *Server) handleImportPreview(w http.ResponseWriter, r *http.Request) {
	res, _, err := s.parseUpload(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	preview := importPreview{
		DateOrder: res.DateOrder,
		Total:     len(res.Messages),
		Senders:   res.Senders,
		Warnings:  res.Warnings,
	}
	if len(res.Messages) > 0 {
		preview.FirstAt = &res.Messages[0].Timestamp
		preview.LastAt = &res.Messages[len(res.Messages)-1].Timestamp
	}

	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if s.cipher == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "encryption_key_not_configured",
		})
		return
	}

	res, meta, err := s.parseUpload(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if meta.selfName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "self_name_required",
		})
		return
	}

	// The owning user is resolved server-side. Accepting an id from the request
	// would let a caller write into someone else's conversations.
	meta.userID, err = s.db.EnsureLocalUser(r.Context(), s.cipher)
	if err != nil {
		s.log.Printf("resolve local user: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user_unavailable"})
		return
	}

	out, err := s.db.ImportExport(
		r.Context(), s.cipher,
		meta.userID, meta.chatKey, meta.displayName, meta.selfName,
		res, waimport.DefaultGap,
	)
	if err != nil {
		s.log.Printf("import failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "import_failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": out.ConversationID,
		"inserted":        out.Inserted,
		"skipped":         out.Skipped,
		"sessions":        out.Sessions,
		"date_order":      res.DateOrder,
		"warnings":        res.Warnings,
	})
}

type uploadMeta struct {
	userID      string
	chatKey     string
	displayName string
	selfName    string
}

func (s *Server) parseUpload(r *http.Request) (*waimport.Result, uploadMeta, error) {
	var meta uploadMeta

	if err := r.ParseMultipartForm(maxExportSize); err != nil {
		return nil, meta, fmt.Errorf("invalid_multipart_form")
	}

	// Two ways in: a file uploaded from the browser, or the name of one already
	// sitting in the import folder. The second exists because an export with
	// media is hundreds of megabytes, and copying that through HTTP achieves
	// nothing when the file is already on this machine.
	source, err := s.openImportSource(r)
	if err != nil {
		return nil, meta, err
	}
	defer source.Close()

	meta.selfName = strings.TrimSpace(r.FormValue("self_name"))
	meta.displayName = strings.TrimSpace(r.FormValue("display_name"))

	// chat_key identifies the conversation across re-imports. Falling back to
	// the display name keeps a single-chat import working without extra input.
	meta.chatKey = strings.TrimSpace(r.FormValue("chat_key"))
	if meta.chatKey == "" {
		meta.chatKey = meta.displayName
	}

	res, err := waimport.Parse(source, meta.selfName)
	if err != nil {
		return nil, meta, fmt.Errorf("parse_failed")
	}
	return res, meta, nil
}

// openImportSource resolves the transcript to read, from an upload or from the
// import folder, unwrapping a zip in either case.
func (s *Server) openImportSource(r *http.Request) (io.ReadCloser, error) {
	if name := strings.TrimSpace(r.FormValue("file_name")); name != "" {
		rc, err := s.openImportFile(name)
		if err != nil {
			s.log.Printf("open import file: %v", err)
			return nil, fmt.Errorf("import_file_unreadable")
		}
		return rc, nil
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("file_required")
	}

	switch strings.ToLower(filepath.Ext(header.Filename)) {
	case ".txt":
		return file, nil
	case ".zip":
		rc, err := waimport.OpenChatText(file, header.Size)
		if err != nil {
			file.Close()
			if errors.Is(err, waimport.ErrNoChatText) {
				return nil, fmt.Errorf("zip_without_chat_text")
			}
			return nil, fmt.Errorf("zip_unreadable")
		}
		return &archiveReader{inner: rc, file: nil, upload: file}, nil
	default:
		file.Close()
		return nil, fmt.Errorf("only_txt_or_zip_supported")
	}
}
