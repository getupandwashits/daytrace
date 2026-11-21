package ui

import (
	"fmt"
	"math"
	"time"

	"github.com/getupandwashits/daytrace/internal/core"
	"github.com/getupandwashits/daytrace/internal/macos"

	vivaapp "github.com/vivaui/viva/app"
)

// StatusMenu converts the menu to the rows the native status item shows.
func (a *App) StatusMenu() []macos.MenuEntry {
	items := a.Menu()
	entries := make([]macos.MenuEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, macos.MenuEntry{
			Title:     item.Title,
			Disabled:  item.Disabled,
			Separator: item.Separator,
			OnSelect:  item.OnSelect,
		})
	}
	return entries
}

// Menu is rebuilt every time the status item opens, so it always shows the
// current phase and today's totals.
func (a *App) Menu() []vivaapp.MenuItem {
	status := a.Focus.Snapshot()
	active, samples := a.todayTotals()
	items := []vivaapp.MenuItem{
		{Title: statusLine(status), Disabled: true},
		{Title: fmt.Sprintf("今日活跃 %s · %d 条采样", core.FormatDuration(active), samples), Disabled: true},
		{Separator: true},
	}
	switch core.FocusState(status.State) {
	case core.StateActive:
		items = append(items,
			vivaapp.MenuItem{Title: "暂停", OnSelect: a.togglePause},
			vivaapp.MenuItem{Title: "停止", OnSelect: a.stopFocus},
		)
	case core.StatePaused, core.StateReminding:
		items = append(items,
			vivaapp.MenuItem{Title: "继续", OnSelect: a.togglePause},
			vivaapp.MenuItem{Title: "停止", OnSelect: a.stopFocus},
		)
	case core.StateStopped:
		items = append(items, vivaapp.MenuItem{Title: "继续专注", OnSelect: a.resumeFocus})
	default:
		items = append(items, vivaapp.MenuItem{Title: "开始专注", OnSelect: a.startFocus})
	}
	items = append(items,
		vivaapp.MenuItem{Title: "重新开始", OnSelect: a.restartFocus},
		vivaapp.MenuItem{Title: "强制休息", OnSelect: a.forceRest},
		vivaapp.MenuItem{Separator: true},
		vivaapp.MenuItem{Title: "立即归档", OnSelect: a.ExportNow},
		vivaapp.MenuItem{Title: "打开仪表盘", OnSelect: func() { a.OpenDashboard("概览") }},
		vivaapp.MenuItem{Title: "设置…", OnSelect: func() { a.OpenDashboard("设置") }},
		vivaapp.MenuItem{Separator: true},
		vivaapp.MenuItem{Title: "退出 DayTrace", OnSelect: a.Quit},
	)
	return items
}

func (a *App) startFocus() {
	a.Focus.StartManually()
	a.afterFocusAction()
}

func (a *App) stopFocus() {
	a.Focus.StopManually()
	a.afterFocusAction()
}

func (a *App) resumeFocus() {
	a.Focus.ResumeManually()
	a.afterFocusAction()
}

func (a *App) restartFocus() {
	a.Focus.RestartManually()
	a.afterFocusAction()
}

func (a *App) togglePause() {
	a.Focus.TogglePause()
	a.afterFocusAction()
}

func (a *App) forceRest() {
	a.Focus.ResetToAway(core.TriggerManual)
	a.afterFocusAction()
}

func (a *App) afterFocusAction() {
	a.persistStatus()
	a.updateStatusItem()
	a.Invalidator.Invalidate()
}

func (a *App) todayTotals() (float64, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.summary.ActiveSeconds, a.summary.Samples
}

func statusLine(status core.StatusSnapshot) string {
	remaining := clockText(status.FocusRemainingSeconds)
	switch core.FocusState(status.State) {
	case core.StateActive:
		return "专注中 · 还剩 " + remaining
	case core.StatePaused:
		return "短暂停顿 · 还剩 " + remaining
	case core.StateReminding:
		return "需要休息"
	case core.StateAway:
		if status.RestRemainingSeconds > 0 {
			return "已休息 · 休息还剩 " + clockText(status.RestRemainingSeconds)
		}
		return "已休息"
	case core.StateStopped:
		return "已停止 · " + clockText(status.FocusElapsedSeconds) + " 未记录"
	default:
		return status.StateLabel
	}
}

func clockText(seconds float64) string {
	total := max(0, int(math.Round(seconds)))
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

func sessionLine(session core.FocusSession) string {
	trigger := "自动"
	if session.Trigger == "manual" {
		trigger = "手动"
	}
	start := session.Start.In(core.Beijing)
	return fmt.Sprintf("%s–%s · %s · %s",
		start.Format("15:04"), session.End.In(core.Beijing).Format("15:04"),
		core.FormatDuration(session.Duration), trigger)
}

func dayLabel(value time.Time) string {
	local := value.In(core.Beijing)
	return local.Format("2006-01-02")
}
