//go:build darwin

package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectorKeepsFirstTextAfterWriteFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "events")
	if err := os.WriteFile(root, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 21, 19, 0, 0, 0, Beijing)
	journal := NewJournal(root)
	collector := NewCollector(CollectorConfig{}, journal)
	collector.running = true
	body := "test body"
	sample := ActivitySample{Timestamp: now, State: "active", App: "Synthetic", UIText: &body}
	errorsReported := 0
	collector.OnError = func(string) { errorsReported++ }
	collector.emit(0, sample)
	if errorsReported != 1 {
		t.Fatal("append failure not reported")
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	collector.emit(0, sample)
	sample.Timestamp = now.Add(time.Second)
	collector.emit(0, sample)
	samples, err := journal.SamplesOn(now)
	if err != nil || len(samples) != 2 {
		t.Fatalf("unexpected log: %v, %v", samples, err)
	}
	if samples[0].UIText == nil || *samples[0].UIText != body || samples[1].UIText != nil ||
		!samples[1].UITextUnchanged || samples[0].ContextID != samples[1].ContextID {
		t.Fatalf("text was not committed before deduplication: %+v", samples)
	}
}
