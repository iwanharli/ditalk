package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// ContentHash identifies a message by what it is rather than by which path
// delivered it.
//
// An exported chat carries no message ids, so import has to invent one, while
// live sync uses the real WhatsApp id. Without a shared identity the same
// message is stored twice whenever an import covers a period already synced.
//
// message_type is deliberately excluded. The same media message is described
// differently by each path — a caption on one side, a filename on the other —
// and including the type would stop those from ever matching while adding
// nothing for plain text, which is the case that actually overlaps.
func ContentHash(ts time.Time, senderRole, text string) string {
	trimmed := strings.TrimSpace(text)
	// An empty message cannot be identified by its content, so it gets no hash
	// and simply does not participate in deduplication.
	if trimmed == "" {
		return ""
	}

	h := sha256.New()
	h.Write([]byte(strconv.FormatInt(ts.Unix(), 10)))
	h.Write([]byte{0})
	h.Write([]byte(senderRole))
	h.Write([]byte{0})
	h.Write([]byte(trimmed))

	return hex.EncodeToString(h.Sum(nil))
}
