import DayTraceCore
import SwiftUI

struct DashboardView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        TabView {
            TodayView()
                .tabItem { Label("今天", systemImage: "sun.max") }
            HistoryView()
                .tabItem { Label("历史", systemImage: "chart.bar.xaxis") }
            SettingsView()
                .tabItem { Label("设置", systemImage: "gearshape") }
        }
        .padding(10)
        .frame(minWidth: 880, minHeight: 620)
    }
}

struct TodayView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                focusCard
                HStack(spacing: 12) {
                    StatCard(
                        title: "今日专注",
                        value: DigestEngine.duration(model.todayFocusSeconds),
                        subtitle: "\(model.todaySessionCount) 段记录",
                        icon: "timer",
                        tint: .orange
                    )
                    StatCard(
                        title: "今日活跃",
                        value: DigestEngine.duration(model.summary.activeSeconds),
                        subtitle: "\(model.summary.samples) 条采样",
                        icon: "figure.walk",
                        tint: .teal
                    )
                    StatCard(
                        title: "最近 7 天专注",
                        value: DigestEngine.duration(model.weekFocusSeconds),
                        subtitle: "\(model.sessions.count) 段累计",
                        icon: "calendar",
                        tint: .blue
                    )
                    StatCard(
                        title: "最近归档",
                        value: model.lastExportAt.map { TimeFormat.clock($0) } ?? "—",
                        subtitle: model.lastExportEntry?.lastPathComponent ?? "尚未归档",
                        icon: "tray.and.arrow.down",
                        tint: .purple
                    )
                }
                HStack(alignment: .top, spacing: 12) {
                    appsCard
                    hourlyCard
                }
                exportCard
            }
            .padding(20)
        }
    }

    private var focusCard: some View {
        Card {
            VStack(alignment: .leading, spacing: 12) {
                HStack(alignment: .firstTextBaseline) {
                    Text(model.engine.phaseLabel)
                        .font(.title2.weight(.semibold))
                    if model.engine.suspended {
                        Label("系统已暂停计时", systemImage: "moon.zzz")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    Text(model.engine.mode.label)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
                HStack(alignment: .firstTextBaseline, spacing: 12) {
                    Text(model.engine.state == .away && model.engine.restRemainingDuration == 0 ? "准备" : model.engine.menuBarTitle)
                        .font(.system(size: 50, weight: .semibold, design: .monospaced))
                        .monospacedDigit()
                    Text(model.engine.state == .away
                         ? (model.engine.restRemainingDuration > 0 ? "休息剩余" : "等待下一次专注")
                         : "下次提醒剩余")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
                ProgressView(value: 1 - model.engine.remainingFraction)
                    .tint(model.engine.state == .away ? .teal : .orange)
                HStack(spacing: 10) {
                    Text("已专注 \(model.engine.formattedElapsed)")
                    Text("·")
                    Text("目标 \(DigestEngine.duration(model.engine.targetFocusTime))")
                    Text("·")
                    Text("本轮完成 \(model.engine.completedPomodoros) / \(model.settings.rounds)")
                    Spacer()
                }
                .font(.caption)
                .foregroundStyle(.secondary)
                controls
            }
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
            Button("采集一次") { model.captureNow() }
                .controlSize(.small)
            Spacer()
        }
        .buttonStyle(.bordered)
    }

    private var appsCard: some View {
        Card {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    Text("应用用时").font(.headline)
                    Spacer()
                    Text("今日").font(.caption).foregroundStyle(.secondary)
                }
                if model.summary.apps.isEmpty {
                    Text("还没有活动采样。")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                } else {
                    let maximum = max(1, model.summary.apps.first?.seconds ?? 1)
                    ForEach(model.summary.apps.prefix(8)) { app in
                        VStack(spacing: 3) {
                            HStack {
                                Text(app.name).lineLimit(1)
                                Spacer()
                                Text(DigestEngine.duration(app.seconds))
                                    .monospacedDigit()
                                    .foregroundStyle(.secondary)
                            }
                            .font(.caption)
                            ProgressView(value: app.seconds / maximum)
                                .tint(.orange)
                        }
                    }
                }
            }
        }
        .frame(maxWidth: .infinity)
    }

    private var hourlyCard: some View {
        Card {
            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    Text("逐小时活动").font(.headline)
                    Spacer()
                    Text("最近 \(min(8, model.summary.hours.count)) 小时")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if model.summary.hours.isEmpty {
                    Text("还没有活动采样。")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(model.summary.hours.suffix(8)) { hour in
                        HStack(alignment: .firstTextBaseline) {
                            Text(TimeFormat.hour(hour.hour))
                                .font(.caption.monospacedDigit())
                                .foregroundStyle(.secondary)
                                .frame(width: 72, alignment: .leading)
                            Text(hour.apps.prefix(3).map { "\($0.name) \(DigestEngine.duration($0.seconds))" }.joined(separator: " · "))
                                .font(.caption)
                                .lineLimit(1)
                            Spacer(minLength: 0)
                        }
                    }
                }
            }
        }
        .frame(maxWidth: .infinity)
    }

    private var exportCard: some View {
        Card {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    Text("每日归档").font(.headline)
                    Spacer()
                    Text(model.settings.outputDirectory)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                }
                HStack(spacing: 16) {
                    Label(model.collectorRunning ? "采集中" : "未采集", systemImage: model.collectorRunning ? "record.circle" : "pause.circle")
                        .foregroundStyle(model.collectorRunning ? .green : .secondary)
                    Label(model.accessibilityGranted ? "辅助功能已授权" : "辅助功能未授权", systemImage: model.accessibilityGranted ? "checkmark.shield" : "exclamationmark.shield")
                        .foregroundStyle(model.accessibilityGranted ? .green : .orange)
                    if let sample = model.lastSample {
                        Text("最近采样 \(TimeFormat.clock(sample.timestamp))")
                    }
                }
                .font(.caption)
                if !model.exportMessage.isEmpty {
                    Text(model.exportMessage)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
                HStack(spacing: 8) {
                    Button("立即归档") { model.exportNow() }
                    Button("重新生成今天") { model.exportFullDay() }
                    Button("打开文件夹") { model.openOutputFolder() }
                    Spacer()
                    if !model.accessibilityGranted {
                        Button("授权辅助功能…") { model.requestAccessibility() }
                    }
                }
                .controlSize(.small)
            }
        }
    }
}
