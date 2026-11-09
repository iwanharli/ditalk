package waimport

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

var (
	ErrNoChatText = errors.New("tidak ada berkas _chat.txt di dalam zip")
	ErrChatTooBig = errors.New("berkas percakapan terlalu besar")
)

// MaxChatTextBytes caps the extracted transcript. A three-year chat is a couple
// of megabytes of text; anything far past that is not a transcript.
const MaxChatTextBytes = 128 << 20

// OpenChatText returns a reader over the transcript inside a WhatsApp export.
//
// An export "with media" is a zip whose transcript is `_chat.txt` alongside
// hundreds of media files. Only the transcript is read: media is not part of
// what this import stores, and unpacking it would write hundreds of megabytes
// for nothing.
func OpenChatText(r io.ReaderAt, size int64) (io.ReadCloser, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("baca zip: %w", err)
	}

	entry := findChatEntry(zr.File)
	if entry == nil {
		return nil, ErrNoChatText
	}
	if entry.UncompressedSize64 > MaxChatTextBytes {
		return nil, ErrChatTooBig
	}

	f, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("buka %s: %w", entry.Name, err)
	}
	return f, nil
}

// findChatEntry prefers the name WhatsApp actually uses, then falls back to any
// .txt in the archive.
func findChatEntry(files []*zip.File) *zip.File {
	var fallback *zip.File

	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}

		// Zip entries can carry directory components; only the final element is
		// meaningful, and a traversal-looking name must never be followed.
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		lower := strings.ToLower(name)

		if lower == "_chat.txt" {
			return f
		}
		if fallback == nil && strings.HasSuffix(lower, ".txt") {
			fallback = f
		}
	}
	return fallback
}
