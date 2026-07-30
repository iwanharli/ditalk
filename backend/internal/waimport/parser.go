// Package waimport parses WhatsApp "Export Chat" .txt files.
//
// This is the lowest-risk ingestion path (doc bab 30, Keputusan 1): it touches
// no unofficial API and the dataset stays fully under the user's control.
//
// Export format varies by platform and locale, so the parser detects the layout
// from the file itself rather than assuming one shape.
package waimport

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type MessageType string

const (
	TypeText     MessageType = "text"
	TypeImage    MessageType = "image"
	TypeAudio    MessageType = "audio"
	TypeVideo    MessageType = "video"
	TypeSticker  MessageType = "sticker"
	TypeDocument MessageType = "document"
	TypeUnknown  MessageType = "unknown"
)

type Message struct {
	Timestamp      time.Time
	Sender         string
	Text           string
	Type           MessageType
	AttachmentName string
	// IsSystem marks lines produced by WhatsApp itself (encryption notice, group
	// changes, missed calls) rather than by a participant.
	IsSystem bool
	// MediaOmitted means the export referenced media that was not included.
	MediaOmitted bool
	IsDeleted    bool
	IsEdited     bool
	LineNumber   int
}

type Result struct {
	Messages []Message
	// Senders counts messages per participant, so the caller can offer a choice
	// of which name is SELF instead of guessing.
	Senders map[string]int
	// DateOrder records the detected layout, useful for surfacing in the UI.
	DateOrder string
	Warnings  []string
}

// headerRe matches the timestamp prefix of a new message. It accepts:
//
//	[21/07/2026, 03.15.00] Name: text     (iOS, brackets)
//	21/07/2026 03.15 - Name: text         (Android)
//	7/21/26, 3:15 PM - Name: text         (US 12-hour)
var headerRe = regexp.MustCompile(
	`^\[?(\d{1,2})[/.\-](\d{1,2})[/.\-](\d{2,4}),?\s+(\d{1,2})[:.](\d{2})(?:[:.](\d{2}))?(?:\s*\x{202F}?\s*([APap][Mm]))?\]?\s*(?:-\s+)?(.*)$`,
)

// Locale-specific strings WhatsApp writes in place of real content.
var (
	mediaOmittedMarkers = []string{
		"<media omitted>",
		"<media tidak disertakan>",
		"image omitted",
		"video omitted",
		"audio omitted",
		"sticker omitted",
		"document omitted",
		"gif omitted",
		"stiker tidak disertakan",
	}
	deletedMarkers = []string{
		"this message was deleted",
		"you deleted this message",
		"pesan ini telah dihapus",
		"anda menghapus pesan ini",
	}
	editedMarkers = []string{
		"<this message was edited>",
		"<pesan ini telah diedit>",
		"<pesan diedit>",
	}
	attachmentSuffixes = []string{
		"(file attached)",
		"(file terlampir)",
	}
	// Lines matching these are informational, not messages from a participant.
	systemMarkers = []string{
		"messages and calls are end-to-end encrypted",
		"pesan dan panggilan terenkripsi secara end-to-end",
		"missed voice call",
		"missed video call",
		"panggilan suara tak terjawab",
		"panggilan video tak terjawab",
		"created group",
		"membuat grup",
		"added you",
		"menambahkan anda",
		"changed the subject",
		"mengubah subjek",
		"changed this group's icon",
		"mengubah ikon grup ini",
		"left",
		"keluar",
		"changed their phone number",
		"mengganti nomor teleponnya",
		"joined using this group's invite link",
		"bergabung menggunakan tautan undangan grup ini",
		"your security code changed",
		"kode keamanan anda berubah",
		"deleted this group",
		"menghapus grup ini",
	}
)

