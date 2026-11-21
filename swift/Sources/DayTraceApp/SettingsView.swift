import AppKit
import DayTraceCore
import SwiftUI

struct SettingsView: View {
    var isSettingsWindow = false

    @EnvironmentObject private var model: AppModel
    @State private var draft = DayTraceSettings()
    @State private var excludedText = ""
    @State private var message = ""

    var body: some View {
        Form {
            Section("计时模式") {
                Picker("模式", selection: $draft.mode) {
                    Text("自动专注").tag("automatic")
                    Text("传统番茄钟").tag("traditional")
                }
                .pickerStyle(.segmented)
                Text(draft.mode == "automatic"
                     ? "根据键鼠空闲判断专注与休息，达到目标后提醒。"
                     : "固定专注时长，完成后进入短休息或长休息。")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            Section("专注与休息") {
                numberRow("自动专注目标", value: $draft.automaticFocusMinutes, unit: "分钟", range: 1...240)
                numberRow("传统专注时长", value: $draft.traditionalFocusMinutes, unit: "分钟", range: 1...180)
                numberRow("短休息", value: $draft.shortBreakMinutes, unit: "分钟", range: 1...60)
                numberRow("长休息", value: $draft.longBreakMinutes, unit: "分钟", range: 1...120)
                Stepper("每组番茄数：\(draft.rounds)", value: $draft.rounds, in: 1...20)
            }

            Section("空闲判定") {
                numberRow("短暂停顿阈值", value: $draft.pauseToleranceMinutes, unit: "分钟", range: 1...60)
                numberRow("休息重置阈值", value: $draft.resetThresholdMinutes, unit: "分钟", range: 2...120)
                numberRow("最短记录时长", value: $draft.minimumRecordMinutes, unit: "分钟", range: 1...60)
                Toggle("传统模式下空闲时自动暂停", isOn: $draft.pauseWhenIdle)
            }

            Section("活动采集") {
                Toggle("持续采集前台窗口", isOn: $draft.collectActivity)
                numberRow("采样间隔", value: $draft.sampleIntervalSeconds, unit: "秒", range: 2...300)
                numberRow("空闲判定阈值", value: $draft.idleThresholdSeconds, unit: "秒", range: 10...3600)
                TextField("排除的应用（逗号分隔）", text: $excludedText)
                HStack {
                    Label(
                        model.accessibilityGranted ? "辅助功能已授权" : "辅助功能未授权，只能记录应用名",
                        systemImage: model.accessibilityGranted ? "checkmark.shield" : "exclamationmark.shield"
                    )
                    .foregroundStyle(model.accessibilityGranted ? .green : .orange)
                    Spacer()
                    Button("请求授权") { model.requestAccessibility() }
                }
            }

            Section("每日归档") {
                Toggle("到点自动归档", isOn: $draft.autoExport)
                HStack {
                    Stepper("归档时间 \(draft.exportHour) 时", value: $draft.exportHour, in: 0...23)
                    Stepper("\(draft.exportMinute) 分", value: $draft.exportMinute, in: 0...59)
                }
                HStack {
                    Text(draft.outputDirectory)
                        .font(.caption)
                        .lineLimit(1)
                        .truncationMode(.middle)
                    Spacer()
                    Button("选择文件夹…") { chooseFolder() }
                }
                HStack {
                    Button("立即归档") { model.exportNow() }
                    Button("重新生成今天") { model.exportFullDay() }
                    Button("打开文件夹") { model.openOutputFolder() }
                }
                if !model.exportMessage.isEmpty {
                    Text(model.exportMessage)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }

            Section("数据位置") {
                pathRow("应用数据", DayTracePaths.home.path)
                pathRow("活动缓存", DayTracePaths.events.path)
                pathRow("专注记录", DayTracePaths.sessions.path)
            }

            Section {
                HStack {
                    Button("保存设置") {
                        applyExcludedApps()
                        model.saveSettings(draft)
                        message = "已保存"
                    }
                    .buttonStyle(.borderedProminent)
                    Button("恢复当前设置") {
                        draft = model.settings
                        excludedText = model.settings.excludedApps.joined(separator: ", ")
                        message = ""
                    }
                    Spacer()
                    Text(message).font(.caption).foregroundStyle(.secondary)
                }
            }
        }
        .formStyle(.grouped)
        .padding(isSettingsWindow ? 12 : 0)
        .onAppear {
            draft = model.settings
            excludedText = model.settings.excludedApps.joined(separator: ", ")
        }
    }

    private func numberRow(_ title: String, value: Binding<Double>, unit: String, range: ClosedRange<Double>) -> some View {
        LabeledContent(title) {
            HStack(spacing: 6) {
                TextField("", value: value, format: .number)
                    .frame(width: 64)
                    .multilineTextAlignment(.trailing)
                Text(unit).foregroundStyle(.secondary)
                Stepper("", value: value, in: range)
                    .labelsHidden()
            }
        }
    }

    private func pathRow(_ title: String, _ path: String) -> some View {
        LabeledContent(title) {
            Text(path)
                .font(.caption.monospaced())
                .textSelection(.enabled)
                .lineLimit(1)
                .truncationMode(.middle)
        }
    }

    private func applyExcludedApps() {
        draft.excludedApps = excludedText
            .split(separator: ",")
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
    }

    private func chooseFolder() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.canCreateDirectories = true
        panel.directoryURL = URL(fileURLWithPath: (draft.outputDirectory as NSString).expandingTildeInPath, isDirectory: true)
        if panel.runModal() == .OK, let url = panel.url {
            draft.outputDirectory = url.path
        }
    }
}
