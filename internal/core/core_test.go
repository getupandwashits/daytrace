package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDigestDropsNoiseMasksSecretsAndJoinsFragments(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	hour := now.In(Beijing).Truncate(time.Hour)
	added := []AddedLine{
		{Stamp: now, App: "Safari", Title: "GitHub", Line: "sk-ant-api03-abcdefghijklmnopqrstuvwxyz123456", Part: 1},
		{Stamp: now, App: "Safari", Title: "GitHub", Line: "────────", Part: 1},
		{Stamp: now, App: "Safari", Title: "GitHub", Line: "GitHub", Part: 1},
		{Stamp: now, App: "Safari", Title: "GitHub", Line: "第 1 步：安装依赖", Part: 1},
		{Stamp: now, App: "Safari", Title: "GitHub", Line: "第 2 步：安装依赖", Part: 1},
		{Stamp: now, App: "Xcode", Title: "Build", Line: "正在生成一段较长的回答", Part: 1},
		{Stamp: now, App: "Xcode", Title: "Build", Line: "正在生成一段较长的回答，包含更多内容", Part: 1},
		{Stamp: now, App: "Notes", Title: "列表", Line: "收到", Part: 1},
		{Stamp: now, App: "Notes", Title: "列表", Line: "下午三点开个短会", Part: 1},
	}
	seconds := map[BlockKey]float64{
		{Hour: hour, App: "Safari", Title: "GitHub"}: 600,
		{Hour: hour, App: "Xcode", Title: "Build"}:   600,
		{Hour: hour, App: "Notes", Title: "列表"}:      600,
	}

	text, stats := BuildDigest(added, seconds, "测试数据", nil)

	if strings.Contains(text, "sk-ant-api03") {
		t.Fatal("secret was not masked")
	}
	if !strings.Contains(text, DigestMask) {
		t.Fatal("mask marker missing")
	}
	if strings.Contains(text, "────────") {
		t.Fatal("decoration line was kept")
	}
	if !strings.Contains(text, "第 1 步：安装依赖") {
		t.Fatal("first digit variant missing")
	}
	if strings.Contains(text, "第 2 步：安装依赖") {
		t.Fatal("digit variant was not dropped")
	}
	if !strings.Contains(text, "正在生成一段较长的回答，包含更多内容") {
		t.Fatal("completed streaming line missing")
	}
	if strings.Contains(text, "正在生成一段较长的回答\n") {
		t.Fatal("superseded streaming draft was kept")
	}
	if !strings.Contains(text, "收到 · 下午三点开个短会") {
		t.Fatal("fragments were not joined")
	}
	if stats.MaskedSecrets != 1 {
		t.Fatalf("masked secrets = %d, want 1", stats.MaskedSecrets)
	}
	for _, key := range []string{"decoration", "streaming_draft", "title_echo", "digit_or_space_variant"} {
		if stats.Dropped[key] == 0 {
			t.Errorf("dropped[%s] = 0, want > 0", key)
		}
	}
}

func TestDigestMasksPasswordValues(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	line := "密码：hunter2secret"
	hour := now.In(Beijing).Truncate(time.Hour)
	seconds := map[BlockKey]float64{{Hour: hour, App: "Safari", Title: "Login"}: 120}
	text, stats := BuildDigest([]AddedLine{{Stamp: now, App: "Safari", Title: "Login", Line: line, Part: 1}}, seconds, "notice", nil)
	if strings.Contains(text, "hunter2secret") {
		t.Fatal("password value was not masked")
	}
	if !strings.Contains(text, "密码："+DigestMask) {
		t.Fatalf("password line = %q", text)
	}
	if stats.MaskedSecrets != 1 {
		t.Fatalf("masked = %d, want 1", stats.MaskedSecrets)
	}
}

func TestPartRangesCollapseConsecutiveValues(t *testing.T) {
	values := map[int]bool{1: true, 2: true, 3: true, 5: true, 7: true, 8: true}
	if got := PartRanges(values); got != "1–3、5、7–8" {
		t.Fatalf("PartRanges = %q", got)
	}
	if got := PartRanges(nil); got != "—" {
		t.Fatalf("empty PartRanges = %q", got)
	}
}

