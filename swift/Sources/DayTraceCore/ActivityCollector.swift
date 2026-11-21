import AppKit
import ApplicationServices
import CryptoKit
import Foundation

/// Reads the foreground window through the accessibility tree.
/// No event taps, keystroke hooks, screenshots or network calls.
public final class ActivityCollector {
    public struct Configuration {
        public var sampleInterval: Double
        public var idleThreshold: Double
        public var uiMaxChars: Int
        public var excludedApps: [String]

        public init(sampleInterval: Double = 10, idleThreshold: Double = 180, uiMaxChars: Int = 16000, excludedApps: [String] = []) {
            self.sampleInterval = max(2, sampleInterval)
            self.idleThreshold = max(1, idleThreshold)
            self.uiMaxChars = max(0, min(20000, uiMaxChars))
            self.excludedApps = excludedApps
        }
    }

    public private(set) var running = false
    public private(set) var lastSample: ActivitySample?
    public var onSample: ((ActivitySample) -> Void)?
    public var onError: ((String) -> Void)?

    private var configuration: Configuration
    private let journal: ActivityJournal
    private let queue = DispatchQueue(label: "local.daytrace.collector", qos: .utility)
    private var timer: DispatchSourceTimer?
    private let stateLock = NSLock()
    private var sleepReasons = Set<String>()
    private var locked = false
    private var observers: [NSObjectProtocol] = []
    private var contexts = Set<String>()
    private var currentDay = ""
    private let workspaceCenter = NSWorkspace.shared.notificationCenter
    private let distributedCenter = DistributedNotificationCenter.default()

    public init(configuration: Configuration, journal: ActivityJournal = ActivityJournal()) {
        self.configuration = configuration
        self.journal = journal
    }

    public var accessibilityGranted: Bool { AXIsProcessTrusted() }

    public static func requestAccessibility() {
        let options = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary
        _ = AXIsProcessTrustedWithOptions(options)
    }

    public func update(configuration: Configuration) {
        queue.async { self.configuration = configuration }
    }

    public func start() {
        guard !running else { return }
        running = true
        installObservers()
        let timer = DispatchSource.makeTimerSource(queue: queue)
        let interval = max(2, configuration.sampleInterval)
        timer.schedule(deadline: .now() + 0.5, repeating: interval, leeway: .milliseconds(500))
        timer.setEventHandler { [weak self] in
            self?.sample()
        }
        self.timer = timer
        timer.resume()
        queue.async { [weak self] in self?.sample() }
    }

    public func stop() {
        running = false
        timer?.cancel()
        timer = nil
        removeObservers()
    }

    public func sampleNow() {
        queue.async { [weak self] in self?.sample() }
    }

    // MARK: - Sampling

    private func sample() {
        autoreleasepool {
            guard running else { return }
            let trusted = AXIsProcessTrusted()
            let idle = Presence.idleSeconds()
            let isLocked = currentLockState()
            let isSleeping = currentSleepState()
            let isActive = !isLocked && !isSleeping && idle < configuration.idleThreshold

            var sample = ActivitySample(timestamp: Date(), state: isLocked ? "locked" : isSleeping ? "sleeping" : isActive ? "active" : "idle")
            sample.idleSeconds = idle
            sample.sampleInterval = configuration.sampleInterval
            sample.idleThreshold = configuration.idleThreshold
            sample.accessibility = trusted

            guard isActive else {
                emit(sample)
                return
            }
            guard let app = frontmostApplication() else {
                sample.state = "unavailable"
                sample.captureStatus = "no_frontmost_app"
                emit(sample)
                return
            }
            let bundle = app.bundleIdentifier ?? ""
            let appName = app.localizedName ?? "Unknown"
            if configuration.excludedApps.contains(where: {
                $0.caseInsensitiveCompare(bundle) == .orderedSame || $0.caseInsensitiveCompare(appName) == .orderedSame
            }) {
                sample.state = "excluded"
                emit(sample)
                return
            }

            sample.app = appName
            sample.bundleID = bundle
            sample.captureStatus = trusted ? "no_accessible_window" : "permission_missing"

            if trusted {
                let application = AXUIElementCreateApplication(app.processIdentifier)
                AXUIElementSetMessagingTimeout(application, 0.15)
                if let window = elementAttribute(application, kAXFocusedWindowAttribute) {
                    AXUIElementSetMessagingTimeout(window, 0.15)
                    sample.windowTitle = stringAttribute(window, kAXTitleAttribute)
                    let document = URL(string: stringAttribute(window, kAXDocumentAttribute))
                    var reader = TextReader(maxChars: configuration.uiMaxChars)
                    reader.walk(window)
                    sample.uiText = reader.lines.joined(separator: "\n")
                    sample.uiTruncated = reader.truncated
                    if reader.truncated { sample.uiTruncatedReason = reader.reasons.sorted() }
                    sample.secureFieldSkipped = reader.secure
                    if let url = reader.url ?? pageAddress(document) {
                        sample.url = url
                    } else if let document, document.isFileURL {
                        sample.document = document.path
                    }
                    sample.captureStatus = reader.lines.isEmpty ? "no_accessible_text" : "ok"
                }
            }

            // Discard UI if the foreground app changed during traversal.
            if frontmostApplication()?.processIdentifier != app.processIdentifier {
                sample.uiText = nil
                sample.windowTitle = nil
                sample.url = nil
                sample.document = nil
                sample.captureStatus = "foreground_changed"
            }

            deduplicateContext(&sample)
            emit(sample)
        }
    }

