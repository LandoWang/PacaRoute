import Foundation

public struct UsageBucket: Identifiable {
    public let start: Date
    public let end: Date
    public let totals: UsageTotals
    public var id: Date { start }
}
public enum UsageTimeline {
    /// Half-open local calendar buckets preserve DST boundaries and empty periods.
    public static func buckets(_ activity: [Activity], start: Date, end: Date,
                               hourly: Bool, source: String = "all", calendar: Calendar = .current) -> [UsageBucket] {
        guard start < end else { return [] }
        let component: Calendar.Component = hourly ? .hour : .day
        var cursor = start
        let requests = activity.filter { $0.kind == "request" && $0.timestamp >= start && $0.timestamp < end
            && (source == "all" || ($0.source ?? "copilot") == source) }
        let grouped = Dictionary(grouping: requests) { calendar.dateInterval(of: component, for: $0.timestamp)!.start }
        var result: [UsageBucket] = []
        while cursor < end {
            guard let interval = calendar.dateInterval(of: component, for: cursor), interval.end > cursor else { break }
            let stop = min(interval.end, end)
            result.append(UsageBucket(start: cursor, end: stop, totals: UsageTotals(grouped[interval.start] ?? [])))
            cursor = stop
        }
        return result
    }
}
