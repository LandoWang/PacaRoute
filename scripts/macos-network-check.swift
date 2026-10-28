import AppKit
import Foundation
import BridgeCore
import Darwin

/// Contract: editing a port then pressing Start must bind and persist that port.
/// An occupied destination must not stop a working service or overwrite its settings.
/// This runs the actual AppStore and bundled AOT server with disposable state only.
@main struct NetworkChecks {
    @MainActor static func main() async throws {
        guard CommandLine.arguments.count == 2 else { throw AppFailure(message: "Pass the bundled app's Contents/Resources directory") }
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("bridge-network-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        let codexHome = root.appendingPathComponent("codex-home")
        try FileManager.default.createDirectory(at: codexHome, withIntermediateDirectories: true)
        setenv("CODEX_HOME", codexHome.path, 1)
        let suite = "com.lando.bridge-network-check." + UUID().uuidString
        let defaults = UserDefaults(suiteName: suite)!
        let oldPort = try freePort(), newPort = try freePort(), restartedPort = try freePort()
        defaults.set(oldPort, forKey: "port")
        let store = AppStore(defaults: defaults) {
            try Runtime(root: root, resources: URL(fileURLWithPath: CommandLine.arguments[1]))
        }
        do {
            try expect(store.ready && !store.hasAccount && !store.hasSource, "Fresh guide must begin with an account")
            try store.saveAccount(AccountProfile(id: "github-pending", name: "Pending", kind: "github"))
            try expect(store.hasAccount && !store.hasSource && !store.canStart && store.sources.isEmpty,
                "A pending GitHub device login must not create a usable source")
            try store.saveSource(ModelSource(id: "azure-fixture", name: "Fixture", endpoint: "https://example.openai.azure.com",
                deployments: ["fixture-deployment"], accountId: "azure-local"), key: "")
            try expect(store.hasSource && store.canStart, "Configured Azure resources enable the source step")
            store.portText = String(newPort); store.proxyMode = .direct
            store.startService()
            try await wait { store.running && store.busy == nil }
            try expect(store.appliedPort == newPort && defaults.integer(forKey: "port") == newPort,
                "Start ignored the edited port or failed to persist it")
            try await assertServer(newPort)
            try expect(!listening(oldPort), "Start unexpectedly used the old saved port")

            store.portText = String(restartedPort)
            await store.saveNetwork()
            try expect(store.running && store.appliedPort == restartedPort, "Restart did not apply the new port")
            try await assertServer(restartedPort)
            try expect(!listening(newPort), "Restart left the old owned listener running")

            let (occupiedFD, occupiedPort) = try reservedPort()
            store.portText = String(occupiedPort)
            await store.saveNetwork()
            try expect(store.running && store.appliedPort == restartedPort && defaults.integer(forKey: "port") == restartedPort,
                "An occupied target changed or stopped the working configuration")
            try expect(store.notice?.contains("已被占用") == true, "Port conflict must be reported")
            try await assertServer(restartedPort)
            Darwin.close(occupiedFD)

            store.stopService()
            try await wait { store.serviceState == "stopped" }
            let sourceFile = root.appendingPathComponent("sources.json")
            let sources = try Data(contentsOf: sourceFile)
            // Startup rejection is local and deterministic; no upstream or credentials.
            try Data("[] invalid JSON".utf8).write(to: sourceFile)
            store.portText = String(newPort); store.notice = nil
            await store.saveNetwork(startIfStopped: true)
            try expect(store.serviceState == "stopped" && store.notice != nil && store.notice?.contains("已运行于") != true,
                "A failed startup was reported as success")
            try sources.write(to: sourceFile)
            store.shutdown()
            try await wait { !store.hasRunningChildren }
            defaults.removePersistentDomain(forName: suite)
            try FileManager.default.removeItem(at: root)
            print("PASS: direct Start applies edited port; AOT listener and saved settings agree; restart moves listener; conflict preserves service; startup failure stays an error; account precedes source")
        } catch {
            store.shutdown()
            try? await wait { !store.hasRunningChildren }
            defaults.removePersistentDomain(forName: suite)
            try? FileManager.default.removeItem(at: root)
            throw error
        }
    }

    @MainActor static func wait(_ condition: () -> Bool) async throws {
        for _ in 0..<300 {
            if condition() { return }
            try await Task.sleep(nanoseconds: 100_000_000)
        }
        throw AppFailure(message: "Timed out waiting for the owned service")
    }
    static func expect(_ value: Bool, _ message: String) throws { if !value { throw AppFailure(message: message) } }
    static func freePort() throws -> Int { let (fd, port) = try reservedPort(); Darwin.close(fd); return port }
    static func reservedPort() throws -> (Int32, Int) {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size); address.sin_family = sa_family_t(AF_INET)
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        let result = withUnsafePointer(to: &address) { $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) } }
        guard fd >= 0, result == 0, listen(fd, 1) == 0 else { Darwin.close(fd); throw AppFailure(message: "Cannot reserve fixture port") }
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        _ = withUnsafeMutablePointer(to: &address) { $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { getsockname(fd, $0, &length) } }
        return (fd, Int(UInt16(bigEndian: address.sin_port)))
    }
    static func listening(_ port: Int) -> Bool {
        let fd = socket(AF_INET, SOCK_STREAM, 0); defer { Darwin.close(fd) }
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size); address.sin_family = sa_family_t(AF_INET)
        address.sin_port = UInt16(port).bigEndian; address.sin_addr.s_addr = inet_addr("127.0.0.1")
        return withUnsafePointer(to: &address) { $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { connect(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) == 0 } }
    }
    static func assertServer(_ port: Int) async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.connectionProxyDictionary = [:]
        let session = URLSession(configuration: configuration); defer { session.invalidateAndCancel() }
        let (data, response) = try await session.data(from: URL(string: "http://127.0.0.1:\(port)/sources/azure-fixture/cc/v1/models")!)
        try expect((response as? HTTPURLResponse)?.statusCode == 200 && String(decoding: data, as: UTF8.self).contains("fixture-deployment"), "Actual AOT server is not listening on the selected port")
    }
}
