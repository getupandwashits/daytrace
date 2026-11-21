package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/getupandwashits/daytrace/internal/core"
	"github.com/getupandwashits/daytrace/internal/macos"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "doctor":
		doctor()
	case "sample":
		sample()
	case "collect":
		collect()
	case "demo":
		demo()
	case "export":
		export(os.Args[2:])
	case "due":
		due()
	case "status":
		status()
	case "settings":
		settings()
	case "legacy":
		legacy()
	case "analyze":
		analyze(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`DayTrace agent

  doctor            路径、权限、旧实例与归档状态
  sample            立即采集一条前台样本并打印
  collect           持续采集并打印（Ctrl-C 退出）
  demo              写入隔离环境中的演示数据并归档
  export [--due|--force]
                    立即归档（--due 走定时判断，--force 从当天开始）
  due               查看下一次定时归档与是否到期
  status            打印当前 status.json
  settings          打印当前设置
  legacy            查看 AutoPomodoro 兼容状态的归属
  analyze [YYYY-MM-DD]
                    打印某天的活动汇总
`)
}

func doctor() {
	settings := core.NewSettingsStore(core.SettingsPath()).Get()
	fmt.Println("DayTrace doctor")
	fmt.Println("home:       ", core.Home())
	fmt.Println("journal:    ", core.JournalDir())
	fmt.Println("settings:   ", core.SettingsPath())
	fmt.Println("status:     ", core.StatusPath())
	fmt.Println("output:     ", settings.OutputDirectory)
	fmt.Println("accessibility:", macos.AXTrusted())
	fmt.Println("legacy paths:")
	for _, path := range core.LegacyStatusPaths() {
		fmt.Println("  -", path)
	}
	fmt.Println("legacy owned by another instance:", core.NewLegacyStatusWriter().OwnedByAnotherInstance())
	exporter := core.NewExporter(nil, nil, "")
	state := exporter.LoadState()
	if state.LastExportEnd.IsZero() {
		fmt.Println("last export: (never)")
	} else {
		fmt.Println("last export:", state.LastExportEnd.In(core.Beijing).Format(time.RFC3339), state.LastExportFile)
	}
	if status, err := core.ReadStatus(core.StatusPath()); err == nil {
		fmt.Println("status:", status.StateLabel, "focus", core.FormatDuration(status.FocusElapsedSeconds))
	} else {
		fmt.Println("status: (missing)")
	}
}

func sample() {
	settings := core.NewSettingsStore(core.SettingsPath()).Get()
	collector := core.NewCollector(core.CollectorConfigFromSettings(settings), core.NewJournal(core.EventsDir()))
	ch := make(chan core.ActivitySample, 1)
	collector.OnSample = func(sample core.ActivitySample) {
		select {
		case ch <- sample:
		default:
		}
	}
	collector.OnError = func(message string) { fmt.Fprintln(os.Stderr, message) }
	collector.Start()
	defer collector.Stop()
	select {
	case sample := <-ch:
		printJSON(sample)
	case <-time.After(20 * time.Second):
		fmt.Fprintln(os.Stderr, "没有采集到样本；请检查辅助功能权限和前台窗口。")
		os.Exit(1)
	}
}

func collect() {
	settings := core.NewSettingsStore(core.SettingsPath()).Get()
	collector := core.NewCollector(core.CollectorConfigFromSettings(settings), core.NewJournal(core.EventsDir()))
	collector.OnSample = func(sample core.ActivitySample) {
		name := sample.AppName()
		if sample.WindowTitle != "" {
			name += " — " + sample.WindowTitle
		}
		fmt.Printf("%s %-9s %s\n", sample.Timestamp.In(core.Beijing).Format("15:04:05"), sample.State, name)
	}
	collector.OnError = func(message string) { fmt.Fprintln(os.Stderr, message) }
	collector.Start()
	defer collector.Stop()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}

