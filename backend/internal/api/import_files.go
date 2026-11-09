package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ditalk/backend/internal/waimport"
)

type importFile struct {
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
	IsArchive  bool      `json:"is_archive"`
}

// handleImportFiles lists exports already sitting in the import folder.
//
// A WhatsApp export with media runs to hundreds of megabytes. Uploading that
// through the browser copies the whole archive for no reason when the file is
// already on the same machine as the backend, so the dashboard offers to pick
// one from a folder instead.
func (s *Server) handleImportFiles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}

	entries, err := os.ReadDir(s.cfg.ImportDir)
	if os.IsNotExist(err) {
		writeJSON(w, http.StatusOK, map[string]any{
			"dir":   s.cfg.ImportDir,
			"files": []importFile{},
		})
		return
	}
	if err != nil {
		s.log.Printf("read import dir: %v", err)
		writeJSON(w, http.StatusInternalServerError, errBody("import_dir_unreadable"))
		return
	}

	files := []importFile{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		lower := strings.ToLower(e.Name())
		isZip := strings.HasSuffix(lower, ".zip")
		if !isZip && !strings.HasSuffix(lower, ".txt") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, importFile{
			Name:       e.Name(),
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime(),
			IsArchive:  isZip,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt.After(files[j].ModifiedAt)
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"dir":   s.cfg.ImportDir,
		"files": files,
	})
}

// openImportFile opens a named file from the import folder.
//
// Only a bare file name is accepted. Anything carrying a path is rejected rather
// than cleaned, so this can never be talked into reading somewhere else.
func (s *Server) openImportFile(name string) (io.ReadCloser, error) {
	if name == "" {
		return nil, fmt.Errorf("file_name kosong")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return nil, fmt.Errorf("file_name harus berupa nama berkas saja")
	}

	full := filepath.Join(s.cfg.ImportDir, name)
	f, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("buka berkas: %w", err)
	}

	if !strings.HasSuffix(strings.ToLower(name), ".zip") {
		return f, nil
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat: %w", err)
	}

	// The transcript is read straight out of the archive; the media entries are
	// never expanded.
	rc, err := waimport.OpenChatText(f, info.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	return &archiveReader{inner: rc, file: f}, nil
}

// archiveReader closes the archive along with the entry inside it. The archive
// is either a file on disk or the uploaded multipart file; exactly one is set.
type archiveReader struct {
	inner  io.ReadCloser
	file   *os.File
	upload io.Closer
}

func (a *archiveReader) Read(p []byte) (int, error) { return a.inner.Read(p) }

func (a *archiveReader) Close() error {
	err := a.inner.Close()
	if a.file != nil {
		if ferr := a.file.Close(); err == nil {
			err = ferr
		}
	}
	if a.upload != nil {
		if uerr := a.upload.Close(); err == nil {
			err = uerr
		}
	}
	return err
}
