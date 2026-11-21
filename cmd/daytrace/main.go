package main

import (
	"log"
	"os"
	"runtime"

	"github.com/getupandwashits/daytrace/internal/macos"
	"github.com/getupandwashits/daytrace/internal/ui"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/gfx"
	vui "github.com/vivaui/viva/ui"
)

func main() {
	// Pin the main goroutine to the process's main thread before anything else
	// touches Cocoa. If it migrates, Viva's RunApplication locks a worker thread
	// instead, AppKit then lives off the main thread, and any dispatch_sync to
	// the main queue inside window creation deadlocks before the run loop starts.
	runtime.LockOSThread()

	daytrace := ui.NewApp()
	defer daytrace.Stop()

	err := app.RunApplication(func(application *app.Application) error {
		var window *app.Window

		open := func() error {
			page := vui.NewPage(vui.PageConfig{View: daytrace.Page, Invalidator: daytrace.Invalidator})
			config := page.Config(app.Config{
				Title:     "DayTrace",
				Width:     920,
				Height:    740,
				MinWidth:  720,
				MinHeight: 520,
			})
			// The dashboard replaces its own close button with Hide, so the
			// menu bar item can bring it back.
			config.OnCloseRequest = func(allow func()) {
				if window != nil {
					window.Hide()
				}
			}
			opened, err := application.OpenWindow(config, new(gfx.Canvas))
			if err != nil {
				return err
			}
			window = opened
			return nil
		}

		if err := open(); err != nil {
			return err
		}
		if os.Getenv("DAYTRACE_SHOW_DASHBOARD") == "" {
			window.Hide()
		} else {
			macos.Activate()
		}

		daytrace.SetOpenDashboard(func(tab string) {
			daytrace.SetTab(tab)
			if window.Closed() {
				if err := open(); err != nil {
					log.Printf("open dashboard: %v", err)
					return
				}
			}
			window.Show()
			macos.Activate()
			daytrace.Invalidator.Invalidate()
		})
		daytrace.SetQuit(application.Quit)

		// Viva's status items are icon-only, so the text item lives in the
		// macos package; the loop feeds it the title and menu every second.
		macos.InstallStatusItem()
		macos.InstallPresence(daytrace.SetPresence)
		daytrace.Start()
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
