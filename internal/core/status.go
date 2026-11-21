package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WriteStatus publishes DayTrace's own live status, used by the dashboard and
// the debug CLI.
func (s StatusSnapshot) WriteTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data)
}

func ReadStatus(path string) (StatusSnapshot, error) {
	var status StatusSnapshot
	data, err := os.ReadFile(path)
	if err != nil {
		return status, err
	}
	err = json.Unmarshal(data, &status)
	return status, err
}