    private func deduplicateContext(_ sample: inout ActivitySample) {
        guard let text = sample.uiText, !text.isEmpty else { return }
        let day = DayTracePaths.dayString(sample.timestamp)
        if day != currentDay {
            currentDay = day
            contexts.removeAll()
        }
        let content = (sample.bundleID ?? "") + "\u{0}" + (sample.windowTitle ?? "") + "\u{0}" + text
        let digest = SHA256.hash(data: Data(content.utf8))
        let identifier = digest.map { String(format: "%02x", $0) }.joined()
        sample.contextID = identifier
        if contexts.contains(identifier) {
            sample.uiText = nil
            sample.uiTextUnchanged = true
        } else {
            contexts.insert(identifier)
        }
    }

    private func emit(_ sample: ActivitySample) {
        do {
            try journal.append(sample)
            lastSample = sample
            onSample?(sample)
        } catch {
            onError?("写入采集记录失败：\(error.localizedDescription)")
        }
    }

    // MARK: - Observers

    private func installObservers() {
        removeObservers()
        observers.append(workspaceCenter.addObserver(
            forName: NSWorkspace.willSleepNotification, object: nil, queue: .main
        ) { [weak self] _ in self?.addSleepReason("systemSleep") })
        observers.append(workspaceCenter.addObserver(
            forName: NSWorkspace.didWakeNotification, object: nil, queue: .main
        ) { [weak self] _ in self?.removeSleepReason("systemSleep") })
        observers.append(workspaceCenter.addObserver(
            forName: NSWorkspace.screensDidSleepNotification, object: nil, queue: .main
        ) { [weak self] _ in self?.addSleepReason("displaySleep") })
        observers.append(workspaceCenter.addObserver(
            forName: NSWorkspace.screensDidWakeNotification, object: nil, queue: .main
        ) { [weak self] _ in self?.removeSleepReason("displaySleep") })
        observers.append(distributedCenter.addObserver(
            forName: NSNotification.Name("com.apple.screenIsLocked"), object: nil, queue: .main
        ) { [weak self] _ in self?.setLocked(true) })
        observers.append(distributedCenter.addObserver(
            forName: NSNotification.Name("com.apple.screenIsUnlocked"), object: nil, queue: .main
        ) { [weak self] _ in self?.setLocked(false) })
        observers.append(workspaceCenter.addObserver(
            forName: NSWorkspace.didActivateApplicationNotification, object: nil, queue: .main
        ) { [weak self] _ in
            self?.queue.async { self?.sample() }
        })
    }

    private func removeObservers() {
        for observer in observers {
            workspaceCenter.removeObserver(observer)
            distributedCenter.removeObserver(observer)
        }
        observers.removeAll()
    }

    private func addSleepReason(_ reason: String) {
        stateLock.lock()
        sleepReasons.insert(reason)
        stateLock.unlock()
    }

