package waimport

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func buildZip(t *testing.T, files map[string]string) ([]byte, int64) {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes(), int64(buf.Len())
}

func readAll(t *testing.T, rc io.ReadCloser) string {
	t.Helper()
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}

func TestOpenChatTextPrefersChatTxt(t *testing.T) {
	data, size := buildZip(t, map[string]string{
		"00000034-STICKER-2023-02-23.webp": "bukan teks",
		"_chat.txt":                        "21/07/2026 03.15 - Andi: halo",
		"catatan.txt":                      "berkas teks lain",
	})

	rc, err := OpenChatText(bytes.NewReader(data), size)
	if err != nil {
		t.Fatalf("OpenChatText: %v", err)
	}
	if got := readAll(t, rc); !strings.Contains(got, "Andi") {
		t.Errorf("isi = %q, want transkrip dari _chat.txt", got)
	}
}

func TestOpenChatTextFallsBackToAnyTxt(t *testing.T) {
	data, size := buildZip(t, map[string]string{
		"foto.jpg":                    "bukan teks",
		"WhatsApp Chat with Andi.txt": "21/07/2026 03.15 - Andi: halo",
	})

	rc, err := OpenChatText(bytes.NewReader(data), size)
	if err != nil {
		t.Fatalf("OpenChatText: %v", err)
	}
	if got := readAll(t, rc); !strings.Contains(got, "Andi") {
		t.Errorf("isi = %q", got)
	}
}

func TestOpenChatTextWithoutTranscript(t *testing.T) {
	data, size := buildZip(t, map[string]string{"foto.jpg": "bukan teks"})

	if _, err := OpenChatText(bytes.NewReader(data), size); !errors.Is(err, ErrNoChatText) {
		t.Errorf("err = %v, want ErrNoChatText", err)
	}
}

func TestOpenChatTextIgnoresDirectoryComponents(t *testing.T) {
	// A zip entry may carry a path. Only the final element decides the match, and
	// a traversal-looking name must not change which file is opened.
	data, size := buildZip(t, map[string]string{
		"WhatsApp Chat - Andi/_chat.txt": "21/07/2026 03.15 - Andi: halo",
	})

	rc, err := OpenChatText(bytes.NewReader(data), size)
	if err != nil {
		t.Fatalf("OpenChatText: %v", err)
	}
	if got := readAll(t, rc); !strings.Contains(got, "Andi") {
		t.Errorf("isi = %q", got)
	}
}

func TestOpenChatTextRejectsNonZip(t *testing.T) {
	data := []byte("21/07/2026 03.15 - Andi: ini teks biasa, bukan zip")

	if _, err := OpenChatText(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Error("berkas non-zip diterima")
	}
}

func TestParseFromZipEndToEnd(t *testing.T) {
	transcript := "21/07/2026 03.15 - Andi: Halo\n" +
		"21/07/2026 03.16 - Budi: Hai\n"
	data, size := buildZip(t, map[string]string{"_chat.txt": transcript})

	rc, err := OpenChatText(bytes.NewReader(data), size)
	if err != nil {
		t.Fatalf("OpenChatText: %v", err)
	}
	defer rc.Close()

	res, err := Parse(rc, "Andi")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(res.Messages))
	}
	if res.Senders["Budi"] != 1 {
		t.Errorf("Senders = %v", res.Senders)
	}
}
