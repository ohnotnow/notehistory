package notehistory

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"howett.net/plist"
)

// AppNames turns bundle identifiers like com.microsoft.teams2 into something
// readable. It asks Spotlight for the installed app and reads its Info.plist;
// if that fails it falls back to a title-cased guess from the identifier.
// Lookups are cached for the life of the process.
type AppNames struct {
	mu    sync.Mutex
	cache map[string]string
}

// NewAppNames returns an empty cache.
func NewAppNames() *AppNames {
	return &AppNames{cache: map[string]string{}}
}

// Name returns the display name for a bundle identifier.
func (a *AppNames) Name(identifier string) string {
	if identifier == "" {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if name, ok := a.cache[identifier]; ok {
		return name
	}
	name := installedAppName(identifier)
	if name == "" {
		name = GuessAppName(identifier)
	}
	a.cache[identifier] = name
	return name
}

func installedAppName(identifier string) string {
	out, err := exec.Command("mdfind", "kMDItemCFBundleIdentifier == '"+identifier+"'c").Output()
	if err != nil {
		return ""
	}
	for _, app := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasSuffix(app, ".app") {
			continue
		}
		if name := bundleName(app); name != "" {
			return name
		}
		return strings.TrimSuffix(filepath.Base(app), ".app")
	}
	return ""
}

func bundleName(app string) string {
	data, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return ""
	}
	var info struct {
		DisplayName string `plist:"CFBundleDisplayName"`
		Name        string `plist:"CFBundleName"`
	}
	if _, err := plist.Unmarshal(data, &info); err != nil {
		return ""
	}
	if info.DisplayName != "" {
		return info.DisplayName
	}
	return info.Name
}

// GuessAppName makes a readable name from a bundle identifier alone:
// com.microsoft.teams2 becomes "Microsoft Teams 2".
func GuessAppName(identifier string) string {
	parts := strings.Split(identifier, ".")
	if len(parts) > 1 {
		parts = parts[1:] // drop the com/org/uk
	}
	var words []string
	for _, p := range parts {
		if p == "" {
			continue
		}
		words = append(words, titleWord(p))
	}
	return strings.Join(words, " ")
}

// titleWord capitalises a word and splits any trailing digits off with a
// space: "teams2" becomes "Teams 2".
func titleWord(w string) string {
	runes := []rune(w)
	runes[0] = unicode.ToUpper(runes[0])
	end := len(runes)
	for end > 1 && unicode.IsDigit(runes[end-1]) {
		end--
	}
	if end == len(runes) {
		return string(runes)
	}
	return string(runes[:end]) + " " + string(runes[end:])
}
