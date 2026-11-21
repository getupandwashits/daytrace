package ui

import (
	"fmt"
	"sort"
	"time"

	"github.com/getupandwashits/daytrace/internal/core"

	vui "github.com/vivaui/viva/ui"
)

type view struct {
	Status           core.StatusSnapshot
	PhaseLabel       string
	Summary          core.ActivitySummary
	TodaySessions    []core.FocusSession
	AllSessions      []core.FocusSession
	LastSample       *core.ActivitySample
	LastExport       string
	LastExportAt     time.Time
	LastError        string
	Accessibility    bool
	CollectorRunning bool
	Settings         core.Settings
}

func (a *App) snapshot() view {
	a.mu.Lock()
	defer a.mu.Unlock()
	var last *core.ActivitySample
	if a.lastSample != nil {
		copy := *a.lastSample
		last = &copy
	}
	return view{
		Status:           a.Focus.Snapshot(),
		PhaseLabel:       a.Focus.PhaseLabel(),
		Summary:          a.summary,
		TodaySessions:    append([]core.FocusSession(nil), a.todaySessions...),
		AllSessions:      append([]core.FocusSession(nil), a.allSessions...),
		LastSample:       last,
		LastExport:       a.lastExport,
		LastExportAt:     a.lastExportAt,
		LastError:        a.lastError,
		Accessibility:    a.accessibility,
		CollectorRunning: a.collectorRunning,
		Settings:         a.Settings.Get(),
	}
}

// Page is the dashboard. It is declared from the snapshot on every repaint.
func (a *App) Page(c vui.C) {
	v := a.snapshot()
	vui.Column(c).Fill().Pad(20).Gap(14).Children(func() {
		a.header(c, v)
		vui.Tabs(c, &a.tab, []string{"概览", "历史", "设置"}, nil)
		switch a.tab {
		case "历史":
			a.historyPage(c, v)
		case "设置":
			a.settingsPage(c, v)
		default:
			a.overviewPage(c, v)
		}
	})
}

func (a *App) header(c vui.C, v view) {
	theme := c.Theme()
	vui.Row(c).FillWidth().Items(vui.Center).Children(func() {
		vui.Column(c).Grow(1).Gap(2).Children(func() {
			vui.Text(c, "DayTrace").FontSize(24).Bold()
			vui.Text(c, a.headerDate()).FontSize(12).Muted()
		})
		if !v.Accessibility {
			vui.Text(c, "缺少辅助功能权限").FontSize(12).Color(theme.Danger)
		}
		vui.Row(c).Gap(8).Children(func() {
			if vui.Button(c, "打开输出目录").Ghost().Clicked() {
				a.OpenOutput()
			}
			if vui.Button(c, "立即归档").Outline().Clicked() {
				a.ExportNow()
			}
		})
	})
}

func (a *App) overviewPage(c vui.C, v view) {
	theme := c.Theme()
	vui.Scroll(c).Grow(1).Gap(14).Children(func() {
		a.focusCard(c, v)

		vui.Row(c).Gap(12).Children(func() {
			focusTotal := 0.0
			for _, session := range v.TodaySessions {
				focusTotal += session.Duration
			}
			statCard(c, "今日活跃", core.FormatDuration(v.Summary.ActiveSeconds), fmt.Sprintf("%d 条采样", v.Summary.Samples))
			statCard(c, "今日专注", core.FormatDuration(focusTotal), fmt.Sprintf("%d 个会话", len(v.TodaySessions)))
			detail := "采集已关闭"
			if v.CollectorRunning {
				detail = "正在采集"
			}
			statCard(c, "活动采集", statusShort(v), detail)
		})

		if v.LastSample != nil {
			a.lastSampleCard(c, v, theme)
		}

		a.appUsageSection(c, v)
		a.hourlySection(c, v)
		a.sessionSection(c, v.TodaySessions, "今日专注记录", 8)
	})
}

func (a *App) focusCard(c vui.C, v view) {
	theme := c.Theme()
	status := v.Status
	vui.Column(c).FillWidth().Pad(18).Radius(14).Background(theme.Card).Border(1, theme.Border).Gap(10).Children(func() {
		vui.Row(c).Items(vui.Center).Gap(18).Children(func() {
			vui.Text(c, bigClock(status)).FontSize(44).Bold().Mono()
			vui.Column(c).Grow(1).Gap(2).Children(func() {
				vui.Text(c, v.PhaseLabel).FontSize(15).Bold()
				if status.TrackingSuspended {
					vui.Text(c, "采集已暂停").FontSize(12).Color(theme.Danger)
				}
			})
			vui.Row(c).Gap(8).Children(func() { a.focusButtons(c, status) })
		})
		fraction := float32(1 - status.RemainingFraction)
		if fraction < 0 {
			fraction = 0
		}
		if fraction > 1 {
			fraction = 1
		}
		vui.Progress(c, fraction).Height(8)
		vui.Row(c).Children(func() {
			vui.Text(c, focusDetail(status)).FontSize(11).Muted()
			vui.Spacer(c)
			vui.Text(c, focusNext(status)).FontSize(11).Muted()
		})
	})
}

