package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"
)

type FocusState string

const (
	StateAway      FocusState = "away"
	StateActive    FocusState = "active"
	StatePaused    FocusState = "paused"
	StateReminding FocusState = "reminding"
	StateStopped   FocusState = "stopped"
)

func (s FocusState) Label() string {
	switch s {
	case StateAway:
		return "已休息"
	case StateActive:
		return "专注中"
	case StatePaused:
		return "短暂停顿"
	case StateReminding:
		return "提醒中"
	case StateStopped:
		return "已停止"
	default:
		return string(s)
	}
}

// PomodorosPerCycle stays in the published status files so older readers keep
// seeing the key they know, even though DayTrace no longer runs pomodoro
// cycles.
const PomodorosPerCycle = 4

// Returning to the desk must be sustained, not a single stray HID event (a
// charging trackpad reconnecting, a mouse nudged on the desk). Starting or
// resuming focus needs at least inputEventsNeeded ticks with input inside the
// last inputEventWindow; pausing still uses the regular idle thresholds.
const (
	inputEventMaxIdle = 0.5
	inputEventWindow  = 10 * time.Second
	inputEventsNeeded = 2
)

type FocusTrigger string

const (
	TriggerAuto   FocusTrigger = "auto"
	TriggerManual FocusTrigger = "manual"
)

type FocusEngine struct {
	mu           sync.Mutex
	clock        func() time.Time
	idleProvider func() float64

	settings Settings
	state    FocusState

	focusDuration float64
	idleTime      float64
	suspended     bool
	updatedAt     time.Time

	lastTickAt        time.Time
	focusStart        *time.Time
	lastFocusActivity *time.Time
	awayStartedAt     *time.Time
	resumableDuration *float64
	resumableStart    *time.Time
	completedCurrent  bool
	stoppedWasIdle    bool
	manualRest        bool
	restTarget        float64
	inputEvents       []time.Time
	suspendReasons    map[string]bool

	OnSession  func(FocusSession)
	OnReminder func(string)
}

func NewFocusEngine(settings Settings, clock func() time.Time, idleProvider func() float64) *FocusEngine {
	if clock == nil {
		clock = time.Now
	}
	if idleProvider == nil {
		idleProvider = func() float64 { return 0 }
	}
	engine := &FocusEngine{
		clock:          clock,
		idleProvider:   idleProvider,
		settings:       settings,
		state:          StateAway,
		restTarget:     settings.RestDuration(),
		suspendReasons: map[string]bool{},
		updatedAt:      clock(),
	}
	return engine
}

func (e *FocusEngine) Configure(settings Settings) {
	e.mu.Lock()
	e.settings = settings
	if e.state == StateAway && e.awayStartedAt == nil {
		e.restTarget = settings.RestDuration()
	}
	e.updatedAt = e.clock()
	e.mu.Unlock()
}

func (e *FocusEngine) Tick(idle *float64) {
	e.mu.Lock()
	now := e.clock()
	e.lastTickAt = now
	e.updatedAt = now
	if idle != nil {
		e.idleTime = max(0, *idle)
	} else {
		e.idleTime = e.idleProvider()
	}
	e.recordInputLocked(now)

	if e.suspended {
		e.mu.Unlock()
		return
	}

	var session *FocusSession
	var reminder string

	switch e.state {
	case StateActive:
		if e.idleTime >= e.settings.ResetThreshold() {
			session = e.resetToAwayLocked(TriggerAuto, now)
		} else if e.idleTime >= e.settings.PauseTolerance() {
			e.state = StatePaused
			e.inputEvents = nil
		} else {
			e.focusDuration += 1
			e.lastFocusActivity = &now
			if e.focusDuration >= e.targetFocusTimeLocked() {
				e.focusDuration = e.targetFocusTimeLocked()
				e.state = StateReminding
				reminder = e.reminderTextLocked()
			}
		}

	case StatePaused:
		if e.sustainedInputLocked() {
			session = e.persistIfNeeded(TriggerAuto, e.lastFocusAtLocked())
			e.beginFocusLocked(now)
		} else if e.idleTime >= e.settings.ResetThreshold() {
			session = e.resetToAwayLocked(TriggerAuto, now)
		}

	case StateAway:
		// After an idle gap, coming back starts a new session immediately: the
		// rest countdown is for the menu bar and status readers, it does not
		// hold the user at the desk. A rest the user asked for (强制休息) does
		// run to the end. A single stray HID event is not enough: the user has
		// to actually type or move the pointer before a session starts.
		if e.manualRest {
			if e.awayElapsedLocked(now) >= e.restTarget && e.sustainedInputLocked() {
				e.beginFocusLocked(now)
			}
		} else if e.sustainedInputLocked() {
			e.beginFocusLocked(now)
		}

	case StateReminding:
		if e.idleTime >= e.settings.ResetThreshold() {
			session = e.resetToAwayLocked(TriggerAuto, now)
		}

	case StateStopped:
		if e.idleTime >= e.settings.PauseTolerance() {
			e.stoppedWasIdle = true
		}
		if e.stoppedWasIdle && e.sustainedInputLocked() {
			e.stoppedWasIdle = false
			session = e.persistIfNeeded(TriggerAuto, e.lastFocusAtLocked())
			e.beginFocusLocked(now)
		}
	}

	e.mu.Unlock()
	e.emit(session, reminder)
}

