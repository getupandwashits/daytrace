package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchiveFiltersPrivateWindowsAndSanitizesURLs(t *testing.T) {
	body := "visit https://account:secret@example.test/path?token=query#fragment"
	for _, private := range []ActivitySample{
		{App: "Bitwarden", UIText: &body},
		{App: "Browser", WindowTitle: "Private Browsing", UIText: &body},
		{App: "Browser", URL: "https://vault.bitwarden.com/path", UIText: &body},
	} {
		cleanSample(&private)
		if private.State != "excluded" || private.App != "" || private.URL != "" || private.UIText != nil {
			t.Fatalf("private window retained fields: %+v", private)
		}
	}
	sample := ActivitySample{State: "active", WindowTitle: body, URL: "https://example.test/path?", UIText: &body}
	cleanSample(&sample)
	if sample.WindowTitle != "visit https://example.test/path" || *sample.UIText != sample.WindowTitle || sample.URL != "https://example.test/path" {
		t.Fatalf("URL cleanup did not match: %+v; body=%q", sample, *sample.UIText)
	}
}

func TestDigestMasksWindowTitleEvenInShortVisit(t *testing.T) {
	now := time.Date(2026, 11, 21, 19, 1, 0, 0, Beijing)
	title := "password: SyntheticTitle987"
	result, stats := BuildDigest([]AddedLine{
		{Stamp: now, App: "Example", Title: title, Line: "body", Part: 1},
	}, nil, "", nil)
	if strings.Contains(result, "SyntheticTitle987") || !strings.Contains(result, "password: "+DigestMask) || stats.MaskedSecrets != 1 {
		t.Fatalf("digest title not masked: %s, %+v", result, stats)
	}
}

func TestSettingsOutputOverrideDoesNotRewriteStoredPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	t.Setenv("DAYTRACE_HOME", root)
	t.Setenv("DAYTRACE_OUTPUT", "")
	settings := NewSettingsStore(path)
	if settings.Get().CollectActivity || settings.Get().OutputDirectory != filepath.Join(root, "archives") {
		t.Fatalf("unexpected first-run settings: %+v", settings.Get())
	}
	original := settings.Get()
	original.OutputDirectory = filepath.Join(root, "chosen")
	if err := settings.Save(original); err != nil {
		t.Fatal(err)
	}
	override := filepath.Join(root, "isolated")
	t.Setenv("DAYTRACE_OUTPUT", override)
	edited := settings.Get()
	edited.OutputDirectory = filepath.Join(root, "other")
	if err := settings.Save(edited); err != nil {
		t.Fatal(err)
	}
	if effective := NewSettingsStore(path).Get().OutputDirectory; effective != override {
		t.Fatalf("reloaded path = %q", effective)
	}
	if effective := settings.Get().OutputDirectory; effective != override {
		t.Fatalf("live path = %q", effective)
	}
	t.Setenv("DAYTRACE_OUTPUT", "")
	if effective := NewSettingsStore(path).Get().OutputDirectory; effective != original.OutputDirectory {
		t.Fatalf("original path lost: %q", effective)
	}
}

func TestArchiveEscapesObservedMarkdownWithoutChangingRawRecords(t *testing.T) {
	e, _ := newArchiveTest(t)
	cutoff := time.Date(2026, 11, 21, 22, 20, 0, 0, Beijing)
	const image = "![example](https://example.invalid/p.png)"
	const html = "<b>sample</b>"
	body := "read this example\n```\n" + image
	for _, stamp := range []time.Time{cutoff.Add(-75 * time.Second), cutoff.Add(-5 * time.Second)} {
		if err := e.journal.Append(ActivitySample{
			Timestamp: stamp, App: "Test " + image, State: "active", WindowTitle: image,
			Document: html, UIText: &body, CaptureStatus: html,
		}); err != nil {
			t.Fatal(err)
		}
	}
	result := e.ExportNow(cutoff, false)
	samples := archivedSamples(t, result)
	if len(samples) != 2 || samples[0].WindowTitle != image || *samples[0].UIText != body {
		t.Fatalf("raw metadata/body changed: %+v", samples)
	}
	root := filepath.Dir(result.EntryURL)
	for _, file := range []string{"index.md", "ui/timeline.md", "ui/digest.md", "ui/part-001.md"} {
		text, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(text), "\\!\\[") || strings.Contains(string(text), html) {
			t.Fatalf("%s has unescaped metadata", file)
		}
		// Body examples may still use literal Markdown, but only inside fences.
		codeFence := ""
		for _, line := range strings.Split(string(text), "\n") {
			if strings.HasPrefix(line, "```") {
				if codeFence == "" {
					codeFence = strings.TrimSuffix(line, "text")
				} else if line == codeFence {
					codeFence = ""
				}
			}
			if codeFence == "" && strings.Contains(line, image) {
				t.Errorf("%s renders an observed image outside a code fence: %q", file, line)
			}
		}
	}
}