func TestDurationFormatting(t *testing.T) {
	cases := map[float64]string{20: "20s", 120: "2m", 3600: "1h 00m", 5400: "1h 30m"}
	for seconds, want := range cases {
		if got := FormatDuration(seconds); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", seconds, got, want)
		}
	}
}

func TestAnalyzerMergesPeriodsAndCountsGaps(t *testing.T) {
	base := time.Date(2026, 11, 18, 9, 0, 0, 0, Beijing)
	samples := []ActivitySample{
		{Timestamp: base, State: "active", App: "Xcode", WindowTitle: "main.go", SampleInterval: 10},
		{Timestamp: base.Add(10 * time.Second), State: "active", App: "Xcode", WindowTitle: "main.go", SampleInterval: 10},
		{Timestamp: base.Add(20 * time.Second), State: "active", App: "Xcode", WindowTitle: "main.go", SampleInterval: 10},
	}
	samples = append(samples, ActivitySample{
		Timestamp: base.Add(20*time.Second + 10*time.Minute), State: "active", App: "Safari", WindowTitle: "GitHub", SampleInterval: 10,
	})
	analysis := Analyze(samples)
	if len(analysis.Periods) != 2 {
		t.Fatalf("periods = %d, want 2", len(analysis.Periods))
	}
	if len(analysis.Gaps) != 1 {
		t.Fatalf("gaps = %d, want 1", len(analysis.Gaps))
	}
	if analysis.Summary.Samples != 4 {
		t.Fatalf("samples = %d", analysis.Summary.Samples)
	}
	if analysis.Summary.Apps[0].Name != "Xcode" {
		t.Fatalf("top app = %s", analysis.Summary.Apps[0].Name)
	}
}

