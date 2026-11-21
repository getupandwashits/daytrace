import AppKit
import DayTraceCore
import SwiftUI

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        AppModel.shared.start()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }
}

@main
struct DayTraceApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @StateObject private var model = AppModel.shared

    var body: some Scene {
        MenuBarExtra {
            MenuBarView()
                .environmentObject(model)
        } label: {
            MenuBarLabel(model: model)
        }
        .menuBarExtraStyle(.window)

        Window("DayTrace", id: "dashboard") {
            DashboardView()
                .environmentObject(model)
        }
        .defaultSize(width: 940, height: 680)
        .defaultLaunchBehavior(.presented)

        Settings {
            SettingsView(isSettingsWindow: true)
                .environmentObject(model)
        }
    }
}

private struct MenuBarLabel: View {
    @ObservedObject var model: AppModel

    var body: some View {
        HStack(spacing: 4) {
            Image(systemName: icon)
                .imageScale(.small)
            Text(model.engine.menuBarTitle)
                .font(.system(size: 12, weight: .semibold, design: .monospaced))
                .monospacedDigit()
        }
        .fixedSize()
    }

    private var icon: String {
        switch model.engine.state {
        case .active: return "timer"
        case .paused: return "pause.circle"
        case .away: return "cup.and.saucer"
        case .reminding: return "bell.badge"
        case .stopped: return "stop.circle"
        }
    }
}
