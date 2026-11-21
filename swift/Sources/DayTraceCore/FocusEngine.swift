import Foundation

public enum FocusState: String, CaseIterable, Codable {
    case away
    case active
    case paused
    case reminding
    case stopped

    public var label: String {
        switch self {
        case .away: return "已休息"
        case .active: return "专注中"
        case .paused: return "短暂停顿"
        case .reminding: return "提醒中"
        case .stopped: return "已停止"
        }
    }
}

public enum FocusMode: String, CaseIterable, Codable {
    case automatic
    case traditional

    public var label: String {
        self == .automatic ? "自动专注" : "传统番茄钟"
    }
}

public enum BreakKind: String, Codable {
    case short
    case long
}

public enum FocusTrigger: String, Codable {
    case auto
    case manual
}

public final class FocusEngine: ObservableObject {
    @Published public private(set) var state: FocusState = .away
    @Published public private(set) var mode: FocusMode = .automatic
    @Published public private(set) var focusDuration: TimeInterval = 0
    @Published public private(set) var idleTime: TimeInterval = 0
    @Published public private(set) var completedPomodoros = 0
    @Published public private(set) var breakKind: BreakKind = .short
    @Published public private(set) var suspended = false
    @Published public private(set) var updatedAt = Date()

    public var onSession: ((FocusSession) -> Void)?
    public var onReminder: ((String) -> Void)?
    public var onStatusChange: (() -> Void)?

    private var settings = DayTraceSettings()
    private var lastTickAt: Date?
    private var focusStart: Date?
    private var lastFocusActivity: Date?
    private var awayStartedAt: Date?
    private var resumableDuration: TimeInterval?
    private var resumableStart: Date?
    private var completedCurrentFocus = false
    private var completedAt: Date?
    private var stoppedWasIdle = false
    private var restTarget: TimeInterval = 5 * 60
    private var phaseTarget: TimeInterval?
    private var suspendReasons: Set<String> = []

    private let clock: () -> Date
    private let idleProvider: () -> TimeInterval
    public var statusURL: URL?
    /// Optional bridge for the external readers that expect AutoPomodoro's
    /// `autopomodoro` status schema.
    public var legacyStatusWriter: LegacyStatusWriter?

    public init(
        clock: @escaping () -> Date = Date.init,
        idleProvider: @escaping () -> TimeInterval = { Presence.idleSeconds() },
        statusURL: URL? = DayTracePaths.status
    ) {
        self.clock = clock
        self.idleProvider = idleProvider
        self.statusURL = statusURL
        apply(settings, reset: false)
    }

    // MARK: - Configuration

    public func configure(_ newSettings: DayTraceSettings) {
        let modeChanged = (FocusMode(rawValue: newSettings.mode) ?? .automatic) != mode
        apply(newSettings, reset: modeChanged)
    }

    private func apply(_ newSettings: DayTraceSettings, reset: Bool) {
        let newMode = FocusMode(rawValue: newSettings.mode) ?? .automatic
        if reset {
            persistIfNeeded(.manual, endedAt: lastFocusActivity ?? clock())
            state = .away
            focusDuration = 0
            focusStart = nil
            lastFocusActivity = nil
            resumableDuration = nil
            resumableStart = nil
            completedCurrentFocus = false
            completedAt = nil
            phaseTarget = nil
            breakKind = .short
            completedPomodoros = 0
            awayStartedAt = nil
        }
        settings = newSettings
        mode = newMode
        if state == .away && awayStartedAt == nil {
            restTarget = max(60, newSettings.shortBreakMinutes * 60)
        }
        if state == .paused && newMode == .traditional && !newSettings.pauseWhenIdle && suspendReasons.isEmpty {
            state = .active
            lastTickAt = clock()
        }
        publishStatus()
    }

    // MARK: - Timer

    public func tick(idle: TimeInterval? = nil, elapsed: TimeInterval? = nil) {
        let tickTime = clock()
        let delta = max(0, elapsed ?? tickTime.timeIntervalSince(lastTickAt ?? tickTime))
        lastTickAt = tickTime
        updatedAt = tickTime
        idleTime = idle ?? idleProvider()

        guard !suspended else {
            publishStatus()
            return
        }

        switch state {
        case .active:
            if mode == .automatic && idleTime >= settings.resetThreshold {
                resetToAway(trigger: .auto)
            } else if (mode == .automatic || settings.pauseWhenIdle) && idleTime >= settings.pauseTolerance {
                state = .paused
            } else {
                focusDuration += mode == .traditional ? delta : 1
                lastFocusActivity = tickTime
                if focusDuration >= targetFocusTime {
                    focusDuration = targetFocusTime
                    state = .reminding
                    if mode == .traditional { completePomodoroIfNeeded() }
                    showReminder()
                }
            }

        case .paused:
            if idleTime < settings.pauseTolerance {
                persistIfNeeded(.auto, endedAt: lastFocusActivity ?? tickTime)
                beginFocus()
            } else if mode == .automatic && idleTime >= settings.resetThreshold {
                resetToAway(trigger: .auto)
            }

        case .away:
            if awayElapsed >= restTargetDuration && idleTime < settings.pauseTolerance {
                beginFocus()
            }

        case .reminding:
            let threshold = mode == .traditional ? nextBreakDuration : settings.resetThreshold
            if idleTime >= threshold {
                resetToAway(trigger: .auto)
            }

        case .stopped:
            if idleTime >= settings.pauseTolerance { stoppedWasIdle = true }
            if stoppedWasIdle && idleTime < settings.pauseTolerance {
                stoppedWasIdle = false
                persistIfNeeded(.auto, endedAt: lastFocusActivity ?? tickTime)
                beginFocus()
            }
        }

        publishStatus()
    }

