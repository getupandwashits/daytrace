import XCTest
@testable import DayTraceCore

final class DigestTests: XCTestCase {
    func testDigestDropsNoiseMasksSecretsAndJoinsFragments() {
        let now = Date(timeIntervalSince1970: 1_700_000_000)
        let hour = DayTracePaths.beijingCalendar().dateInterval(of: .hour, for: now)?.start ?? now
        let added = [
            AddedLine(stamp: now, app: "Safari", title: "GitHub", line: "sk-ant-api03-abcdefghijklmnopqrstuvwxyz123456", part: 1),
            AddedLine(stamp: now, app: "Safari", title: "GitHub", line: "────────", part: 1),
            AddedLine(stamp: now, app: "Safari", title: "GitHub", line: "GitHub", part: 1),
            AddedLine(stamp: now, app: "Safari", title: "GitHub", line: "第 1 步：安装依赖", part: 1),
            AddedLine(stamp: now, app: "Safari", title: "GitHub", line: "第 2 步：安装依赖", part: 1),
            AddedLine(stamp: now, app: "Xcode", title: "Build", line: "正在生成一段较长的回答", part: 1),
            AddedLine(stamp: now, app: "Xcode", title: "Build", line: "正在生成一段较长的回答，包含更多内容", part: 1),
            AddedLine(stamp: now, app: "Notes", title: "列表", line: "收到", part: 1),
            AddedLine(stamp: now, app: "Notes", title: "列表", line: "下午三点开个短会", part: 1),
        ]
        let seconds: [BlockKey: TimeInterval] = [
            BlockKey(hour: hour, app: "Safari", title: "GitHub"): 600,
            BlockKey(hour: hour, app: "Xcode", title: "Build"): 600,
            BlockKey(hour: hour, app: "Notes", title: "列表"): 600,
        ]

        let (text, stats) = DigestEngine.build(
            added: added,
            blockSeconds: seconds,
            notice: "测试数据"
        )

        XCTAssertFalse(text.contains("sk-ant-api03"))
        XCTAssertTrue(text.contains("〔已遮蔽〕"))
        XCTAssertFalse(text.contains("────────"))
        XCTAssertTrue(text.contains("第 1 步：安装依赖"))
        XCTAssertTrue(text.contains("正在生成一段较长的回答，包含更多内容"))
        XCTAssertTrue(text.contains("收到 · 下午三点开个短会"))
        XCTAssertEqual(stats.maskedSecrets, 1)
        XCTAssertGreaterThan(stats.dropped["decoration"] ?? 0, 0)
        XCTAssertGreaterThan(stats.dropped["streaming_draft"] ?? 0, 0)
        XCTAssertGreaterThan(stats.dropped["title_echo"] ?? 0, 0)
        XCTAssertGreaterThan(stats.dropped["digit_or_space_variant"] ?? 0, 0)
    }

    func testPartRangesCollapseConsecutiveValues() {
        XCTAssertEqual(DigestEngine.partRanges([1, 2, 3, 5, 7, 8]), "1–3、5、7–8")
        XCTAssertEqual(DigestEngine.partRanges([]), "—")
    }

    func testShortDurationFormatting() {
        XCTAssertEqual(DigestEngine.duration(20), "20s")
        XCTAssertEqual(DigestEngine.duration(120), "2m")
        XCTAssertEqual(DigestEngine.duration(3600), "1h 00m")
    }
}

final class ExporterTests: XCTestCase {
    func testExportWritesFourLayerRecord() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("daytrace-tests-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)

        var settings = DayTraceSettings()
        settings.outputDirectory = root.appendingPathComponent("output").path
        settings.autoExport = false
        let settingsStore = SettingsStore(url: root.appendingPathComponent("settings.json"))
        try settingsStore.save(settings)

        let journal = ActivityJournal(root: root.appendingPathComponent("events"))
        let now = Date()
        var first = ActivitySample(timestamp: now.addingTimeInterval(-1), state: "active")
        first.app = "Xcode"
        first.bundleID = "com.apple.dt.Xcode"
        first.windowTitle = "FocusEngine.swift"
        first.sampleInterval = 60
        first.idleThreshold = 180
        first.captureStatus = "ok"
        first.uiText = "final class FocusEngine\n@Published var state"

        var second = ActivitySample(timestamp: now, state: "active")
        second.app = "Safari"
        second.bundleID = "com.apple.Safari"
        second.windowTitle = "GitHub"
        second.sampleInterval = 60
        second.idleThreshold = 180
        second.captureStatus = "ok"
        second.uiText = "DayTrace — a local activity journal"

        try journal.append(first)
        try journal.append(second)

        let exporter = Exporter(
            journal: journal,
            settingsStore: settingsStore,
            stateURL: root.appendingPathComponent("export-state.json")
        )
        let result = exporter.exportNow(now: now.addingTimeInterval(1), force: true)

        let entry = try XCTUnwrap(result.entryURL)
        XCTAssertTrue(FileManager.default.fileExists(atPath: entry.path))
        let entryText = try String(contentsOf: entry, encoding: .utf8)
        XCTAssertTrue(entryText.contains("电脑活动记录"))
        XCTAssertTrue(entryText.contains("Xcode"))
        XCTAssertTrue(entryText.contains("Safari"))
        XCTAssertTrue(entryText.contains("新增原文：段"))

        let uiDirectory = entry.deletingPathExtension().appendingPathExtension("ui")
        XCTAssertTrue(FileManager.default.fileExists(atPath: uiDirectory.appendingPathComponent("digest.md").path))
        XCTAssertTrue(FileManager.default.fileExists(atPath: uiDirectory.appendingPathComponent("timeline.md").path))
        XCTAssertTrue(FileManager.default.fileExists(atPath: uiDirectory.appendingPathComponent("part-001.md").path))

        let metaURL = entry.deletingPathExtension().appendingPathExtension("meta.json")
        let metaData = try Data(contentsOf: metaURL)
        let meta = try JSONSerialization.jsonObject(with: metaData) as? [String: Any]
        XCTAssertEqual(meta?["samples"] as? Int, 2)
        XCTAssertEqual(meta?["records"] as? Int, 2)
        XCTAssertEqual(meta?["ui_format_version"] as? Int, 4)
        XCTAssertEqual(meta?["producer"] as? String, "DayTrace")
        XCTAssertNotNil(meta?["cutoff"] as? String)
        XCTAssertNotNil(meta?["first_timestamp"] as? String)
        XCTAssertNotNil(meta?["last_timestamp"] as? String)

        let base = entry.deletingPathExtension().lastPathComponent
        let rawURL = entry.deletingLastPathComponent()
            .appendingPathComponent(".raw/\(base).jsonl.gz")
        let rawData = try Data(contentsOf: rawURL)
        XCTAssertEqual(Array(rawData.prefix(2)), [0x1f, 0x8b])

        let state = exporter.loadState()
        XCTAssertEqual(state.lastExportFile, entry.lastPathComponent)
    }
}

