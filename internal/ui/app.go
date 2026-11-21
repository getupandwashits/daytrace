package ui

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getupandwashits/daytrace/internal/core"
	"github.com/getupandwashits/daytrace/internal/macos"

	vivaapp "github.com/vivaui/viva/app"
)

// App owns the long-lived services. The Viva page reads it through snapshot;
// the menu and the background loop change it through its methods.
type App struct {
	Settings  *core.SettingsStore
	Journal   *core.Journal
	Sessions  *core.SessionStore
	Focus     *core.FocusEngine
	Collector *core.Collector
	Exporter  *core.Exporter

	Invalidator *vivaapp.Invalidator

	mu               sync.Mutex
	summary          core.ActivitySummary
	todaySessions    []core.FocusSession
	allSessions      []core.FocusSession
	lastSample       *core.ActivitySample
	lastExport       string
	lastExportAt     time.Time
	errors           map[string]string
	accessibility    bool
	collectorRunning bool
	refreshedAt      time.Time

	exportMu  sync.Mutex
	exporting bool

	// tab and form are only touched on the window thread (page paint and menu
	// callbacks both run there).
	tab  string
	form settingsForm

	openDashboard func(tab string)
	quit          func()

	stop chan struct{}
	once sync.Once
}

func NewApp() *App {
	settings := core.NewSettingsStore(core.SettingsPath())
	journal := core.NewJournal(core.EventsDir())
	sessions := core.NewSessionStore(core.SessionsDir())
	focus := core.NewFocusEngine(settings.Get(), time.Now, macos.IdleSeconds)
	collector := core.NewCollector(core.CollectorConfigFromSettings(settings.Get()), journal)
	exporter := core.NewExporter(journal, settings, core.ExportStatePath())

	app := &App{
		Settings:    settings,
		Journal:     journal,
		Sessions:    sessions,
		Focus:       focus,
		Collector:   collector,
		Exporter:    exporter,
		errors:      make(map[string]string),
		Invalidator: &vivaapp.Invalidator{},
		tab:         "概览",
		form:        formFromSettings(settings.Get()),
	}
	focus.OnSession = func(session core.FocusSession) {
		if err := sessions.Append(session); err != nil {
			app.reportError("写入专注记录失败", err.Error())
		} else {
			app.reportError("写入专注记录失败", "")
		}
	}
	focus.OnReminder = func(message string) {
		app.askRest(message)
	}
	collector.OnError = func(message string) { app.reportError("活动采集", message) }
	collector.OnSample = func(core.ActivitySample) { app.reportError("活动采集", "") }
	state := exporter.LoadState()
	if state.LastExportFile != "" {
		app.lastExport, app.lastExportAt = "最近归档 "+state.LastExportFile, state.LastExportAt
	}
	return app
}

func (a *App) SetOpenDashboard(open func(tab string)) { a.openDashboard = open }
func (a *App) SetQuit(quit func())                    { a.quit = quit }

func (a *App) Start() {
	a.stop = make(chan struct{})
	settings := a.Settings.Get()
	if settings.CollectActivity {
		a.Collector.Start()
	}
	a.refresh()
	a.persistStatus()
	a.updateStatusItem()
	go a.loop()
}

func (a *App) Stop() {
	a.once.Do(func() {
		if a.stop != nil {
			close(a.stop)
		}
		a.Collector.Stop()
	})
}

func (a *App) OpenDashboard(tab string) {
	if a.openDashboard != nil {
		a.openDashboard(tab)
	}
}

func (a *App) Quit() {
	if a.quit != nil {
		a.quit()
	}
}

func (a *App) SetTab(tab string) { a.tab = tab }

func (a *App) OpenOutput() {
	go func() {
		path, err := core.ArchiveDirectory(a.Settings.Get())
		if err == nil {
			err = os.MkdirAll(path, 0o700)
		}
		if err == nil {
			err = exec.Command("/usr/bin/open", path).Run()
		}
		a.reportError("打开输出目录失败", errorText(err))
		a.Invalidator.Invalidate()
	}()
}

func (a *App) SetPresence(reason string, suspended bool) {
	if suspended {
		a.Focus.Suspend(reason)
		macos.DismissRestPrompt()
	} else {
		a.Focus.Resume(reason)
	}
	if a.stop != nil {
		a.afterFocusAction()
	}
}

func (a *App) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	tick := 0
	for {
		select {
		case <-a.stop:
			return
		case now := <-ticker.C:
			tick++
			idle := macos.IdleSeconds()
			a.Focus.Tick(&idle)
			a.persistStatus()
			a.updateStatusItem()
			if tick%5 == 0 {
				a.refresh()
			}
			if tick%30 == 0 {
				a.exportIfDue(now)
			}
			a.Invalidator.Invalidate()
		}
	}
}

func (a *App) persistStatus() {
	err := a.Focus.Snapshot().WriteTo(core.StatusPath())
	a.reportError("写入状态失败", errorText(err))
}

