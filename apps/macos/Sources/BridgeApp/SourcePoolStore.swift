import Foundation
import BridgeCore

extension AppStore {
    func pool(_ connectionID: String) -> SourcePool? {
        sourcePools.first { $0.connectionID == connectionID }
    }
    func reloadPools() {
        guard let runtime else { return }
        let file = runtime.root.appendingPathComponent("source-pools.json")
        do {
            sourcePools = FileManager.default.fileExists(atPath: file.path)
                ? try SourcePool.decode(Data(contentsOf: file), sources: sources) : []
            poolError = nil
        } catch { poolError = "来源池配置无法读取：" + error.localizedDescription }
    }
    func savePool(_ candidate: SourcePool) throws {
        guard let runtime, poolError == nil else { throw AppFailure(message: poolError ?? "应用尚未准备好。") }
        let pool = try candidate.validated(sources: sources)
        var next = sourcePools.filter { $0.id != pool.id }; next.append(pool)
        let file = runtime.root.appendingPathComponent("source-pools.json")
        try runtime.saveMetadata("pools", data: SourcePool.encode(next, sources: sources))
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: file.path)
        sourcePools = next
        notice = "来源池已保存，无需重启 Bridge。权重只影响新会话；0 权重保留已有会话。"
    }
    func removePool(_ pool: SourcePool) throws {
        guard let runtime else { return }
        guard !clients.contains(where: { $0.source == pool.connectionID }) else {
            throw AppFailure(message: "此来源池仍被 Agent 使用，请先选择其他来源并应用配置。")
        }
        let next = sourcePools.filter { $0.id != pool.id }
        try runtime.saveMetadata("pools", data: SourcePool.encode(next, sources: sources))
        sourcePools = next
    }
}
