package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Exporter struct {
	journal  *Journal
	settings *SettingsStore
	state    string
	sessions *SessionStore
}

func NewExporter(journal *Journal, settings *SettingsStore, statePath string) *Exporter {
	if journal == nil {
		journal = NewJournal(EventsDir())
	}
	if settings == nil {
		settings = NewSettingsStore(SettingsPath())
	}
	if statePath == "" {
		statePath = ExportStatePath()
	}
	return &Exporter{
		journal: journal, settings: settings, state: statePath,
		sessions: NewSessionStore(filepath.Join(filepath.Dir(statePath), "sessions")),
	}
}

func (e *Exporter) LoadState() ExportState {
	state, _ := e.readState()
	return state
}

func (e *Exporter) readState() (ExportState, error) {
	data, err := os.ReadFile(e.state)
	if os.IsNotExist(err) {
		return ExportState{}, nil
	}
	if err != nil {
		return ExportState{}, err
	}
	var state ExportState
	if err := json.Unmarshal(data, &state); err != nil {
		return ExportState{}, err
	}
	if state.Version == 0 {
		// A timestamp from older builds cannot prove which records were delivered.
		state.LastExportEnd = time.Time{}
	}
	return state, nil
}

func (e *Exporter) SaveState(state ExportState) error {
	if err := os.MkdirAll(filepath.Dir(e.state), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(e.state, append(data, '\n')); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(e.state))
}

func (e *Exporter) ScheduledCheckpoint(now time.Time) time.Time {
	return scheduledCheckpoint(e.settings.Get(), now)
}

func scheduledCheckpoint(settings Settings, now time.Time) time.Time {
	local := now.In(Beijing)
	hour := min(23, max(0, settings.ExportHour))
	minute := min(59, max(0, settings.ExportMinute))
	today := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, Beijing)
	if !now.Before(today) {
		return today
	}
	return today.AddDate(0, 0, -1)
}

func (e *Exporter) IsDue(now time.Time) bool {
	settings := e.settings.Get()
	state := e.LoadState()
	return settings.AutoExport && (state.Pending != nil || state.LastExportEnd.Before(scheduledCheckpoint(settings, now)))
}

func (e *Exporter) ExportDue(now time.Time) ExportResult {
	return e.run(now, false, false)
}

func (e *Exporter) ExportNow(now time.Time, force bool) ExportResult {
	return e.run(now, true, force)
}

type pendingExport struct {
	Staging     string      `json:"staging"`
	Destination string      `json:"destination"`
	Next        ExportState `json:"next"`
}

func (e *Exporter) run(now time.Time, manual, force bool) ExportResult {
	if err := os.MkdirAll(filepath.Dir(e.state), 0o700); err != nil {
		return exportError(err)
	}
	var result ExportResult
	err := withFileLock(e.state+".lock", func() error {
		state, err := e.readState()
		if err != nil {
			return err
		}
		settings := e.settings.Get()
		if !manual && !settings.AutoExport {
			result.Message = "自动归档已关闭"
			return nil
		}
		if pending := state.Pending; pending != nil {
			output, err := ArchiveDirectory(settings)
			if err != nil {
				return err
			}
			if output == filepath.Dir(pending.Destination) {
				recovered, err := e.recoverArchive(&state)
				if err != nil {
					return err
				}
				if recovered != "" {
					result = ExportResult{
						EntryURL: filepath.Join(recovered, "index.md"),
						Message:  "已恢复归档 " + filepath.Base(recovered),
					}
					return nil
				}
			} else {
				// The user moved the output. Start from committed cursors
				// and leave any files at the previous location alone.
				state.Pending = nil
			}
		}
		cutoff := now
		next := state
		next.Version = 1
		if !manual {
			cutoff = scheduledCheckpoint(settings, now)
			if !state.LastExportEnd.Before(cutoff) {
				result.Message = "未到下一次归档时间"
				return nil
			}
			next.LastExportEnd = cutoff
		}

		sessionCursor := state.Sessions
		if force {
			sessionCursor = logCursor{}
		}
		var samples []ActivitySample
		var cursors map[string]logCursor
		if force {
			path := filepath.Join(e.journal.Root(), DayString(now)+".jsonl")
			samples, _, err = readLog[ActivitySample](path, logCursor{}, cutoff, sampleTime)
		} else {
			samples, cursors, err = e.journal.pending(state.Journal, cutoff)
		}
		if err != nil {
			return fmt.Errorf("读取采集记录: %w", err)
		}
		sessions, sessionCursor, err := e.sessions.pending(sessionCursor, cutoff)
		if err != nil {
			return fmt.Errorf("读取专注记录: %w", err)
		}
		if force {
			sessions = SessionsOn(sessions, now)
		} else {
			next.Journal, next.Sessions = cursors, sessionCursor
		}
		if len(samples) == 0 && len(sessions) == 0 {
			result.Message = "没有待归档记录"
			if !manual {
				return e.SaveState(next)
			}
			return nil
		}

		output, err := ArchiveDirectory(settings)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(output, 0o700); err != nil {
			return err
		}
		return withFileLock(filepath.Join(output, ".daytrace.lock"), func() error {
			destination, err := archiveDestination(output, cutoff, manual)
			if err != nil {
				return err
			}
			staging, err := os.MkdirTemp(output, ".daytrace-")
			if err != nil {
				return err
			}
			reserved := false
			defer func() {
				if !reserved {
					_ = os.RemoveAll(staging)
				}
			}()

			written, err := e.renderArchive(staging, samples, sessions, cutoff, now)
			if err != nil {
				return err
			}
			next.LastExportFile = filepath.Base(destination)
			next.LastExportAt = now
			state.Pending = &pendingExport{Staging: staging, Destination: destination, Next: next}
			// If the state rename succeeded but its directory sync failed,
			// recovery can still find the staging directory.
			if err := e.SaveState(state); err != nil {
				onDisk, readErr := e.readState()
				reserved = readErr != nil || (onDisk.Pending != nil && onDisk.Pending.Staging == staging)
				return err
			}
			reserved = true
			if err := e.publish(*state.Pending); err != nil {
				return err
			}
			for i := range written {
				written[i] = filepath.Join(destination, written[i])
			}
			result = ExportResult{
				EntryURL: filepath.Join(destination, "index.md"),
				Written:  written, SampleCount: len(samples),
				Message: fmt.Sprintf("已归档 %d 条采样和 %d 段专注到 %s", len(samples), len(sessions), filepath.Base(destination)),
			}
			if !force {
				if err := e.journal.pruneDelivered(next.Journal, now); err != nil {
					result.Message += "。缓存清理失败：" + err.Error()
				}
			}
			return nil
		})
	})
	if err != nil {
		return exportError(err)
	}
	return result
}

