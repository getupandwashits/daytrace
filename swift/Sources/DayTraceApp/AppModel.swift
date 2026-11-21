import AppKit
import ApplicationServices
import Combine
import Foundation
import DayTraceCore

@MainActor
final class AppModel: ObservableObject {
    static let shared = AppModel()

    @Published var settings: DayTraceSettings
    @Published var summary = ActivitySummary()
    @Published var sessions: [FocusSession] = []
    @Published var lastSample: ActivitySample?
    @Published var collectorRunning = false
    @Published var accessibilityGranted = false
    @Published var collectorError: String?
    @Published var exportMessage = ""
    @Published var lastExportEntry: URL?
    @Published var lastExportAt: Date?

    let engine: FocusEngine
    private let settingsStore: SettingsStore
    private let journal: ActivityJournal
    private let sessionStore: SessionStore
    private let exporter: Exporter
    private let collector: ActivityCollector
    private let presence = PresenceObserver()
    private let reminder = ReminderController()

    private var tickTimer: Timer?
    private var refreshTimer: Timer?
    private var exportTimer: Timer?
    private var started = false
    private var cancellables = Set<AnyCancellable>()

    private init() {
        let store = SettingsStore()
        settingsStore = store
        settings = store.settings
        journal = ActivityJournal()
        sessionStore = SessionStore()
        exporter = Exporter(journal: journal, settingsStore: store)
        collector = ActivityCollector(configuration: Self.configuration(store.settings))
        engine = FocusEngine()
        engine.legacyStatusWriter = LegacyStatusWriter()
        engine.configure(store.settings)

        engine.onSession = { [weak self] session in
            guard let self else { return }
            do {
                try self.sessionStore.append(session)
                self.sessions = self.sessionStore.loadAll()
            } catch {
                self.exportMessage = "保存专注记录失败：\(error.localizedDescription)"
            }
        }
        engine.onReminder = { [weak self] message in
            guard let self else { return }
            self.reminder.show(message: message) { [weak self] in
                self?.engine.resetToAway(trigger: .manual)
            }
        }
        engine.objectWillChange
            .receive(on: RunLoop.main)
            .sink { [weak self] _ in self?.objectWillChange.send() }
            .store(in: &cancellables)

        collector.onSample = { [weak self] sample in
            DispatchQueue.main.async {
                guard let self else { return }
                self.lastSample = sample
                self.accessibilityGranted = AXIsProcessTrusted()
            }
        }
        collector.onError = { [weak self] message in
            DispatchQueue.main.async { self?.collectorError = message }
        }

        accessibilityGranted = AXIsProcessTrusted()
    }

    func start() {
        guard !started else { return }
        started = true
        try? DayTracePaths.ensureDirectories()
        engine.configure(settings)
        engine.publishStatus()

        presence.onSuspend = { [weak self] reason in self?.engine.suspend(reason: reason) }
        presence.onResume = { [weak self] reason in self?.engine.resume(reason: reason) }
        presence.start()

        if settings.collectActivity { startCollector() }

        let tickTimerValue = Timer(timeInterval: 1, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.engine.tick() }
        }
        RunLoop.main.add(tickTimerValue, forMode: .common)
        tickTimer = tickTimerValue

