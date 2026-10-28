import Foundation

public struct ProxyProfile: Codable, Identifiable, Equatable {
    public var id: String
    public var name: String
    public var mode: String
    public var address: String?
    public init(id: String, name: String, mode: String, address: String? = nil) {
        self.id = id; self.name = name; self.mode = mode; self.address = address
    }
    public static let builtins = [ProxyProfile(id: "system", name: "跟随系统", mode: "system"),
                                  ProxyProfile(id: "direct", name: "直连 · 不使用代理", mode: "direct")]
    public func validated() throws -> ProxyProfile {
        guard id.range(of: "^[a-z][a-z0-9-]{0,62}$", options: .regularExpression) != nil,
              !["system", "direct"].contains(id), !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              name.count <= 100, let mode = ProxyMode(rawValue: mode), mode.validate(address ?? "") else { throw ProxyFailure.invalid }
        return self
    }
}
public struct ProxyConfiguration: Codable, Equatable {
    public var profiles: [ProxyProfile] = []
    public var bindings: [String: String] = [:]
    public var accountProxyId = "system"
    public init() {}
    public var choices: [ProxyProfile] { ProxyProfile.builtins + profiles }
    public func sourceProxy(_ id: String) -> String { bindings[id] ?? "system" }
    public func validated() throws -> ProxyConfiguration {
        _ = try profiles.map { try $0.validated() }
        let ids = Set(choices.map(\.id))
        guard ids.count == choices.count, ids.contains(accountProxyId), bindings.values.allSatisfy(ids.contains) else { throw ProxyFailure.invalid }
        return self
    }
    public static func migrated(mode: ProxyMode, address: String, sourceIDs: [String]) -> ProxyConfiguration {
        var result = ProxyConfiguration()
        guard mode != .system else { return result }
        let id = mode == .direct ? "direct" : "imported-proxy"
        if mode == .custom { result.profiles = [ProxyProfile(id: id, name: "原有代理", mode: "custom", address: address)] }
        result.bindings = Dictionary(uniqueKeysWithValues: Set(sourceIDs + ["copilot"]).map { ($0, id) })
        result.accountProxyId = id
        return result
    }
    public static func decode(_ data: Data) throws -> ProxyConfiguration {
        let decoder = JSONDecoder(); decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(Self.self, from: data).validated()
    }
    public func encoded() throws -> Data {
        let encoder = JSONEncoder(); encoder.keyEncodingStrategy = .convertToSnakeCase
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return try encoder.encode(validated())
    }
}
public enum ProxyFailure: LocalizedError {
    case invalid
    public var errorDescription: String? { "代理名称、地址或来源绑定无效；请检查 Proxy 配置。" }
}
