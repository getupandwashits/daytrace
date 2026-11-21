package core

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newArchiveTest(t *testing.T) (*Exporter, string) {
	t.Helper()
	root := t.TempDir()
	output := filepath.Join(root, "archives")
	t.Setenv("DAYTRACE_HOME", root)
	t.Setenv("DAYTRACE_OUTPUT", output)
	return NewExporter(nil, nil, ""), output
}

func appendArchiveSample(t *testing.T, e *Exporter, stamp time.Time, body string) {
	t.Helper()
	if err := e.journal.Append(ActivitySample{
		Timestamp: stamp, State: "active", App: "Example", SampleInterval: 10,
		WindowTitle: "Synthetic Window", UIText: &body,
	}); err != nil {
		t.Fatal(err)
	}
}

func archivedSamples(t *testing.T, result ExportResult) []ActivitySample {
	t.Helper()
	if result.Err != nil || result.EntryURL == "" {
		t.Fatalf("archive failed: %+v", result)
	}
	file, err := os.Open(filepath.Join(filepath.Dir(result.EntryURL), "activity.jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var samples []ActivitySample
	decoder := json.NewDecoder(reader)
	for {
		var sample ActivitySample
		err := decoder.Decode(&sample)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, sample)
	}
	return samples
}

func TestArchiveFindsHistoricalAndLateRecordsAfterForce(t *testing.T) {
	e, _ := newArchiveTest(t)
	cutoff := time.Date(2026, 11, 21, 23, 0, 0, 0, Beijing)
	old := cutoff.AddDate(0, 0, -10)
	appendArchiveSample(t, e, old, "old")
	appendArchiveSample(t, e, cutoff.Add(-time.Hour), "today")
	// Old state claimed that the entire historical interval had been delivered.
	if err := e.SaveState(ExportState{LastExportEnd: cutoff}); err != nil {
		t.Fatal(err)
	}
	forced := archivedSamples(t, e.ExportNow(cutoff.Add(-time.Minute), true))
	if len(forced) != 1 || *forced[0].UIText != "today" {
		t.Fatalf("unexpected forced archive: %+v", forced)
	}
	if _, err := os.Stat(DayFile(old)); err != nil {
		t.Fatalf("forced export removed an undelivered source: %v", err)
	}
	first := archivedSamples(t, e.ExportDue(cutoff))
	if len(first) != 2 || *first[0].UIText != "old" || *first[1].UIText != "today" {
		t.Fatalf("first scheduled archive: %+v", first)
	}

	// A sample may be flushed after the export that crossed its timestamp.
	appendArchiveSample(t, e, cutoff.Add(-time.Second), "late")
	appendArchiveSample(t, e, cutoff.Add(time.Second), "next")
	next := archivedSamples(t, e.ExportDue(cutoff.AddDate(0, 0, 1)))
	if len(next) != 2 || *next[0].UIText != "late" || *next[1].UIText != "next" {
		t.Fatalf("second scheduled archive: %+v", next)
	}
	if again := e.ExportNow(cutoff.AddDate(0, 0, 1), false); again.EntryURL != "" || again.Err != nil {
		t.Fatalf("already delivered samples exported again: %+v", again)
	}
}

func TestFailedArchiveCanRetryWithoutDroppingSamples(t *testing.T) {
	e, output := newArchiveTest(t)
	cutoff := time.Date(2026, 11, 21, 23, 0, 0, 0, Beijing)
	appendArchiveSample(t, e, cutoff, "still pending")
	if err := os.WriteFile(output, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if failed := e.ExportDue(cutoff); failed.Err == nil || failed.EntryURL != "" {
		t.Fatalf("failure not reported: %+v", failed)
	}
	if state := e.LoadState(); len(state.Journal) != 0 || !state.LastExportEnd.IsZero() {
		t.Fatalf("failed archive advanced state: %+v", state)
	}
	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}
	result := e.ExportDue(cutoff)
	if len(archivedSamples(t, result)) != 1 || filepath.Dir(result.EntryURL) != filepath.Join(output, "2026-11-21_23-00") {
		t.Fatalf("retry did not publish the expected archive: %+v", result)
	}
}

func TestInterruptionDuringArchivePublish(t *testing.T) {
	for _, phase := range []string{"prepared", "published", "missing"} {
		t.Run(phase, func(t *testing.T) {
			e, output := newArchiveTest(t)
			cutoff := time.Date(2026, 11, 21, 23, 0, 0, 0, Beijing)
			appendArchiveSample(t, e, cutoff, "first")
			samples, cursors, err := e.journal.pending(nil, cutoff)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(output, 0o700); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(output, "2026-11-21_23-00")
			staging, err := os.MkdirTemp(output, ".daytrace-")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.renderArchive(staging, samples, nil, cutoff, cutoff); err != nil {
				t.Fatal(err)
			}
			if err := e.SaveState(ExportState{Version: 1, Pending: &pendingExport{
				Staging: staging, Destination: destination,
				Next: ExportState{Version: 1, LastExportEnd: cutoff, Journal: cursors},
			}}); err != nil {
				t.Fatal(err)
			}
			switch phase {
			case "published":
				if err := os.Rename(staging, destination); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.RemoveAll(staging); err != nil {
					t.Fatal(err)
				}
			}
			reopened := NewExporter(nil, nil, "")
			result := reopened.ExportDue(cutoff)
			if len(archivedSamples(t, result)) != 1 || filepath.Dir(result.EntryURL) != destination {
				t.Fatalf("wrong recovery: %+v", result)
			}
			state := reopened.LoadState()
			if state.Pending != nil || state.Journal[DayString(cutoff)] != cursors[DayString(cutoff)] {
				t.Fatalf("wrong recovered state: %+v", state)
			}
			if next := reopened.ExportNow(cutoff, false); next.EntryURL != "" || next.Err != nil {
				t.Fatalf("recovery duplicated a record: %+v", next)
			}
		})
	}
}

func TestArchiveSerializesTwoExporters(t *testing.T) {
	e, _ := newArchiveTest(t)
	cutoff := time.Date(2026, 11, 21, 23, 0, 0, 0, Beijing)
	appendArchiveSample(t, e, cutoff, "only once")
	results := make(chan ExportResult, 2)
	var workers sync.WaitGroup
	for _, exporter := range []*Exporter{e, NewExporter(nil, nil, "")} {
		workers.Go(func() { results <- exporter.ExportDue(cutoff) })
	}
	workers.Wait()
	close(results)
	total := 0
	for result := range results {
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		total += result.SampleCount
	}
	if total != 1 {
		t.Fatalf("archived %d samples, want 1", total)
	}
}

func TestArchiveWithOnlyFocusSessions(t *testing.T) {
	e, _ := newArchiveTest(t)
	cutoff := time.Date(2026, 11, 21, 23, 0, 0, 0, Beijing)
	if err := e.sessions.Append(FocusSession{ID: "focus", Start: cutoff.Add(-5 * time.Minute), End: cutoff, Duration: 300}); err != nil {
		t.Fatal(err)
	}
	result := e.ExportDue(cutoff)
	if samples := archivedSamples(t, result); len(samples) != 0 {
		t.Fatalf("unexpected samples: %+v", samples)
	}
	entry, err := os.ReadFile(result.EntryURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(entry), "## 专注记录") || !strings.Contains(string(entry), "11-21 22:55:00 – 11-21 23:00:00") {
		t.Fatalf("missing focus session in %s", entry)
	}
	if next := e.ExportDue(cutoff.AddDate(0, 0, 1)); next.EntryURL != "" || next.Err != nil {
		t.Fatalf("focus session exported twice: %+v", next)
	}
}

func TestTodayIgnoresUnrelatedLogAndArchiveReportsCorruption(t *testing.T) {
	e, _ := newArchiveTest(t)
	cutoff := time.Date(2026, 11, 21, 23, 0, 0, 0, Beijing)
	appendArchiveSample(t, e, cutoff, "today")
	if err := os.WriteFile(DayFile(cutoff.AddDate(0, 0, -1)), []byte("broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if today, err := e.journal.SamplesOn(cutoff); err != nil || len(today) != 1 {
		t.Fatalf("today failed because of another day's log: %v, %v", today, err)
	}
	if result := e.ExportDue(cutoff); result.Err == nil || result.EntryURL != "" {
		t.Fatalf("archive did not report corrupted log: %+v", result)
	}
	if state := e.LoadState(); !state.LastExportEnd.IsZero() {
		t.Fatalf("failed export advanced the checkpoint: %+v", state)
	}
}

func TestRetiredLogPathDoesNotReuseOldCursor(t *testing.T) {
	e, _ := newArchiveTest(t)
	cutoff := time.Date(2026, 11, 21, 23, 0, 0, 0, Beijing)
	old := cutoff.AddDate(0, 0, -10)
	appendArchiveSample(t, e, old, "first")
	archivedSamples(t, e.ExportDue(cutoff))
	if _, err := os.Stat(DayFile(old)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("delivered log not pruned: %v", err)
	}
	appendArchiveSample(t, e, old, "later")
	samples := archivedSamples(t, e.ExportDue(cutoff.AddDate(0, 0, 1)))
	if len(samples) != 1 || *samples[0].UIText != "later" {
		t.Fatalf("new file skipped: %+v", samples)
	}
}

func TestArchiveSwitchesOutputWhilePreviousReservationIsOffline(t *testing.T) {
	e, output := newArchiveTest(t)
	cutoff := time.Date(2026, 11, 21, 23, 0, 0, 0, Beijing)
	appendArchiveSample(t, e, cutoff.Add(-time.Hour), "already delivered")
	archivedSamples(t, e.ExportNow(cutoff.Add(-time.Minute), false))
	committed := e.LoadState()

	appendArchiveSample(t, e, cutoff, "pending record")
	records, cursors, err := e.journal.pending(committed.Journal, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	staging, err := os.MkdirTemp(output, ".daytrace-")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.renderArchive(staging, records, nil, cutoff, cutoff); err != nil {
		t.Fatal(err)
	}
	next := committed
	next.Journal, next.LastExportEnd = cursors, cutoff
	reserved := committed
	reserved.Pending = &pendingExport{
		Staging: staging, Destination: filepath.Join(output, "2026-11-21_23-00"), Next: next,
	}
	if err := e.SaveState(reserved); err != nil {
		t.Fatal(err)
	}
	offline := output + "-offline"
	if err := os.Rename(output, offline); err != nil {
		t.Fatal(err)
	}
	newOutput := filepath.Join(filepath.Dir(output), "new-output")
	t.Setenv("DAYTRACE_OUTPUT", newOutput)

	if err := os.WriteFile(newOutput, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if res := e.ExportNow(cutoff, false); res.Err == nil {
		t.Fatal("expected failure creating the new output")
	}
	if state := e.LoadState(); state.Pending == nil || state.Pending.Staging != staging {
		t.Fatal("a failed switch discarded the previous reservation")
	}
	if err := os.Remove(newOutput); err != nil {
		t.Fatal(err)
	}

	result := e.ExportNow(cutoff, false)
	samples := archivedSamples(t, result)
	if len(samples) != 1 || *samples[0].UIText != "pending record" ||
		filepath.Dir(filepath.Dir(result.EntryURL)) != newOutput {
		t.Fatalf("unexpected replacement archive: %+v, %+v", result, samples)
	}
	if _, err := os.Stat(filepath.Join(offline, filepath.Base(staging), "meta.json")); err != nil {
		t.Fatalf("previous staging was modified: %v", err)
	}
	if state := e.LoadState(); state.Pending != nil {
		t.Fatal("new archive did not clear pending state")
	}
	if result := e.ExportNow(cutoff, false); result.Err != nil || result.EntryURL != "" {
		t.Fatalf("replacement export was duplicated: %+v", result)
	}
}