    // MARK: - Transitions

    public func resetToAway(trigger: FocusTrigger) {
        if mode == .traditional && state == .away { return }
        if mode == .traditional {
            breakKind = completedCurrentFocus && completedPomodoros % max(1, settings.rounds) == 0 ? .long : .short
            restTarget = max(60, (breakKind == .long ? settings.longBreakMinutes : settings.shortBreakMinutes) * 60)
        } else {
            restTarget = settings.resetThreshold
        }

        let timestamp = clock()
        persistIfNeeded(trigger, endedAt: lastFocusActivity ?? timestamp)
        state = .away
        awayStartedAt = computeAwayStart(trigger: trigger, now: timestamp)
        focusDuration = 0
        completedCurrentFocus = false
        completedAt = nil
        phaseTarget = nil
        lastFocusActivity = nil
        resumableDuration = nil
        resumableStart = nil
        focusStart = nil
        publishStatus()
    }

    private func beginFocus() {
        phaseTarget = max(60, settings.traditionalFocusMinutes * 60)
        lastTickAt = clock()
        completedCurrentFocus = false
        completedAt = nil
        lastFocusActivity = nil
        resumableDuration = nil
        resumableStart = nil
        stoppedWasIdle = false
        state = .active
        focusDuration = 0
        focusStart = clock()
        awayStartedAt = nil
        publishStatus()
    }

    public func startFocusManually() {
        persistIfNeeded(.manual, endedAt: lastFocusActivity ?? clock())
        beginFocus()
    }

    public func stopFocusManually() {
        guard state == .active || state == .paused else { return }
        resumableDuration = focusDuration
        resumableStart = focusStart
        state = .stopped
        stoppedWasIdle = false
        focusStart = nil
        awayStartedAt = nil
        publishStatus()
    }

    public func resumeFocusManually() {
        guard state == .stopped else { return }
        let duration = max(resumableDuration ?? focusDuration, 0)
        state = .active
        stoppedWasIdle = false
        lastTickAt = clock()
        focusDuration = duration
        focusStart = resumableStart ?? clock().addingTimeInterval(-duration)
        awayStartedAt = nil
        resumableDuration = nil
        resumableStart = nil
        publishStatus()
    }

    public func restartFocusManually() {
        startFocusManually()
    }

    public func togglePause() {
        switch state {
        case .active: stopFocusManually()
        case .stopped: resumeFocusManually()
        case .paused:
            state = .active
            lastTickAt = clock()
            publishStatus()
        case .away, .reminding:
            break
        }
    }

    public func suspend(reason: String) {
        suspendReasons.insert(reason)
        guard !suspended else { return }
        suspended = true
        let timestamp = clock()
        persistIfNeeded(.manual, endedAt: lastFocusActivity ?? timestamp)
        state = .away
        awayStartedAt = timestamp
        focusDuration = 0
        completedCurrentFocus = false
        completedAt = nil
        phaseTarget = nil
        lastFocusActivity = nil
        resumableDuration = nil
        resumableStart = nil
        focusStart = nil
        publishStatus()
    }

    public func resume(reason: String) {
        suspendReasons.remove(reason)
        guard suspendReasons.isEmpty, suspended else { return }
        suspended = false
        lastTickAt = clock()
        if state == .away { awayStartedAt = clock() }
        publishStatus()
    }

    // MARK: - Session records

    private func completePomodoroIfNeeded() {
        guard !completedCurrentFocus else { return }
        persistIfNeeded(.auto, endedAt: lastFocusActivity ?? clock())
        completedCurrentFocus = true
        completedAt = clock()
        completedPomodoros += 1
    }

    private func persistIfNeeded(_ trigger: FocusTrigger, endedAt: Date) {
        guard !completedCurrentFocus, focusDuration >= settings.minimumRecordDuration else { return }
        let start = focusStart ?? resumableStart ?? endedAt.addingTimeInterval(-focusDuration)
        let session = FocusSession(start: start, end: endedAt, duration: focusDuration, trigger: trigger.rawValue)
        onSession?(session)
    }

