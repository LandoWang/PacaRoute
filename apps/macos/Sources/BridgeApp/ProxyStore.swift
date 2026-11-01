import Foundation
import BridgeCore

extension AppStore {
    func saveProxies(_ candidate: ProxyConfiguration) throws {
        guard let runtime, !signingIn, busy == nil else {
            throw AppFailure(message: "请等待账号操作完成，再修改 Proxy。")
        }
        var value = try candidate.validated()
        value.bindings = value.bindings.filter { binding in binding.key == "copilot" || sources.contains(where: { $0.id == binding.key }) }
        let file = runtime.root.appendingPathComponent("proxies.json")
        try runtime.saveMetadata("proxies", data: value.encoded())
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: file.path)
        proxies = value
    }
    func bindProxy(_ id: String, source: String) {
        do { var next = proxies; next.bindings[source] = id; try saveProxies(next) }
        catch { showError(error) }
    }
    func proxyName(for source: String) -> String {
        proxies.choices.first { $0.id == proxies.sourceProxy(source) }?.name ?? "未知代理"
    }
}

