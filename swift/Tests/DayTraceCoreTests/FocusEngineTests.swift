import XCTest
@testable import DayTraceCore

final class FocusEngineTests: XCTestCase {
    final class Clock {
        var now = Date(timeIntervalSince1970: 1_700_000_000)
    }

    final class SessionBox {
        var sessions: [FocusSession] = []
    }

    private func makeEngine(mode: String = "automatic") -> (FocusEngine, Clock, SessionBox) {
        var settings = DayTraceSettings()
        settings.mode = mode
        settings.automaticFocusMinutes = 45
        settings.traditionalFocusMinutes = 1
        settings.shortBreakMinutes = 5
        settings.longBreakMinutes = 15
        settings.pauseToleranceMinutes = 3
        settings.resetThresholdMinutes = 10
        settings.minimumRecordMinutes = 0.5
        settings.rounds = 4

        let clock = Clock()
        let engine = FocusEngine(clock: { clock.now }, idleProvider: { 0 }, statusURL: nil)
        engine.configure(settings)
        let box = SessionBox()
        engine.onSession = { box.sessions.append($0) }
        return (engine, clock, box)
    }

    private func advance(_ engine: FocusEngine, _ clock: Clock, seconds: Int) {
        for _ in 0..<seconds {
            clock.now.addTimeInterval(1)
            engine.tick(idle: 0, elapsed: 1)
        }
    }

    func testAutomaticModeStartsAndPausesOnIdle() {
        let (engine, clock, box) = makeEngine()
        engine.tick(idle: 0, elapsed: 0)
        XCTAssertEqual(engine.state, .active)
        advance(engine, clock, seconds: 40)
        XCTAssertEqual(engine.focusDuration, 40, accuracy: 0.01)

        engine.tick(idle: 200, elapsed: 1)
        XCTAssertEqual(engine.state, .paused)

        engine.tick(idle: 0, elapsed: 1)
        XCTAssertEqual(engine.state, .active)
        XCTAssertEqual(engine.focusDuration, 0, accuracy: 0.01)
        XCTAssertEqual(box.sessions.count, 1)
        XCTAssertEqual(box.sessions.first?.duration ?? 0, 40, accuracy: 0.01)
    }

    func testAutomaticModeResetsToAwayAfterLongIdle() {
        let (engine, _, _) = makeEngine()
        engine.tick(idle: 0, elapsed: 0)
        XCTAssertEqual(engine.state, .active)
        engine.tick(idle: 700, elapsed: 1)
        XCTAssertEqual(engine.state, .away)
        XCTAssertEqual(engine.focusDuration, 0, accuracy: 0.01)
    }

    func testTraditionalModeCompletesPomodoroAndStartsBreak() {
        let (engine, clock, box) = makeEngine(mode: "traditional")
        engine.tick(idle: 0, elapsed: 0)
        XCTAssertEqual(engine.state, .active)
        advance(engine, clock, seconds: 60)
        XCTAssertEqual(engine.state, .reminding)
        XCTAssertEqual(box.sessions.count, 1)
        XCTAssertEqual(engine.completedPomodoros, 1)

        engine.resetToAway(trigger: .manual)
        XCTAssertEqual(engine.state, .away)
        XCTAssertEqual(engine.breakKind, .short)
        XCTAssertEqual(engine.restRemainingDuration, 5 * 60, accuracy: 0.01)
    }

    func testManualStopRetainsProgressAndResumeRestoresIt() {
        let (engine, clock, box) = makeEngine()
        engine.tick(idle: 0, elapsed: 0)
        advance(engine, clock, seconds: 40)
        engine.stopFocusManually()
        XCTAssertEqual(engine.state, .stopped)
        let retained = engine.focusDuration
        XCTAssertEqual(retained, 40, accuracy: 0.01)

        engine.resumeFocusManually()
        XCTAssertEqual(engine.state, .active)
        XCTAssertEqual(engine.focusDuration, retained, accuracy: 0.01)

        engine.resetToAway(trigger: .manual)
        XCTAssertEqual(box.sessions.count, 1)
        XCTAssertEqual(box.sessions.first?.duration ?? 0, retained, accuracy: 0.01)
    }

    func testSuspendPausesAndResumeClearsSuspension() {
        let (engine, _, _) = makeEngine()
        engine.tick(idle: 0, elapsed: 0)
        engine.suspend(reason: "lock")
        XCTAssertTrue(engine.suspended)
        XCTAssertEqual(engine.state, .away)
        engine.resume(reason: "lock")
        XCTAssertFalse(engine.suspended)
    }

    func testShortSessionIsNotRecorded() {
        let (engine, clock, box) = makeEngine()
        engine.tick(idle: 0, elapsed: 0)
        advance(engine, clock, seconds: 5)
        engine.resetToAway(trigger: .manual)
        XCTAssertTrue(box.sessions.isEmpty)
    }
}

final class LegacyStatusTests: XCTestCase {
    func testLegacyStatusMatchesAutoPomodoroSchemaAndPhase() throws {
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent("daytrace-legacy-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let url = directory.appendingPathComponent("status.json")

        let engine = FocusEngine(clock: Date.init, idleProvider: { 0 }, statusURL: nil)
        engine.legacyStatusWriter = LegacyStatusWriter(urls: [url], checkRunningApplication: false)
        engine.tick(idle: 0, elapsed: 0)
        XCTAssertEqual(engine.state, .active)

        var data = try Data(contentsOf: url)
        var object = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(object["source"] as? String, "autopomodoro")
        XCTAssertEqual(object["schemaVersion"] as? Int, 1)
        XCTAssertEqual(object["state"] as? String, "active")
        XCTAssertEqual(object["phase"] as? String, "focus")
        XCTAssertEqual(object["mode"] as? String, "automatic")
        XCTAssertEqual(object["processID"] as? Int32, getpid())

        engine.resetToAway(trigger: .manual)
        data = try Data(contentsOf: url)
        object = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(object["state"] as? String, "away")
        XCTAssertEqual(object["phase"] as? String, "shortBreak")
    }

    func testWriterBacksOffWhenAnotherLiveWriterOwnsTheFile() throws {
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent("daytrace-legacy-owner-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let url = directory.appendingPathComponent("status.json")

        var foreign = LegacyLiveStatus(
            processID: Int32(getppid()),
            updatedAt: Date(),
            state: "active",
            stateLabel: "专注中",
            focusElapsedSeconds: 10,
            focusTargetSeconds: 1500,
            focusRemainingSeconds: 1490,
            restRemainingSeconds: 0,
            restTargetSeconds: 300,
            remainingFraction: 0.99,
            trackingSuspended: false,
            mode: "traditional",
            phase: "focus",
            completedPomodoros: 1,
            pomodorosPerCycle: 4
        )
        try foreign.write(to: url)
        let original = try Data(contentsOf: url)
        let writer = LegacyStatusWriter(urls: [url], checkRunningApplication: false)
        XCTAssertTrue(writer.ownedByAnotherInstance)
        writer.write(foreign)
        XCTAssertEqual(try Data(contentsOf: url), original)

        foreign.processID = 999_999
        try foreign.write(to: url)
        var ours = foreign
        ours.processID = Int32(getpid())
        ours.state = "paused"
        ours.stateLabel = "短暂停顿"
        writer.write(ours)
        let replaced = try JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any]
        XCTAssertEqual(replaced?["processID"] as? Int32, Int32(getpid()))
        XCTAssertEqual(replaced?["state"] as? String, "paused")
    }
}
