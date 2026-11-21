//go:build darwin

package macos

import "C"

//export daytraceRestAnswer
func daytraceRestAnswer(rest C.int) {
	answerRestPrompt(rest != 0)
}
