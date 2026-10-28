import Foundation

public struct ModelSource: Codable, Identifiable, Equatable {
    public var id: String
    public var name: String
    public var kind = "azure-openai"
    public var endpoint: String
    public var authMode = "azure-cli"
    public var accountId: String?
    public var subscriptionId: String?
    public var deployments: [String]

    public init(id: String, name: String, endpoint: String, authMode: String = "azure-cli",
                subscriptionId: String? = nil, deployments: [String], accountId: String? = nil, kind: String = "azure-openai") {
        self.id = id; self.name = name; self.endpoint = endpoint; self.authMode = authMode
        self.subscriptionId = subscriptionId; self.deployments = deployments; self.accountId = accountId; self.kind = kind
    }

    public func validated() throws -> ModelSource {
        if let accountId, accountId.range(of: "^[a-z][a-z0-9-]{0,62}$", options: .regularExpression) == nil { throw SourceFailure.invalid }
        if kind == "copilot" {
            guard id.range(of: "^[a-z][a-z0-9-]{0,62}$", options: .regularExpression) != nil, id != "copilot",
                  !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, name.count <= 100,
                  accountId != nil else { throw SourceFailure.invalid }
            return self
        }
        guard id.range(of: "^[a-z][a-z0-9-]{0,62}$", options: .regularExpression) != nil, id != "copilot",
              !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, name.count <= 100,
              kind == "azure-openai", ["azure-cli", "api-key"].contains(authMode),
              let url = URLComponents(string: endpoint), url.scheme == "https", url.host?.isEmpty == false,
              url.user == nil, url.password == nil, url.query == nil, url.fragment == nil,
              url.port == nil || url.port == 443, ["", "/", "/openai/v1", "/openai/v1/"].contains(url.path),
              !deployments.isEmpty, Set(deployments).count == deployments.count,
              deployments.allSatisfy({ $0.range(of: "^[A-Za-z0-9][A-Za-z0-9._-]{0,159}$", options: .regularExpression) != nil }),
              subscriptionId == nil || subscriptionId == "" || UUID(uuidString: subscriptionId!) != nil else {
            throw SourceFailure.invalid
        }
        var result = self
        var base = url; base.path = "/openai/v1"; base.port = nil
        result.endpoint = base.string!
        return result
    }

    public static func decode(_ data: Data) throws -> [ModelSource] {
        let decoder = JSONDecoder(); decoder.keyDecodingStrategy = .convertFromSnakeCase
        let sources = try decoder.decode([ModelSource].self, from: data).map { try $0.validated() }
        guard Set(sources.map(\.id)).count == sources.count else { throw SourceFailure.invalid }
        return sources
    }
    public static func encode(_ sources: [ModelSource]) throws -> Data {
        let encoder = JSONEncoder(); encoder.keyEncodingStrategy = .convertToSnakeCase
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return try encoder.encode(sources.map { try $0.validated() })
    }
}

public enum SourceFailure: LocalizedError {
    case invalid
    public var errorDescription: String? { "请填写有效的来源 ID、HTTPS 资源地址及 deployment 名称。订阅 ID 需为 UUID。" }
}
