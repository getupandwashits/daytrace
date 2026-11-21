import Foundation
import DayTraceCore
import ApplicationServices

@main
struct AgentMain {
    static func main() {
        var arguments = Array(CommandLine.arguments.dropFirst())
        guard let command = arguments.first else {
            printHelp()
            return
        }
        arguments.removeFirst()
        switch command {
        case "doctor":
            doctor()
        case "demo":
            demo(arguments)
        case "export":
            export(arguments)
        case "due":
            due()
        case "sample":
            sample()
        case "settings":
            showSettings()
        default:
            printHelp()
        }
    }

    static func printHelp() {
        print("""
        daytrace-agent <command>

        doctor              显示路径、权限和当前数据状态
        demo [--minutes 360] [--reset]
                            生成一段演示采样，并立即归档到自己的测试目录
        export [--force]    立即归档（--force 重新生成今天的记录）
        due                 执行一次到点检查
        sample              采集一次当前前台窗口并打印
        settings            打印当前设置 JSON
        """)
    }

    static func doctor() {
        let settings = SettingsStore().settings
        let journal = ActivityJournal()
        let sessions = SessionStore().loadAll()
        let state = Exporter().loadState()
        let samples = journal.read(from: nil, to: nil)
        print("home: \(DayTracePaths.home.path)")
        print("journal: \(DayTracePaths.events.path)")
        print("output: \((settings.outputDirectory as NSString).expandingTildeInPath)")
        print("settings: \(DayTracePaths.settings.path)")
        print("accessibility: \(AXIsProcessTrusted())")
        print("samples: \(samples.count)")
        print("sessions: \(sessions.count)")
        if let first = samples.first?.timestamp, let last = samples.last?.timestamp {
            print("range: \(first) — \(last)")
        }
        if let last = state.lastExportEnd {
            print("last export end: \(last)")
            print("last export file: \(state.lastExportFile ?? "-")")
        } else {
            print("last export end: -")
        }
        let today = DayTracePaths.dayString(Date())
        print("today: \(today), samples: \(journal.samples(on: Date()).count)")
    }

    static func showSettings() {
        let settings = SettingsStore().settings
        if let data = try? DayTraceJSON.prettyEncoder.encode(settings),
           let text = String(data: data, encoding: .utf8) {
            print(text)
        }
    }

    static func sample() {
        let settings = SettingsStore().settings
        let collector = ActivityCollector(configuration: .init(
            sampleInterval: settings.sampleIntervalSeconds,
            idleThreshold: settings.idleThresholdSeconds,
            uiMaxChars: settings.uiMaxChars,
            excludedApps: settings.excludedApps
        ))
        collector.sampleNow()
        Thread.sleep(forTimeInterval: 1.5)
        if let sample = collector.lastSample,
           let data = try? DayTraceJSON.prettyEncoder.encode(sample),
           let text = String(data: data, encoding: .utf8) {
            print(text)
        } else {
            print("no sample captured")
        }
    }

    static func export(_ arguments: [String]) {
        let force = arguments.contains("--force")
        let result = Exporter().exportNow(force: force)
        print(result.message)
        if let entry = result.entryURL {
            print(entry.path)
        }
        for url in result.written where url.lastPathComponent != result.entryURL?.lastPathComponent {
            print(url.path)
        }
    }

    static func due() {
        let result = Exporter().exportDue()
        print(result.message)
        if let entry = result.entryURL { print(entry.path) }
    }

    static func demo(_ arguments: [String]) {
        let minutes = value(after: "--minutes", in: arguments).flatMap(Int.init) ?? 360
        let reset = arguments.contains("--reset")
        let journal = ActivityJournal()
        let today = Date()

        if reset {
            let file = DayTracePaths.dayFile(for: today)
            try? FileManager.default.removeItem(at: file)
            try? FileManager.default.removeItem(at: DayTracePaths.exportState)
        }

        if journal.samples(on: today).isEmpty {
            let samples = DemoData.samples(minutes: minutes, endingAt: today)
            do {
                for sample in samples {
                    try journal.append(sample)
                }
                print("generated \(samples.count) demo samples")
            } catch {
                print("demo generation failed: \(error)")
                return
            }
        } else {
            print("today already has samples; reusing them")
        }

        let result = Exporter().exportNow(force: true)
        print(result.message)
        if let entry = result.entryURL { print(entry.path) }
    }

    static func value(after flag: String, in arguments: [String]) -> String? {
        guard let index = arguments.firstIndex(of: flag), index + 1 < arguments.count else { return nil }
        return arguments[index + 1]
    }
}

enum DemoData {
    static func samples(minutes: Int, endingAt end: Date) -> [ActivitySample] {
        let count = max(12, minutes / 2)
        let apps: [(String, String, [String])] = [
            ("Xcode", "DayTrace — Sources/DayTraceCore/FocusEngine.swift", [
                "final class FocusEngine: ObservableObject",
                "@Published public private(set) var state: FocusState = .away",
                "func tick(idle: TimeInterval? = nil, elapsed: TimeInterval? = nil) {",
                "swift build --target DayTraceCore",
                "error: cannot find type ActivitySample in scope",
                "Build Succeeded",
                "────────",
            ]),
            ("Safari", "GitHub — daytrace", [
                "DayTrace — a local activity journal",
                "最近提交",
                "docs: simplify digest rules",
                "第 1 步：安装依赖",
                "第 2 步：运行本地服务",
                "sk-ant-api03-demo000000000000000000000000000000000000000000",
            ]),
            ("Terminal", "daytrace — swift build", [
                "$ swift build",
                "Compiling DayTraceCore",
                "Build complete!",
                "$ ./daytrace-agent demo",
                "generated 180 demo samples",
                "正在生成一段较长的回答",
                "正在生成一段较长的回答，包含更多内容和示例",
            ]),
            ("Notes", "今天要做的三件事", [
                "整理活动记录的摘要规则",
                "把番茄钟和记录合并成一个菜单栏应用",
                "验证 23:00 的归档流程",
                "未完成：日历同步",
            ]),
            ("WeChat", "工作群", [
                "下午三点开个短会",
                "收到",
                "文件已经发到群里了",
                "────────────────",
            ]),
        ]

        var result: [ActivitySample] = []
        let start = end.addingTimeInterval(-Double(count) * 120)
        for index in 0..<count {
            let timestamp = start.addingTimeInterval(Double(index) * 120)
            let app = apps[(index / 12) % apps.count]
            var sample = ActivitySample(timestamp: timestamp, state: "active")
            sample.app = app.0
            sample.bundleID = "demo.\(app.0.lowercased())"
            sample.windowTitle = app.1
            sample.sampleInterval = 120
            sample.idleThreshold = 180
            sample.idleSeconds = 0
            sample.accessibility = true
            sample.captureStatus = "ok"
            var lines = app.2
            if index % 7 == 0 {
                lines.append("正在处理第 \(index % 5 + 1) 项")
            }
            sample.uiText = lines.joined(separator: "\n")
            result.append(sample)
        }

        // A short idle stretch in the middle, so gaps and states appear in the report.
        for index in stride(from: result.count / 3, to: min(result.count, result.count / 3 + 4), by: 1) {
            result[index].state = "idle"
            result[index].uiText = nil
            result[index].app = nil
            result[index].windowTitle = nil
            result[index].captureStatus = nil
        }
        return result.sorted { $0.timestamp < $1.timestamp }
    }
}