func exportError(err error) ExportResult {
	return ExportResult{Message: "归档失败：" + err.Error(), Err: err}
}

func (e *Exporter) recoverArchive(state *ExportState) (string, error) {
	pending := state.Pending
	parent := filepath.Dir(pending.Destination)
	var recovered string
	err := withFileLock(filepath.Join(parent, ".daytrace.lock"), func() error {
		_, stagingErr := os.Lstat(pending.Staging)
		_, destinationErr := os.Lstat(pending.Destination)
		if os.IsNotExist(stagingErr) && os.IsNotExist(destinationErr) {
			// Neither copy exists. The old cursors are still intact.
			state.Pending = nil
			return e.SaveState(*state)
		}
		if stagingErr == nil && destinationErr == nil {
			// Another process used this name after our reservation was made.
			path, err := availableArchivePath(parent, filepath.Base(pending.Destination))
			if err != nil {
				return err
			}
			pending.Destination = path
			pending.Next.LastExportFile = filepath.Base(path)
			if err := e.SaveState(*state); err != nil {
				return err
			}
		}
		if err := e.publish(*pending); err != nil {
			return err
		}
		recovered = pending.Destination
		return nil
	})
	return recovered, err
}

// publish installs the prepared directory atomically. The persisted reservation
// lets the next process finish the same archive after a crash or state-write error.
func (e *Exporter) publish(pending pendingExport) error {
	id := filepath.Base(pending.Staging)
	parent := filepath.Dir(pending.Destination)
	if !strings.HasPrefix(id, ".daytrace-") || filepath.Dir(pending.Staging) != parent || pending.Next.Pending != nil {
		return fmt.Errorf("归档恢复状态无效")
	}
	if _, err := os.Lstat(pending.Staging); err == nil {
		if _, err := os.Lstat(pending.Destination); !os.IsNotExist(err) {
			if err != nil {
				return err
			}
			return fmt.Errorf("归档目录已存在: %s", pending.Destination)
		}
		if err := os.Rename(pending.Staging, pending.Destination); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := os.ReadFile(filepath.Join(pending.Destination, "meta.json"))
	if err != nil {
		return err
	}
	var meta exportMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	if meta.ArchiveID != id {
		return fmt.Errorf("归档标识不匹配: %s", pending.Destination)
	}
	if err := syncDirectory(parent); err != nil {
		return err
	}
	return e.SaveState(pending.Next)
}

func archiveDestination(output string, cutoff time.Time, manual bool) (string, error) {
	base := cutoff.In(Beijing).Format("2006-01-02_15-04")
	if manual {
		base += "_manual"
	}
	return availableArchivePath(output, base)
}

func availableArchivePath(output, base string) (string, error) {
	for attempt := 1; ; attempt++ {
		name := base
		if attempt > 1 {
			name = fmt.Sprintf("%s_%02d", base, attempt)
		}
		path := filepath.Join(output, name)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return path, nil
		} else if err != nil {
			return "", err
		}
	}
}
