import Foundation

public struct Reply: Decodable {
    public let ok: Bool
    public let event: String
    public let error: String?
    public let loggedIn: Bool?
    public let login: String?
    public let userCode: String?
    public let verificationUri: String?
    public let expiresIn: Double?
    public let clients: [ClientState]?
    public let models: [BridgeModel]?
    public let quota: AccountQuota?
    public let quotaError: String?
    public let path: String?
    public let fingerprint: String?
    public let changes: [String]?
    public let backupPath: String?
    public let subscriptions: [AzureSubscription]?
    public let resources: [AzureResource]?

    public static func decode(_ data: Data) throws -> Reply {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return try decoder.decode(Reply.self, from: data)
    }
}

public struct ClientState: Decodable, Identifiable {
    public let source: String?
    public let id: String
    public let path: String
    public let connected: Bool
    public let drifted: Bool
    public let model: String?
    public let error: String?
    public var name: String { id == "codex" ? "Codex" : "Claude Code" }
}

public struct BridgeModel: Codable, Identifiable {
    public let id: String
    public let name: String
    public let vendor: String?
    public let claude: Bool
    public let codex: Bool
    public init(id: String, name: String, vendor: String?, claude: Bool, codex: Bool) {
        self.id = id; self.name = name; self.vendor = vendor; self.claude = claude; self.codex = codex
    }
}

public struct AccountQuota: Decodable {
    public let copilotPlan: String?
    public let quotaResetDate: String?
    public let quotaSnapshots: [String: Quota]?
}

public struct Quota: Decodable {
    public let entitlement: Double?
    public let remaining: Double?
    public let quotaRemaining: Double?
    public let percentRemaining: Double?
    public let overageCount: Double?
    public let unlimited: Bool?
    public var available: Double? { quotaRemaining ?? remaining }
}

public struct Activity: Decodable, Identifiable {
    public let source: String?
    public let id: String
    public let timestamp: Date
    public let kind: String
    public let model: String?
    public let client: String?
    public let inputTokens: Int?
    public let outputTokens: Int?
    public let cacheReadTokens: Int?
    public let cacheWriteTokens: Int?
    public let status: Int?
    public let durationMs: Int?
    public let error: String?
    public var failed: Bool { kind == "error" || (status.map { $0 == 0 || $0 >= 400 } ?? false) }
}

public struct UsageTotals {
    public var requests = 0
    public var failures = 0
    public var input = 0
    public var output = 0
    public var cached = 0
    public var missingUsage = 0
    public init(_ activity: [Activity]) {
        for item in activity where item.kind == "request" {
            requests += 1
            if item.failed { failures += 1 }
            input += item.inputTokens ?? 0
            output += item.outputTokens ?? 0
            cached += item.cacheReadTokens ?? 0
            if item.inputTokens == nil || item.outputTokens == nil { missingUsage += 1 }
        }
    }
}

/// Keeps incomplete UTF-8/JSON lines until the next read, including split multibyte text.
public struct JSONLineBuffer {
    private var bytes = Data()
    public init() {}
    init(pendingBytes: Data) { bytes = pendingBytes }
    var pendingBytes: Data { bytes }
    public mutating func append(_ chunk: Data) -> [Data] {
        bytes.append(chunk)
        var result: [Data] = []
        var start = bytes.startIndex
        for index in bytes.indices where bytes[index] == 10 {
            let line = Data(bytes[start..<index])
            start = index + 1
            if !line.isEmpty { result.append(line) }
        }
        // Remove the consumed prefix once: removing it per line copies the
        // remaining file repeatedly and makes a whole-file read quadratic.
        if start > bytes.startIndex { bytes.removeSubrange(..<start) }
        if bytes.count > 1_048_576 { bytes.removeAll() }
        return result
    }
}

public enum ActivityReader {
    public static func decode(_ data: Data) -> [Activity] {
        var lines = JSONLineBuffer()
        let decoder = makeDecoder()
        return lines.append(data).compactMap { try? decoder.decode(Activity.self, from: $0) }
    }

    static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let wholeSeconds = ISO8601DateFormatter()
        wholeSeconds.formatOptions = [.withInternetDateTime]
        decoder.dateDecodingStrategy = .custom { value in
            let string = try value.singleValueContainer().decode(String.self)
            guard let date = fractional.date(from: string) ?? wholeSeconds.date(from: string) else {
                throw CocoaError(.coderReadCorrupt)
            }
            return date
        }
        return decoder
    }
}

public enum ProxyMode: String, CaseIterable, Identifiable {
    case system, direct, custom
    public var id: String { rawValue }
    public var label: String {
        switch self { case .system: return "跟随系统"; case .direct: return "不使用代理"; case .custom: return "自定义代理" }
    }
    public func validate(_ address: String) -> Bool {
        guard self == .custom else { return true }
        guard let u = URLComponents(string: address), ["http", "https", "socks5"].contains(u.scheme ?? ""),
              let host = u.host, !host.isEmpty, u.user == nil, u.password == nil,
              u.query == nil, u.fragment == nil, u.path.isEmpty || u.path == "/" else { return false }
        return u.port.map { (1...65535).contains($0) } ?? true
    }
}


public struct AccountProfile: Codable, Identifiable, Equatable {
    public var id: String
    public var name: String
    public var kind: String
    public var oauthClientId: String?
    public init(id: String, name: String, kind: String) { self.id = id; self.name = name; self.kind = kind }
    public static let copilot = AccountProfile(id: "copilot", name: "GitHub Copilot", kind: "github")
    public static let localAzure = AccountProfile(id: "azure-local", name: "本机 Azure CLI", kind: "azure")
    public var builtin: Bool { id == "copilot" || id == "azure-local" }
    public func validated() throws -> AccountProfile {
        guard id.range(of: "^[a-z][a-z0-9-]{0,62}$", options: .regularExpression) != nil,
              !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, name.count <= 100,
              ["github", "azure", "api-key"].contains(kind) else { throw SourceFailure.invalid }
        return self
    }
}
public struct AzureSubscription: Decodable, Identifiable {
    public let id: String
    public let name: String
    public let login: String
}
public struct AzureResource: Decodable, Identifiable {
    public let id: String
    public let name: String
    public let endpoint: String
}
