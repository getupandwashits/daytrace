package core

import (
	"encoding/json"
	"time"
)

type ActivitySample struct {
	Timestamp          time.Time `json:"timestamp"`
	State              string    `json:"state"`
	IdleSeconds        float64   `json:"idle_seconds,omitempty"`
	SampleInterval     float64   `json:"sample_interval_seconds,omitempty"`
	IdleThreshold      float64   `json:"idle_threshold_seconds,omitempty"`
	Accessibility      bool      `json:"accessibility,omitempty"`
	App                string    `json:"app,omitempty"`
	BundleID           string    `json:"bundle_id,omitempty"`
	WindowTitle        string    `json:"window_title,omitempty"`
	UIText             *string   `json:"ui_text,omitempty"`
	UITextUnchanged    bool      `json:"ui_text_unchanged,omitempty"`
	ContextID          string    `json:"context_id,omitempty"`
	URL                string    `json:"url,omitempty"`
	Document           string    `json:"document,omitempty"`
	CaptureStatus      string    `json:"capture_status,omitempty"`
	UITruncated        bool      `json:"ui_truncated,omitempty"`
	UITruncatedReasons []string  `json:"ui_truncated_reason,omitempty"`
	SecureFieldSkipped bool      `json:"secure_field_skipped,omitempty"`
}

// MarshalJSON writes the same key shape and second-precision timestamp the
// Swift prototype stores in the journal.
func (s ActivitySample) MarshalJSON() ([]byte, error) {
	type wire struct {
		Timestamp          string   `json:"timestamp"`
		State              string   `json:"state"`
		IdleSeconds        float64  `json:"idle_seconds,omitempty"`
		SampleInterval     float64  `json:"sample_interval_seconds,omitempty"`
		IdleThreshold      float64  `json:"idle_threshold_seconds,omitempty"`
		Accessibility      bool     `json:"accessibility,omitempty"`
		App                string   `json:"app,omitempty"`
		BundleID           string   `json:"bundle_id,omitempty"`
		WindowTitle        string   `json:"window_title,omitempty"`
		UIText             *string  `json:"ui_text,omitempty"`
		UITextUnchanged    bool     `json:"ui_text_unchanged,omitempty"`
		ContextID          string   `json:"context_id,omitempty"`
		URL                string   `json:"url,omitempty"`
		Document           string   `json:"document,omitempty"`
		CaptureStatus      string   `json:"capture_status,omitempty"`
		UITruncated        bool     `json:"ui_truncated,omitempty"`
		UITruncatedReasons []string `json:"ui_truncated_reason,omitempty"`
		SecureFieldSkipped bool     `json:"secure_field_skipped,omitempty"`
	}
	return json.Marshal(wire{
		Timestamp:          s.Timestamp.In(Beijing).Format("2006-01-02T15:04:05-07:00"),
		State:              s.State,
		IdleSeconds:        s.IdleSeconds,
		SampleInterval:     s.SampleInterval,
		IdleThreshold:      s.IdleThreshold,
		Accessibility:      s.Accessibility,
		App:                s.App,
		BundleID:           s.BundleID,
		WindowTitle:        s.WindowTitle,
		UIText:             s.UIText,
		UITextUnchanged:    s.UITextUnchanged,
		ContextID:          s.ContextID,
		URL:                s.URL,
		Document:           s.Document,
		CaptureStatus:      s.CaptureStatus,
		UITruncated:        s.UITruncated,
		UITruncatedReasons: s.UITruncatedReasons,
		SecureFieldSkipped: s.SecureFieldSkipped,
	})
}

func (s ActivitySample) Title() string { return s.WindowTitle }

func (s ActivitySample) AppName() string {
	if s.App != "" {
		return s.App
	}
	return s.StateLabel()
}

func (s ActivitySample) StateLabel() string {
	switch s.State {
	case "active":
		return "活跃"
	case "idle":
		return "无键鼠输入"
	case "locked":
		return "锁屏"
	case "sleeping":
		return "睡眠"
	case "excluded":
		return "排除的应用"
	default:
		return "未取得窗口"
	}
}

