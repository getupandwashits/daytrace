import Foundation

public enum DayTracePaths {
    public static var home: URL {
        if let override = ProcessInfo.processInfo.environment["DAYTRACE_HOME"], !override.isEmpty {
            return URL(fileURLWithPath: override, isDirectory: true)
        }
        return FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/DayTrace", isDirectory: true)
    }

    public static var journal: URL { home.appendingPathComponent("journal", isDirectory: true) }
    public static var events: URL { journal.appendingPathComponent("events", isDirectory: true) }
    public static var sessions: URL { home.appendingPathComponent("sessions", isDirectory: true) }
    public static var settings: URL { home.appendingPathComponent("settings.json") }
    public static var exportState: URL { home.appendingPathComponent("export-state.json") }
    public static var status: URL { home.appendingPathComponent("status.json") }

    public static var defaultOutput: URL {
        if let override = ProcessInfo.processInfo.environment["DAYTRACE_OUTPUT"], !override.isEmpty {
            return URL(fileURLWithPath: override, isDirectory: true)
        }
        if let legacy = legacyMonitorOutput { return legacy }
        return FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Documents/MyMonitor", isDirectory: true)
    }

    /// Reuse MyMonitor's configured output folder so the existing Daily_record
    /// reader (and its conclusions workflow) keeps seeing new records.
    private static var legacyMonitorOutput: URL? {
        let settings = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/MyMonitor/settings.json")
        guard let data = try? Data(contentsOf: settings),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let value = object["output_dir"] as? String,
              !value.isEmpty
        else { return nil }
        let expanded = (value as NSString).expandingTildeInPath
        guard expanded.hasPrefix("/") else { return nil }
        return URL(fileURLWithPath: expanded, isDirectory: true)
    }

    public static let beijing: TimeZone = TimeZone(identifier: "Asia/Shanghai")!

    public static func ensureDirectories() throws {
        for directory in [home, journal, events, sessions] {
            try FileManager.default.createDirectory(
                at: directory,
                withIntermediateDirectories: true,
                attributes: [.posixPermissions: 0o700]
            )
        }
    }

    public static func dayString(_ date: Date) -> String {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.timeZone = beijing
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.string(from: date)
    }

    public static func dayFile(for date: Date) -> URL {
        events.appendingPathComponent("\(dayString(date)).jsonl")
    }

    public static func beijingCalendar() -> Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = beijing
        calendar.locale = Locale(identifier: "zh_CN")
        return calendar
    }

    public static func startOfDay(_ date: Date) -> Date {
        beijingCalendar().startOfDay(for: date)
    }
}

public enum DayTraceJSON {
    public static var encoder: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        encoder.outputFormatting = [.sortedKeys]
        return encoder
    }

    public static var prettyEncoder: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return encoder
    }

    public static var decoder: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}
