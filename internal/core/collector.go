//go:build darwin

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/getupandwashits/daytrace/internal/macos"
)

type CollectorConfig struct {
	SampleInterval float64
	IdleThreshold  float64
	UIMaxChars     int
	ExcludedApps   []string
}

func CollectorConfigFromSettings(settings Settings) CollectorConfig {
	return CollectorConfig{
		SampleInterval: max(2, settings.SampleIntervalSec),
		IdleThreshold:  max(1, settings.IdleThresholdSec),
		UIMaxChars:     min(20000, max(0, settings.UIMaxChars)),
		ExcludedApps:   append([]string(nil), settings.ExcludedApps...),
	}
}

// Collector reads the foreground window through the accessibility tree. It
// never installs event taps, hooks keystrokes, takes screenshots or uses the
// network.
type Collector struct {
	mu         sync.Mutex
	config     CollectorConfig
	journal    *Journal
	running    bool
	generation int
	lastSample *ActivitySample
	contexts   map[string]bool
	currentDay string
	stop       chan struct{}
	trigger    chan struct{}

	OnSample func(ActivitySample)
	OnError  func(string)
}

func NewCollector(config CollectorConfig, journal *Journal) *Collector {
	if journal == nil {
		journal = NewJournal(EventsDir())
	}
	return &Collector{
		config:   config,
		journal:  journal,
		contexts: map[string]bool{},
		trigger:  make(chan struct{}, 1),
	}
}

func (c *Collector) AccessibilityGranted() bool { return macos.AXTrusted() }

func (c *Collector) RequestAccessibility() { macos.RequestAccessibility() }

// Configure invalidates in-flight samples and rebuilds the timer when its
// settings change. It never waits for Accessibility or the AppKit thread.
func (c *Collector) Configure(config CollectorConfig, enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running == enabled && c.config.SampleInterval == config.SampleInterval &&
		c.config.IdleThreshold == config.IdleThreshold && c.config.UIMaxChars == config.UIMaxChars &&
		slices.Equal(c.config.ExcludedApps, config.ExcludedApps) {
		return
	}
	c.stopLocked()
	c.config = config
	if enabled {
		c.startLocked()
	}
}

func (c *Collector) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *Collector) LastSample() *ActivitySample {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastSample == nil {
		return nil
	}
	value := *c.lastSample
	return &value
}

func (c *Collector) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startLocked()
}

func (c *Collector) startLocked() {
	if c.running {
		return
	}
	c.running = true
	c.generation++
	generation := c.generation
	interval := max(2.0, c.config.SampleInterval)
	stop := make(chan struct{})
	c.stop = stop

	go func() {
		c.sample(generation)
		ticker := time.NewTicker(time.Duration(interval * float64(time.Second)))
		defer ticker.Stop()
		// The old collector sampled as soon as another app came forward.
		// Watching the frontmost process costs one NSWorkspace query per
		// second and keeps short visits from falling between ticks.
		watch := time.NewTicker(time.Second)
		defer watch.Stop()
		lastApp := c.foregroundKey()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				c.sample(generation)
				lastApp = c.foregroundKey()
			case <-c.trigger:
				c.sample(generation)
				lastApp = c.foregroundKey()
			case <-watch.C:
				if key := c.foregroundKey(); key != "" && key != lastApp {
					c.sample(generation)
					lastApp = key
				}
			}
		}
	}()
}

// foregroundKey identifies the frontmost app, or "" when nothing is known.
func (c *Collector) foregroundKey() string {
	app := macos.Frontmost()
	if app.PID == 0 {
		return ""
	}
	return app.BundleID + "\x00" + app.Name
}

// Stop ends the current collection run. It does not wait for an in-flight
// sample: sampling calls AppKit on the main thread, and Stop itself runs
// there (settings save, quit), so waiting would deadlock. The generation
// check in sample keeps the old goroutine's results out of the journal.
func (c *Collector) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}

func (c *Collector) stopLocked() {
	if !c.running {
		return
	}
	c.running = false
	c.generation++
	close(c.stop)
	c.stop = nil
}

func (c *Collector) SampleNow() {
	select {
	case c.trigger <- struct{}{}:
	default:
	}
}

