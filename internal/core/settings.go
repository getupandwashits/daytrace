package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	AutomaticFocusMin float64  `json:"automaticFocusMinutes"`
	PauseToleranceMin float64  `json:"pauseToleranceMinutes"`
	ResetThresholdMin float64  `json:"resetThresholdMinutes"`
	RestMin           float64  `json:"restMinutes"`
	MinimumRecordMin  float64  `json:"minimumRecordMinutes"`
	SampleIntervalSec float64  `json:"sampleIntervalSeconds"`
	IdleThresholdSec  float64  `json:"idleThresholdSeconds"`
	UIMaxChars        int      `json:"uiMaxChars"`
	ExcludedApps      []string `json:"excludedApps"`
	CollectActivity   bool     `json:"collectActivity"`
	ExportHour        int      `json:"exportHour"`
	ExportMinute      int      `json:"exportMinute"`
	AutoExport        bool     `json:"autoExport"`
	OutputDirectory   string   `json:"outputDirectory"`
}

func DefaultSettings() Settings {
	return Settings{
		AutomaticFocusMin: 25,
		PauseToleranceMin: 3,
		ResetThresholdMin: 10,
		RestMin:           5,
		MinimumRecordMin:  5,
		SampleIntervalSec: 10,
		IdleThresholdSec:  180,
		UIMaxChars:        16000,
		CollectActivity:   false,
		ExportHour:        23,
		ExportMinute:      0,
		AutoExport:        true,
		OutputDirectory:   DefaultOutput(),
	}
}

func (s Settings) PauseTolerance() float64 {
	return max(30, s.PauseToleranceMin*60)
}

func (s Settings) ResetThreshold() float64 {
	return max(s.PauseTolerance()+30, s.ResetThresholdMin*60)
}

// RestDuration is how long the rest phase counts down for. It is shown in the
// menu bar and published as restRemainingSeconds.
func (s Settings) RestDuration() float64 {
	return max(60, s.RestMin*60)
}

func (s Settings) MinimumRecordDuration() float64 {
	return max(30, s.MinimumRecordMin*60)
}

type SettingsStore struct {
	mu       sync.Mutex
	path     string
	settings Settings
}

func NewSettingsStore(path string) *SettingsStore {
	store := &SettingsStore{path: path, settings: DefaultSettings()}
	store.reload()
	return store
}

func (s *SettingsStore) reload() {
	data, err := os.ReadFile(s.path)
	if err == nil {
		// Decode on top of defaults so missing keys keep their default value.
		merged := DefaultSettings()
		if json.Unmarshal(data, &merged) == nil {
			s.settings = merged
		}
	}
}

func (s *SettingsStore) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := s.settings
	if override := os.Getenv("DAYTRACE_OUTPUT"); override != "" {
		value.OutputDirectory = override
	}
	return value
}

func (s *SettingsStore) Save(value Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if os.Getenv("DAYTRACE_OUTPUT") != "" {
		value.OutputDirectory = s.settings.OutputDirectory
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeFileAtomic(s.path, data); err != nil {
		return err
	}
	s.settings = value
	return nil
}
