import Foundation
import BridgeCore

func checkAgentLinkContracts() throws {
    // Existing sources preserve their previous transport; new sources use system.
    let migrated = ProxyConfiguration.migrated(mode: .custom, address: "http://127.0.0.1:9999", sourceIDs: ["work", "azure"])
    let restored = try ProxyConfiguration.decode(migrated.encoded())
    expectEqual(restored.sourceProxy("work"), "imported-proxy")
    expectEqual(restored.sourceProxy("copilot"), "imported-proxy")
    expectEqual(restored.sourceProxy("new-source"), "system")
    expectEqual(restored.accountProxyId, "imported-proxy")
    var broken = restored; broken.profiles = []
    do { _ = try broken.encoded(); fatalError("Dangling proxy reference accepted") } catch ProxyFailure.invalid { }
    let direct = ProxyConfiguration.migrated(mode: .direct, address: "", sourceIDs: ["work"])
    expectEqual(direct.sourceProxy("work"), "direct")

    // Half-open intervals: no end-boundary count, no estimate/error double count.
    let input = """
    {"id":"a","timestamp":"2026-10-28T00:00:00Z","kind":"request","source":"work","input_tokens":100,"output_tokens":10,"status":200}
    {"id":"b","timestamp":"2026-10-28T01:00:00Z","kind":"request","source":"home","input_tokens":20,"output_tokens":2,"status":200}
    {"id":"c","timestamp":"2026-10-28T02:00:00Z","kind":"request","source":"work","status":500}
    {"id":"d","timestamp":"2026-10-28T02:00:00Z","kind":"error","source":"work","status":500}
    {"id":"e","timestamp":"2026-10-28T03:00:00Z","kind":"request","source":"work","input_tokens":999,"status":200}
    {"id":"f","timestamp":"2026-10-28T00:30:00Z","kind":"estimate","source":"work","input_tokens":999}

    """
    let formatter = ISO8601DateFormatter()
    var cal = Calendar(identifier: .gregorian); cal.timeZone = TimeZone(secondsFromGMT: 0)!
    let records = ActivityReader.decode(Data(input.utf8))
    let start = formatter.date(from: "2026-10-28T00:00:00Z")!, end = formatter.date(from: "2026-10-28T03:00:00Z")!
    let all = UsageTimeline.buckets(records, start: start, end: end, hourly: true, calendar: cal)
    let work = UsageTimeline.buckets(records, start: start, end: end, hourly: true, source: "work", calendar: cal)
    expectEqual(all.map { $0.totals.requests }, [1, 1, 1])
    expectEqual(work.map { $0.totals.requests }, [1, 0, 1])
    expectEqual(work.map { $0.totals.input }, [100, 0, 0])
    expectEqual(work[2].totals.failures, 1); expectEqual(work[2].totals.missingUsage, 1)
    cal.timeZone = TimeZone(identifier: "America/Los_Angeles")!
    let dstStart = formatter.date(from: "2026-11-01T07:00:00Z")!, dstEnd = formatter.date(from: "2026-11-02T08:00:00Z")!
    let dst = UsageTimeline.buckets([], start: dstStart, end: dstEnd, hourly: true, calendar: cal)
    expectEqual(dst.count, 25); expectEqual(Set(dst.map(\.id)).count, 25)
    print("Passed AgentLink migration, binding, time interval, source filter and DST contracts")
}
