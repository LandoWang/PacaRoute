import Foundation
import BridgeCore

@main struct SourcePoolChecks {
    @MainActor static func main() throws {
        let fm = FileManager.default
        let root = fm.temporaryDirectory.appendingPathComponent("agentlink-pool-check-" + UUID().uuidString)
        try fm.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? fm.removeItem(at: root) }
        let resources = root.appendingPathComponent("resources"), binaries = root.appendingPathComponent("resources/bridge")
        try fm.createDirectory(at: binaries, withIntermediateDirectories: true)
        for name in ["copilot-bridge", "copilot-updater", "appsettings.json"] {
            let file = binaries.appendingPathComponent(name)
            try Data("{}".utf8).write(to: file); try fm.setAttributes([.posixPermissions: 0o700], ofItemAtPath: file.path)
        }
        let suite = "agentlink-pool-test-" + UUID().uuidString
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let store = AppStore(defaults: defaults) { try Runtime(root: root.appendingPathComponent("state"), resources: resources) }
        defer { store.shutdown() }
        let azure = ModelSource(id: "azure-one", name: "Azure", endpoint: "https://example.openai.azure.com", deployments: ["deployment-one"])
        try store.saveSource(azure, key: "")
        store.codexSource = azure.id; store.selectedCodex = "deployment-one"
        var pool = SourcePool(id: "gpt", name: "Balanced GPT", agent: "codex", model: "gpt-6-astra", members: [
            SourcePoolMember(id: "azure", sourceId: azure.id, model: "deployment-one", weight: 60),
            SourcePoolMember(id: "github", sourceId: "copilot", model: "gpt-6-astra", weight: 40)])
        store.serviceState = "running"
        try store.savePool(pool)
        precondition(store.serviceState == "running" && store.codexSource == azure.id && store.selectedCodex == "deployment-one",
            "Saving a pool must not stop Bridge or change an Agent's selection")
        pool.members[0].weight = 0
        try store.savePool(pool)
        store.reloadPools()
        precondition(store.sourcePools == [pool])
        precondition(store.availableModels(pool.connectionID).first?.id == pool.model)
        precondition(store.availableModels(pool.connectionID).first?.codex == true)
        precondition(store.availableModels(pool.connectionID).first?.claude == false)
        store.serviceState = "stopped"
        do { try store.removeSource(azure); fatalError("Pool member source was removed") } catch { }
        var invalid = pool; invalid.members[1].weight = 0
        do { _ = try invalid.validated(sources: [azure]); fatalError("Zero total weight accepted") } catch { }
        invalid = pool; invalid.members[0].model = "missing-deployment"
        do { _ = try invalid.validated(sources: [azure]); fatalError("Unknown deployment accepted") } catch { }
        let wire = String(decoding: try SourcePool.encode([pool], sources: [azure]), as: UTF8.self)
        precondition(wire.contains("source_id") && wire.contains("failover"))
        store.clients = try Reply.decode(Data("{\"ok\":true,\"event\":\"result\",\"clients\":[{\"id\":\"codex\",\"path\":\"fixture\",\"connected\":true,\"drifted\":false,\"source\":\"pool:gpt\"}]}".utf8)).clients!
        do { try store.removePool(pool); fatalError("An Agent's active pool was removed") } catch { }
        print("PASS: source pool wire format, membership, Agent/model selection, live weight edits and deletion guards")
    }
}
