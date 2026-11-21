package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func appendLogBytes(t *testing.T, path string, fragment []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(fragment); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func checkLogBackup(t *testing.T, path string, fragment []byte) {
	t.Helper()
	backups, err := filepath.Glob(path + ".damaged-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one fragment backup: %v, %v", backups, err)
	}
	contents, err := os.ReadFile(backups[0])
	if err != nil || !bytes.Equal(contents, fragment) {
		t.Fatalf("fragment not preserved: %q, %v", contents, err)
	}
	info, err := os.Stat(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("backup permissions = %v, want 0600", info.Mode().Perm())
	}
}

func TestJournalAppendRepairsTornTailWithoutChangingDeliveredPrefix(t *testing.T) {
	e, _ := newArchiveTest(t)
	now := time.Date(2026, 11, 21, 18, 0, 0, 0, Beijing)
	appendArchiveSample(t, e, now.Add(-time.Minute), "already delivered")
	archivedSamples(t, e.ExportNow(now, false))

	path := DayFile(now)
	fragment := []byte("{\"timestamp\":\"2026-11-21")
	appendLogBytes(t, path, fragment)
	appendArchiveSample(t, e, now.Add(time.Minute), "after repair")
	samples := archivedSamples(t, e.ExportNow(now.Add(time.Minute), false))
	if len(samples) != 1 || *samples[0].UIText != "after repair" {
		t.Fatalf("recovery changed delivery progress: %+v", samples)
	}
	checkLogBackup(t, path, fragment)
}

func TestArchiveReadsValidRecordsOnBothSidesOfHistoricalTornTail(t *testing.T) {
	e, _ := newArchiveTest(t)
	cutoff := time.Date(2026, 11, 21, 23, 0, 0, 0, Beijing)
	old := cutoff.AddDate(0, 0, -10)
	appendArchiveSample(t, e, old, "old valid")
	path := DayFile(old)
	fragment := []byte("{\"timestamp\":")
	appendLogBytes(t, path, fragment)
	appendArchiveSample(t, e, cutoff.Add(-time.Hour), "new valid")

	samples := archivedSamples(t, e.ExportDue(cutoff))
	if len(samples) != 2 || *samples[0].UIText != "old valid" || *samples[1].UIText != "new valid" {
		t.Fatalf("recovery lost a valid record: %+v", samples)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("fully delivered old log wasn't pruned: %v", err)
	}
	checkLogBackup(t, path, fragment)
}

func TestSessionLogKeepsCompleteRecordWithoutFinalNewline(t *testing.T) {
	root := t.TempDir()
	store := NewSessionStore(root)
	now := time.Date(2026, 11, 21, 18, 0, 0, 0, Beijing)
	session := FocusSession{ID: "one", Start: now, End: now.Add(time.Minute), Duration: 60}
	if err := store.Append(session); err != nil {
		t.Fatal(err)
	}
	session.ID = "two"
	line, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	appendLogBytes(t, store.path(), line)
	sessions, err := store.LoadAll()
	if err != nil || len(sessions) != 2 || sessions[1].ID != "two" {
		t.Fatalf("complete record lost during recovery: %+v, %v", sessions, err)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 1 {
		t.Fatalf("unexpected backup for a complete record: %+v, %v", files, err)
	}
}

func TestLogKeepsOriginalBytesIfFragmentBackupFails(t *testing.T) {
	e, _ := newArchiveTest(t)
	now := time.Date(2026, 11, 21, 18, 0, 0, 0, Beijing)
	appendArchiveSample(t, e, now.Add(-time.Minute), "valid")
	path := DayFile(now)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fragment := []byte("{\"timestamp\":")
	appendLogBytes(t, path, fragment)

	backup := fmt.Sprintf("%s.damaged-%d-%x", path, len(before), sha256.Sum256(fragment))
	if err := os.Mkdir(backup, 0o700); err != nil {
		t.Fatal(err)
	}
	err = e.journal.Append(ActivitySample{Timestamp: now, State: "active"})
	if err == nil {
		t.Fatal("expected backup collision to block repair")
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, append(before, fragment...)) {
		t.Fatalf("log was changed without a durable backup: %q, %v", actual, err)
	}
}
