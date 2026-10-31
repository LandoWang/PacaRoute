import Foundation
import Combine
import BridgeCore
@main struct ActivityRefreshChecks {
    @MainActor static func main() async throws {
        let fm = FileManager.default
        let root = fm.temporaryDirectory.appendingPathComponent("agentlink-activity-check-" + UUID().uuidString)
        try fm.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? fm.removeItem(at: root) }
        let resources = root.appendingPathComponent("resources")
        let bridge = resources.appendingPathComponent("bridge")
        try fm.createDirectory(at: bridge, withIntermediateDirectories: true)
        for name in ["copilot-bridge", "copilot-updater", "appsettings.json"] {
            try Data("{}".utf8).write(to: bridge.appendingPathComponent(name))
            try fm.setAttributes([.posixPermissions: 0o700], ofItemAtPath: bridge.appendingPathComponent(name).path)
        }
        let suite = "agentlink-activity-" + UUID().uuidString
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let store = AppStore(defaults: defaults) { try Runtime(root: root.appendingPathComponent("state"), resources: resources) }
        defer { store.shutdown() }
        precondition(store.ready)
        let logs = store.runtime!.directory.appendingPathComponent("desktop-activity")
        try fm.createDirectory(at: logs, withIntermediateDirectories: true)
        let file = logs.appendingPathComponent("2026-10-31.jsonl")
        let line = "{\"id\":\"one\",\"kind\":\"request\",\"timestamp\":\"2026-10-31T19:00:00.1234567Z\",\"input_tokens\":20,\"output_tokens\":8}\n"
        try Data(line.utf8).write(to: file)
        await store.refreshActivity()
        precondition(store.activity.count == 1)
        var publications = 0
        let subscription = store.$activity.dropFirst().sink { _ in publications += 1 }
        for _ in 0..<5 { await store.refreshActivity() }
        print("unchanged_refresh_publications=\(publications)")
        precondition(publications == 0, "Unchanged logs must not republish activity and redraw usage")
        let handle = try FileHandle(forWritingTo: file)
        try handle.seekToEnd()
        try handle.write(contentsOf: Data(line.replacingOccurrences(of: "one", with: "two").utf8))
        try handle.close()
        await withTaskGroup(of: Void.self) { group in
            for _ in 0..<8 { group.addTask { await store.refreshActivity() } }
        }
        precondition(publications == 1 && store.activity.count == 2, "Overlapping refresh triggers publish one update")
        precondition(UsageTotals(store.activity).input == 40)
        subscription.cancel()
        print("AppStore activity refresh contracts passed")
    }
}
