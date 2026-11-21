//go:build darwin

package macos

import "C"

//export daytraceStatusAction
func daytraceStatusAction(title *C.char) {
	runStatusAction(C.GoString(title))
}
