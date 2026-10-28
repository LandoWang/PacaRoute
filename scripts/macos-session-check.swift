import Foundation
import BridgeCore
import Darwin

/// Observable desktop contracts: resume saved login/catalogs, retain the last usable
/// catalog on network failure, read each Keychain key once per app session, and apply
/// Agent source/model changes without terminating the actual bundled AOT listener.
@main struct SessionChecks {
    @MainActor static func main() async throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("agentlink-session-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let codexHome = root.appendingPathComponent("codex-home")
        try FileManager.default.createDirectory(at: codexHome, withIntermediateDirectories: true)
        setenv("CODEX_HOME", codexHome.path, 1)
        let suite = "agentlink-session-check." + UUID().uuidString
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        try await checkCatalogs(root: root.appendingPathComponent("catalogs"), defaults: defaults)
        try await checkRunningSwitch(root: root.appendingPathComponent("service"), defaults: defaults,
            resources: URL(fileURLWithPath: CommandLine.arguments[1]), codexHome: codexHome)
        print("PASS: saved login/catalog survive reopening; offline refresh preserves models; logout clears catalog; Keychain read once per session; AOT restarts reuse keys; Agent source/model apply preserves listener PID")
    }

    @MainActor static func checkCatalogs(root: URL, defaults: UserDefaults) async throws {
        let resources = root.appendingPathComponent("fixture-resources")
        let binaries = resources.appendingPathComponent("bridge")
        try FileManager.default.createDirectory(at: binaries, withIntermediateDirectories: true)
        // Contract fixture speaks the same public JSON-lines API as the CLI. Its
        // persistent marker represents the authentication facade's local status.
        let driver = #"""
        #!/bin/sh
        case "$2" in
          status|account-status)
            if [ -f ../signed-out ]; then
              printf '%s\n' '{"ok":true,"event":"result","logged_in":false,"clients":[]}'
            else
              printf '%s\n' '{"ok":true,"event":"result","logged_in":true,"clients":[{"id":"codex","path":"fixture","connected":true,"drifted":false,"source":"copilot","model":"second-model"}]}'
            fi ;;
          models)
            echo model >> ../model-calls
            if [ -f ../offline ]; then
              printf '%s\n' '{"ok":false,"event":"result","error":"Fixture network unavailable"}'
              exit 1
            fi
            printf '%s\n' '{"ok":true,"event":"result","models":[{"id":"first-model","name":"First","claude":true,"codex":true},{"id":"second-model","name":"Second","claude":true,"codex":true}]}' ;;
          logout)
            touch ../signed-out
            printf '%s\n' '{"ok":true,"event":"result","logged_in":false}' ;;
          *) exit 2 ;;
        esac
        """#
        for name in ["copilot-bridge", "copilot-updater"] {
            let file = binaries.appendingPathComponent(name)
            try driver.write(to: file, atomically: true, encoding: .utf8)
            try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: file.path)
        }
        try Data("{}".utf8).write(to: binaries.appendingPathComponent("appsettings.json"))
        var store: AppStore? = AppStore(defaults: defaults) { try Runtime(root: root, resources: resources) }
        await store!.refresh()
        try require(store!.loggedIn && store!.models.count == 2, "Saved login must restore without a login command")
        try require(store!.selectedCodex == "second-model", "Loading a catalog must not overwrite the configured model")
        store!.shutdown(); store = nil
        store = AppStore(defaults: defaults) { try Runtime(root: root, resources: resources) }
        try require(store!.models.count == 2, "Reopening must immediately restore picker metadata")
        await store!.refresh()
        try require(store!.loggedIn)
        let calls = try String(contentsOf: root.appendingPathComponent("model-calls"), encoding: .utf8)
        try require(calls.split(separator: "\n").count == 1, "A fresh catalog must not require another token exchange on reopen")
        try Data().write(to: root.appendingPathComponent("offline"))
        await store!.refreshModels()
        try require(store!.loggedIn && store!.models.count == 2 && store!.modelErrors["copilot"] != nil,
                     "An unavailable catalog endpoint must not require login or discard usable models")
        await store!.signOutAccount(.copilot)
        try require(!store!.loggedIn && store!.models.isEmpty)
        store!.shutdown(); store = nil
        store = AppStore(defaults: defaults) { try Runtime(root: root, resources: resources) }
        await store!.refresh()
        try require(!store!.loggedIn && store!.models.isEmpty, "Catalog metadata must never resurrect signed-out authentication")
        store!.shutdown(); store = nil
    }

    @MainActor static func checkRunningSwitch(root: URL, defaults: UserDefaults, resources: URL, codexHome: URL) async throws {
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        let sources = ["azure-one", "azure-two"].map { id in
            ModelSource(id: id, name: id, endpoint: "https://example.openai.azure.com", authMode: "api-key",
                        deployments: [id + "-model", id + "-alternate"], accountId: "azure-local")
        }
        try ModelSource.encode(sources).write(to: root.appendingPathComponent("sources.json"))
        var reads: [String: Int] = [:]
        var vault = ["azure-one": "fixture-key-one", "azure-two": "fixture-key-two"]
        let credentials = SourceCredentialCache(read: { id in reads[id, default: 0] += 1; return vault[id]! },
            save: { value, id in vault[id] = value }, delete: { id in vault[id] = nil })
        defaults.set(try freePort(), forKey: "port")
        let store = AppStore(defaults: defaults, credentials: credentials) { try Runtime(root: root, resources: resources) }
        do {
            store.startService(); try await wait { store.running && store.busy == nil }
            try require(reads == ["azure-one": 1, "azure-two": 1])
            store.stopService(); try await wait { store.serviceState == "stopped" }
            store.startService(); try await wait { store.running && store.busy == nil }
            try require(reads == ["azure-one": 1, "azure-two": 1], "Restarting Bridge must not re-read Keychain")
            let pid = try listenerPID(store.appliedPort)
            for source in sources {
                store.selectSource(source.id, client: "codex")
                try require(store.selectedCodex == source.deployments[0])
                await store.prepareConfig("codex")
                guard let plan = store.preview else { throw AppFailure(message: store.notice ?? "No client preview") }
                await store.applyConfig(plan)
                let currentPID = try listenerPID(store.appliedPort)
                try require(store.running && currentPID == pid,
                             "Applying an Agent source/model must preserve the listener process")
                let config = try String(contentsOf: codexHome.appendingPathComponent("config.toml"), encoding: .utf8)
                try require(config.contains("/sources/" + source.id + "/codex") && config.contains(source.deployments[0]))
            }
            store.selectedCodex = sources[1].deployments[1]
            await store.prepareConfig("codex")
            guard let modelPlan = store.preview else { throw AppFailure(message: "Missing model-only preview") }
            await store.applyConfig(modelPlan)
            let afterModelChange = try listenerPID(store.appliedPort)
            try require(afterModelChange == pid, "A model-only change must not restart Bridge")
            store.selectSource(sources[0].id, client: "codex")
            store.selectSource(sources[1].id, client: "codex")
            try require(store.selectedCodex == sources[1].deployments[1], "Returning to a source must remember its selected model")
            store.stopService(); try await wait { store.serviceState == "stopped" }
            try store.saveSource(sources[0], key: "rotated-fixture-key")
            let rotated = try credentials.read("azure-one")
            try require(rotated == "rotated-fixture-key", "Saved key must replace the session value")
            store.startService(); try await wait { store.running && store.busy == nil }
            try require(reads == ["azure-one": 1, "azure-two": 1])
            store.shutdown(); try await wait { !store.hasRunningChildren }
            _ = try credentials.read("azure-one")
            try require(reads["azure-one"] == 2, "App shutdown must clear session credentials")
        } catch {
            store.shutdown(); try? await wait { !store.hasRunningChildren }; throw error
        }
    }

    static func require(_ value: Bool, _ message: String = "Desktop session contract failed") throws {
        if !value { throw AppFailure(message: message) }
    }

    @MainActor static func wait(_ condition: () -> Bool) async throws {
        for _ in 0..<300 { if condition() { return }; try await Task.sleep(nanoseconds: 100_000_000) }
        throw AppFailure(message: "Timed out waiting for fixture child")
    }
    static func freePort() throws -> Int {
        let fd = socket(AF_INET, SOCK_STREAM, 0); defer { close(fd) }
        var address = sockaddr_in(); address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET); address.sin_addr.s_addr = inet_addr("127.0.0.1")
        let result = withUnsafePointer(to: &address) { $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) } }
        guard result == 0 else { throw AppFailure(message: "Cannot reserve fixture port") }
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        _ = withUnsafeMutablePointer(to: &address) { $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { getsockname(fd, $0, &length) } }
        return Int(UInt16(bigEndian: address.sin_port))
    }
    static func listenerPID(_ port: Int) throws -> String {
        let p = Process(), out = Pipe(); p.executableURL = URL(fileURLWithPath: "/usr/sbin/lsof")
        p.arguments = ["-t", "-iTCP:\(port)", "-sTCP:LISTEN"]; p.standardOutput = out
        try p.run(); let data = out.fileHandleForReading.readDataToEndOfFile(); p.waitUntilExit()
        let pid = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        guard p.terminationStatus == 0, !pid.isEmpty else { throw AppFailure(message: "Missing fixture listener") }
        return pid
    }
}
