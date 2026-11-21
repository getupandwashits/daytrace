package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	appSupportName = "DayTrace"
	timezoneName   = "Asia/Shanghai"
)

// Beijing is the timezone every user-facing date and schedule uses.
var Beijing = func() *time.Location {
	location, err := time.LoadLocation(timezoneName)
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}()

func Home() string {
	if value := os.Getenv("DAYTRACE_HOME"); value != "" {
		return value
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", appSupportName)
}

func JournalDir() string  { return filepath.Join(Home(), "journal") }
func EventsDir() string   { return filepath.Join(JournalDir(), "events") }
func SessionsDir() string { return filepath.Join(Home(), "sessions") }
func SettingsPath() string {
	return filepath.Join(Home(), "settings.json")
}
func ExportStatePath() string {
	return filepath.Join(Home(), "export-state.json")
}
func StatusPath() string {
	return filepath.Join(Home(), "status.json")
}

// DefaultOutput picks where daily records go: an explicit DAYTRACE_OUTPUT
// wins, then the output_dir the previous version left in its settings (only
// present for people upgrading from it), then a plain per-user default.
func DefaultOutput() string {
	if value := os.Getenv("DAYTRACE_OUTPUT"); value != "" {
		return value
	}
	if value := legacyMonitorOutput(); value != "" {
		return value
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents", "DayTrace")
}

func legacyMonitorOutput() string {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, "Library", "Application Support", "MyMonitor", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return ""
	}
	value, _ := object["output_dir"].(string)
	if value == "" {
		return ""
	}
	if expanded, err := expandHome(value); err == nil {
		return expanded
	}
	return ""
}

func expandHome(value string) (string, error) {
	if len(value) >= 2 && value[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, value[2:]), nil
	}
	return value, nil
}

// LegacyStatusPaths returns the files AutoPomodoro's readers look at. The
// sandbox container comes first, matching scripts/read-status.py.
func LegacyStatusPaths() []string {
	if dir := os.Getenv("DAYTRACE_LEGACY_STATUS_DIR"); dir != "" {
		return []string{filepath.Join(dir, "status.json")}
	}
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, "Library", "Containers", "twickenhamma.AutoPomodoro",
			"Data", "Library", "Application Support", "AutoPomodoro", "status.json"),
		filepath.Join(home, "Library", "Application Support", "AutoPomodoro", "status.json"),
	}
}

func DayString(t time.Time) string {
	return t.In(Beijing).Format("2006-01-02")
}

func DayFile(t time.Time) string {
	return filepath.Join(EventsDir(), DayString(t)+".jsonl")
}

func StartOfDay(t time.Time) time.Time {
	local := t.In(Beijing)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, Beijing)
}

func EnsureDirectories() error {
	for _, dir := range []string{Home(), JournalDir(), EventsDir(), SessionsDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
