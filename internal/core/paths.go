package core

import (
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

func DefaultOutput() string { return filepath.Join(Home(), "archives") }

func ArchiveDirectory(settings Settings) (string, error) {
	path := settings.OutputDirectory
	if path == "" {
		path = DefaultOutput()
	}
	expanded, err := expandHome(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(expanded)
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