// Parse reads an export file. It runs in two passes: the first collects raw
// headers to decide whether dates are day-first or month-first, the second
// builds the messages. Guessing per-line would produce silently wrong dates.
func Parse(r io.Reader, selfName string) (*Result, error) {
	raw, err := collect(r)
	if err != nil {
		return nil, err
	}

	res := &Result{Senders: map[string]int{}}
	if len(raw) == 0 {
		res.DateOrder = "unknown"
		res.Warnings = append(res.Warnings, "tidak ada baris pesan yang dikenali")
		return res, nil
	}

	order, warn := detectDateOrder(raw)
	res.DateOrder = order
	if warn != "" {
		res.Warnings = append(res.Warnings, warn)
	}

	for _, rl := range raw {
		msg, err := rl.toMessage(order)
		if err != nil {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("baris %d dilewati: %v", rl.line, err))
			continue
		}
		if msg.Sender != "" {
			res.Senders[msg.Sender]++
		}
		res.Messages = append(res.Messages, msg)
	}

	if selfName != "" && res.Senders[selfName] == 0 {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("nama %q tidak ditemukan di antara pengirim", selfName))
	}

	return res, nil
}

type rawLine struct {
	day, month, year int
	hour, minute     int
	second           int
	meridiem         string
	body             string
	line             int
}

// collect groups physical lines into logical messages. A line without a
// timestamp header is a continuation of the previous message.
func collect(r io.Reader) ([]rawLine, error) {
	sc := bufio.NewScanner(r)
	// Exported chats can contain very long single messages.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var out []rawLine
	lineNo := 0

	for sc.Scan() {
		lineNo++
		text := sanitize(sc.Text())

		m := headerRe.FindStringSubmatch(text)
		if m == nil {
			if len(out) > 0 && strings.TrimSpace(text) != "" {
				out[len(out)-1].body += "\n" + text
			}
			continue
		}

		first, _ := strconv.Atoi(m[1])
		second, _ := strconv.Atoi(m[2])
		year, _ := strconv.Atoi(m[3])
		hour, _ := strconv.Atoi(m[4])
		minute, _ := strconv.Atoi(m[5])
		sec := 0
		if m[6] != "" {
			sec, _ = strconv.Atoi(m[6])
		}

		out = append(out, rawLine{
			day: first, month: second, year: year,
			hour: hour, minute: minute, second: sec,
			meridiem: strings.ToUpper(m[7]),
			body:     m[8],
			line:     lineNo,
		})
	}

	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	return out, nil
}

// sanitize removes the bidirectional and narrow-space marks WhatsApp injects,
// which otherwise break both regex matching and sender comparison.
func sanitize(s string) string {
	s = strings.TrimPrefix(s, "\ufeff") // BYTE ORDER MARK
	s = strings.NewReplacer(
		"\u200e", "", // LEFT-TO-RIGHT MARK
		"\u200f", "", // RIGHT-TO-LEFT MARK
		"\u00a0", " ", // NO-BREAK SPACE
		"\u202f", " ", // NARROW NO-BREAK SPACE
		"\r", "",
	).Replace(s)
	return s
}

// detectDateOrder decides between day-first and month-first. A component above
// 12 is decisive; without one the format is genuinely ambiguous and we fall
// back to day-first, which covers Indonesian and most non-US exports.
func detectDateOrder(lines []rawLine) (string, string) {
	firstOver12, secondOver12 := false, false
	for _, l := range lines {
		if l.day > 12 {
			firstOver12 = true
		}
		if l.month > 12 {
			secondOver12 = true
		}
	}

	switch {
	case firstOver12 && secondOver12:
		return "day-first", "tanggal tidak konsisten: kedua posisi melebihi 12; memakai hari-dahulu"
	case firstOver12:
		return "day-first", ""
	case secondOver12:
		return "month-first", ""
	default:
		return "day-first", "format tanggal ambigu (tidak ada komponen di atas 12); diasumsikan hari-dahulu"
	}
}

