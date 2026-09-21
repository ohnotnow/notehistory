package notehistory

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// envVars are the environment variables the tool reads. install copies any
// that are set into the launchd plist so the background watcher sees them.
var envVars = []string{"NOTIFICATION_DB", "NOTIFICATION_HISTORY", "NOTIFICATION_KEEP_DAYS"}

// defaultKeepDays is how long notifications are kept when
// NOTIFICATION_KEEP_DAYS is not set.
const defaultKeepDays = 365

// Version is stamped at release time via -ldflags.
var Version = "dev"

// Config holds the two file paths the tool works with.
type Config struct {
	// SourceDB is Notification Centre's live database.
	SourceDB string
	// HistoryDB is our own copy.
	HistoryDB string
	// KeepDays is how long to keep notifications; 0 keeps them forever.
	KeepDays int
}

// LoadConfig returns the default paths, honouring the same environment
// overrides as the original Python script.
func LoadConfig() Config {
	home, _ := os.UserHomeDir()
	cfg := Config{
		SourceDB:  filepath.Join(home, "Library/Group Containers/group.com.apple.usernoted/db2/db"),
		HistoryDB: filepath.Join(home, ".local/share/notification-history.db"),
		KeepDays:  defaultKeepDays,
	}
	if v := os.Getenv("NOTIFICATION_DB"); v != "" {
		cfg.SourceDB = v
	}
	if v := os.Getenv("NOTIFICATION_HISTORY"); v != "" {
		cfg.HistoryDB = v
	}
	if v := os.Getenv("NOTIFICATION_KEEP_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.KeepDays = n
		} else {
			fmt.Fprintf(os.Stderr, "ignoring NOTIFICATION_KEEP_DAYS=%q: expected a whole number of days\n", v)
		}
	}
	return cfg
}
