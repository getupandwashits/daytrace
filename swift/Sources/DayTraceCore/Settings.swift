import Foundation

public struct DayTraceSettings: Codable, Equatable {
    public var mode: String = "automatic"
    public var automaticFocusMinutes: Double = 45
    public var traditionalFocusMinutes: Double = 25
    public var shortBreakMinutes: Double = 5
    public var longBreakMinutes: Double = 15
    public var rounds: Int = 4
    public var pauseToleranceMinutes: Double = 3
    public var resetThresholdMinutes: Double = 10
    public var minimumRecordMinutes: Double = 5
    public var pauseWhenIdle: Bool = true

    public var sampleIntervalSeconds: Double = 10
    public var idleThresholdSeconds: Double = 180
    public var uiMaxChars: Int = 16000
    public var excludedApps: [String] = [
        "com.1password.1password",
        "com.agilebits.onepassword7",
        "org.keepassxc.keepassxc",
        "com.apple.Passwords",
    ]
    public var collectActivity: Bool = true

    public var exportHour: Int = 23
    public var exportMinute: Int = 0
    public var autoExport: Bool = true
    public var outputDirectory: String = DayTracePaths.defaultOutput.path

    public init() {}

    public var focusMinutes: Double {
        mode == "traditional" ? traditionalFocusMinutes : automaticFocusMinutes
    }

    public var pauseTolerance: TimeInterval { max(30, pauseToleranceMinutes * 60) }
    public var resetThreshold: TimeInterval { max(pauseTolerance + 30, resetThresholdMinutes * 60) }
    public var minimumRecordDuration: TimeInterval { max(30, minimumRecordMinutes * 60) }

    enum CodingKeys: String, CodingKey {
        case mode
        case automaticFocusMinutes
        case traditionalFocusMinutes
        case shortBreakMinutes
        case longBreakMinutes
        case rounds
        case pauseToleranceMinutes
        case resetThresholdMinutes
        case minimumRecordMinutes
        case pauseWhenIdle
        case sampleIntervalSeconds
        case idleThresholdSeconds
        case uiMaxChars
        case excludedApps
        case collectActivity
        case exportHour
        case exportMinute
        case autoExport
        case outputDirectory
    }

    public init(from decoder: Decoder) throws {
        let defaults = DayTraceSettings()
        let c = try decoder.container(keyedBy: CodingKeys.self)
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? defaults.mode
        automaticFocusMinutes = try c.decodeIfPresent(Double.self, forKey: .automaticFocusMinutes) ?? defaults.automaticFocusMinutes
        traditionalFocusMinutes = try c.decodeIfPresent(Double.self, forKey: .traditionalFocusMinutes) ?? defaults.traditionalFocusMinutes
        shortBreakMinutes = try c.decodeIfPresent(Double.self, forKey: .shortBreakMinutes) ?? defaults.shortBreakMinutes
        longBreakMinutes = try c.decodeIfPresent(Double.self, forKey: .longBreakMinutes) ?? defaults.longBreakMinutes
        rounds = try c.decodeIfPresent(Int.self, forKey: .rounds) ?? defaults.rounds
        pauseToleranceMinutes = try c.decodeIfPresent(Double.self, forKey: .pauseToleranceMinutes) ?? defaults.pauseToleranceMinutes
        resetThresholdMinutes = try c.decodeIfPresent(Double.self, forKey: .resetThresholdMinutes) ?? defaults.resetThresholdMinutes
        minimumRecordMinutes = try c.decodeIfPresent(Double.self, forKey: .minimumRecordMinutes) ?? defaults.minimumRecordMinutes
        pauseWhenIdle = try c.decodeIfPresent(Bool.self, forKey: .pauseWhenIdle) ?? defaults.pauseWhenIdle
        sampleIntervalSeconds = try c.decodeIfPresent(Double.self, forKey: .sampleIntervalSeconds) ?? defaults.sampleIntervalSeconds
        idleThresholdSeconds = try c.decodeIfPresent(Double.self, forKey: .idleThresholdSeconds) ?? defaults.idleThresholdSeconds
        uiMaxChars = try c.decodeIfPresent(Int.self, forKey: .uiMaxChars) ?? defaults.uiMaxChars
        excludedApps = try c.decodeIfPresent([String].self, forKey: .excludedApps) ?? defaults.excludedApps
        collectActivity = try c.decodeIfPresent(Bool.self, forKey: .collectActivity) ?? defaults.collectActivity
        exportHour = try c.decodeIfPresent(Int.self, forKey: .exportHour) ?? defaults.exportHour
        exportMinute = try c.decodeIfPresent(Int.self, forKey: .exportMinute) ?? defaults.exportMinute
        autoExport = try c.decodeIfPresent(Bool.self, forKey: .autoExport) ?? defaults.autoExport
        outputDirectory = try c.decodeIfPresent(String.self, forKey: .outputDirectory) ?? defaults.outputDirectory
    }
}

public final class SettingsStore {
    public private(set) var settings: DayTraceSettings
    private let url: URL
    private let lock = NSLock()

    public init(url: URL = DayTracePaths.settings) {
        self.url = url
        self.settings = DayTraceSettings()
        load()
    }

    public func load() {
        lock.lock()
        defer { lock.unlock() }
        guard let data = try? Data(contentsOf: url),
              let decoded = try? DayTraceJSON.decoder.decode(DayTraceSettings.self, from: data) else {
            applyOutputOverride()
            return
        }
        settings = decoded
        applyOutputOverride()
    }

    private func applyOutputOverride() {
        guard let override = ProcessInfo.processInfo.environment["DAYTRACE_OUTPUT"], !override.isEmpty else { return }
        settings.outputDirectory = override
    }

    public func save(_ newValue: DayTraceSettings) throws {
        lock.lock()
        defer { lock.unlock() }
        settings = newValue
        try FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try DayTraceJSON.prettyEncoder.encode(newValue).write(to: url, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: url.path)
    }
}
