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