func (c *Collector) sample(generation int) {
	c.mu.Lock()
	if !c.running || c.generation != generation {
		c.mu.Unlock()
		return
	}
	config := c.config
	c.mu.Unlock()

	trusted := macos.AXTrusted()
	idle := macos.IdleSeconds()
	locked := macos.ScreenLocked()
	active := !locked && idle < config.IdleThreshold

	sample := ActivitySample{
		Timestamp:      time.Now(),
		State:          "idle",
		IdleSeconds:    idle,
		SampleInterval: config.SampleInterval,
		IdleThreshold:  config.IdleThreshold,
		Accessibility:  trusted,
	}
	if locked {
		sample.State = "locked"
	} else if active {
		sample.State = "active"
	}

	if !active {
		c.emit(generation, sample)
		return
	}

	app := macos.Frontmost()
	if app.PID == 0 && app.Name == "" {
		sample.State = "unavailable"
		sample.CaptureStatus = "no_frontmost_app"
		c.emit(generation, sample)
		return
	}
	if c.isExcluded(config, app.BundleID, app.Name) {
		sample.State = "excluded"
		c.emit(generation, sample)
		return
	}

	name := app.Name
	if name == "" {
		name = "Unknown"
	}
	sample.App = name
	sample.BundleID = app.BundleID
	if trusted {
		sample.CaptureStatus = "no_accessible_window"
	} else {
		sample.CaptureStatus = "permission_missing"
	}

	if trusted {
		snapshot := macos.ReadWindow(app.PID, config.UIMaxChars)
		if snapshot.Status != "no_accessible_window" {
			text := strings.Join(snapshot.Lines, "\n")
			sample.WindowTitle = snapshot.WindowTitle
			sample.UIText = &text
			sample.UITruncated = snapshot.Truncated
			if snapshot.Truncated {
				sample.UITruncatedReasons = append([]string(nil), snapshot.Reasons...)
				sort.Strings(sample.UITruncatedReasons)
			}
			sample.SecureFieldSkipped = snapshot.Secure
			sample.URL = snapshot.URL
			sample.Document = snapshot.Document
			sample.CaptureStatus = snapshot.Status
		}
	}

	// Discard UI if the foreground app changed during traversal.
	after := macos.Frontmost()
	if macos.ScreenLocked() {
		sample = ActivitySample{
			Timestamp: sample.Timestamp, State: "locked", IdleSeconds: idle,
			SampleInterval: config.SampleInterval, IdleThreshold: config.IdleThreshold, Accessibility: trusted,
		}
	} else if after.PID != app.PID {
		sample.UIText = nil
		sample.WindowTitle = ""
		sample.URL = ""
		sample.Document = ""
		sample.CaptureStatus = "foreground_changed"
	}

	cleanSample(&sample)
	c.emit(generation, sample)
}

func (c *Collector) isExcluded(config CollectorConfig, bundleID, name string) bool {
	if protectedApp(bundleID, name) {
		return true
	}
	for _, excluded := range config.ExcludedApps {
		if strings.EqualFold(excluded, bundleID) || strings.EqualFold(excluded, name) {
			return true
		}
	}
	return false
}

func (c *Collector) deduplicateLocked(sample *ActivitySample) {
	if sample.UIText == nil || *sample.UIText == "" {
		return
	}
	day := DayString(sample.Timestamp)
	if day != c.currentDay {
		c.currentDay = day
		c.contexts = map[string]bool{}
	}
	content := sample.BundleID + "\x00" + sample.WindowTitle + "\x00" + *sample.UIText
	sum := sha256.Sum256([]byte(content))
	sample.ContextID = hex.EncodeToString(sum[:])
	if c.contexts[sample.ContextID] {
		sample.UIText = nil
		sample.UITextUnchanged = true
	}
}

func (c *Collector) emit(generation int, sample ActivitySample) {
	c.mu.Lock()
	if !c.running || c.generation != generation {
		c.mu.Unlock()
		return
	}
	c.deduplicateLocked(&sample)
	err := c.journal.Append(sample)
	if err == nil {
		if sample.ContextID != "" {
			c.contexts[sample.ContextID] = true
		}
		c.lastSample = &sample
	}
	c.mu.Unlock()
	if err != nil {
		if c.OnError != nil {
			c.OnError("写入采集记录失败：" + err.Error())
		}
		return
	}
	if c.OnSample != nil {
		c.OnSample(sample)
	}
}
