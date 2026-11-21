import AppKit
import CoreGraphics
import Foundation

public enum Presence {
    public static func idleSeconds() -> TimeInterval {
        let value = CGEventSource.secondsSinceLastEventType(
            .combinedSessionState,
            eventType: CGEventType(rawValue: UInt32.max)!
        )
        return value.isFinite ? max(0, value) : 0
    }
}

public final class PresenceObserver {
    public var onSuspend: ((String) -> Void)?
    public var onResume: ((String) -> Void)?

    private var reasons = Set<String>()
    private var observers: [NSObjectProtocol] = []
    private let workspaceCenter = NSWorkspace.shared.notificationCenter
    private let distributedCenter = DistributedNotificationCenter.default()

    public init() {}

    public func start() {
        stop()
        observers.append(workspaceCenter.addObserver(
            forName: NSWorkspace.willSleepNotification, object: nil, queue: .main
        ) { [weak self] _ in self?.add("systemSleep") })
        observers.append(workspaceCenter.addObserver(
            forName: NSWorkspace.didWakeNotification, object: nil, queue: .main
        ) { [weak self] _ in self?.remove("systemSleep") })
        observers.append(workspaceCenter.addObserver(
            forName: NSWorkspace.screensDidSleepNotification, object: nil, queue: .main
        ) { [weak self] _ in self?.add("displaySleep") })
        observers.append(workspaceCenter.addObserver(
            forName: NSWorkspace.screensDidWakeNotification, object: nil, queue: .main
        ) { [weak self] _ in self?.remove("displaySleep") })
        observers.append(distributedCenter.addObserver(
            forName: NSNotification.Name("com.apple.screenIsLocked"), object: nil, queue: .main
        ) { [weak self] _ in self?.add("lock") })
        observers.append(distributedCenter.addObserver(
            forName: NSNotification.Name("com.apple.screenIsUnlocked"), object: nil, queue: .main
        ) { [weak self] _ in self?.remove("lock") })
    }

    public func stop() {
        for observer in observers {
            workspaceCenter.removeObserver(observer)
            distributedCenter.removeObserver(observer)
        }
        observers.removeAll()
        reasons.removeAll()
    }

    private func add(_ reason: String) {
        if reasons.insert(reason).inserted {
            onSuspend?(reason)
        }
    }

    private func remove(_ reason: String) {
        if reasons.remove(reason) != nil {
            onResume?(reason)
        }
    }
}
