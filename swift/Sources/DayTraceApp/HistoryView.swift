import Charts
import DayTraceCore
import SwiftUI

struct HistoryView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                HStack(spacing: 12) {
                    StatCard(
                        title: "今日专注",
                        value: DigestEngine.duration(model.todayFocusSeconds),
                        subtitle: "\(model.todaySessionCount) 段记录",
                        icon: "timer",
                        tint: .orange
                    )
                    StatCard(
                        title: "最近 7 天专注",
                        value: DigestEngine.duration(model.weekFocusSeconds),
                        subtitle: "按完成时段累计",
                        icon: "calendar",
                        tint: .blue
                    )
                    StatCard(
                        title: "全部记录",
                        value: "\(model.sessions.count)",
                        subtitle: "段专注",
                        icon: "list.bullet.rectangle",
                        tint: .teal
                    )
                }

                Card {
                    VStack(alignment: .leading, spacing: 10) {
                        Text("最近 7 天专注").font(.headline)
                        if model.lastSevenDays.allSatisfy({ $0.seconds == 0 }) {
                            Text("还没有完成过专注记录。")
                                .font(.callout)
                                .foregroundStyle(.secondary)
                                .frame(height: 160, alignment: .center)
                                .frame(maxWidth: .infinity)
                        } else {
                            Chart(model.lastSevenDays) { day in
                                BarMark(
                                    x: .value("日期", day.date, unit: .day),
                                    y: .value("分钟", day.seconds / 60)
                                )
                                .foregroundStyle(.orange.gradient)
                                .cornerRadius(4)
                            }
                            .chartYAxis { AxisMarks(position: .leading) }
                            .frame(height: 170)
                        }
                    }
                }

                Card {
                    VStack(alignment: .leading, spacing: 12) {
                        Text("专注记录").font(.headline)
                        if grouped.isEmpty {
                            Text("还没有完成过专注记录。")
                                .font(.callout)
                                .foregroundStyle(.secondary)
                        } else {
                            ForEach(grouped) { day in
                                VStack(alignment: .leading, spacing: 6) {
                                    HStack {
                                        Text(day.id)
                                            .font(.subheadline.weight(.semibold))
                                        Spacer()
                                        Text(DigestEngine.duration(day.total))
                                            .font(.caption)
                                            .foregroundStyle(.secondary)
                                    }
                                    ForEach(day.sessions) { session in
                                        HStack(spacing: 10) {
                                            Text("\(TimeFormat.clock(session.start))–\(TimeFormat.clock(session.end))")
                                                .font(.caption.monospacedDigit())
                                            Text(session.triggerLabel)
                                                .font(.caption2)
                                                .padding(.horizontal, 6)
                                                .padding(.vertical, 2)
                                                .background(Capsule().fill(Color.secondary.opacity(0.12)))
                                            Spacer()
                                            Text(DigestEngine.duration(session.duration))
                                                .font(.caption.monospacedDigit())
                                        }
                                    }
                                }
                                if day.id != grouped.last?.id { Divider() }
                            }
                        }
                    }
                }
            }
            .padding(20)
        }
    }

    private var grouped: [SessionDay] {
        let groups = Dictionary(grouping: model.sessions) { DayTracePaths.dayString($0.start) }
        return groups.map { key, value in
            SessionDay(id: key, sessions: value.sorted { $0.start > $1.start })
        }
        .sorted { $0.id > $1.id }
    }
}

private struct SessionDay: Identifiable {
    let id: String
    let sessions: [FocusSession]

    var total: TimeInterval {
        sessions.reduce(0) { $0 + $1.duration }
    }
}
