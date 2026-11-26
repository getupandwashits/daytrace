//go:build darwin

package macos

/*
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>

void dtDismissRestPrompt(void);
void dtShowRestPrompt(const char *title, const char *message);
*/
import "C"

import (
	"sync"
	"unsafe"
)

var restPrompt struct {
	mu     sync.Mutex
	answer func(bool)
}

// ShowRestPrompt shows the floating "time to rest" card. It does not block
// and never takes keyboard focus: the answer (true for rest, false for
// later) arrives on the main thread when the user clicks a button.
func ShowRestPrompt(title, message string, answer func(bool)) {
	restPrompt.mu.Lock()
	restPrompt.answer = answer
	restPrompt.mu.Unlock()

	ctitle := C.CString(title)
	cmessage := C.CString(message)
	defer C.free(unsafe.Pointer(ctitle))
	defer C.free(unsafe.Pointer(cmessage))
	C.dtShowRestPrompt(ctitle, cmessage)
}

func DismissRestPrompt() {
	restPrompt.mu.Lock()
	restPrompt.answer = nil
	restPrompt.mu.Unlock()
	C.dtDismissRestPrompt()
}

func answerRestPrompt(rest bool) {
	restPrompt.mu.Lock()
	answer := restPrompt.answer
	restPrompt.answer = nil
	restPrompt.mu.Unlock()
	if answer != nil {
		answer(rest)
	}
}
