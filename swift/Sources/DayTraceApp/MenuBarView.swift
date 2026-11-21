import DayTraceCore
import SwiftUI

struct MenuBarView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            header
            controls
            Divider()
            today
            if !model.accessibilityGranted {
                Divider()
                accessibility
            }
            Divider()
            actions
        }
        .padding(14)
        .frame(width: 330)
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(model.engine.phaseLabel)
                    .font(.headline)
                Spacer()
                Text(model.engine.mode.label)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Text(model.engine.menuBarTitle)
                .font(.system(size: 34, weight: .semibold, design: .monospaced))
                .monospacedDigit()
                .contentTransition(.numericText())
            ProgressView(value: 1 - model.engine.remainingFraction)
                .tint(model.engine.state == .away ? .teal : .orange)
            HStack {
                Text("已专注 \(model.engine.formattedElapsed)")
                Spacer()
                Text("完成 \(model.todaySessionCount) 段")
            }
            .font(.caption)
            .foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private var controls: some View {
        HStack(spacing: 8) {
            switch model.engine.state {
            case .active:
                Button("暂停") { model.engine.stopFocusManually() }
                Button("强制休息") { model.engine.resetToAway(trigger: .manual) }
            case .paused:
                Button("继续") { model.engine.togglePause() }
                Button("强制休息") { model.engine.resetToAway(trigger: .manual) }
            case .stopped:
                Button("继续专注") { model.engine.resumeFocusManually() }
                Button("重新开始") { model.engine.restartFocusManually() }
            case .away:
                Button("开始专注") { model.engine.startFocusManually() }
            case .reminding:
                Button("开始休息") { model.engine.resetToAway(trigger: .manual) }
                Button("重新开始") { model.engine.startFocusManually() }
            }
        }
        .buttonStyle(.bordered)
        .controlSize(.small)
    }

    private var today: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Label("今日活跃", systemImage: "figure.walk")
                Spacer()
                Text(DigestEngine.duration(model.summary.activeSeconds))
                    .monospacedDigit()
            }
            .font(.callout)
            HStack {
                Label("今日专注", systemImage: "timer")
                Spacer()
                Text(DigestEngine.duration(model.todayFocusSeconds))
                    .monospacedDigit()
            }
            .font(.callout)
            if let app = model.summary.apps.first {
                HStack {
                    Label(app.name, systemImage: "app")
                        .lineLimit(1)
                    Spacer()
                    Text(DigestEngine.duration(app.seconds))
                        .monospacedDigit()
                }
                .font(.caption)
                .foregroundStyle(.secondary)
            }
        }
    }

    private var accessibility: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("尚未获得辅助功能权限，只能记录应用名。")
                .font(.caption)
                .foregroundStyle(.secondary)
            Button("授权辅助功能…") { model.requestAccessibility() }
                .controlSize(.small)
        }
    }

    private var actions: some View {
        VStack(alignment: .leading, spacing: 8) {
            if !model.exportMessage.isEmpty {
                Text(model.exportMessage)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            HStack {
                Button("打开仪表盘") {
                    openWindow(id: "dashboard")
                    NSApp.activate(ignoringOtherApps: true)
                }
                Button("立即归档") { model.exportNow() }
                SettingsLink { Text("设置…") }
            }
            .controlSize(.small)
            Button("退出 DayTrace") { NSApp.terminate(nil) }
                .controlSize(.small)
                .foregroundStyle(.secondary)
        }
    }
}
