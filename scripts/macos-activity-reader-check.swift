import Foundation
import BridgeCore

/// Filesystem contracts for telemetry tailing: exactly-once complete lines,
/// unchanged snapshots, rotation, unknown usage and bounded incremental work.
@main struct ActivityReaderChecks {
    static func main() async throws {
        let fm = FileManager.default
        let root = fm.temporaryDirectory.appendingPathComponent("agentlink-tail-" + UUID().uuidString)
        try fm.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? fm.removeItem(at: root) }
        let file = root.appendingPathComponent("2026-10-31.jsonl")
        var reader = ActivityLogReader()
        let empty = await reader.refresh(directory: root)
        precondition(empty?.isEmpty == true)
        let initial = line("one", time: "2026-10-31T12:00:00-07:00")
        try initial.write(to: file)
        var records = await reader.refresh(directory: root)
        precondition(records?.map(\.id) == ["one"])
        let firstDate = records![0].timestamp
        let unchanged = await reader.refresh(directory: root)
        precondition(unchanged == nil)

        let second = line("羊驼", time: "2026-10-31T19:00:00.1234567Z")
        // Split within a multi-byte UTF-8 character, then before the newline.
        let split = second.range(of: Data("羊".utf8))!.lowerBound + 1
        try append(second.prefix(split), to: file)
        let partial = await reader.refresh(directory: root)
        precondition(partial == nil)
        reader = ActivityLogReader()
        let resumedPartial = await reader.refresh(directory: root)
        precondition(resumedPartial?.map(\.id) == ["one"], "Reopen must retain complete records and a split UTF-8 suffix")
        try append(second[split..<second.count - 1], to: file)
        let unterminated = await reader.refresh(directory: root)
        precondition(unterminated == nil)
        try append(Data([10]), to: file)
        records = await reader.refresh(directory: root)
        precondition(records?.map(\.id) == ["羊驼", "one"])
        precondition(abs(records![0].timestamp.timeIntervalSince(firstDate) - 0.123) < 0.002)
        precondition(UsageTotals(records!).input == 40 && UsageTotals(records!).cached == 8)
        try append(Data("malformed\n{\"id\":\"bad\",\"kind\":\"request\",\"timestamp\":\"not-a-date\"}\n".utf8), to: file)
        let malformed = await reader.refresh(directory: root)
        precondition(malformed == nil)
        try append(line("unknown", usage: ""), to: file)
        records = await reader.refresh(directory: root)
        precondition(records?.count == 3 && UsageTotals(records!).missingUsage == 1)
        precondition(UsageTotals(records!).input == 40)

        // Truncation discards both old records and any buffered partial line.
        try append(Data("{\"id\":\"unfinished".utf8), to: file)
        _ = await reader.refresh(directory: root)
        try line("short").write(to: file)
        records = await reader.refresh(directory: root)
        precondition(records?.map(\.id) == ["short"])
        // In-place replacement with equal length and a changed timestamp.
        let sameSize = line("other")
        try sameSize.write(to: file)
        try fm.setAttributes([.modificationDate: Date().addingTimeInterval(2)], ofItemAtPath: file.path)
        records = await reader.refresh(directory: root)
        precondition(records?.map(\.id) == ["other"])
        // An in-place rewrite can also grow; never keep the replaced prefix.
        try (line("replacement-longer") + line("replacement-second")).write(to: file)
        records = await reader.refresh(directory: root)
        precondition(Set(records!.map(\.id)) == ["replacement-longer", "replacement-second"])
        // Atomic replacement changes inode even with identical length/mtime.
        let oldDate = (try fm.attributesOfItem(atPath: file.path))[.modificationDate] as! Date
        try (line("replacement-foobar") + line("replacement-second")).write(to: file, options: .atomic)
        try fm.setAttributes([.modificationDate: oldDate], ofItemAtPath: file.path)
        records = await reader.refresh(directory: root)
        precondition(Set(records!.map(\.id)) == ["replacement-foobar", "replacement-second"])
        let yesterday = root.appendingPathComponent("2026-10-30.jsonl")
        try line("yesterday").write(to: yesterday)
        records = await reader.refresh(directory: root)
        precondition(records?.count == 3)
        try fm.removeItem(at: file)
        records = await reader.refresh(directory: root)
        precondition(records?.map(\.id) == ["yesterday"])
        try fm.removeItem(at: yesterday)
        records = await reader.refresh(directory: root)
        precondition(records?.isEmpty == true)

        // Production-sized synthetic history. Appending one request must cost a
        // small fraction of an initial scan and must preserve all usage totals.
        var history = Data()
        for index in 0..<20_000 { history.append(line("request-\(index)")) }
        try history.write(to: file)
        let coldStart = Date()
        records = await reader.refresh(directory: root)
        let cold = Date().timeIntervalSince(coldStart)
        precondition(records?.count == 20_000)
        let idleStart = Date()
        for _ in 0..<100 {
            let idle = await reader.refresh(directory: root)
            precondition(idle == nil)
        }
        let idle = Date().timeIntervalSince(idleStart)
        try append(line("last"), to: file)
        let appendStart = Date()
        records = await reader.refresh(directory: root)
        let increment = Date().timeIntervalSince(appendStart)
        precondition(records?.count == 20_001 && UsageTotals(records!).input == 400_020)
        print("20k records: cold=\(cold)s, 100 unchanged polls=\(idle)s, append=\(increment)s")
        precondition(increment < max(0.15, cold / 5), "One append must not re-decode a whole daily log")
        precondition(idle < max(0.5, cold), "Unchanged polls must not repeatedly scan history")

        let indexFile = root.appendingPathComponent(".agentlink-index.sqlite")
        precondition(fm.fileExists(atPath: indexFile.path), "Activity checkpoints must persist locally")
        reader = ActivityLogReader()
        let reopenStart = Date()
        records = await reader.refresh(directory: root)
        let reopen = Date().timeIntervalSince(reopenStart)
        precondition(records?.count == 20_001 && UsageTotals(records!).output == 160_008)
        print("SQLite reopen=\(reopen)s")
        precondition(reopen < max(0.25, cold / 3), "Reopen must use the index instead of parsing all source logs")
        reader = ActivityLogReader()
        try Data("corrupt disposable index".utf8).write(to: indexFile, options: .atomic)
        records = await reader.refresh(directory: root)
        precondition(records?.count == 20_001 && UsageTotals(records!).input == 400_020,
                     "A corrupt index must rebuild without losing or duplicating usage")

        // A deleted/rotated daily file must not hold the 366-file window open.
        try fm.removeItem(at: file)
        for index in 0..<367 {
            let daily = root.appendingPathComponent(String(format: "%04d.jsonl", index))
            try line("day-\(index)").write(to: daily)
        }
        records = await reader.refresh(directory: root)
        precondition(records?.count == 366 && !records!.contains { $0.id == "day-0" })
        print("Activity log reader contracts passed")
    }

    static func line(_ id: String, time: String = "2026-10-31T19:00:00Z",
                     usage: String = ",\"input_tokens\":20,\"output_tokens\":8,\"cache_read_tokens\":4") -> Data {
        Data("{\"id\":\"\(id)\",\"kind\":\"request\",\"timestamp\":\"\(time)\"\(usage)}\n".utf8)
    }
    static func append(_ data: Data, to file: URL) throws {
        let handle = try FileHandle(forWritingTo: file)
        defer { try? handle.close() }
        try handle.seekToEnd(); try handle.write(contentsOf: data)
    }
}
