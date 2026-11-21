import Foundation

public struct ActivitySample: Codable, Identifiable {
    public var timestamp: Date
    public var state: String
    public var app: String?
    public var bundleID: String?
    public var windowTitle: String?
    public var url: String?
    public var document: String?
    public var idleSeconds: Double?
    public var sampleInterval: Double?
    public var idleThreshold: Double?
    public var uiText: String?
    public var uiTruncated: Bool?
    public var uiTruncatedReason: [String]?
    public var captureStatus: String?
    public var contextID: String?
    public var uiTextUnchanged: Bool?
    public var secureFieldSkipped: Bool?
    public var accessibility: Bool?

    public var id: String { "\(timestamp.timeIntervalSince1970)-\(app ?? state)-\(windowTitle ?? "")" }

    public var lines: [String] {
        guard let uiText else { return [] }
        return uiText.split(whereSeparator: \.isNewline)
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .filter { !$0.isEmpty }
    }

    public var title: String { windowTitle ?? "" }
    public var appName: String { app ?? stateLabel }

    public var stateLabel: String {
        switch state {
        case "active": return "活跃"
        case "idle": return "无键鼠输入"
        case "locked": return "锁屏"
        case "sleeping": return "睡眠"
        case "excluded": return "排除的应用"
        default: return "未取得窗口"
        }
    }

    enum CodingKeys: String, CodingKey {
        case timestamp
        case state
        case app
        case bundleID = "bundle_id"
        case windowTitle = "window_title"
        case url
        case document
        case idleSeconds = "idle_seconds"
        case sampleInterval = "sample_interval_seconds"
        case idleThreshold = "idle_threshold_seconds"
        case uiText = "ui_text"
        case uiTruncated = "ui_truncated"
        case uiTruncatedReason = "ui_truncated_reason"
        case captureStatus = "capture_status"
        case contextID = "context_id"
        case uiTextUnchanged = "ui_text_unchanged"
        case secureFieldSkipped = "secure_field_skipped"
        case accessibility
    }

    public init(timestamp: Date = Date(), state: String) {
        self.timestamp = timestamp
        self.state = state
    }
}

public struct FocusSession: Codable, Identifiable {
    public var id: UUID
    public var start: Date
    public var end: Date
    public var duration: TimeInterval
    public var trigger: String

    public init(id: UUID = UUID(), start: Date, end: Date, duration: TimeInterval, trigger: String) {
        self.id = id
        self.start = start
        self.end = end
        self.duration = duration
        self.trigger = trigger
    }

    public var triggerLabel: String { trigger == "auto" ? "自动" : "手动" }
}

public struct ExportState: Codable {
    public var lastExportEnd: Date?
    public var lastExportFile: String?
    public var lastExportAt: Date?

    public init(lastExportEnd: Date? = nil, lastExportFile: String? = nil, lastExportAt: Date? = nil) {
        self.lastExportEnd = lastExportEnd
        self.lastExportFile = lastExportFile
        self.lastExportAt = lastExportAt
    }
}

public struct StatusSnapshot: Codable {
    public var schemaVersion = 1
    public var source = "daytrace"
    public var processID: Int32
    public var updatedAt: Date
    public var state: String
    public var stateLabel: String
    public var mode: String
    public var focusElapsedSeconds: Double
    public var focusTargetSeconds: Double
    public var focusRemainingSeconds: Double
    public var restRemainingSeconds: Double
    public var restTargetSeconds: Double
    public var remainingFraction: Double
    public var trackingSuspended: Bool
    public var completedPomodoros: Int
    public var pomodorosPerCycle: Int

    public func write(to url: URL = DayTracePaths.status) throws {
        try FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try DayTraceJSON.prettyEncoder.encode(self).write(to: url, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: url.path)
    }
}

public struct AppDuration: Identifiable, Hashable {
    public let name: String
    public let seconds: TimeInterval
    public var id: String { name }
}

public struct HourUsage: Identifiable {
    public let hour: Date
    public let apps: [AppDuration]
    public var id: Date { hour }
}

public struct ActivitySummary {
    public var samples: Int = 0
    public var activeSeconds: TimeInterval = 0
    public var first: Date?
    public var last: Date?
    public var apps: [AppDuration] = []
    public var hours: [HourUsage] = []

    public init() {}
}

public struct ExportResult {
    public var written: [URL]
    public var entryURL: URL?
    public var sampleCount: Int
    public var message: String

    public init(written: [URL] = [], entryURL: URL? = nil, sampleCount: Int = 0, message: String = "") {
        self.written = written
        self.entryURL = entryURL
        self.sampleCount = sampleCount
        self.message = message
    }
}