// recordInputLocked notes ticks where the pointer or keyboard was used just
// before the tick, so a single stray event cannot pass for the user returning.
func (e *FocusEngine) recordInputLocked(now time.Time) {
	if e.idleTime < inputEventMaxIdle {
		e.inputEvents = append(e.inputEvents, now)
	}
	cutoff := now.Add(-inputEventWindow)
	for len(e.inputEvents) > 0 && !e.inputEvents[0].After(cutoff) {
		e.inputEvents = e.inputEvents[1:]
	}
}

func (e *FocusEngine) sustainedInputLocked() bool {
	return len(e.inputEvents) >= inputEventsNeeded
}

func (e *FocusEngine) emit(session *FocusSession, reminder string) {
	if session != nil && e.OnSession != nil {
		e.OnSession(*session)
	}
	if reminder != "" && e.OnReminder != nil {
		e.OnReminder(reminder)
	}
}

// BeginRest starts the rest countdown after the user acknowledged the rest
// prompt. The finished focus session is recorded first, so the rest starts
// from a clean state and runs for its full length.
func (e *FocusEngine) BeginRest() {
	e.mu.Lock()
	now := e.clock()
	if e.state != StateReminding {
		e.mu.Unlock()
		return
	}
	session := e.persistIfNeeded(TriggerManual, e.lastFocusAtLocked())
	e.state = StateAway
	e.inputEvents = nil
	e.restTarget = e.settings.RestDuration()
	start := now
	e.awayStartedAt = &start
	e.manualRest = true
	e.focusDuration = 0
	e.completedCurrent = false
	e.lastFocusActivity = nil
	e.resumableDuration = nil
	e.resumableStart = nil
	e.focusStart = nil
	e.mu.Unlock()
	e.emit(session, "")
}

func (e *FocusEngine) ResetToAway(trigger FocusTrigger) {
	e.mu.Lock()
	now := e.clock()
	session := e.resetToAwayLocked(trigger, now)
	e.mu.Unlock()
	e.emit(session, "")
}

func (e *FocusEngine) resetToAwayLocked(trigger FocusTrigger, now time.Time) *FocusSession {
	e.restTarget = e.settings.RestDuration()

	session := e.persistIfNeeded(trigger, e.lastFocusAtLocked())
	e.state = StateAway
	e.inputEvents = nil
	// The rest countdown starts when the rest is detected, not when the last
	// activity was: by the time a long idle gap is recognised, the gap is
	// already longer than the reset threshold and would leave nothing to
	// count.
	start := now
	e.awayStartedAt = &start
	e.manualRest = trigger == TriggerManual
	e.focusDuration = 0
	e.completedCurrent = false
	e.lastFocusActivity = nil
	e.resumableDuration = nil
	e.resumableStart = nil
	e.focusStart = nil
	return session
}

func (e *FocusEngine) StartManually() {
	e.mu.Lock()
	now := e.clock()
	session := e.persistIfNeeded(TriggerManual, e.lastFocusAtLocked())
	e.beginFocusLocked(now)
	e.mu.Unlock()
	e.emit(session, "")
}

