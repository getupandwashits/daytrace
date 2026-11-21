//go:build darwin

package macos

/*
#cgo LDFLAGS: -framework AppKit
void dtInstallPresence(void);
*/
import "C"

import "sync"

var presence struct {
	sync.Mutex
	handler func(string, bool)
}

// InstallPresence registers for macOS sleep, display and lock notifications.
// Install it once on the application thread.
func InstallPresence(handler func(string, bool)) {
	presence.Lock()
	presence.handler = handler
	presence.Unlock()
	C.dtInstallPresence()
}

//export daytracePresenceChanged
func daytracePresenceChanged(reason *C.char, suspended C.int) {
	presence.Lock()
	handler := presence.handler
	presence.Unlock()
	if handler != nil {
		handler(C.GoString(reason), suspended != 0)
	}
}