func (a *App) focusButtons(c vui.C, status core.StatusSnapshot) {
	switch core.FocusState(status.State) {
	case core.StateActive:
		if vui.Button(c, "暂停").Outline().Clicked() {
			a.togglePause()
		}
		if vui.Button(c, "停止").Ghost().Clicked() {
			a.stopFocus()
		}
	case core.StatePaused, core.StateReminding:
		if vui.Button(c, "继续").Clicked() {
			a.togglePause()
		}
		if vui.Button(c, "停止").Ghost().Clicked() {
			a.stopFocus()
		}
	case core.StateStopped:
		if vui.Button(c, "继续专注").Clicked() {
			a.resumeFocus()
		}
	default:
		if vui.Button(c, "开始专注").Clicked() {
			a.startFocus()
		}
	}
	if vui.Button(c, "强制休息").Ghost().Clicked() {
		a.forceRest()
	}
}

func (a *App) headerDate() string {
	now := time.Now().In(core.Beijing)
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	return now.Format("2006-01-02") + " " + weekdays[now.Weekday()] + " · 自动专注与活动记录"
}

func (a *App) lastSampleCard(c vui.C, v view, theme *vui.Theme) {
	sample := v.LastSample
	vui.Column(c).FillWidth().Pad(14).Radius(10).Background(theme.Card).Border(1, theme.Border).Gap(3).Children(func() {
		vui.Row(c).Children(func() {
			vui.Text(c, "最近活动").FontSize(12).Muted()
			vui.Spacer(c)
			vui.Text(c, sample.Timestamp.In(core.Beijing).Format("15:04:05")+" · "+sample.StateLabel()).FontSize(12).Muted()
		})
		name := sample.AppName()
		if sample.WindowTitle != "" {
			name += " — " + sample.WindowTitle
		}
		vui.Text(c, name).FontSize(13).Ellipsis()
	})
}

func (a *App) appUsageSection(c vui.C, v view) {
	theme := c.Theme()
	apps := v.Summary.Apps
	if len(apps) > 8 {
		apps = apps[:8]
	}
	maximum := 0.0
	for _, app := range v.Summary.Apps {
		maximum = max(maximum, app.Seconds)
	}
	vui.Column(c).FillWidth().Pad(16).Radius(12).Background(theme.Card).Border(1, theme.Border).Gap(10).Children(func() {
		vui.Row(c).Children(func() {
			vui.Text(c, "应用用时（活跃）").FontSize(15).Bold()
			vui.Spacer(c)
			vui.Text(c, core.FormatDuration(v.Summary.ActiveSeconds)).FontSize(12).Muted()
		})
		if len(apps) == 0 {
			vui.Text(c, "今天还没有活跃记录。").FontSize(12).Muted()
			return
		}
		for _, app := range apps {
			app := app
			vui.Column(c).Gap(3).Children(func() {
				vui.Row(c).Children(func() {
					vui.Text(c, app.Name).FontSize(13).Ellipsis()
					vui.Spacer(c)
					vui.Text(c, core.FormatDuration(app.Seconds)).FontSize(12).Muted()
				})
				fraction := float32(0)
				if maximum > 0 {
					fraction = float32(app.Seconds / maximum)
				}
				vui.Progress(c, fraction).Height(5)
			})
		}
	})
}

func (a *App) hourlySection(c vui.C, v view) {
	theme := c.Theme()
	vui.Column(c).FillWidth().Pad(16).Radius(12).Background(theme.Card).Border(1, theme.Border).Gap(8).Children(func() {
		vui.Text(c, "逐小时活动").FontSize(15).Bold()
		if len(v.Summary.Hours) == 0 {
			vui.Text(c, "没有可显示的小时。").FontSize(12).Muted()
			return
		}
		for _, hour := range v.Summary.Hours {
			total := 0.0
			for _, app := range hour.Apps {
				total += app.Seconds
			}
			top := ""
			if len(hour.Apps) > 0 {
				top = hour.Apps[0].Name + " " + core.FormatDuration(hour.Apps[0].Seconds)
			}
			vui.Row(c).Items(vui.Center).Children(func() {
				vui.Text(c, hour.Hour.In(core.Beijing).Format("15:00")).FontSize(13).Mono().Width(56)
				vui.Text(c, core.FormatDuration(total)).FontSize(12).Width(72)
				vui.Text(c, top).FontSize(12).Muted().Ellipsis()
			})
		}
	})
}

