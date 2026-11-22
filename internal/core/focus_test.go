package core

import (
	"math"
	"testing"
	"time"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

func makeEngine() (*FocusEngine, *testClock, *[]FocusSession) {
	settings := DefaultSettings()
	settings.AutomaticFocusMin = 45
	settings.PauseToleranceMin = 3
	settings.ResetThresholdMin = 10
	settings.MinimumRecordMin = 0.5

	clock := &testClock{now: time.Unix(1_700_000_000, 0)}
	engine := NewFocusEngine(settings, clock.Now, func() float64 { return 0 })
	sessions := &[]FocusSession{}
	engine.OnSession = func(session FocusSession) { *sessions = append(*sessions, session) }
	return engine, clock, sessions
}

func advance(engine *FocusEngine, clock *testClock, seconds int) {
	for index := 0; index < seconds; index++ {
		clock.now = clock.now.Add(time.Second)
		engine.Tick(floatPtr(0))
	}
}

func floatPtr(value float64) *float64 { return &value }

func TestAutomaticModeStartsAndPausesOnIdle(t *testing.T) {
	engine, clock, sessions := makeEngine()
	zero := 0.0
	engine.Tick(&zero)
	engine.Tick(&zero)
	if got := engine.Snapshot().State; got != string(StateActive) {
		t.Fatalf("state = %s", got)
	}
	advance(engine, clock, 40)
	if got := engine.Snapshot().FocusElapsedSeconds; math.Abs(got-40) > 0.01 {
		t.Fatalf("focus = %v", got)
	}
	idle := 200.0
	engine.Tick(&idle)
	if got := engine.Snapshot().State; got != string(StatePaused) {
		t.Fatalf("state after idle = %s", got)
	}
	zero = 0
	engine.Tick(&zero)
	engine.Tick(&zero)
	if got := engine.Snapshot().State; got != string(StateActive) {
		t.Fatalf("state after return = %s", got)
	}
	if got := engine.Snapshot().FocusElapsedSeconds; got != 0 {
		t.Fatalf("focus after return = %v", got)
	}
	if len(*sessions) != 1 || math.Abs((*sessions)[0].Duration-40) > 0.01 {
		t.Fatalf("sessions = %+v", *sessions)
	}
}

func TestSingleStrayInputDoesNotStartFocus(t *testing.T) {
	engine, clock, _ := makeEngine()
	zero := 0.0
	engine.Tick(&zero) // one stray event, e.g. a charging trackpad reconnecting
	for index := 1; index <= 20; index++ {
		clock.now = clock.now.Add(time.Second)
		idle := float64(index)
		engine.Tick(&idle)
	}
	if got := engine.Snapshot().State; got != string(StateAway) {
		t.Fatalf("state after a single event = %s", got)
	}
	// Let that event age out, then another isolated event is still not "back
	// at the desk".
	for index := 1; index <= 11; index++ {
		clock.now = clock.now.Add(time.Second)
		idle := float64(index)
		engine.Tick(&idle)
	}
	engine.Tick(&zero)
	if got := engine.Snapshot().State; got != string(StateAway) {
		t.Fatalf("state after another isolated event = %s", got)
	}
	// Sustained input starts the session.
	engine.Tick(&zero)
	if got := engine.Snapshot().State; got != string(StateActive) {
		t.Fatalf("state after sustained input = %s", got)
	}
}

func TestAutomaticModeResetsToAwayAfterLongIdle(t *testing.T) {
	engine, _, _ := makeEngine()
	zero := 0.0
	engine.Tick(&zero)
	engine.Tick(&zero)
	idle := 700.0
	engine.Tick(&idle)
	if got := engine.Snapshot().State; got != string(StateAway) {
		t.Fatalf("state = %s", got)
	}
	if got := engine.Snapshot().FocusElapsedSeconds; got != 0 {
		t.Fatalf("focus = %v", got)
	}
}

func TestReachingTheFocusTargetRemindsAndResetsAfterIdle(t *testing.T) {
	settings := DefaultSettings()
	settings.AutomaticFocusMin = 1
	settings.PauseToleranceMin = 3
	settings.ResetThresholdMin = 10
	settings.MinimumRecordMin = 0.5
	clock := &testClock{now: time.Unix(1_700_000_000, 0)}
	engine := NewFocusEngine(settings, clock.Now, func() float64 { return 0 })
	sessions := &[]FocusSession{}
	engine.OnSession = func(session FocusSession) { *sessions = append(*sessions, session) }

	zero := 0.0
	engine.Tick(&zero)
	engine.Tick(&zero)
	advance(engine, clock, 60)
	status := engine.Snapshot()
	if status.State != string(StateReminding) {
		t.Fatalf("state = %s", status.State)
	}

	idle := 700.0
	engine.Tick(&idle)
	status = engine.Snapshot()
	if status.State != string(StateAway) {
		t.Fatalf("state after long idle = %s", status.State)
	}
	if math.Abs(status.RestTargetSeconds-300) > 0.01 {
		t.Fatalf("rest target = %v", status.RestTargetSeconds)
	}
	if math.Abs(status.RestRemainingSeconds-300) > 0.01 {
		t.Fatalf("rest remaining = %v", status.RestRemainingSeconds)
	}
	// Coming back starts a new session; the rest countdown is informational.
	engine.Tick(&zero)
	engine.Tick(&zero)
	if got := engine.Snapshot().State; got != string(StateActive) {
		t.Fatalf("state after returning = %s", got)
	}
	if len(*sessions) != 1 || math.Abs((*sessions)[0].Duration-60) > 0.01 {
		t.Fatalf("sessions = %+v", *sessions)
	}
}

func TestManualStopRetainsProgressAndResumeRestoresIt(t *testing.T) {
	engine, clock, sessions := makeEngine()
	zero := 0.0
	engine.Tick(&zero)
	engine.Tick(&zero)
	advance(engine, clock, 40)
	engine.StopManually()
	if got := engine.Snapshot().State; got != string(StateStopped) {
		t.Fatalf("state = %s", got)
	}
	retained := engine.Snapshot().FocusElapsedSeconds
	if math.Abs(retained-40) > 0.01 {
		t.Fatalf("retained = %v", retained)
	}
	engine.ResumeManually()
	if got := engine.Snapshot().State; got != string(StateActive) {
		t.Fatalf("state after resume = %s", got)
	}
	if got := engine.Snapshot().FocusElapsedSeconds; got != retained {
		t.Fatalf("focus after resume = %v", got)
	}
	engine.ResetToAway(TriggerManual)
	if len(*sessions) != 1 || math.Abs((*sessions)[0].Duration-retained) > 0.01 {
		t.Fatalf("sessions = %+v", *sessions)
	}
}

func TestSuspendPausesAndResumeClearsSuspension(t *testing.T) {
	engine, _, _ := makeEngine()
	zero := 0.0
	engine.Tick(&zero)
	engine.Suspend("lock")
	if !engine.Snapshot().TrackingSuspended {
		t.Fatal("not suspended")
	}
	if got := engine.Snapshot().State; got != string(StateAway) {
		t.Fatalf("state = %s", got)
	}
	engine.Resume("lock")
	if engine.Snapshot().TrackingSuspended {
		t.Fatal("still suspended")
	}
}

func TestShortSessionIsNotRecorded(t *testing.T) {
	engine, clock, sessions := makeEngine()
	zero := 0.0
	engine.Tick(&zero)
	engine.Tick(&zero)
	advance(engine, clock, 5)
	engine.ResetToAway(TriggerManual)
	if len(*sessions) != 0 {
		t.Fatalf("sessions = %+v", *sessions)
	}
}

func TestAutomaticRestKeepsACountdown(t *testing.T) {
	engine, _, _ := makeEngine()
	zero := 0.0
	engine.Tick(&zero)
	engine.Tick(&zero)
	idle := 700.0
	engine.Tick(&idle)
	status := engine.Snapshot()
	if status.State != string(StateAway) {
		t.Fatalf("state = %s", status.State)
	}
	if math.Abs(status.RestRemainingSeconds-300) > 0.01 {
		t.Fatalf("rest remaining = %v", status.RestRemainingSeconds)
	}
	if status.RemainingFraction < 0.9 {
		t.Fatalf("fraction = %v", status.RemainingFraction)
	}
}

func TestManualRestRunsToTheEnd(t *testing.T) {
	engine, clock, _ := makeEngine()
	zero := 0.0
	engine.Tick(&zero)
	engine.ResetToAway(TriggerManual)
	status := engine.Snapshot()
	if status.State != string(StateAway) {
		t.Fatalf("state = %s", status.State)
	}
	if math.Abs(status.RestRemainingSeconds-300) > 0.01 {
		t.Fatalf("rest remaining = %v", status.RestRemainingSeconds)
	}
	advance(engine, clock, 60)
	engine.Tick(&zero)
	if got := engine.Snapshot().State; got != string(StateAway) {
		t.Fatalf("manual rest ended early: %s", got)
	}
	advance(engine, clock, 250)
	engine.Tick(&zero)
	engine.Tick(&zero)
	if got := engine.Snapshot().State; got != string(StateActive) {
		t.Fatalf("state after rest = %s", got)
	}
}

func TestBeginRestStartsAFullCountdownAndRecordsTheFocus(t *testing.T) {
	settings := DefaultSettings()
	settings.AutomaticFocusMin = 1
	settings.MinimumRecordMin = 0.5
	clock := &testClock{now: time.Unix(1_700_000_000, 0)}
	engine := NewFocusEngine(settings, clock.Now, func() float64 { return 0 })
	sessions := &[]FocusSession{}
	engine.OnSession = func(session FocusSession) { *sessions = append(*sessions, session) }

	zero := 0.0
	engine.Tick(&zero)
	engine.Tick(&zero)
	advance(engine, clock, 60)
	if got := engine.Snapshot().State; got != string(StateReminding) {
		t.Fatalf("state = %s", got)
	}
	engine.BeginRest()
	status := engine.Snapshot()
	if status.State != string(StateAway) {
		t.Fatalf("state after rest = %s", status.State)
	}
	if math.Abs(status.RestRemainingSeconds-300) > 0.01 {
		t.Fatalf("rest remaining = %v", status.RestRemainingSeconds)
	}
	if len(*sessions) != 1 || math.Abs((*sessions)[0].Duration-60) > 0.01 {
		t.Fatalf("sessions = %+v", *sessions)
	}
	advance(engine, clock, 301)
	engine.Tick(&zero)
	if got := engine.Snapshot().State; got != string(StateActive) {
		t.Fatalf("state after rest ends = %s", got)
	}
}

func TestEveryFourthRoundUsesALongRest(t *testing.T) {
	settings := DefaultSettings()
	settings.AutomaticFocusMin = 1
	settings.MinimumRecordMin = 0.5
	clock := &testClock{now: time.Unix(1_700_000_000, 0)}
	engine := NewFocusEngine(settings, clock.Now, func() float64 { return 0 })
	zero := 0.0

	completeRound := func(round int) StatusSnapshot {
		engine.Tick(&zero)
		engine.Tick(&zero)
		advance(engine, clock, 60)
		status := engine.Snapshot()
		if status.State != string(StateReminding) {
			t.Fatalf("round %d state = %s", round, status.State)
		}
		return status
	}

	for round := 1; round <= 3; round++ {
		status := completeRound(round)
		if status.RestKind != "short" || math.Abs(status.RestTargetSeconds-300) > 0.01 {
			t.Fatalf("round %d rest = %s %.0fs", round, status.RestKind, status.RestTargetSeconds)
		}
		engine.BeginRest()
		advance(engine, clock, 301)
	}

	status := completeRound(4)
	if status.RestKind != "long" || math.Abs(status.RestTargetSeconds-900) > 0.01 {
		t.Fatalf("fourth rest = %s %.0fs", status.RestKind, status.RestTargetSeconds)
	}
	engine.BeginRest()
	status = engine.Snapshot()
	if status.State != string(StateAway) || status.RestKind != "long" || math.Abs(status.RestRemainingSeconds-900) > 0.01 {
		t.Fatalf("long rest = %s %s %.0fs", status.State, status.RestKind, status.RestRemainingSeconds)
	}
	advance(engine, clock, 901)

	// The cycle starts over: the next completed round gets a short rest.
	status = completeRound(5)
	if status.RestKind != "short" || math.Abs(status.RestTargetSeconds-300) > 0.01 {
		t.Fatalf("rest after long rest = %s %.0fs", status.RestKind, status.RestTargetSeconds)
	}
}

func TestSkippingALongRestStartsANewCycle(t *testing.T) {
	settings := DefaultSettings()
	settings.AutomaticFocusMin = 1
	settings.MinimumRecordMin = 0.5
	clock := &testClock{now: time.Unix(1_700_000_000, 0)}
	engine := NewFocusEngine(settings, clock.Now, func() float64 { return 0 })
	zero := 0.0

	completeRound := func() StatusSnapshot {
		engine.Tick(&zero)
		engine.Tick(&zero)
		advance(engine, clock, 60)
		return engine.Snapshot()
	}
	for round := 1; round <= 3; round++ {
		completeRound()
		engine.BeginRest()
		advance(engine, clock, 301)
	}
	if status := completeRound(); status.RestKind != "long" {
		t.Fatalf("fourth rest = %s", status.RestKind)
	}
	// 「继续专注」 skips the long rest; the next completed round is short.
	engine.RestartManually()
	status := completeRound()
	if status.RestKind != "short" || math.Abs(status.RestTargetSeconds-300) > 0.01 {
		t.Fatalf("rest after skipping = %s %.0fs", status.RestKind, status.RestTargetSeconds)
	}
}

func TestFocusWaitsForFreshInputAfterAllSuspendReasonsClear(t *testing.T) {
	engine, clock, _ := makeEngine()
	engine.Suspend("lock")
	engine.Suspend("displaySleep")
	advance(engine, clock, 2)
	engine.Resume("displaySleep")
	if !engine.Snapshot().TrackingSuspended {
		t.Fatal("display wake resumed a locked session")
	}
	engine.Resume("lock")
	advance(engine, clock, 1)
	if engine.Snapshot().State != string(StateAway) {
		t.Fatal("input during suspension started focus")
	}
	advance(engine, clock, 1)
	if engine.Snapshot().State != string(StateActive) {
		t.Fatal("focus did not start after fresh input")
	}
}