    private func removeSleepReason(_ reason: String) {
        stateLock.lock()
        sleepReasons.remove(reason)
        stateLock.unlock()
    }

    private func setLocked(_ value: Bool) {
        stateLock.lock()
        locked = value
        stateLock.unlock()
    }

    private func currentSleepState() -> Bool {
        stateLock.lock()
        defer { stateLock.unlock() }
        return !sleepReasons.isEmpty
    }

    private func currentLockState() -> Bool {
        stateLock.lock()
        defer { stateLock.unlock() }
        if locked { return true }
        let session = CGSessionCopyCurrentDictionary() as? [String: Any] ?? [:]
        return session["CGSSessionScreenIsLocked"] as? Bool ?? false
    }

    private func frontmostApplication() -> NSRunningApplication? {
        if Thread.isMainThread {
            return NSWorkspace.shared.frontmostApplication
        }
        return DispatchQueue.main.sync { NSWorkspace.shared.frontmostApplication }
    }
}

// MARK: - Accessibility helpers

private func attribute(_ element: AXUIElement, _ name: String) -> CFTypeRef? {
    var result: CFTypeRef?
    guard AXUIElementCopyAttributeValue(element, name as CFString, &result) == .success else { return nil }
    return result
}

private func stringAttribute(_ element: AXUIElement, _ name: String) -> String {
    attribute(element, name) as? String ?? ""
}

private func elementAttribute(_ element: AXUIElement, _ name: String) -> AXUIElement? {
    guard let value = attribute(element, name), CFGetTypeID(value) == AXUIElementGetTypeID() else { return nil }
    return (value as! AXUIElement)
}

/// Keep only the page address: query strings and fragments often carry tokens.
private func pageAddress(_ url: URL?) -> String? {
    guard let url, ["http", "https"].contains(url.scheme?.lowercased() ?? ""),
          var parts = URLComponents(url: url, resolvingAgainstBaseURL: false) else { return nil }
    parts.user = nil
    parts.password = nil
    parts.query = nil
    parts.fragment = nil
    return parts.string
}

private struct TextReader {
    var lines: [String] = []
    var seen = Set<String>()
    var characters = 0
    var nodes = 0
    var stopped = false
    var reasons = Set<String>()
    var secure = false
    var url: String?
    let maxChars: Int
    let deadline = ProcessInfo.processInfo.systemUptime + 0.8

    var truncated: Bool { !reasons.isEmpty }

    mutating func stop(_ reason: String) {
        stopped = true
        reasons.insert(reason)
    }

    mutating func add(_ value: String) {
        let clean = value.replacingOccurrences(of: "\u{200B}", with: "")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard !clean.isEmpty, !seen.contains(clean), characters < maxChars else { return }
        seen.insert(clean)
        let text = String(clean.prefix(maxChars - characters))
        if text.count < clean.count { stop("chars") }
        lines.append(text)
        characters += text.count
    }

    mutating func walk(_ element: AXUIElement, depth: Int = 0) {
        if characters >= maxChars { stop("chars") }
        if nodes >= 3000 { stop("nodes") }
        if ProcessInfo.processInfo.systemUptime >= deadline { stop("time") }
        if stopped { return }
        if depth > 60 {
            reasons.insert("depth")
            return
        }
        nodes += 1
        let role = stringAttribute(element, kAXRoleAttribute)
        let subrole = stringAttribute(element, kAXSubroleAttribute)
        if role == "AXSecureTextField" || subrole == "AXSecureTextField" {
            secure = true
            return
        }
        if ["AXMenuBar", "AXMenu", "AXToolbar"].contains(role) { return }
        if role == "AXWebArea", url == nil, let value = attribute(element, "AXURL") {
            url = pageAddress(CFGetTypeID(value) == CFURLGetTypeID() ? (value as! CFURL) as URL : (value as? String).flatMap { URL(string: $0) })
        }
        for name in [kAXTitleAttribute, kAXValueAttribute, kAXDescriptionAttribute] {
            add(stringAttribute(element, name))
        }
        guard let children = attribute(element, kAXChildrenAttribute) as? [AXUIElement] else { return }
        for child in children {
            walk(child, depth: depth + 1)
            if stopped { break }
        }
    }
}
