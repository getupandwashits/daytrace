//go:build darwin

package macos

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreGraphics -framework AppKit
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>
#include <stdlib.h>
#include <string.h>

int dtAXTrusted(void);
int dtAXPrompt(void);
double dtIdleSeconds(void);
int dtScreenLocked(void);
CFTypeRef dtAXCreateApplication(pid_t pid);
void dtAXSetTimeout(CFTypeRef element, float seconds);
CFTypeRef dtAXCopyAttribute(CFTypeRef element, const char *name);
char *dtAXCopyString(CFTypeRef element, const char *name);
int dtAXIsArray(CFTypeRef value);
long dtAXArrayCount(CFTypeRef value);
CFTypeRef dtAXArrayItem(CFTypeRef value, long index);
int dtAXIsElement(CFTypeRef value);
char *dtAXURLCopyString(CFTypeRef value);
void dtAXRelease(CFTypeRef value);
int dtAXIsNull(CFTypeRef value);
void dtFrontmost(char **bundle, char **name, pid_t *pid);
int dtAppRunning(const char *bundleID, const char *name);
void dtActivate(void);
*/
import "C"

import (
	"net/url"
	"strings"
	"time"
	"unsafe"
)

// Activate brings this app forward. DayTrace runs as a menu bar accessory, so
// a window shown from the status menu otherwise opens behind the frontmost app.
func Activate() { C.dtActivate() }

func AXTrusted() bool { return C.dtAXTrusted() != 0 }

func RequestAccessibility() { _ = C.dtAXPrompt() }

func IdleSeconds() float64 {
	value := float64(C.dtIdleSeconds())
	if value < 0 {
		return 0
	}
	return value
}

func ScreenLocked() bool { return C.dtScreenLocked() != 0 }

// AppRunning reports whether an application with this bundle identifier or
// localized name is currently running.
func AppRunning(bundleID, name string) bool {
	cbundle := C.CString(bundleID)
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cbundle))
	defer C.free(unsafe.Pointer(cname))
	return C.dtAppRunning(cbundle, cname) != 0
}

type FrontmostApp struct {
	BundleID string
	Name     string
	PID      int
}

func Frontmost() FrontmostApp {
	var bundle, name *C.char
	var pid C.pid_t
	C.dtFrontmost(&bundle, &name, &pid)
	defer func() {
		if bundle != nil {
			C.free(unsafe.Pointer(bundle))
		}
		if name != nil {
			C.free(unsafe.Pointer(name))
		}
	}()
	app := FrontmostApp{PID: int(pid)}
	if bundle != nil {
		app.BundleID = C.GoString(bundle)
	}
	if name != nil {
		app.Name = C.GoString(name)
	}
	return app
}

type TextSnapshot struct {
	WindowTitle string
	Lines       []string
	URL         string
	Document    string
	Truncated   bool
	Reasons     []string
	Secure      bool
	Status      string
}

func ReadWindow(pid int, maxChars int) TextSnapshot {
	application := C.dtAXCreateApplication(C.pid_t(pid))
	if axNull(application) {
		return TextSnapshot{Status: "no_accessible_window"}
	}
	defer C.dtAXRelease(application)
	C.dtAXSetTimeout(application, 0.15)

	window := copyAttribute(application, "AXFocusedWindow")
	if axNull(window) {
		return TextSnapshot{Status: "no_accessible_window"}
	}
	defer C.dtAXRelease(window)
	C.dtAXSetTimeout(window, 0.15)

	snapshot := TextSnapshot{
		WindowTitle: axString(window, "AXTitle"),
		Status:      "no_accessible_text",
	}
	reader := textReader{
		seen:     map[string]bool{},
		reasons:  map[string]bool{},
		maxChars: max(0, maxChars),
		deadline: time.Now().Add(800 * time.Millisecond),
	}
	reader.walk(window, 0)
	snapshot.Lines = reader.lines
	snapshot.Truncated = len(reader.reasons) > 0
	snapshot.Secure = reader.secure
	for reason := range reader.reasons {
		snapshot.Reasons = append(snapshot.Reasons, reason)
	}
	snapshot.URL = reader.url
	if snapshot.URL == "" {
		if document := axString(window, "AXDocument"); document != "" {
			if parsed, err := url.Parse(document); err == nil && parsed.Scheme == "file" {
				snapshot.Document = parsed.Path
			}
		}
	}
	if len(snapshot.Lines) > 0 {
		snapshot.Status = "ok"
	}
	return snapshot
}

type textReader struct {
	lines    []string
	seen     map[string]bool
	chars    int
	nodes    int
	stopped  bool
	reasons  map[string]bool
	secure   bool
	url      string
	maxChars int
	deadline time.Time
}

func (r *textReader) stop(reason string) {
	r.stopped = true
	r.reasons[reason] = true
}

func (r *textReader) add(value string) {
	clean := strings.TrimSpace(strings.ReplaceAll(value, "​", ""))
	if clean == "" || r.seen[clean] || r.chars >= r.maxChars {
		return
	}
	r.seen[clean] = true
	runes := []rune(clean)
	if remaining := r.maxChars - r.chars; len(runes) > remaining {
		clean = string(runes[:remaining])
		r.stop("chars")
	}
	r.lines = append(r.lines, clean)
	r.chars += len([]rune(clean))
}

func (r *textReader) walk(element C.CFTypeRef, depth int) {
	if r.stopped {
		return
	}
	if r.chars >= r.maxChars {
		r.stop("chars")
		return
	}
	if r.nodes >= 3000 {
		r.stop("nodes")
		return
	}
	if time.Now().After(r.deadline) {
		r.stop("time")
		return
	}
	if depth > 60 {
		r.reasons["depth"] = true
		return
	}
	r.nodes++

	role := axString(element, "AXRole")
	subrole := axString(element, "AXSubrole")
	if role == "AXSecureTextField" || subrole == "AXSecureTextField" {
		r.secure = true
		return
	}
	if role == "AXMenuBar" || role == "AXMenu" || role == "AXToolbar" {
		return
	}
	if role == "AXWebArea" && r.url == "" {
		if raw := axURL(element, "AXURL"); raw != "" {
			r.url = pageAddress(raw)
		}
	}
	for _, name := range []string{"AXTitle", "AXValue", "AXDescription"} {
		r.add(axString(element, name))
	}

	children := copyAttribute(element, "AXChildren")
	if axNull(children) {
		return
	}
	defer C.dtAXRelease(children)
	if C.dtAXIsArray(children) == 0 {
		return
	}
	count := int(C.dtAXArrayCount(children))
	for index := 0; index < count && !r.stopped; index++ {
		child := C.dtAXArrayItem(children, C.long(index))
		if axNull(child) || C.dtAXIsElement(child) == 0 {
			continue
		}
		r.walk(child, depth+1)
	}
}

func copyAttribute(element C.CFTypeRef, name string) C.CFTypeRef {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	return C.dtAXCopyAttribute(element, cname)
}

func axNull(value C.CFTypeRef) bool { return C.dtAXIsNull(value) != 0 }

func axString(element C.CFTypeRef, name string) string {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	value := C.dtAXCopyString(element, cname)
	if value == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value)
}

func axURL(element C.CFTypeRef, name string) string {
	value := copyAttribute(element, name)
	if axNull(value) {
		return ""
	}
	defer C.dtAXRelease(value)
	cvalue := C.dtAXURLCopyString(value)
	if cvalue == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cvalue))
	return C.GoString(cvalue)
}

func pageAddress(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
