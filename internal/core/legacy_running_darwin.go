//go:build darwin

package core

import "github.com/getupandwashits/daytrace/internal/macos"

func autoPomodoroRunning() bool {
	return macos.AppRunning("twickenhamma.AutoPomodoro", "AutoPomodoro")
}
