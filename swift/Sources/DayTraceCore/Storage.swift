import Foundation

public final class ActivityJournal {
    private let lock = NSLock()
    private let fileManager = FileManager.default
    private let root: URL

    public init(root: URL = DayTracePaths.events) {
        self.root = root
    }

    public func append(_ sample: ActivitySample) throws {
        try fileManager.createDirectory(at: root, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let file = root.appendingPathComponent("\(DayTracePaths.dayString(sample.timestamp)).jsonl")
        let data = try DayTraceJSON.encoder.encode(sample)
        lock.lock()
        defer { lock.unlock() }
        if !fileManager.fileExists(atPath: file.path) {
            fileManager.createFile(atPath: file.path, contents: nil, attributes: [.posixPermissions: 0o600])
        }
        let handle = try FileHandle(forWritingTo: file)
        defer { try? handle.close() }
        try handle.seekToEnd()
        try handle.write(contentsOf: data + Data([10]))
        try handle.synchronize()
    }

    public func files() -> [URL] {
        (try? fileManager.contentsOfDirectory(
            at: root,
            includingPropertiesForKeys: nil,
            options: [.skipsHiddenFiles]
        ))?.filter { $0.pathExtension == "jsonl" }.sorted { $0.lastPathComponent < $1.lastPathComponent } ?? []
    }

    public func read(from start: Date?, to end: Date?) -> [ActivitySample] {
        var result: [ActivitySample] = []
        for file in files() {
            guard let data = try? Data(contentsOf: file) else { continue }
            for line in data.split(separator: UInt8(ascii: "\n")) {
                guard let sample = try? DayTraceJSON.decoder.decode(ActivitySample.self, from: Data(line)) else { continue }
                if let start, sample.timestamp < start { continue }
                if let end, sample.timestamp > end { continue }
                result.append(sample)
            }
        }
        return result.sorted { $0.timestamp < $1.timestamp }
    }

    public func samples(on day: Date) -> [ActivitySample] {
        let start = DayTracePaths.startOfDay(day)
        let end = DayTracePaths.beijingCalendar().date(byAdding: .day, value: 1, to: start) ?? start.addingTimeInterval(86400)
        return read(from: start, to: end.addingTimeInterval(-0.001))
    }

    public func pending(since: Date?, until end: Date) -> [ActivitySample] {
        read(from: since, to: end).filter { sample in
            guard let since else { return true }
            return sample.timestamp > since
        }
    }
}

public final class SessionStore {
    private let lock = NSLock()
    private let fileManager = FileManager.default
    private let root: URL

    public init(root: URL = DayTracePaths.sessions) {
        self.root = root
    }

    private var url: URL { root.appendingPathComponent("sessions.jsonl") }

    public func append(_ session: FocusSession) throws {
        try fileManager.createDirectory(at: root, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let data = try DayTraceJSON.encoder.encode(session)
        lock.lock()
        defer { lock.unlock() }
        if !fileManager.fileExists(atPath: url.path) {
            fileManager.createFile(atPath: url.path, contents: nil, attributes: [.posixPermissions: 0o600])
        }
        let handle = try FileHandle(forWritingTo: url)
        defer { try? handle.close() }
        try handle.seekToEnd()
        try handle.write(contentsOf: data + Data([10]))
        try handle.synchronize()
    }

    public func loadAll() -> [FocusSession] {
        guard let data = try? Data(contentsOf: url) else { return [] }
        return data.split(separator: UInt8(ascii: "\n"))
            .compactMap { try? DayTraceJSON.decoder.decode(FocusSession.self, from: Data($0)) }
            .sorted { $0.start < $1.start }
    }

    public func sessions(on day: Date) -> [FocusSession] {
        let start = DayTracePaths.startOfDay(day)
        let end = DayTracePaths.beijingCalendar().date(byAdding: .day, value: 1, to: start) ?? start.addingTimeInterval(86400)
        return loadAll().filter { $0.start < end && $0.end > start }
    }
}
