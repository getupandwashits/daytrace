package core

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// LegacyStatusWriter publishes the AutoPomodoro live status file that external
// readers such as scripts/read-status.py and the keyboard dot-matrix service
// consume. It backs off while the old app is still running.
type LegacyStatusWriter struct {
	mu       sync.Mutex
	URLs     []string
	Isolated bool

	CheckRunningApplication bool
	Clock                   func() time.Time
	runningCheck            func() bool
}

func NewLegacyStatusWriter() *LegacyStatusWriter {
	return &LegacyStatusWriter{
		URLs:                    LegacyStatusPaths(),
		Isolated:                os.Getenv("DAYTRACE_LEGACY_STATUS_DIR") != "",
		CheckRunningApplication: true,
		Clock:                   time.Now,
		runningCheck:            autoPomodoroRunning,
	}
}

func (w *LegacyStatusWriter) now() time.Time {
	if w.Clock != nil {
		return w.Clock()
	}
	return time.Now()
}

func (w *LegacyStatusWriter) OwnedByAnotherInstance() bool {
	return w.ownedByAnotherInstance(w.now())
}

func (w *LegacyStatusWriter) ownedByAnotherInstance(now time.Time) bool {
	if w.CheckRunningApplication && !w.Isolated && w.runningCheck != nil && w.runningCheck() {
		return true
	}
	for _, path := range w.URLs {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var status LegacyLiveStatus
		if json.Unmarshal(data, &status) != nil {
			continue
		}
		if status.Source != "autopomodoro" {
			continue
		}
		if math.Abs(now.Sub(status.UpdatedAt).Seconds()) > 5 {
			continue
		}
		if status.ProcessID == os.Getpid() {
			continue
		}
		if processAlive(status.ProcessID) {
			return true
		}
	}
	return false
}

// Write publishes to every legacy path. Existing files from another live
// instance are never overwritten.
func (w *LegacyStatusWriter) Write(status LegacyLiveStatus) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ownedByAnotherInstance(w.now()) {
		return nil
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	var lastErr error
	for _, path := range w.URLs {
		if err := writeAtomic(path, data); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".status-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(name)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// MarshalJSON keeps the legacy timestamp in the same second-precision ISO 8601
// shape the original app uses.
func (s LegacyLiveStatus) MarshalJSON() ([]byte, error) {
	type wire struct {
		SchemaVersion         int     `json:"schemaVersion"`
		Source                string  `json:"source"`
		ProcessID             int     `json:"processID"`
		UpdatedAt             string  `json:"updatedAt"`
		State                 string  `json:"state"`
		StateLabel            string  `json:"stateLabel"`
		FocusElapsedSeconds   float64 `json:"focusElapsedSeconds"`
		FocusTargetSeconds    float64 `json:"focusTargetSeconds"`
		FocusRemainingSeconds float64 `json:"focusRemainingSeconds"`
		RestRemainingSeconds  float64 `json:"restRemainingSeconds"`
		RestTargetSeconds     float64 `json:"restTargetSeconds"`
		RemainingFraction     float64 `json:"remainingFraction"`
		TrackingSuspended     bool    `json:"trackingSuspended"`
		Mode                  string  `json:"mode"`
		Phase                 string  `json:"phase"`
		CompletedPomodoros    int     `json:"completedPomodoros"`
		PomodorosPerCycle     int     `json:"pomodorosPerCycle"`
	}
	return json.Marshal(wire{
		SchemaVersion:         s.SchemaVersion,
		Source:                s.Source,
		ProcessID:             s.ProcessID,
		UpdatedAt:             s.UpdatedAt.In(Beijing).Format("2006-01-02T15:04:05-07:00"),
		State:                 s.State,
		StateLabel:            s.StateLabel,
		FocusElapsedSeconds:   s.FocusElapsedSeconds,
		FocusTargetSeconds:    s.FocusTargetSeconds,
		FocusRemainingSeconds: s.FocusRemainingSeconds,
		RestRemainingSeconds:  s.RestRemainingSeconds,
		RestTargetSeconds:     s.RestTargetSeconds,
		RemainingFraction:     s.RemainingFraction,
		TrackingSuspended:     s.TrackingSuspended,
		Mode:                  s.Mode,
		Phase:                 s.Phase,
		CompletedPomodoros:    s.CompletedPomodoros,
		PomodorosPerCycle:     s.PomodorosPerCycle,
	})
}
