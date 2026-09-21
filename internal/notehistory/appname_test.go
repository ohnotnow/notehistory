package notehistory

import (
	"strings"
	"testing"
)

func TestGuessAppName(t *testing.T) {
	cases := map[string]string{
		"com.microsoft.teams2":       "Microsoft Teams 2",
		"com.apple.mail":             "Apple Mail",
		"com.apple.scripteditor2":    "Apple Scripteditor 2",
		"com.electron.dockerdesktop": "Electron Dockerdesktop",
		"single":                     "Single",
	}
	for in, want := range cases {
		if got := GuessAppName(in); got != want {
			t.Errorf("GuessAppName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClean(t *testing.T) {
	in := "Hello‌‌   world\n\t again"
	if got := Clean(in); got != "Hello world again" {
		t.Errorf("Clean = %q", got)
	}
}

func TestMatches(t *testing.T) {
	n := Notification{App: "com.microsoft.teams2", Title: "Standup", Body: "Starting in 5 minutes"}
	cases := []struct {
		filter string
		want   bool
	}{
		{"", true},
		{"teams", true},
		{"Microsoft standup", true},
		{"minutes standup", true},
		{"slack", false},
		{"standup slack", false},
	}
	for _, c := range cases {
		words := strings.Fields(strings.ToLower(c.filter))
		if got := matches(n, "Microsoft Teams", words); got != c.want {
			t.Errorf("matches(%q) = %v, want %v", c.filter, got, c.want)
		}
	}
}