final class SessionStoreTests: XCTestCase {
    func testAppendAndLoadSessionsForDay() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("daytrace-sessions-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let store = SessionStore(root: root)
        let day = DayTracePaths.startOfDay(Date())
        let first = FocusSession(
            start: day.addingTimeInterval(9 * 3600),
            end: day.addingTimeInterval(9 * 3600 + 1500),
            duration: 1500,
            trigger: "auto"
        )
        let second = FocusSession(
            start: day.addingTimeInterval(14 * 3600),
            end: day.addingTimeInterval(14 * 3600 + 2700),
            duration: 2700,
            trigger: "manual"
        )
        try store.append(second)
        try store.append(first)
        let loaded = store.loadAll()
        XCTAssertEqual(loaded.count, 2)
        XCTAssertEqual(loaded.first?.start, first.start)
        XCTAssertEqual(store.sessions(on: day).reduce(0) { $0 + $1.duration }, 4200)
        XCTAssertTrue(FileManager.default.fileExists(atPath: root.appendingPathComponent("sessions.jsonl").path))
    }
}

final class ExportScheduleTests: XCTestCase {
    func testLateScheduledExportKeepsTheConfiguredFilenameAndCutoff() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("daytrace-schedule-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)

        let calendar = DayTracePaths.beijingCalendar()
        let day = calendar.startOfDay(for: Date())
        let late = calendar.date(byAdding: .minute, value: 23 * 60 + 20, to: day)!
        let earlier = calendar.date(byAdding: .minute, value: 22 * 60 + 50, to: day)!

        var settings = DayTraceSettings()
        settings.outputDirectory = root.appendingPathComponent("output").path
        settings.autoExport = true
        settings.exportHour = 23
        settings.exportMinute = 0
        let settingsStore = SettingsStore(url: root.appendingPathComponent("settings.json"))
        try settingsStore.save(settings)

        let journal = ActivityJournal(root: root.appendingPathComponent("events"))
        for (index, stamp) in [earlier, late].enumerated() {
            var sample = ActivitySample(timestamp: stamp, state: "active")
            sample.app = "Xcode"
            sample.bundleID = "com.apple.dt.Xcode"
            sample.windowTitle = "FocusEngine.swift"
            sample.sampleInterval = 60
            sample.idleThreshold = 180
            sample.captureStatus = "ok"
            sample.uiText = "sample \(index)"
            try journal.append(sample)
        }

        let exporter = Exporter(
            journal: journal,
            settingsStore: settingsStore,
            stateURL: root.appendingPathComponent("export-state.json")
        )
        let now = calendar.date(byAdding: .minute, value: 23 * 60 + 30, to: day)!
        XCTAssertTrue(exporter.isDue(now: now))
        let result = exporter.exportDue(now: now)
        let entry = try XCTUnwrap(result.entryURL)
        XCTAssertEqual(entry.lastPathComponent, "\(DayTracePaths.dayString(day))_23-00.md")

        let metaURL = entry.deletingPathExtension().appendingPathExtension("meta.json")
        let meta = try JSONSerialization.jsonObject(with: Data(contentsOf: metaURL)) as? [String: Any]
        XCTAssertEqual(meta?["records"] as? Int, 1)
        XCTAssertTrue((meta?["cutoff"] as? String)?.contains("T23:00:00.000+08:00") == true)
        XCTAssertEqual(exporter.loadState().lastExportEnd, calendar.date(byAdding: .minute, value: 23 * 60, to: day)!)
        XCTAssertFalse(exporter.isDue(now: now.addingTimeInterval(60)))
    }
}
