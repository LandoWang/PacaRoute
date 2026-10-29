import Foundation

public enum SessionAgent: String, CaseIterable, Identifiable, Sendable {
    case codex = "Codex", claude = "Claude Code"
    public var id: String { rawValue }
}

public struct LocalSession: Identifiable, Sendable {
    public var id: String { agent.rawValue + ":" + key }
    public let key: String
    public let agent: SessionAgent
    public let title: String
    public let cwd: String
    public let provider: String?
    public let model: String?
    public let updated: Date
    public let archived: Bool
    public let file: URL?
    public var tokens: Int64? = nil
    public var providerLabel: String {
        guard let provider, !provider.isEmpty else { return "来源未记录" }
        return provider == "copilot-bridge" ? "Bridge（上游来源未记录）" : provider
    }
}

public struct SessionItem: Identifiable, Sendable, Equatable {
    public enum Kind: String, Sendable { case user, assistant, tool, result, error, context }
    public let id: String
    public let kind: Kind
    public let title: String
    public let text: String
    public let timestamp: Date?
    public let model: String?
    public init(id: String, kind: Kind, title: String, text: String, timestamp: Date? = nil, model: String? = nil) {
        self.id = id; self.kind = kind; self.title = title
        self.text = SessionText.clean(text); self.timestamp = timestamp; self.model = model
    }
}

public enum SessionCursor: Sendable, Equatable { case database(Int64), file(Int64) }

public struct SessionPage: Sendable {
    public let items: [SessionItem]
    public let olderCursor: SessionCursor?
    public let notice: String?
    public let recordedStatus: String?
    public init(items: [SessionItem], olderCursor: SessionCursor? = nil, notice: String? = nil, recordedStatus: String? = nil) {
        self.items = items; self.olderCursor = olderCursor; self.notice = notice; self.recordedStatus = recordedStatus
    }
}

public struct SessionCatalog: Sendable {
    public let sessions: [LocalSession]
    public let hasMore: Bool
    public let notice: String?
}

public struct SessionReadError: LocalizedError {
    public let message: String
    public var errorDescription: String? { message }
    init(_ message: String) { self.message = message }
}

public enum SessionText {
    // Best-effort redaction, not a guarantee that arbitrary transcript secrets are detectable.
    private static let patterns = [
        #"(?i)\bBearer\s+[^\s\"'<>]+"#,
        #"\b(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|sk-[A-Za-z0-9_-]{8,})"#,
        #"(?i)([\"']?(?:api[-_]?key|access[-_]?token|refresh[-_]?token|authorization|password|client_secret)[\"']?\s*[:=]\s*)[\"']?[^\s\"',;}]+[\"']?"#,
        #"(?i)(https?://)[^/@\s]+:[^/@\s]+@"#,
        #"(?s)-----BEGIN (?:[A-Z ]*PRIVATE KEY)-----.*?-----END (?:[A-Z ]*PRIVATE KEY)-----"#,
    ].compactMap { try? NSRegularExpression(pattern: $0) }
    public static func clean(_ input: String, limit: Int = 16_000) -> String {
        var value = input
        for (index, regex) in patterns.enumerated() {
            value = regex.stringByReplacingMatches(in: value, range: NSRange(value.startIndex..., in: value),
                withTemplate: index == 2 ? "$1[redacted]" : index == 3 ? "$1[redacted]@" : "[redacted]")
        }
        return value.count > limit ? String(value.prefix(limit)) + "\n[内容已截短]" : value
    }
    static func date(_ string: String?) -> Date? {
        guard let string else { return nil }
        let f = ISO8601DateFormatter(); f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = f.date(from: string) { return date }
        f.formatOptions = [.withInternetDateTime]; return f.date(from: string)
    }
    static func json(_ data: Data) -> [String: Any]? { (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] }
    static func jsonText(_ value: Any?) -> String {
        guard let value else { return "" }
        if let value = value as? String { return value }
        guard JSONSerialization.isValidJSONObject(value), let data = try? JSONSerialization.data(withJSONObject: value, options: [.sortedKeys, .prettyPrinted]) else { return "" }
        return String(decoding: data, as: UTF8.self)
    }
    static func content(_ value: Any?) -> String {
        if let text = value as? String { return text }
        guard let blocks = value as? [[String: Any]] else { return "" }
        return blocks.compactMap { block -> String? in
            switch block["type"] as? String {
            case "text", "input_text", "output_text": return block["text"] as? String
            case "tool_use": return "工具：\(block["name"] as? String ?? "tool")\n\(jsonText(block["input"]))"
            case "tool_result": return "工具结果：\(content(block["content"]))"
            case "image", "input_image", "localImage": return "[图片附件]"
            default: return nil
            }
        }.joined(separator: "\n\n")
    }
}