func demo() {
	if os.Getenv("DAYTRACE_HOME") == "" || os.Getenv("DAYTRACE_OUTPUT") == "" {
		fmt.Fprintln(os.Stderr, "demo 需要隔离环境：")
		fmt.Fprintln(os.Stderr, `  DAYTRACE_HOME="$PWD/.demo/home" DAYTRACE_OUTPUT="$PWD/.demo/output" daytrace-agent demo`)
		os.Exit(2)
	}
	if err := core.EnsureDirectories(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	journal := core.NewJournal(core.EventsDir())
	now := time.Now()
	base := now.Add(-45 * time.Minute)
	apps := []struct {
		name  string
		title string
		lines []string
	}{
		{"Safari", "Viva — Go native UI", []string{
			"Viva draws native windows with Go and Metal.",
			"菜单栏应用也可以直接打开面板。",
			"The page function declares the whole window again on every repaint.",
		}},
		{"Code", "daytrace — internal/core/exporter.go", []string{
			"func (e *Exporter) ExportNow(now time.Time, force bool) ExportResult",
			"归档文件名保持 YYYY-MM-DD_23-00，手动归档带 _manual。",
			"sk-ant-api03-abcdefghijklmnopqrstuvwxyz012345 should be masked",
		}},
		{"Terminal", "daytrace — go test", []string{
			"ok  github.com/getupandwashits/daytrace/internal/core",
			"PASS",
			"coverage: 82.4% of statements",
		}},
	}
	for index := 0; index < 90; index++ {
		app := apps[(index/30)%len(apps)]
		text := strings.Join(app.lines, "\n") + "\n第 " + fmt.Sprint(index+1) + " 次采样"
		stamp := base.Add(time.Duration(index) * 10 * time.Second)
		sample := core.ActivitySample{
			Timestamp:      stamp,
			State:          "active",
			SampleInterval: 10,
			IdleSeconds:    0,
			Accessibility:  true,
			App:            app.name,
			BundleID:       "demo." + strings.ToLower(app.name),
			WindowTitle:    app.title,
			UIText:         &text,
			ContextID:      "demo-" + app.name,
			CaptureStatus:  "ok",
		}
		if err := journal.Append(sample); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	idle := "锁屏期间的界面没有采集。"
	locked := core.ActivitySample{Timestamp: base.Add(20 * time.Minute), State: "locked", UIText: &idle}
	_ = journal.Append(locked)

	exporter := core.NewExporter(journal, core.NewSettingsStore(core.SettingsPath()), core.ExportStatePath())
	result := exporter.ExportNow(now, true)
	fmt.Println(result.Message)
	for _, path := range result.Written {
		fmt.Println("  ", path)
	}
}

func export(args []string) {
	flags := flag.NewFlagSet("export", flag.ExitOnError)
	isDue := flags.Bool("due", false, "use the scheduled cutoff")
	force := flags.Bool("force", false, "start from the beginning of today")
	_ = flags.Parse(args)
	exporter := core.NewExporter(nil, nil, "")
	now := time.Now()
	var result core.ExportResult
	switch {
	case *isDue:
		result = exporter.ExportDue(now)
	case *force:
		result = exporter.ExportNow(now, true)
	default:
		result = exporter.ExportNow(now, false)
	}
	fmt.Println(result.Message)
	for _, path := range result.Written {
		fmt.Println("  ", path)
	}
}

func due() {
	exporter := core.NewExporter(nil, nil, "")
	now := time.Now()
	fmt.Println("checkpoint:", exporter.ScheduledCheckpoint(now).In(core.Beijing).Format(time.RFC3339))
	fmt.Println("due:       ", exporter.IsDue(now))
	state := exporter.LoadState()
	if !state.LastExportEnd.IsZero() {
		fmt.Println("last end:  ", state.LastExportEnd.In(core.Beijing).Format(time.RFC3339))
	}
}

func status() {
	status, err := core.ReadStatus(core.StatusPath())
	if err != nil {
		settings := core.NewSettingsStore(core.SettingsPath()).Get()
		status = core.NewFocusEngine(settings, time.Now, nil).Snapshot()
	}
	printJSON(status)
}

func settings() {
	printJSON(core.NewSettingsStore(core.SettingsPath()).Get())
}

func legacy() {
	writer := core.NewLegacyStatusWriter()
	fmt.Println("owned by another instance:", writer.OwnedByAnotherInstance())
	for _, path := range writer.URLs {
		fmt.Println(path)
		if data, err := os.ReadFile(path); err == nil {
			fmt.Println(string(data))
		} else {
			fmt.Println("  (missing)")
		}
	}
}

func analyze(args []string) {
	day := core.StartOfDay(time.Now())
	if len(args) > 0 {
		parsed, err := time.ParseInLocation("2006-01-02", args[0], core.Beijing)
		if err != nil {
			fmt.Fprintln(os.Stderr, "日期格式应为 YYYY-MM-DD")
			os.Exit(2)
		}
		day = parsed
	}
	samples, err := core.NewJournal(core.EventsDir()).SamplesOn(day)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	printJSON(core.Analyze(samples).Summary)
}

func printJSON(value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
