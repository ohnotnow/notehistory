package notehistory

import (
	"strings"
	"unicode"
)

// Notification is one recorded notification. Times are local-time strings in
// "2006-01-02 15:04:05" form, matching the existing history database.
type Notification struct {
	UUID        string
	DeliveredAt string
	App         string
	Title       string
	Subtitle    string
	Body        string
	RecordedAt  string
}

// Clean drops invisible format characters (email preheader padding and the
// like) and squashes runs of whitespace into single spaces.
func Clean(text string) string {
	var b strings.Builder
	for _, r := range text {
		if !unicode.Is(unicode.Cf, r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Line renders a notification the way the Python script printed it:
// time | app | title | subtitle | body, skipping empty parts.
func (n Notification) Line(appName string) string {
	parts := []string{}
	for _, p := range []string{Clean(n.Title), Clean(n.Subtitle), Clean(n.Body)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	when := n.DeliveredAt
	if when == "" {
		when = "?"
	}
	if appName == "" {
		appName = "?"
	}
	return when + " | " + appName + " | " + strings.Join(parts, " | ")
}