    private func showReminder() {
        let message: String
        if mode == .traditional {
            let next = nextBreakDuration
            let kind = completedPomodoros % max(1, settings.rounds) == 0 ? "长休息" : "短休息"
            message = "已完成第 \(completedPomodoros) 个番茄，接下来\(kind) \(Int(next / 60)) 分钟。"
        } else {
            message = "已经专注 \(Int(focusDuration / 60)) 分钟，站起来走动一下，喝口水再继续。"
        }
        onReminder?(message)
    }

    // MARK: - Derived values

    public var targetFocusTime: TimeInterval {
        mode == .traditional
            ? max(60, phaseTarget ?? settings.traditionalFocusMinutes * 60)
            : max(60, settings.automaticFocusMinutes * 60)
    }

    public var restTargetDuration: TimeInterval { restTarget }

    public var nextBreakDuration: TimeInterval {
        let kind: BreakKind = completedCurrentFocus && completedPomodoros % max(1, settings.rounds) == 0 ? .long : .short
        return max(60, (kind == .long ? settings.longBreakMinutes : settings.shortBreakMinutes) * 60)
    }

    public var remainingDuration: TimeInterval { max(targetFocusTime - focusDuration, 0) }

    /// Focus seconds not yet written to the session journal.
    public var unrecordedFocusDuration: TimeInterval {
        completedCurrentFocus ? 0 : focusDuration
    }

    public var restRemainingDuration: TimeInterval {
        state == .away ? max(restTargetDuration - awayElapsed, 0) : 0
    }

    public var remainingFraction: Double {
        let remaining = state == .away ? restRemainingDuration : remainingDuration
        let target = state == .away ? restTargetDuration : targetFocusTime
        return min(1, max(0, remaining / max(1, target)))
    }

    public var formattedRemaining: String { Self.format(remainingDuration) }
    public var formattedRestRemaining: String { Self.format(restRemainingDuration) }
    public var formattedElapsed: String { Self.format(focusDuration) }

    public var phaseLabel: String {
        switch state {
        case .away:
            if mode == .traditional {
                return breakKind == .long ? "长休息" : "短休息"
            }
            return "休息"
        case .active: return "专注中"
        case .paused: return "短暂停顿"
        case .reminding: return "需要休息"
        case .stopped: return "已停止"
        }
    }

    public var menuBarTitle: String {
        switch state {
        case .active, .paused, .reminding:
            return formattedRemaining
        case .away:
            return restRemainingDuration > 0 ? "\(phaseLabel) \(formattedRestRemaining)" : "休息"
        case .stopped:
            return "停止"
        }
    }

    private var legacyPhase: String {
        guard state == .away else { return "focus" }
        return mode == .traditional && breakKind == .long ? "longBreak" : "shortBreak"
    }

    private var awayElapsed: TimeInterval {
        guard let awayStartedAt else { return restTargetDuration }
        return max(0, clock().timeIntervalSince(awayStartedAt))
    }

    private func computeAwayStart(trigger: FocusTrigger, now: Date) -> Date {
        if trigger == .auto {
            let inferred = now.addingTimeInterval(-idleTime)
            if mode == .traditional, let completedAt {
                return min(now, max(inferred, completedAt))
            }
            return min(inferred, now)
        }
        return now
    }

    private static func format(_ duration: TimeInterval) -> String {
        let total = max(Int(duration), 0)
        let minutes = total / 60
        let seconds = total % 60
        return String(format: "%02d:%02d", minutes, seconds)
    }

    // MARK: - Status file

    public func publishStatus() {
        let snapshot = StatusSnapshot(
            processID: ProcessInfo.processInfo.processIdentifier,
            updatedAt: clock(),
            state: state.rawValue,
            stateLabel: state.label,
            mode: mode.rawValue,
            focusElapsedSeconds: focusDuration,
            focusTargetSeconds: targetFocusTime,
            focusRemainingSeconds: remainingDuration,
            restRemainingSeconds: restRemainingDuration,
            restTargetSeconds: restTargetDuration,
            remainingFraction: remainingFraction,
            trackingSuspended: suspended,
            completedPomodoros: completedPomodoros,
            pomodorosPerCycle: max(1, settings.rounds)
        )
        if let statusURL { try? snapshot.write(to: statusURL) }
        if let legacyStatusWriter {
            legacyStatusWriter.write(LegacyLiveStatus(
                processID: snapshot.processID,
                updatedAt: snapshot.updatedAt,
                state: snapshot.state,
                stateLabel: snapshot.stateLabel,
                focusElapsedSeconds: snapshot.focusElapsedSeconds,
                focusTargetSeconds: snapshot.focusTargetSeconds,
                focusRemainingSeconds: snapshot.focusRemainingSeconds,
                restRemainingSeconds: snapshot.restRemainingSeconds,
                restTargetSeconds: snapshot.restTargetSeconds,
                remainingFraction: snapshot.remainingFraction,
                trackingSuspended: snapshot.trackingSuspended,
                mode: snapshot.mode,
                phase: legacyPhase,
                completedPomodoros: snapshot.completedPomodoros,
                pomodorosPerCycle: snapshot.pomodorosPerCycle
            ))
        }
        onStatusChange?()
    }
}