        let refreshTimerValue = Timer(timeInterval: 15, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.refresh() }
        }
        RunLoop.main.add(refreshTimerValue, forMode: .common)
        refreshTimer = refreshTimerValue

        let exportTimerValue = Timer(timeInterval: 30, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.checkExport() }
        }
        RunLoop.main.add(exportTimerValue, forMode: .common)
        exportTimer = exportTimerValue

        refresh()
        checkExport()
    }

    // MARK: - Collector

    func startCollector() {
        collector.start()
        collectorRunning = true
    }

    func stopCollector() {
        collector.stop()
        collectorRunning = false
    }

    func requestAccessibility() {
        ActivityCollector.requestAccessibility()
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) { [weak self] in
            guard let self else { return }
            self.accessibilityGranted = AXIsProcessTrusted()
            if self.accessibilityGranted && !self.collectorRunning {
                self.startCollector()
            }
        }
    }

    func captureNow() {
        collector.sampleNow()
    }

    // MARK: - Settings

    func saveSettings(_ newValue: DayTraceSettings) {
        do {
            try settingsStore.save(newValue)
        } catch {
            exportMessage = "保存设置失败：\(error.localizedDescription)"
            return
        }
        settings = newValue
        engine.configure(newValue)
        collector.update(configuration: Self.configuration(newValue))
        if newValue.collectActivity && !collectorRunning {
            startCollector()
        } else if !newValue.collectActivity && collectorRunning {
            stopCollector()
        }
        refresh()
        exportMessage = "设置已保存"
    }

    // MARK: - Export

    func exportNow() {
        let result = exporter.exportNow(force: false)
        exportMessage = result.message
        if let entry = result.entryURL {
            lastExportEntry = entry
            lastExportAt = Date()
        }
        refreshExportState()
    }

    func exportFullDay() {
        let result = exporter.exportNow(force: true)
        exportMessage = result.message
        if let entry = result.entryURL {
            lastExportEntry = entry
            lastExportAt = Date()
        }
        refreshExportState()
    }

    func checkExport() {
        guard exporter.isDue() else { return }
        let result = exporter.exportDue()
        if result.entryURL != nil {
            exportMessage = result.message
            lastExportEntry = result.entryURL
            lastExportAt = Date()
        }
        refreshExportState()
    }

    func refreshExportState() {
        let state = exporter.loadState()
        lastExportEntry = state.lastExportFile.map {
            URL(fileURLWithPath: (settings.outputDirectory as NSString).expandingTildeInPath)
                .appendingPathComponent($0)
        }
        lastExportAt = state.lastExportAt
    }

    func openOutputFolder() {
        let url = URL(fileURLWithPath: (settings.outputDirectory as NSString).expandingTildeInPath, isDirectory: true)
        try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        NSWorkspace.shared.open(url)
    }

    // MARK: - Summary

    func refresh() {
        let samples = journal.samples(on: Date())
        summary = ActivityAnalyzer.summarize(samples)
        sessions = sessionStore.loadAll()
        if lastSample == nil { lastSample = samples.last }
        accessibilityGranted = AXIsProcessTrusted()
        refreshExportState()
    }

    var todayFocusSeconds: TimeInterval {
        let recorded = sessionStore.sessions(on: Date()).reduce(0) { $0 + $1.duration }
        return recorded + engine.unrecordedFocusDuration
    }

    var todaySessionCount: Int {
        sessionStore.sessions(on: Date()).count
    }

    var weekFocusSeconds: TimeInterval {
        let calendar = DayTracePaths.beijingCalendar()
        let start = calendar.date(byAdding: .day, value: -6, to: DayTracePaths.startOfDay(Date())) ?? Date()
        let recorded = sessionStore.loadAll().filter { $0.start >= start }.reduce(0) { $0 + $1.duration }
        return recorded + engine.unrecordedFocusDuration
    }

    func focusSeconds(for day: Date) -> TimeInterval {
        let recorded = sessionStore.sessions(on: day).reduce(0) { $0 + $1.duration }
        if DayTracePaths.beijingCalendar().isDate(day, inSameDayAs: Date()) {
            return recorded + engine.unrecordedFocusDuration
        }
        return recorded
    }

    var lastSevenDays: [DayFocus] {
        let calendar = DayTracePaths.beijingCalendar()
        let today = DayTracePaths.startOfDay(Date())
        return (0..<7).reversed().compactMap { offset in
            guard let day = calendar.date(byAdding: .day, value: -offset, to: today) else { return nil }
            return DayFocus(date: day, seconds: focusSeconds(for: day))
        }
    }

    var outputDirectory: URL {
        URL(fileURLWithPath: (settings.outputDirectory as NSString).expandingTildeInPath, isDirectory: true)
    }

    private static func configuration(_ settings: DayTraceSettings) -> ActivityCollector.Configuration {
        ActivityCollector.Configuration(
            sampleInterval: settings.sampleIntervalSeconds,
            idleThreshold: settings.idleThresholdSeconds,
            uiMaxChars: settings.uiMaxChars,
            excludedApps: settings.excludedApps
        )
    }
}


struct DayFocus: Identifiable {
    let date: Date
    let seconds: TimeInterval
    var id: Date { date }
}