func (rl rawLine) toMessage(order string) (Message, error) {
	day, month := rl.day, rl.month
	if order == "month-first" {
		day, month = rl.month, rl.day
	}

	year := rl.year
	if year < 100 {
		year += 2000
	}

	hour := rl.hour
	switch rl.meridiem {
	case "PM":
		if hour < 12 {
			hour += 12
		}
	case "AM":
		if hour == 12 {
			hour = 0
		}
	}

	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || rl.minute > 59 {
		return Message{}, fmt.Errorf("tanggal/waktu tidak valid: %02d/%02d/%d %02d:%02d",
			day, month, year, hour, rl.minute)
	}

	ts := time.Date(year, time.Month(month), day, hour, rl.minute, rl.second, 0, time.Local)
	// time.Date normalizes out-of-range days (Feb 31 -> Mar 3), which would
	// silently move a message; reject instead.
	if ts.Day() != day || int(ts.Month()) != month {
		return Message{}, fmt.Errorf("tanggal tidak ada dalam kalender: %02d/%02d/%d", day, month, year)
	}

	msg := Message{Timestamp: ts, Type: TypeText, LineNumber: rl.line}

	sender, body, ok := splitSender(rl.body)
	if !ok {
		msg.IsSystem = true
		msg.Text = strings.TrimSpace(rl.body)
		return msg, nil
	}

	msg.Sender = sender
	msg.Text = strings.TrimSpace(body)

	if isSystemText(msg.Text) {
		msg.IsSystem = true
	}

	classify(&msg)
	return msg, nil
}

// splitSender separates "Name: text". Group system lines have no colon, and a
// message body may itself contain colons, so only the first one counts.
func splitSender(body string) (sender, text string, ok bool) {
	idx := strings.Index(body, ": ")
	if idx <= 0 {
		// A line ending exactly with "Name:" and empty text is still a message.
		if strings.HasSuffix(body, ":") && len(body) > 1 && !strings.Contains(body, " ") {
			return strings.TrimSuffix(body, ":"), "", true
		}
		return "", body, false
	}

	sender = strings.TrimSpace(body[:idx])
	// Sender names never contain newlines; a colon appearing after one belongs
	// to the message text of a system line.
	if sender == "" || strings.Contains(sender, "\n") {
		return "", body, false
	}
	return sender, body[idx+2:], true
}

func isSystemText(text string) bool {
	lower := strings.ToLower(text)
	for _, m := range systemMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// classify infers message type and status flags from locale-specific markers.
func classify(msg *Message) {
	lower := strings.ToLower(msg.Text)

	for _, m := range deletedMarkers {
		if strings.Contains(lower, m) {
			msg.IsDeleted = true
			return
		}
	}

	for _, m := range editedMarkers {
		if strings.Contains(lower, m) {
			msg.IsEdited = true
			// Strip the marker so it does not pollute analysis text.
			if i := strings.LastIndex(lower, m); i >= 0 {
				msg.Text = strings.TrimSpace(msg.Text[:i])
			}
			break
		}
	}

	for _, m := range mediaOmittedMarkers {
		if strings.Contains(lower, m) {
			msg.MediaOmitted = true
			msg.Type = typeFromMarker(m)
			return
		}
	}

	for _, sfx := range attachmentSuffixes {
		if !strings.HasSuffix(lower, sfx) {
			continue
		}
		name := strings.TrimSpace(msg.Text[:len(msg.Text)-len(sfx)])
		msg.AttachmentName = name
		msg.Type = typeFromFilename(name)
		return
	}
}

func typeFromMarker(marker string) MessageType {
	switch {
	case strings.HasPrefix(marker, "image"):
		return TypeImage
	case strings.HasPrefix(marker, "video"), strings.HasPrefix(marker, "gif"):
		return TypeVideo
	case strings.HasPrefix(marker, "audio"):
		return TypeAudio
	case strings.HasPrefix(marker, "sticker"), strings.HasPrefix(marker, "stiker"):
		return TypeSticker
	case strings.HasPrefix(marker, "document"):
		return TypeDocument
	default:
		// "<Media omitted>" does not say which kind.
		return TypeUnknown
	}
}

func typeFromFilename(name string) MessageType {
	lower := strings.ToLower(name)
	ext := ""
	if i := strings.LastIndex(lower, "."); i >= 0 {
		ext = lower[i:]
	}

	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".heic":
		if strings.Contains(lower, "sticker") || strings.Contains(lower, "stk") {
			return TypeSticker
		}
		return TypeImage
	case ".opus", ".ogg", ".mp3", ".m4a", ".aac", ".wav":
		return TypeAudio
	case ".mp4", ".mkv", ".avi", ".mov", ".3gp":
		return TypeVideo
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".zip":
		return TypeDocument
	default:
		return TypeUnknown
	}
}
