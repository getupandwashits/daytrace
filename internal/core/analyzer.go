package core

import (
	"sort"
	"time"
)

type BlockKey struct {
	Hour  time.Time
	App   string
	Title string
}

type ObservationPeriod struct {
	App           string
	Title         string
	State         string
	CaptureStatus string
	First         time.Time
	Last          time.Time
	Samples       int
	Addresses     map[string]bool
	Truncated     bool
}

type SamplingGap struct {
	Start   time.Time
	End     time.Time
	Seconds float64
}

type HourAggregate struct {
	States map[string]float64
	Apps   map[string]map[string]float64
}

type ActivityAnalysis struct {
	Samples      []ActivitySample
	Summary      ActivitySummary
	SpentByState map[string]float64
	AppUsage     map[string]map[string]float64
	Hourly       map[time.Time]HourAggregate
	Gaps         []SamplingGap
	Periods      []ObservationPeriod
	BlockSeconds map[BlockKey]float64
}

func Summarize(samples []ActivitySample) ActivitySummary {
	return Analyze(samples).Summary
}

func Analyze(samples []ActivitySample) ActivityAnalysis {
	sorted := append([]ActivitySample(nil), samples...)
	sort.SliceStable(sorted, func(a, b int) bool {
		return sorted[a].Timestamp.Before(sorted[b].Timestamp)
	})

	analysis := ActivityAnalysis{
		Samples:      sorted,
		SpentByState: map[string]float64{},
		AppUsage:     map[string]map[string]float64{},
		Hourly:       map[time.Time]HourAggregate{},
		BlockSeconds: map[BlockKey]float64{},
	}

	for index, sample := range sorted {
		interval := sample.SampleInterval
		if interval <= 0 {
			interval = 10
		}
		if interval < 2 {
			interval = 2
		}
		gapLimit := max(90.0, interval*3)
		delta := interval
		if index+1 < len(sorted) {
			delta = sorted[index+1].Timestamp.Sub(sample.Timestamp).Seconds()
		}
		spent := delta
		if delta > gapLimit {
			spent = interval
			if index+1 < len(sorted) {
				analysis.Gaps = append(analysis.Gaps, SamplingGap{
					Start:   sample.Timestamp,
					End:     sorted[index+1].Timestamp,
					Seconds: max(0, delta-spent),
				})
			}
		}
		if spent < 0 {
			spent = 0
		}

		analysis.SpentByState[sample.State] += spent
		if sample.State != "active" || sample.App == "" {
			analysis.appendPeriod(sample, gapLimit)
			continue
		}

		title := sample.Title()
		if analysis.AppUsage[sample.App] == nil {
			analysis.AppUsage[sample.App] = map[string]float64{}
		}
		analysis.AppUsage[sample.App][title] += spent

		hour := sample.Timestamp.In(Beijing).Truncate(time.Hour)
		aggregate, ok := analysis.Hourly[hour]
		if !ok {
			aggregate = HourAggregate{
				States: map[string]float64{},
				Apps:   map[string]map[string]float64{},
			}
		}
		aggregate.States[sample.State] += spent
		if aggregate.Apps[sample.App] == nil {
			aggregate.Apps[sample.App] = map[string]float64{}
		}
		aggregate.Apps[sample.App][title] += spent
		analysis.Hourly[hour] = aggregate
		analysis.BlockSeconds[BlockKey{Hour: hour, App: sample.App, Title: title}] += spent

		analysis.appendPeriod(sample, gapLimit)
	}

	summary := ActivitySummary{
		Samples:       len(sorted),
		ActiveSeconds: analysis.SpentByState["active"],
	}
	if len(sorted) > 0 {
		first, last := sorted[0].Timestamp, sorted[len(sorted)-1].Timestamp
		summary.First = &first
		summary.Last = &last
	}
	for app, titles := range analysis.AppUsage {
		var total float64
		for _, seconds := range titles {
			total += seconds
		}
		summary.Apps = append(summary.Apps, AppDuration{Name: app, Seconds: total})
	}
	sort.Slice(summary.Apps, func(a, b int) bool {
		if summary.Apps[a].Seconds == summary.Apps[b].Seconds {
			return summary.Apps[a].Name < summary.Apps[b].Name
		}
		return summary.Apps[a].Seconds > summary.Apps[b].Seconds
	})

	hours := make([]time.Time, 0, len(analysis.Hourly))
	for hour := range analysis.Hourly {
		hours = append(hours, hour)
	}
	sort.Slice(hours, func(a, b int) bool { return hours[a].Before(hours[b]) })
	for _, hour := range hours {
		var apps []AppDuration
		for app, titles := range analysis.Hourly[hour].Apps {
			var total float64
			for _, seconds := range titles {
				total += seconds
			}
			apps = append(apps, AppDuration{Name: app, Seconds: total})
		}
		sort.Slice(apps, func(a, b int) bool {
			if apps[a].Seconds == apps[b].Seconds {
				return apps[a].Name < apps[b].Name
			}
			return apps[a].Seconds > apps[b].Seconds
		})
		summary.Hours = append(summary.Hours, HourUsage{Hour: hour, Apps: apps})
	}
	analysis.Summary = summary
	return analysis
}

func (a *ActivityAnalysis) appendPeriod(sample ActivitySample, gapLimit float64) {
	app := sample.AppName()
	if count := len(a.Periods); count > 0 {
		period := a.Periods[count-1]
		if period.App == app &&
			period.Title == sample.Title() &&
			period.State == sample.State &&
			period.CaptureStatus == sample.CaptureStatus &&
			sample.Timestamp.Sub(period.Last).Seconds() <= gapLimit {
			period.Last = sample.Timestamp
			period.Samples++
			period.Truncated = period.Truncated || sample.UITruncated
			a.addAddress(&period, sample)
			a.Periods[count-1] = period
			return
		}
	}
	period := ObservationPeriod{
		App:           app,
		Title:         sample.Title(),
		State:         sample.State,
		CaptureStatus: sample.CaptureStatus,
		First:         sample.Timestamp,
		Last:          sample.Timestamp,
		Samples:       1,
		Addresses:     map[string]bool{},
		Truncated:     sample.UITruncated,
	}
	a.addAddress(&period, sample)
	a.Periods = append(a.Periods, period)
}

func (a *ActivityAnalysis) addAddress(period *ObservationPeriod, sample ActivitySample) {
	address := sample.URL
	if address == "" {
		address = sample.Document
	}
	if address != "" {
		period.Addresses[address] = true
	}
}
