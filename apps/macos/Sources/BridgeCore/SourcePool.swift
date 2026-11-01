import Foundation

public struct SourcePoolMember: Codable, Identifiable, Equatable {
    public var id: String
    public var sourceId: String
    public var model: String
    public var weight: Int
    public init(id: String, sourceId: String, model: String, weight: Int = 1) {
        self.id = id; self.sourceId = sourceId; self.model = model; self.weight = weight
    }
}

public struct SourcePool: Codable, Identifiable, Equatable {
    public var id: String
    public var name: String
    public var agent: String
    public var model: String
    public var failover: Bool
    public var members: [SourcePoolMember]
    public var connectionID: String { "pool:" + id }
    public var agentName: String { agent == "codex" ? "Codex" : "Claude Code" }
    public init(id: String, name: String, agent: String, model: String, failover: Bool = true, members: [SourcePoolMember]) {
        self.id = id; self.name = name; self.agent = agent; self.model = model; self.failover = failover; self.members = members
    }
    public func validated(sources: [ModelSource]) throws -> SourcePool {
        func identifier(_ text: String) -> Bool { text.range(of: "^[a-z][a-z0-9-]{0,62}$", options: .regularExpression) != nil }
        func modelID(_ text: String) -> Bool { text.range(of: "^[A-Za-z0-9][A-Za-z0-9._-]{0,159}$", options: .regularExpression) != nil }
        guard identifier(id), !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, name.count <= 100,
              ["codex", "claude-code"].contains(agent), modelID(model), (1...32).contains(members.count),
              Set(members.map(\.id)).count == members.count, members.contains(where: { $0.weight > 0 }) else {
            throw SourcePoolFailure("填写来源池名称、Agent、模型及至少一个有权重的成员。")
        }
        for member in members {
            guard identifier(member.id), identifier(member.sourceId), modelID(member.model), (0...1_000_000).contains(member.weight) else {
                throw SourcePoolFailure("每个成员需要有效的来源、模型及 0–1000000 的权重。")
            }
            if member.sourceId == "copilot" { continue }
            guard let source = sources.first(where: { $0.id == member.sourceId }),
                  source.kind == "copilot" || source.deployments.contains(member.model) else {
                throw SourcePoolFailure("成员 \(member.id) 的来源或模型已不存在，请重新选择。")
            }
        }
        return self
    }
    public static func decode(_ data: Data, sources: [ModelSource]) throws -> [SourcePool] {
        let decoder = JSONDecoder(); decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try validate(decoder.decode([SourcePool].self, from: data), sources: sources)
    }
    public static func encode(_ pools: [SourcePool], sources: [ModelSource]) throws -> Data {
        let encoder = JSONEncoder(); encoder.keyEncodingStrategy = .convertToSnakeCase
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return try encoder.encode(validate(pools, sources: sources))
    }
    private static func validate(_ pools: [SourcePool], sources: [ModelSource]) throws -> [SourcePool] {
        guard pools.count <= 128, Set(pools.map(\.id)).count == pools.count else { throw SourcePoolFailure("来源池 ID 必须唯一。") }
        return try pools.map { try $0.validated(sources: sources) }
    }
}
public struct SourcePoolFailure: LocalizedError {
    public let message: String
    public init(_ message: String) { self.message = message }
    public var errorDescription: String? { message }
}