func TestExporterWritesFourLayerRecord(t *testing.T) {
	root := t.TempDir()
	settings := DefaultSettings()
	settings.OutputDirectory = filepath.Join(root, "output")
	settings.AutoExport = false
	store := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}

	journal := NewJournal(filepath.Join(root, "events"))
	now := time.Date(2026, 11, 21, 18, 0, 0, 0, Beijing)
	first := ActivitySample{
		Timestamp: now.Add(-time.Second), State: "active", App: "Xcode",
		BundleID: "com.apple.dt.Xcode", WindowTitle: "FocusEngine.swift",
		SampleInterval: 60, IdleThreshold: 180, CaptureStatus: "ok",
	}
	text := "final class FocusEngine\n@Published var state"
	first.UIText = &text
	second := ActivitySample{
		Timestamp: now, State: "active", App: "Safari", BundleID: "com.apple.Safari",
		WindowTitle: "GitHub", SampleInterval: 60, IdleThreshold: 180, CaptureStatus: "ok",
	}
	other := "DayTrace — a local activity journal"
	second.UIText = &other
	if err := journal.Append(first); err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(second); err != nil {
		t.Fatal(err)
	}

	exporter := NewExporter(journal, store, filepath.Join(root, "export-state.json"))
	result := exporter.ExportNow(now.Add(time.Second), true)
	if result.EntryURL == "" {
		t.Fatalf("export failed: %s", result.Message)
	}
	entry, err := os.ReadFile(result.EntryURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"电脑活动记录", "Xcode", "Safari", "新增原文：段"} {
		if !strings.Contains(string(entry), want) {
			t.Errorf("entry missing %q", want)
		}
	}

	archive := filepath.Dir(result.EntryURL)
	uiDirectory := filepath.Join(archive, "ui")
	for _, name := range []string{"digest.md", "timeline.md", "part-001.md"} {
		if _, err := os.Stat(filepath.Join(uiDirectory, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	metaData, err := os.ReadFile(filepath.Join(archive, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["samples"].(float64) != 2 || meta["records"].(float64) != 2 {
		t.Errorf("meta samples/records = %v/%v", meta["samples"], meta["records"])
	}
	if meta["ui_format_version"].(float64) != 4 {
		t.Errorf("ui_format_version = %v", meta["ui_format_version"])
	}
	if meta["producer"].(string) != "DayTrace" {
		t.Errorf("producer = %v", meta["producer"])
	}
	for _, key := range []string{"cutoff", "first_timestamp", "last_timestamp"} {
		if value, ok := meta[key].(string); !ok || value == "" {
			t.Errorf("%s missing", key)
		}
	}

	rawData, err := os.ReadFile(filepath.Join(archive, "activity.jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rawData) < 2 || rawData[0] != 0x1f || rawData[1] != 0x8b {
		t.Error("raw archive is not gzip")
	}
	if exporter.LoadState().LastExportFile != filepath.Base(archive) {
		t.Error("export state was not saved")
	}
}

func TestLateScheduledExportKeepsConfiguredFilenameAndCutoff(t *testing.T) {
	root := t.TempDir()
	day := StartOfDay(time.Now())
	late := day.Add(23*time.Hour + 20*time.Minute)
	earlier := day.Add(22*time.Hour + 50*time.Minute)

	settings := DefaultSettings()
	settings.OutputDirectory = filepath.Join(root, "output")
	settings.AutoExport = true
	settings.ExportHour = 23
	settings.ExportMinute = 0
	store := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}

	journal := NewJournal(filepath.Join(root, "events"))
	for index, stamp := range []time.Time{earlier, late} {
		sample := ActivitySample{
			Timestamp: stamp, State: "active", App: "Xcode", BundleID: "com.apple.dt.Xcode",
			WindowTitle: "FocusEngine.swift", SampleInterval: 60, IdleThreshold: 180, CaptureStatus: "ok",
		}
		text := "sample " + string(rune('0'+index))
		sample.UIText = &text
		if err := journal.Append(sample); err != nil {
			t.Fatal(err)
		}
	}

	exporter := NewExporter(journal, store, filepath.Join(root, "export-state.json"))
	now := day.Add(23*time.Hour + 30*time.Minute)
	if !exporter.IsDue(now) {
		t.Fatal("export should be due")
	}
	result := exporter.ExportDue(now)
	if result.EntryURL == "" {
		t.Fatalf("export failed: %s", result.Message)
	}
	if got, want := filepath.Base(filepath.Dir(result.EntryURL)), DayString(day)+"_23-00"; got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
	metaData, _ := os.ReadFile(filepath.Join(filepath.Dir(result.EntryURL), "meta.json"))
	var meta map[string]any
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["records"].(float64) != 1 {
		t.Errorf("records = %v, want 1", meta["records"])
	}
	if cutoff, _ := meta["cutoff"].(string); !strings.Contains(cutoff, "T23:00:00.000+08:00") {
		t.Errorf("cutoff = %q", cutoff)
	}
	if !exporter.LoadState().LastExportEnd.Equal(day.Add(23 * time.Hour)) {
		t.Errorf("lastExportEnd = %v", exporter.LoadState().LastExportEnd)
	}
	if exporter.IsDue(now.Add(time.Minute)) {
		t.Error("export should not be due again")
	}
}

func TestSessionStoreAppendAndLoadForDay(t *testing.T) {
	root := t.TempDir()
	store := NewSessionStore(root)
	day := StartOfDay(time.Now())
	first := FocusSession{ID: "a", Start: day.Add(9 * time.Hour), End: day.Add(9*time.Hour + 1500*time.Second), Duration: 1500, Trigger: "auto"}
	second := FocusSession{ID: "b", Start: day.Add(14 * time.Hour), End: day.Add(14*time.Hour + 2700*time.Second), Duration: 2700, Trigger: "manual"}
	if err := store.Append(second); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(first); err != nil {
		t.Fatal(err)
	}
	all, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || !all[0].Start.Equal(first.Start) {
		t.Fatalf("all = %+v", all)
	}
	today, err := store.SessionsOn(day)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for _, session := range today {
		total += session.Duration
	}
	if total != 4200 {
		t.Fatalf("total = %v", total)
	}
	if _, err := os.Stat(filepath.Join(root, "sessions.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestUnchangedTextFromEarlierArchiveIsCarriedForward(t *testing.T) {
	root := t.TempDir()
	settings := DefaultSettings()
	settings.OutputDirectory = filepath.Join(root, "output")
	settings.AutoExport = false
	store := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}

	journal := NewJournal(filepath.Join(root, "events"))
	base := time.Date(2026, 11, 21, 18, 0, 0, 0, Beijing)
	text := "session notes line one\nsession notes line two"
	contextID := "ctx-1"
	first := ActivitySample{
		Timestamp: base, State: "active", App: "Notes", BundleID: "com.apple.Notes",
		WindowTitle: "Notes", SampleInterval: 10, IdleThreshold: 180, CaptureStatus: "ok",
		UIText: &text, ContextID: contextID,
	}
	if err := journal.Append(first); err != nil {
		t.Fatal(err)
	}
	exporter := NewExporter(journal, store, filepath.Join(root, "export-state.json"))
	firstResult := exporter.ExportNow(base.Add(time.Second), false)
	if firstResult.EntryURL == "" {
		t.Fatalf("first export failed: %s", firstResult.Message)
	}

	second := ActivitySample{
		Timestamp: base.Add(2 * time.Second), State: "active", App: "Notes", BundleID: "com.apple.Notes",
		WindowTitle: "Notes", SampleInterval: 10, IdleThreshold: 180, CaptureStatus: "ok",
		UITextUnchanged: true, ContextID: contextID,
	}
	if err := journal.Append(second); err != nil {
		t.Fatal(err)
	}
	secondResult := exporter.ExportNow(base.Add(3*time.Second), false)
	if secondResult.EntryURL == "" {
		t.Fatalf("second export failed: %s", secondResult.Message)
	}
	uiDir := filepath.Join(filepath.Dir(secondResult.EntryURL), "ui")

	part, err := os.ReadFile(filepath.Join(uiDir, "part-001.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(part), "session notes line one") {
		t.Errorf("carried text missing from part-001:\n%s", part)
	}
	timeline, err := os.ReadFile(filepath.Join(uiDir, "timeline.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(timeline), "沿用") {
		t.Errorf("timeline has no carried reference:\n%s", timeline)
	}
	digest, err := os.ReadFile(filepath.Join(uiDir, "digest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(digest), "session notes line one") {
		t.Errorf("carried text should not be a new digest line:\n%s", digest)
	}
}

func TestExportPrunesDeliveredCacheDays(t *testing.T) {
	root := t.TempDir()
	settings := DefaultSettings()
	settings.OutputDirectory = filepath.Join(root, "output")
	settings.AutoExport = false
	store := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}

	journal := NewJournal(filepath.Join(root, "events"))
	now := time.Date(2026, 11, 21, 18, 0, 0, 0, Beijing)
	stamps := []time.Time{now.AddDate(0, 0, -10), now.AddDate(0, 0, -3), now}
	for _, stamp := range stamps {
		text := "cache text for " + DayString(stamp)
		sample := ActivitySample{
			Timestamp: stamp, State: "active", App: "Xcode", WindowTitle: "main.go",
			SampleInterval: 60, IdleThreshold: 180, CaptureStatus: "ok", UIText: &text,
		}
		if err := journal.Append(sample); err != nil {
			t.Fatal(err)
		}
	}
	exporter := NewExporter(journal, store, filepath.Join(root, "export-state.json"))
	first := exporter.ExportNow(now.AddDate(0, 0, -5), false)
	if first.Err != nil || first.SampleCount != 1 {
		t.Fatalf("could not deliver first day: %+v", first)
	}
	result := exporter.ExportNow(now, false)
	if result.EntryURL == "" {
		t.Fatalf("export failed: %s", result.Message)
	}

	name := func(stamp time.Time) string { return filepath.Join(root, "events", DayString(stamp)+".jsonl") }
	if _, err := os.Stat(name(stamps[0])); !os.IsNotExist(err) {
		t.Errorf("delivered cache day %s was not pruned (%v)", DayString(stamps[0]), err)
	}
	for _, stamp := range stamps[1:] {
		if _, err := os.Stat(name(stamp)); err != nil {
			t.Errorf("cache day %s should be kept: %v", DayString(stamp), err)
		}
	}
}
