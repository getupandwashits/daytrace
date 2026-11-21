package ui

import (
	"strings"
	"testing"

	vivaapp "github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"
	vui "github.com/vivaui/viva/ui"
)

func open(t *testing.T, app *App) *vivaapp.Driver {
	t.Helper()
	page := vui.NewPage(vui.PageConfig{View: app.Page, Invalidator: app.Invalidator})
	driver, err := vivaapp.NewDriver(page.Config(vivaapp.Config{Title: "DayTrace", Width: 920, Height: 740}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(driver.Close)
	return driver
}

func click(t *testing.T, driver *vivaapp.Driver, name string) {
	t.Helper()
	for _, hit := range driver.Snapshot().Hits {
		if hit.Label == name && !hit.IsLabel {
			bounds := hit.Bounds
			if err := driver.Click(bounds.X+bounds.Width/2, bounds.Y+bounds.Height/2); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no control named %q", name)
}

func scroll(t *testing.T, driver *vivaapp.Driver, deltaY float32) {
	t.Helper()
	if err := driver.Scroll(gfx.ScrollEvent{X: 450, Y: 500, DeltaY: deltaY, Viewport: gfx.Size{Width: 920, Height: 740}}); err != nil {
		t.Fatal(err)
	}
}

func shows(driver *vivaapp.Driver, want string) bool {
	snapshot := driver.Snapshot()
	for _, label := range snapshot.Labels {
		if strings.Contains(label.Value, want) {
			return true
		}
	}
	for _, hit := range snapshot.Hits {
		if strings.Contains(hit.Label, want) {
			return true
		}
	}
	return false
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("DAYTRACE_HOME", t.TempDir())
	t.Setenv("DAYTRACE_OUTPUT", t.TempDir())
	return NewApp()
}

func TestDashboardStartsFocus(t *testing.T) {
	app := newTestApp(t)
	driver := open(t, app)
	click(t, driver, "开始专注")
	if !shows(driver, "专注中") {
		t.Fatal("dashboard does not show the active focus state")
	}
	if !shows(driver, "暂停") {
		t.Fatal("pause control missing after starting")
	}
}

func TestSettingsTabShowsForm(t *testing.T) {
	app := newTestApp(t)
	driver := open(t, app)
	click(t, driver, "设置")
	if !shows(driver, "专注目标（分钟）") {
		t.Fatalf("settings form missing (tab=%q)", app.tab)
	}
	if !shows(driver, "排除的应用") {
		scroll(t, driver, 600)
		driver.Pump()
	}
	if !shows(driver, "排除的应用") {
		t.Fatal("excluded apps editor missing")
	}
}

func TestMenuFollowsFocusState(t *testing.T) {
	app := newTestApp(t)
	app.startFocus()
	items := app.Menu()
	found := false
	for _, item := range items {
		if strings.Contains(item.Title, "专注中") {
			found = true
		}
	}
	if !found {
		t.Fatalf("menu does not show the focus line: %+v", items)
	}
}

func TestStoppedFocusOffersResume(t *testing.T) {
	app := newTestApp(t)
	app.startFocus()
	driver := open(t, app)
	click(t, driver, "停止")
	if !shows(driver, "继续专注") {
		t.Fatal("stopped state does not offer resume")
	}
	click(t, driver, "继续专注")
	if !shows(driver, "专注中") {
		t.Fatalf("resume did not return to focus (state=%s)", app.Focus.Snapshot().State)
	}
}

func TestMenuResumesStoppedFocus(t *testing.T) {
	app := newTestApp(t)
	app.startFocus()
	app.stopFocus()
	found := false
	for _, item := range app.Menu() {
		if item.Title == "继续专注" {
			found = true
		}
	}
	if !found {
		t.Fatalf("menu has no resume item: %+v", app.Menu())
	}
	app.resumeFocus()
	if got := app.Focus.Snapshot().State; got != "active" {
		t.Fatalf("state after resume = %q", got)
	}
}

func TestSaveSettingsClampsAndPersists(t *testing.T) {
	app := newTestApp(t)
	app.form.collect = false
	app.form.automaticFocus = "0"
	app.form.sampleInterval = "0"
	app.form.excluded = "com.apple.Terminal, 1Password"
	app.form.output = t.TempDir()
	app.SaveSettings()

	saved := app.Settings.Get()
	if saved.AutomaticFocusMin != 1 {
		t.Errorf("automatic focus = %v, want clamped to 1", saved.AutomaticFocusMin)
	}
	if saved.SampleIntervalSec != 2 {
		t.Errorf("sample interval = %v, want clamped to 2", saved.SampleIntervalSec)
	}
	if len(saved.ExcludedApps) != 2 || saved.ExcludedApps[0] != "com.apple.Terminal" {
		t.Errorf("excluded apps = %v", saved.ExcludedApps)
	}
	if saved.OutputDirectory != app.form.output {
		t.Errorf("output = %q", saved.OutputDirectory)
	}
	if app.form.automaticFocus != "1" {
		t.Errorf("form was not refreshed from saved settings: %q", app.form.automaticFocus)
	}
}
