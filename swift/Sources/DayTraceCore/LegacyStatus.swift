import AppKit
import Foundation

/// The live status file AutoPomodoro publishes for external readers such as
/// `scripts/read-status.py` and the keyboard dot-matrix service.
///
/// DayTrace writes the same schema while the old app is not running, so those
/// readers keep working without any changes. When another AutoPomodoro instance
/// owns the file, DayTrace backs off and lets the old app keep publishing.
public struct LegacyLiveStatus: Codable {
    public var schemaVersion = 1
    public var source = "autopomodoro"
    public var processID: Int32
    public var updatedAt: Date
    public var state: String
    public var stateLabel: String
    public var focusElapsedSeconds: Double
    public var focusTargetSeconds: Double
    public var focusRemainingSeconds: Double
    public var restRemainingSeconds: Double
    public var restTargetSeconds: Double
    public var remainingFraction: Double
    public var trackingSuspended: Bool
    public var mode: String
    public var phase: String
    public var completedPomodoros: Int
    public var pomodorosPerCycle: Int

    public func write(to url: URL) throws {
        try FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(),
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try DayTraceJSON.prettyEncoder.encode(self).write(to: url, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: url.path)
    }
}

public enum LegacyStatusPaths {
    public static var directoryOverride: URL? {
        guard let value = ProcessInfo.processInfo.environment["DAYTRACE_LEGACY_STATUS_DIR"],
              !value.isEmpty else { return nil }
        return URL(fileURLWithPath: value, isDirectory: true)
    }

    public static var urls: [URL] {
        if let override = directoryOverride {
            return [override.appendingPathComponent("status.json")]
        }
        let home = FileManager.default.homeDirectoryForCurrentUser
        return [
            home.appendingPathComponent(
                "Library/Containers/twickenhamma.AutoPomodoro/Data/Library/Application Support/AutoPomodoro/status.json"
            ),
            home.appendingPathComponent("Library/Application Support/AutoPomodoro/status.json"),
        ]
    }
}

public final class LegacyStatusWriter {
    public let urls: [URL]
    private let isolated: Bool
    private let checkRunningApplication: Bool

    public init(urls: [URL] = LegacyStatusPaths.urls, checkRunningApplication: Bool = true) {
        self.urls = urls
        self.isolated = LegacyStatusPaths.directoryOverride != nil
        self.checkRunningApplication = checkRunningApplication
    }

    /// True when another AutoPomodoro instance is clearly still publishing.
    /// In that case DayTrace leaves the old file alone instead of making the
    /// two writers fight over it.
    public var ownedByAnotherInstance: Bool {
        if checkRunningApplication, !isolated, Self.autoPomodoroIsRunning() { return true }
        let decoder = DayTraceJSON.decoder
        for url in urls {
            guard let data = try? Data(contentsOf: url),
                  let status = try? decoder.decode(LegacyLiveStatus.self, from: data),
                  status.source == "autopomodoro",
                  abs(status.updatedAt.timeIntervalSinceNow) <= 5,
                  status.processID != getpid(),
                  Self.isAlive(status.processID)
            else { continue }
            return true
        }
        return false
    }

    public func write(_ status: LegacyLiveStatus) {
        guard !ownedByAnotherInstance else { return }
        for url in urls {
            try? status.write(to: url)
        }
    }

    private static func autoPomodoroIsRunning() -> Bool {
        let applications = NSRunningApplication.runningApplications(
            withBundleIdentifier: "twickenhamma.AutoPomodoro"
        ) + NSWorkspace.shared.runningApplications.filter {
            $0.localizedName == "AutoPomodoro"
        }
        return !applications.isEmpty
    }

    private static func isAlive(_ processID: Int32) -> Bool {
        guard processID > 0 else { return false }
        if kill(processID, 0) == 0 { return true }
        return errno == EPERM
    }
}