type FocusSession struct {
	ID       string    `json:"id"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Duration float64   `json:"duration"`
	Trigger  string    `json:"trigger"`
}

type StatusSnapshot struct {
	SchemaVersion         int       `json:"schemaVersion"`
	Source                string    `json:"source"`
	ProcessID             int       `json:"processID"`
	UpdatedAt             time.Time `json:"updatedAt"`
	State                 string    `json:"state"`
	StateLabel            string    `json:"stateLabel"`
	Mode                  string    `json:"mode"`
	FocusElapsedSeconds   float64   `json:"focusElapsedSeconds"`
	FocusTargetSeconds    float64   `json:"focusTargetSeconds"`
	FocusRemainingSeconds float64   `json:"focusRemainingSeconds"`
	RestRemainingSeconds  float64   `json:"restRemainingSeconds"`
	RestTargetSeconds     float64   `json:"restTargetSeconds"`
	RemainingFraction     float64   `json:"remainingFraction"`
	TrackingSuspended     bool      `json:"trackingSuspended"`
	CompletedPomodoros    int       `json:"completedPomodoros"`
	PomodorosPerCycle     int       `json:"pomodorosPerCycle"`
}

// MarshalJSON matches the Swift encoder's .iso8601 strategy: UTC, second
// precision, trailing Z.
func (s StatusSnapshot) MarshalJSON() ([]byte, error) {
	type wire struct {
		SchemaVersion         int     `json:"schemaVersion"`
		Source                string  `json:"source"`
		ProcessID             int     `json:"processID"`
		UpdatedAt             string  `json:"updatedAt"`
		State                 string  `json:"state"`
		StateLabel            string  `json:"stateLabel"`
		Mode                  string  `json:"mode"`
		FocusElapsedSeconds   float64 `json:"focusElapsedSeconds"`
		FocusTargetSeconds    float64 `json:"focusTargetSeconds"`
		FocusRemainingSeconds float64 `json:"focusRemainingSeconds"`
		RestRemainingSeconds  float64 `json:"restRemainingSeconds"`
		RestTargetSeconds     float64 `json:"restTargetSeconds"`
		RemainingFraction     float64 `json:"remainingFraction"`
		TrackingSuspended     bool    `json:"trackingSuspended"`
		CompletedPomodoros    int     `json:"completedPomodoros"`
		PomodorosPerCycle     int     `json:"pomodorosPerCycle"`
	}
	return json.Marshal(wire{
		SchemaVersion:         s.SchemaVersion,
		Source:                s.Source,
		ProcessID:             s.ProcessID,
		UpdatedAt:             s.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		State:                 s.State,
		StateLabel:            s.StateLabel,
		Mode:                  s.Mode,
		FocusElapsedSeconds:   s.FocusElapsedSeconds,
		FocusTargetSeconds:    s.FocusTargetSeconds,
		FocusRemainingSeconds: s.FocusRemainingSeconds,
		RestRemainingSeconds:  s.RestRemainingSeconds,
		RestTargetSeconds:     s.RestTargetSeconds,
		RemainingFraction:     s.RemainingFraction,
		TrackingSuspended:     s.TrackingSuspended,
		CompletedPomodoros:    s.CompletedPomodoros,
		PomodorosPerCycle:     s.PomodorosPerCycle,
	})
}

type LegacyLiveStatus struct {
	SchemaVersion         int       `json:"schemaVersion"`
	Source                string    `json:"source"`
	ProcessID             int       `json:"processID"`
	UpdatedAt             time.Time `json:"updatedAt"`
	State                 string    `json:"state"`
	StateLabel            string    `json:"stateLabel"`
	FocusElapsedSeconds   float64   `json:"focusElapsedSeconds"`
	FocusTargetSeconds    float64   `json:"focusTargetSeconds"`
	FocusRemainingSeconds float64   `json:"focusRemainingSeconds"`
	RestRemainingSeconds  float64   `json:"restRemainingSeconds"`
	RestTargetSeconds     float64   `json:"restTargetSeconds"`
	RemainingFraction     float64   `json:"remainingFraction"`
	TrackingSuspended     bool      `json:"trackingSuspended"`
	Mode                  string    `json:"mode"`
	Phase                 string    `json:"phase"`
	CompletedPomodoros    int       `json:"completedPomodoros"`
	PomodorosPerCycle     int       `json:"pomodorosPerCycle"`
}

type ExportState struct {
	LastExportAt   time.Time `json:"lastExportAt,omitzero"`
	LastExportEnd  time.Time `json:"lastExportEnd,omitzero"`
	LastExportFile string    `json:"lastExportFile,omitempty"`
}

type AppDuration struct {
	Name    string  `json:"name"`
	Seconds float64 `json:"seconds"`
}

type HourUsage struct {
	Hour time.Time     `json:"hour"`
	Apps []AppDuration `json:"apps"`
}

type ActivitySummary struct {
	Samples       int           `json:"samples"`
	ActiveSeconds float64       `json:"activeSeconds"`
	First         *time.Time    `json:"first,omitempty"`
	Last          *time.Time    `json:"last,omitempty"`
	Apps          []AppDuration `json:"apps"`
	Hours         []HourUsage   `json:"hours"`
}

type ExportResult struct {
	EntryURL    string   `json:"entryURL,omitempty"`
	Written     []string `json:"written,omitempty"`
	SampleCount int      `json:"sampleCount"`
	Message     string   `json:"message"`
}