func (e *FocusEngine) StopManually() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state != StateActive && e.state != StatePaused {
		return
	}
	value := e.focusDuration
	e.resumableDuration = &value
	e.resumableStart = e.focusStart
	e.state = StateStopped
	e.stoppedWasIdle = false
	e.focusStart = nil
	e.awayStartedAt = nil
}

func (e *FocusEngine) ResumeManually() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state != StateStopped {
		return
	}
	duration := e.focusDuration
	if e.resumableDuration != nil {
		duration = *e.resumableDuration
	}
	duration = max(0, duration)
	now := e.clock()
	e.state = StateActive
	e.stoppedWasIdle = false
	e.lastTickAt = now
	e.focusDuration = duration
	if e.resumableStart != nil {
		e.focusStart = e.resumableStart
	} else {
		start := now.Add(-time.Duration(duration * float64(time.Second)))
		e.focusStart = &start
	}
	e.awayStartedAt = nil
	e.resumableDuration = nil
	e.resumableStart = nil
}

func (e *FocusEngine) RestartManually() { e.StartManually() }

func (e *FocusEngine) TogglePause() {
	e.mu.Lock()
	switch e.state {
	case StateActive:
		e.mu.Unlock()
		e.StopManually()
		return
	case StateStopped:
		e.mu.Unlock()
		e.ResumeManually()
		return
	case StatePaused:
		e.state = StateActive
		e.lastTickAt = e.clock()
	}
	e.mu.Unlock()
}

func (e *FocusEngine) Suspend(reason string) {
	e.mu.Lock()
	e.suspendReasons[reason] = true
	if e.suspended {
		e.mu.Unlock()
		return
	}
	e.suspended = true
	now := e.clock()
	session := e.persistIfNeeded(TriggerManual, e.lastFocusAtLocked())
	e.state = StateAway
	e.manualRest = false
	e.awayStartedAt = &now
	e.focusDuration = 0
	e.completedCurrent = false
	e.lastFocusActivity = nil
	e.resumableDuration = nil
	e.resumableStart = nil
	e.focusStart = nil
	e.mu.Unlock()
	e.emit(session, "")
}

func (e *FocusEngine) Resume(reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.suspendReasons, reason)
	if len(e.suspendReasons) != 0 || !e.suspended {
		return
	}
	e.suspended = false
	now := e.clock()
	e.lastTickAt = now
	if e.state == StateAway {
		e.awayStartedAt = &now
	}
}

func (e *FocusEngine) beginFocusLocked(now time.Time) {
	e.lastTickAt = now
	e.completedCurrent = false
	e.lastFocusActivity = nil
	e.resumableDuration = nil
	e.resumableStart = nil
	e.stoppedWasIdle = false
	e.manualRest = false
	e.state = StateActive
	e.focusDuration = 0
	e.focusStart = &now
	e.awayStartedAt = nil
}

func (e *FocusEngine) persistIfNeeded(trigger FocusTrigger, endedAt time.Time) *FocusSession {
	if e.completedCurrent || e.focusDuration < e.settings.MinimumRecordDuration() {
		return nil
	}
	start := endedAt.Add(-time.Duration(e.focusDuration * float64(time.Second)))
	if e.focusStart != nil {
		start = *e.focusStart
	} else if e.resumableStart != nil {
		start = *e.resumableStart
	}
	session := FocusSession{
		ID:       newID(),
		Start:    start,
		End:      endedAt,
		Duration: e.focusDuration,
		Trigger:  string(trigger),
	}
	return &session
}

func (e *FocusEngine) reminderTextLocked() string {
	return fmt.Sprintf("已经专注 %d 分钟，站起来走动一下，喝口水再继续。", int(e.focusDuration/60))
}

func (e *FocusEngine) lastFocusAtLocked() time.Time {
	if e.lastFocusActivity != nil {
		return *e.lastFocusActivity
	}
	return e.clock()
}

func (e *FocusEngine) targetFocusTimeLocked() float64 {
	return max(60, e.settings.AutomaticFocusMin*60)
}

