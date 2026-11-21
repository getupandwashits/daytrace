//go:build darwin

package macos

import "C"

//export daytraceStatusAction
func daytraceStatusAction(index C.int) {
	runStatusAction(int(index))
}
