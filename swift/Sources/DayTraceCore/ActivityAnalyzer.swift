import Foundation

public struct BlockKey: Hashable {
    public var hour: Date
    public var app: String
    public var title: String

    public init(hour: Date, app: String, title: String) {
        self.hour = hour
        self.app = app
        self.title = title
    }
}

public struct ObservationPeriod {
    public var app: String
    public var title: String
    public var state: String
    public var captureStatus: String?
    public var first: Date
    public var last: Date
    public var samples: Int
    public var addresses: Set<String>
    public var truncated: Bool
}

public struct SamplingGap {
    public var start: Date
    public var end: Date
    public var seconds: TimeInterval
}

public struct ActivityAnalysis {
    public var samples: [ActivitySample]
    public var summary: ActivitySummary
    public var spentByState: [String: TimeInterval]
    public var appUsage: [String: [String: TimeInterval]]
    public var hourly: [Date: HourAggregate]
    public var gaps: [SamplingGap]
    public var periods: [ObservationPeriod]
    public var blockSeconds: [BlockKey: TimeInterval]
}

public struct HourAggregate {
    public var states: [String: TimeInterval] = [:]
    public var apps: [String: [String: TimeInterval]] = [:]
}

public enum ActivityAnalyzer {
    public static func summarize(_ samples: [ActivitySample]) -> ActivitySummary {
        analyze(samples).summary
    }

    public static func analyze(_ samples: [ActivitySample]) -> ActivityAnalysis {
        let sorted = samples.sorted { $0.timestamp < $1.timestamp }
        let calendar = DayTracePaths.beijingCalendar()

        var spentByState: [String: TimeInterval] = [:]
        var appUsage: [String: [String: TimeInterval]] = [:]
        var hourly: [Date: HourAggregate] = [:]
        var gaps: [SamplingGap] = []
        var periods: [ObservationPeriod] = []
        var blockSeconds: [BlockKey: TimeInterval] = [:]

        var sampleIndex = 0
        while sampleIndex < sorted.count {
            let sample = sorted[sampleIndex]
            let interval = max(2, sample.sampleInterval ?? 10)
            let gapLimit = max(90, interval * 3)
            let delta: TimeInterval
            if sampleIndex + 1 < sorted.count {
                delta = sorted[sampleIndex + 1].timestamp.timeIntervalSince(sample.timestamp)
            } else {
                delta = interval
            }
            let spent = delta > gapLimit ? interval : max(0, delta)
            if delta > gapLimit, sampleIndex + 1 < sorted.count {
                gaps.append(SamplingGap(start: sample.timestamp, end: sorted[sampleIndex + 1].timestamp, seconds: delta - spent))
            }

            spentByState[sample.state, default: 0] += spent
            if sample.state == "active", let app = sample.app {
                let title = sample.title
                appUsage[app, default: [:]][title, default: 0] += spent
                let hour = calendar.dateInterval(of: .hour, for: sample.timestamp)?.start ?? sample.timestamp
                var aggregate = hourly[hour] ?? HourAggregate()
                aggregate.states[sample.state, default: 0] += spent
                aggregate.apps[app, default: [:]][title, default: 0] += spent
                hourly[hour] = aggregate
                blockSeconds[BlockKey(hour: hour, app: app, title: title), default: 0] += spent
            }

            if var period = periods.last,
               period.app == (sample.app ?? ""),
               period.title == sample.title,
               period.state == sample.state,
               period.captureStatus == sample.captureStatus,
               sample.timestamp.timeIntervalSince(period.last) <= gapLimit {
                period.last = sample.timestamp
                period.samples += 1
                period.truncated = period.truncated || (sample.uiTruncated ?? false)
                if let address = sample.url ?? sample.document { period.addresses.insert(address) }
                periods[periods.count - 1] = period
            } else {
                var period = ObservationPeriod(
                    app: sample.app ?? sample.stateLabel,
                    title: sample.title,
                    state: sample.state,
                    captureStatus: sample.captureStatus,
                    first: sample.timestamp,
                    last: sample.timestamp,
                    samples: 1,
                    addresses: [],
                    truncated: sample.uiTruncated ?? false
                )
                if let address = sample.url ?? sample.document { period.addresses.insert(address) }
                periods.append(period)
            }

            sampleIndex += 1
        }

        var summary = ActivitySummary()
        summary.samples = sorted.count
        summary.first = sorted.first?.timestamp
        summary.last = sorted.last?.timestamp
        summary.activeSeconds = spentByState["active"] ?? 0
        summary.apps = appUsage
            .map { AppDuration(name: $0.key, seconds: $0.value.values.reduce(0, +)) }
            .sorted { $0.seconds > $1.seconds }
        summary.hours = hourly
            .map { hour, aggregate in
                HourUsage(
                    hour: hour,
                    apps: aggregate.apps
                        .map { AppDuration(name: $0.key, seconds: $0.value.values.reduce(0, +)) }
                        .sorted { $0.seconds > $1.seconds }
                )
            }
            .sorted { $0.hour < $1.hour }

        return ActivityAnalysis(
            samples: sorted,
            summary: summary,
            spentByState: spentByState,
            appUsage: appUsage,
            hourly: hourly,
            gaps: gaps,
            periods: periods,
            blockSeconds: blockSeconds
        )
    }
}