func (e *FocusEngine) awayElapsedLocked(now time.Time) float64 {
	if e.awayStartedAt == nil {
		return e.restTarget
	}
	return max(0, now.Sub(*e.awayStartedAt).Seconds())
}

// Snapshot is safe to call from any goroutine.
func (e *FocusEngine) Snapshot() StatusSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.clock()
	return StatusSnapshot{
		SchemaVersion:         1,
		Source:                "daytrace",
		ProcessID:             os.Getpid(),
		UpdatedAt:             now,
		State:                 string(e.state),
		StateLabel:            e.state.Label(),
		Mode:                  "automatic",
		FocusElapsedSeconds:   e.focusDuration,
		FocusTargetSeconds:    e.targetFocusTimeLocked(),
		FocusRemainingSeconds: e.remainingDurationLocked(),
		RestRemainingSeconds:  e.restRemainingLocked(now),
		RestTargetSeconds:     e.restTarget,
		RemainingFraction:     e.remainingFractionLocked(now),
		TrackingSuspended:     e.suspended,
		CompletedPomodoros:    0,
		PomodorosPerCycle:     PomodorosPerCycle,
	}
}

func (e *FocusEngine) LegacySnapshot() LegacyLiveStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.clock()
	phase := "focus"
	if e.state == StateAway {
		phase = "shortBreak"
	}
	return LegacyLiveStatus{
		SchemaVersion:         1,
		Source:                "autopomodoro",
		ProcessID:             os.Getpid(),
		UpdatedAt:             now,
		State:                 string(e.state),
		StateLabel:            e.state.Label(),
		FocusElapsedSeconds:   e.focusDuration,
		FocusTargetSeconds:    e.targetFocusTimeLocked(),
		FocusRemainingSeconds: e.remainingDurationLocked(),
		RestRemainingSeconds:  e.restRemainingLocked(now),
		RestTargetSeconds:     e.restTarget,
		RemainingFraction:     e.remainingFractionLocked(now),
		TrackingSuspended:     e.suspended,
		Mode:                  "automatic",
		Phase:                 phase,
		CompletedPomodoros:    0,
		PomodorosPerCycle:     PomodorosPerCycle,
	}
}

func (e *FocusEngine) remainingDurationLocked() float64 {
	return max(e.targetFocusTimeLocked()-e.focusDuration, 0)
}

func (e *FocusEngine) restRemainingLocked(now time.Time) float64 {
	if e.state != StateAway {
		return 0
	}
	return max(e.restTarget-e.awayElapsedLocked(now), 0)
}

func (e *FocusEngine) remainingFractionLocked(now time.Time) float64 {
	remaining := e.remainingDurationLocked()
	target := e.targetFocusTimeLocked()
	if e.state == StateAway {
		remaining = e.restRemainingLocked(now)
		target = e.restTarget
	}
	return min(1, max(0, remaining/max(1, target)))
}

func (e *FocusEngine) UnrecordedFocusDuration() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.completedCurrent {
		return 0
	}
	return e.focusDuration
}

func (e *FocusEngine) phaseLabelLocked() string {
	switch e.state {
	case StateAway:
		return "休息"
	case StateActive:
		return "专注中"
	case StatePaused:
		return "短暂停顿"
	case StateReminding:
		return "需要休息"
	case StateStopped:
		return "已停止"
	}
	return string(e.state)
}

func (e *FocusEngine) PhaseLabel() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.phaseLabelLocked()
}

// MenuBarTitle is the text shown in the menu bar: the remaining focus time
// all the time, so the clock never disappears.
func (e *FocusEngine) MenuBarTitle() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.state {
	case StateActive, StatePaused:
		return formatSeconds(e.remainingDurationLocked())
	case StateReminding:
		return "该休息"
	case StateAway:
		if remaining := e.restRemainingLocked(e.clock()); remaining > 0 {
			return "休息 " + formatSeconds(remaining)
		}
		return "休息"
	case StateStopped:
		return "已停止"
	}
	return ""
}

func (e *FocusEngine) Settings() Settings {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.settings
}

func formatSeconds(seconds float64) string {
	total := max(int(seconds), 0)
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func newID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}