func (a *App) sessionSection(c vui.C, sessions []core.FocusSession, title string, limit int) {
	theme := c.Theme()
	vui.Column(c).FillWidth().Pad(16).Radius(12).Background(theme.Card).Border(1, theme.Border).Gap(8).Children(func() {
		vui.Text(c, title).FontSize(15).Bold()
		if len(sessions) == 0 {
			vui.Text(c, "还没有专注记录。").FontSize(12).Muted()
			return
		}
		start := max(0, len(sessions)-limit)
		for index := len(sessions) - 1; index >= start; index-- {
			session := sessions[index]
			vui.Row(c).Items(vui.Center).Children(func() {
				vui.Text(c, session.Start.In(core.Beijing).Format("01-02 15:04")).FontSize(12).Mono().Width(100)
				vui.Text(c, core.FormatDuration(session.Duration)).FontSize(12).Width(72)
				trigger := "自动"
				if session.Trigger == "manual" {
					trigger = "手动"
				}
				vui.Text(c, trigger).FontSize(12).Muted()
			})
		}
	})
}

func (a *App) historyPage(c vui.C, v view) {
	theme := c.Theme()
	vui.Scroll(c).Grow(1).Gap(14).Children(func() {
		vui.Column(c).FillWidth().Pad(16).Radius(12).Background(theme.Card).Border(1, theme.Border).Gap(6).Children(func() {
			vui.Text(c, "归档").FontSize(15).Bold()
			line := "还没有执行过归档。"
			if v.LastExport != "" {
				line = v.LastExport
				if !v.LastExportAt.IsZero() {
					line += " · " + v.LastExportAt.In(core.Beijing).Format("15:04:05")
				}
			}
			vui.Text(c, line).FontSize(12).Muted()
			vui.Text(c, v.Settings.OutputDirectory).FontSize(12).Mono().Ellipsis()
			if vui.Button(c, "立即归档").Outline().Clicked() {
				a.ExportNow()
			}
		})

		daily := dailyTotals(v.AllSessions)
		vui.Column(c).FillWidth().Pad(16).Radius(12).Background(theme.Card).Border(1, theme.Border).Gap(8).Children(func() {
			vui.Text(c, "每日专注").FontSize(15).Bold()
			if len(daily) == 0 {
				vui.Text(c, "还没有专注记录。").FontSize(12).Muted()
				return
			}
			for index, day := range daily {
				if index >= 14 {
					break
				}
				vui.Row(c).Items(vui.Center).Children(func() {
					vui.Text(c, day.day).FontSize(13).Mono().Width(110)
					vui.Text(c, core.FormatDuration(day.seconds)).FontSize(12).Width(80)
					vui.Text(c, fmt.Sprintf("%d 个会话", day.count)).FontSize(12).Muted()
				})
			}
		})

		a.sessionSection(c, v.AllSessions, "全部专注记录", 60)
	})
}

type dayTotal struct {
	day     string
	seconds float64
	count   int
}

func dailyTotals(sessions []core.FocusSession) []dayTotal {
	byDay := map[string]*dayTotal{}
	for _, session := range sessions {
		key := dayLabel(session.Start)
		entry := byDay[key]
		if entry == nil {
			entry = &dayTotal{day: key}
			byDay[key] = entry
		}
		entry.seconds += session.Duration
		entry.count++
	}
	out := make([]dayTotal, 0, len(byDay))
	for _, entry := range byDay {
		out = append(out, *entry)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].day > out[b].day })
	return out
}

