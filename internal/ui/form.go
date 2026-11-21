package ui

import (
	"strconv"
	"strings"

	"github.com/getupandwashits/daytrace/internal/core"
)

type settingsForm struct {
	automaticFocus string
	pauseTolerance string
	resetThreshold string
	rest           string
	minimumRecord  string
	sampleInterval string
	idleThreshold  string
	collect        bool
	autoExport     bool
	output         string
	excluded       string
}

func formFromSettings(settings core.Settings) settingsForm {
	return settingsForm{
		automaticFocus: formatFloat(settings.AutomaticFocusMin),
		pauseTolerance: formatFloat(settings.PauseToleranceMin),
		resetThreshold: formatFloat(settings.ResetThresholdMin),
		rest:           formatFloat(settings.RestMin),
		minimumRecord:  formatFloat(settings.MinimumRecordMin),
		sampleInterval: formatFloat(settings.SampleIntervalSec),
		idleThreshold:  formatFloat(settings.IdleThresholdSec),
		collect:        settings.CollectActivity,
		autoExport:     settings.AutoExport,
		output:         settings.OutputDirectory,
		excluded:       strings.Join(settings.ExcludedApps, ", "),
	}
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
