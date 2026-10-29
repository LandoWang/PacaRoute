import SwiftUI
import Charts
import BridgeCore

struct UsageTimelineView: View {
    @EnvironmentObject var store: AppStore
    @State private var metric = "tokens"
    @State private var selectedDate: Date?
    private var selected: UsageBucket? {
        guard let date = selectedDate else { return nil }
        return store.timeline.first { date >= $0.start && date < $0.end }
    }
    var body: some View {
        Card {
            HStack {
                Text(store.usageHourly ? "按小时统计" : "按天统计").font(.headline); Spacer()
                Picker("指标", selection: $metric) {
                    Text("Token").tag("tokens"); Text("请求 / 失败").tag("requests")
                }.pickerStyle(.segmented).frame(width: 230)
            }
            Chart {
                ForEach(store.timeline) { bucket in
                    if metric == "tokens" {
                        BarMark(x: .value("时间", bucket.start, unit: store.usageHourly ? .hour : .day), y: .value("Token", bucket.totals.input))
                            .foregroundStyle(by: .value("类别", "输入"))
                        BarMark(x: .value("时间", bucket.start, unit: store.usageHourly ? .hour : .day), y: .value("Token", bucket.totals.output))
                            .foregroundStyle(by: .value("类别", "输出"))
                    } else {
                        LineMark(x: .value("时间", bucket.start), y: .value("次数", bucket.totals.requests))
                            .foregroundStyle(by: .value("类别", "请求"))
                        LineMark(x: .value("时间", bucket.start), y: .value("次数", bucket.totals.failures))
                            .foregroundStyle(by: .value("类别", "失败"))
                            .lineStyle(StrokeStyle(lineWidth: 2, dash: [5, 4]))
                    }
                }
                if let selected {
                    RuleMark(x: .value("时间", selected.start)).foregroundStyle(.secondary.opacity(0.5))
                }
            }
            .chartXScale(domain: store.usageInterval.start...store.usageInterval.end)
            .chartXSelection(value: $selectedDate)
            .chartForegroundStyleScale(domain: metric == "tokens" ? ["输入", "输出"] : ["请求", "失败"],
                                       range: metric == "tokens" ? [AuroraTheme.violetInk, AuroraTheme.accent] : [AuroraTheme.accent, Color.orange])
            .chartLegend(position: .bottom)
            .frame(height: 220)
            if let selected {
                Text("\(selected.start.formatted(date: .abbreviated, time: store.usageHourly ? .shortened : .omitted)) · \(selected.totals.requests) 次请求 · 失败 \(selected.totals.failures) · 输入 \(selected.totals.input.formatted()) / 输出 \(selected.totals.output.formatted()) token · 缺少用量 \(selected.totals.missingUsage) 次")
                    .font(.caption).monospacedDigit()
            }
            Text("本地时区：\(TimeZone.current.identifier) · 点击或拖动图表查看时间段明细。空白时段表示没有已记录请求。读取最近 366 个日志文件；本机统计不等于账单。")
                .font(.caption).foregroundStyle(.secondary)
        }
    }
}