func (a *App) settingsPage(c vui.C, v view) {
	theme := c.Theme()
	vui.Scroll(c).Grow(1).Gap(14).Children(func() {
		if v.LastError != "" {
			vui.Column(c).FillWidth().Pad(12).Radius(10).Background(theme.Card).Border(1, theme.Danger).Children(func() {
				vui.Text(c, v.LastError).FontSize(12).Color(theme.Danger)
			})
		}
		if !v.Accessibility {
			vui.Column(c).FillWidth().Pad(14).Radius(12).Background(theme.Card).Border(1, theme.Danger).Gap(8).Children(func() {
				vui.Text(c, "需要辅助功能权限").FontSize(14).Bold()
				vui.Text(c, "DayTrace 只有读取当前窗口文字才能生成活动记录；不会截屏、记录按键或联网。").FontSize(12).Muted()
				if vui.Button(c, "请求权限").Clicked() {
					a.Collector.RequestAccessibility()
				}
			})
		}

		settingsCard(c, "专注", func() {
			fieldRow(c, "专注目标（分钟）", "25", &a.form.automaticFocus)
			fieldRow(c, "停顿阈值（分钟）", "3", &a.form.pauseTolerance)
			fieldRow(c, "离开重置（分钟）", "10", &a.form.resetThreshold)
			fieldRow(c, "休息（分钟）", "5", &a.form.rest)
			fieldRow(c, "最短记录（分钟）", "5", &a.form.minimumRecord)
		})

		settingsCard(c, "活动采集", func() {
			vui.Switch(c, &a.form.collect, "运行时采集前台窗口")
			fieldRow(c, "采样间隔（秒）", "10", &a.form.sampleInterval)
			fieldRow(c, "闲置阈值（秒）", "180", &a.form.idleThreshold)
			vui.Column(c).Gap(4).Children(func() {
				vui.Text(c, "排除的应用").FontSize(13)
				vui.TextArea(c, &a.form.excluded, "排除的应用", "bundle id 或应用名，用逗号分隔").Height(64)
			})
			if v.CollectorRunning {
				vui.Text(c, "采集正在运行").FontSize(12).Color(theme.Success)
			}
		})

		settingsCard(c, "归档", func() {
			vui.Switch(c, &a.form.autoExport, "每天 23:00 自动归档")
			fieldRow(c, "输出目录", "~/Documents/DayTrace", &a.form.output)
			vui.Text(c, "文件名保持 Daily_record 的约定：定时归档为 YYYY-MM-DD_23-00，手动归档带 _manual。").FontSize(12).Muted()
		})

		vui.Row(c).Gap(8).Justify(vui.End).Children(func() {
			if vui.Button(c, "恢复当前设置").Ghost().Clicked() {
				a.form = formFromSettings(a.Settings.Get())
			}
			if vui.Button(c, "保存设置").Clicked() {
				a.SaveSettings()
			}
		})
	})
}

func settingsCard(c vui.C, title string, body func()) {
	theme := c.Theme()
	vui.Column(c).FillWidth().Pad(16).Radius(12).Background(theme.Card).Border(1, theme.Border).Gap(10).Children(func() {
		vui.Text(c, title).FontSize(15).Bold()
		body()
	})
}

func fieldRow(c vui.C, label, placeholder string, value *string) {
	vui.Row(c).Items(vui.Center).Gap(12).Children(func() {
		vui.Text(c, label).FontSize(13).Width(150)
		vui.TextField(c, value, label, placeholder).Grow(1).MaxWidth(360)
	})
}

func choiceRow(c vui.C, label string, body func()) {
	vui.Row(c).Items(vui.Center).Gap(12).Children(func() {
		vui.Text(c, label).FontSize(13).Width(150)
		body()
	})
}

func statCard(c vui.C, label, value, detail string) {
	theme := c.Theme()
	vui.Column(c).Grow(1).Pad(14).Radius(10).Background(theme.Card).Border(1, theme.Border).Gap(3).Children(func() {
		vui.Text(c, label).FontSize(12).Muted()
		vui.Text(c, value).FontSize(24).Bold()
		vui.Text(c, detail).FontSize(11).Muted()
	})
}

func bigClock(status core.StatusSnapshot) string {
	switch core.FocusState(status.State) {
	case core.StateAway:
		if status.RestRemainingSeconds > 0 {
			return clockText(status.RestRemainingSeconds)
		}
		return "休息"
	case core.StateStopped:
		return clockText(status.FocusElapsedSeconds)
	case core.StateReminding:
		return "该休息"
	default:
		return clockText(status.FocusRemainingSeconds)
	}
}

func focusDetail(status core.StatusSnapshot) string {
	switch core.FocusState(status.State) {
	case core.StateAway:
		return fmt.Sprintf("休息目标 %s", core.FormatDuration(status.RestTargetSeconds))
	case core.StateStopped:
		return "停止后不计入专注记录"
	default:
		return fmt.Sprintf("已专注 %s / %s", core.FormatDuration(status.FocusElapsedSeconds), core.FormatDuration(status.FocusTargetSeconds))
	}
}

func focusNext(status core.StatusSnapshot) string {
	switch core.FocusState(status.State) {
	case core.StateActive:
		return "离开一会儿会自动暂停"
	case core.StatePaused:
		return "回来后自动继续"
	case core.StateReminding:
		return "休息后自动开始下一轮"
	case core.StateAway:
		return "开始操作后自动专注"
	case core.StateStopped:
		return "点击继续接着计时"
	default:
		return ""
	}
}

func statusShort(v view) string {
	if v.CollectorRunning {
		return "开启"
	}
	return "关闭"
}