// updateStatusItem keeps the menu bar text and menu in step with the engine.
// Both calls are safe off the main thread; the native side hops back.
func (a *App) updateStatusItem() {
	macos.SetStatusTitle(a.Focus.MenuBarTitle())
	macos.SetStatusMenu(a.StatusMenu())
}

func (a *App) exportIfDue(now time.Time) {
	if a.Exporter.IsDue(now) {
		a.startExport(func() core.ExportResult { return a.Exporter.ExportDue(now) }, false)
	}
}

func (a *App) refresh() {
	now := time.Now()
	summary := core.ActivitySummary{}
	samples, err := a.Journal.SamplesOn(now)
	if err == nil {
		summary = core.Analyze(samples).Summary
	}
	a.reportError("读取采集记录失败", errorText(err))
	allSessions, err := a.Sessions.LoadAll()
	a.reportError("读取专注记录失败", errorText(err))
	todaySessions := core.SessionsOn(allSessions, now)
	lastSample := a.Collector.LastSample()

	a.mu.Lock()
	a.summary = summary
	a.todaySessions = todaySessions
	a.allSessions = allSessions
	a.lastSample = lastSample
	a.accessibility = macos.AXTrusted()
	a.collectorRunning = a.Collector.Running()
	a.refreshedAt = now
	a.mu.Unlock()
}

func (a *App) ExportNow() {
	a.startExport(func() core.ExportResult { return a.Exporter.ExportNow(time.Now(), false) }, true)
}

func (a *App) startExport(export func() core.ExportResult, manual bool) {
	a.exportMu.Lock()
	if a.exporting {
		a.exportMu.Unlock()
		return
	}
	a.exporting = true
	a.exportMu.Unlock()

	go func() {
		defer func() {
			a.exportMu.Lock()
			a.exporting = false
			a.exportMu.Unlock()
			a.Invalidator.Invalidate()
		}()
		result := export()
		a.reportError("归档失败", errorText(result.Err))
		if result.EntryURL != "" || (manual && result.Err == nil) {
			a.setExport(result.Message)
			a.refresh()
		}
	}()
}

func (a *App) SaveSettings() {
	current := a.Settings.Get()
	next := current
	next.AutomaticFocusMin = clampFloat(parseFloat(a.form.automaticFocus, current.AutomaticFocusMin), 1, 180)
	next.PauseToleranceMin = clampFloat(parseFloat(a.form.pauseTolerance, current.PauseToleranceMin), 0.5, 60)
	next.ResetThresholdMin = clampFloat(parseFloat(a.form.resetThreshold, current.ResetThresholdMin), 1, 180)
	next.RestMin = clampFloat(parseFloat(a.form.rest, current.RestMin), 1, 60)
	next.MinimumRecordMin = clampFloat(parseFloat(a.form.minimumRecord, current.MinimumRecordMin), 0.5, 120)
	next.SampleIntervalSec = clampFloat(parseFloat(a.form.sampleInterval, current.SampleIntervalSec), 2, 600)
	next.IdleThresholdSec = clampFloat(parseFloat(a.form.idleThreshold, current.IdleThresholdSec), 10, 3600)
	next.CollectActivity = a.form.collect
	next.AutoExport = a.form.autoExport
	if output := strings.TrimSpace(a.form.output); output != "" {
		next.OutputDirectory = output
	}
	next.ExcludedApps = splitList(a.form.excluded)

	if err := a.Settings.Save(next); err != nil {
		a.reportError("保存设置失败", err.Error())
		return
	}
	a.reportError("保存设置失败", "")
	next = a.Settings.Get()
	a.Focus.Configure(next)
	a.Collector.Configure(core.CollectorConfigFromSettings(next), next.CollectActivity)
	if !next.CollectActivity {
		a.reportError("活动采集", "")
	}
	a.form = formFromSettings(next)
	a.persistStatus()
	a.refresh()
	a.Invalidator.Invalidate()
}

func (a *App) reportError(operation, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if message == "" {
		delete(a.errors, operation)
	} else {
		a.errors[operation] = message
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *App) setExport(message string) {
	a.mu.Lock()
	a.lastExport = message
	a.lastExportAt = time.Now()
	a.mu.Unlock()
}

// askRest shows the floating rest card and acts on the answer. Starting the
// rest resets the focus timer; 「稍后」 starts a new focus round instead of
// leaving the timer pinned at zero.
func (a *App) askRest(message string) {
	rest := a.Settings.Get().RestMin
	detail := fmt.Sprintf("%s 休息 %g 分钟，连按两下空格或点「开始休息」。", message, rest)
	if a.Focus.Snapshot().State != string(core.StateReminding) {
		return
	}
	macos.ShowRestPrompt("该休息一下了", detail, func(rest bool) {
		if a.Focus.Snapshot().State != string(core.StateReminding) {
			return
		}
		if rest {
			a.Focus.BeginRest()
		} else {
			a.Focus.RestartManually()
		}
		a.afterFocusAction()
	})
}

func parseFloat(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fallback
	}
	return parsed
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func clampFloat(value, low, high float64) float64 {
	return min(high, max(low, value))
}

func clampInt(value, low, high int) int {
	return min(high, max(low, value))
}

func splitList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
