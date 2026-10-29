import Foundation

public struct SessionReader: Sendable {
    public let root: URL
    public let agent: SessionAgent
    public init(root: URL, agent: SessionAgent) { self.root = root; self.agent = agent }

    public func catalog(search: String = "", offset: Int = 0, limit: Int = 80) throws -> SessionCatalog {
        let limit = min(max(limit, 1), 200)
        if agent == .codex, let file = database("state") {
            let db = try ReadOnlyDatabase(file), columns = try db.columns("threads")
            guard ["id", "updated_at", "cwd"].allSatisfy(columns.contains) else { throw SessionReadError("Codex 会话索引格式暂不支持。") }
            func column(_ name: String, _ fallback: String = "NULL") -> String { columns.contains(name) ? name : fallback }
            let title = columns.contains("name") ? "COALESCE(NULLIF(name, ''), \(column("title", "''")))" : column("title", "''")
            let query = """
                SELECT id, \(title), cwd, \(column("model_provider")), \(column("model")), updated_at,
                    \(column("archived", "0")), \(column("rollout_path")), \(column("tokens_used")) FROM threads
                WHERE (? = '' OR instr(lower(\(title)), lower(?)) > 0 OR instr(lower(cwd), lower(?)) > 0)
                ORDER BY updated_at DESC, id LIMIT ? OFFSET ?
                """
            var sessions: [LocalSession] = []
            try db.rows(query, bindings: [search, search, search, String(limit + 1), String(max(offset, 0))]) { row in
                guard let id = row.string(0) else { return }
                let path = row.string(7).map { URL(fileURLWithPath: $0) }
                sessions.append(LocalSession(key: id, agent: .codex,
                    title: SessionText.clean(row.string(1).flatMap { $0.isEmpty ? nil : $0 } ?? "未命名会话", limit: 180),
                    cwd: SessionText.clean(row.string(2) ?? "", limit: 500), provider: row.string(3), model: row.string(4),
                    updated: Date(timeIntervalSince1970: Double(row.int(5))), archived: row.int(6) != 0, file: path,
                    tokens: row.string(8) == nil ? nil : row.int(8)))
            }
            return SessionCatalog(sessions: Array(sessions.prefix(limit)), hasMore: sessions.count > limit, notice: nil)
        }
        return try fileCatalog(search: search, offset: offset, limit: limit)
    }

    public func page(_ session: LocalSession, before: SessionCursor? = nil, limit: Int = 100) throws -> SessionPage {
        let limit = min(max(limit, 1), 200)
        if case .file(let offset) = before { return try filePage(session, before: offset, limit: limit) }
        let ordinal: Int64
        if case .database(let value) = before { ordinal = value } else { ordinal = Int64.max }
        if agent == .codex, let path = database("thread_history") {
            let db = try ReadOnlyDatabase(path)
            let columns = try db.columns("thread_items")
            guard ["thread_id", "rollout_ordinal", "item_json", "item_type"].allSatisfy(columns.contains) else {
                throw SessionReadError("Codex 历史数据库格式暂不支持。")
            }
            var items: [(Int64, SessionItem)] = [], malformed = 0
            let created = columns.contains("created_at_ms") ? "created_at_ms" : "NULL"
            let query = """
                SELECT rollout_ordinal, CASE WHEN length(item_json) <= 2000000 THEN item_json ELSE NULL END, \(created)
                FROM thread_items WHERE thread_id = ? AND rollout_ordinal < ?
                AND item_type IN ('userMessage','agentMessage','commandExecution','mcpToolCall','fileChange','contextCompaction','functionCallOutput','imageView','webSearch')
                ORDER BY rollout_ordinal DESC LIMIT ?
                """
            var scanned: [Int64] = []
            try db.rows(query, bindings: [session.key, String(ordinal), String(limit + 1)]) { row in
                let ordinal = row.int(0); scanned.append(ordinal)
                guard let value = row.string(1), let object = SessionText.json(Data(value.utf8)) else {
                    malformed += 1
                    items.append((ordinal, SessionItem(id: "omitted-\(ordinal)", kind: .context, title: "记录未展开", text: "该记录过大或 JSON 不完整。")))
                    return
                }
                let date = row.string(2) == nil ? nil : Date(timeIntervalSince1970: Double(row.int(2)) / 1000)
                if let item = Self.codexItem(object, date: date, fallbackID: String(ordinal)) { items.append((ordinal, item)) }
            }
            // Some older threads still have only a rollout. Do not mix DB and byte cursors.
            if scanned.isEmpty && before == nil {
                var exists = false
                try db.rows("SELECT 1 FROM thread_items WHERE thread_id = ? LIMIT 1", bindings: [session.key]) { _ in exists = true }
                if !exists { return try filePage(session, before: nil, limit: limit) }
            }
            var status: String?, lastError: String?
            if try Set(["status", "error_json", "rollout_ordinal", "thread_id"]).isSubset(of: db.columns("thread_turns")) {
                try db.rows("SELECT status, error_json FROM thread_turns WHERE thread_id = ? ORDER BY rollout_ordinal DESC LIMIT 1", bindings: [session.key]) { status = $0.string(0); lastError = $0.string(1) }
            }
            var visible = Array(items.prefix(limit).reversed().map(\.1))
            if before == nil, let lastError, lastError != "null" {
                visible.append(SessionItem(id: "last-turn-error", kind: .error, title: "最后一轮错误", text: lastError))
            }
            return SessionPage(items: visible, olderCursor: scanned.count > limit ? .database(scanned[limit - 1]) : nil,
                notice: malformed > 0 ? "部分记录过大或不完整，已标注。" : nil, recordedStatus: status)
        }
        return try filePage(session, before: nil, limit: limit)
    }

    func database(_ prefix: String) -> URL? {
        let files = (try? FileManager.default.contentsOfDirectory(at: root, includingPropertiesForKeys: nil)) ?? []
        return files.compactMap { file -> (Int, URL)? in
            let name = file.lastPathComponent
            guard name.hasPrefix(prefix + "_"), name.hasSuffix(".sqlite"),
                let version = Int(name.dropFirst(prefix.count + 1).dropLast(7)) else { return nil }
            return (version, file)
        }.sorted { $0.0 > $1.0 }.first?.1
    }

    static func codexItem(_ value: [String: Any], date: Date?, fallbackID: String) -> SessionItem? {
        let id = value["id"] as? String ?? fallbackID
        var kind = SessionItem.Kind.context, title = "记录", text = ""
        switch value["type"] as? String {
        case "userMessage": kind = .user; title = "你"; text = SessionText.content(value["content"])
        case "agentMessage": kind = .assistant; title = "Assistant"; text = value["text"] as? String ?? ""
        case "commandExecution":
            let failed = (value["exitCode"] as? Int).map { $0 != 0 } ?? (value["status"] as? String == "failed")
            kind = failed ? .error : .tool; title = failed ? "命令失败" : "命令"
            text = (value["command"] as? String ?? "") + "\n\n" + (value["aggregatedOutput"] as? String ?? "")
        case "mcpToolCall":
            kind = value["status"] as? String == "failed" ? .error : .tool
            title = value["tool"] as? String ?? "工具"
            let result = value["result"] as? [String: Any]
            text = SessionText.jsonText(value["arguments"]) + "\n\n" + SessionText.content(result?["content"])
            if let error = value["error"], !(error is NSNull) { text += "\n" + SessionText.jsonText(error) }
        case "fileChange":
            kind = value["status"] as? String == "failed" ? .error : .tool; title = "文件修改"
            text = (value["changes"] as? [[String: Any]] ?? []).map { ($0["path"] as? String ?? "") + "\n" + ($0["diff"] as? String ?? "") }.joined(separator: "\n")
        case "functionCallOutput": kind = .result; title = "工具结果"; text = SessionText.jsonText(value["output"])
        case "contextCompaction": title = "上下文压缩"; text = "客户端在这里压缩了上下文。历史记录不等于模型此刻的完整上下文。"
        case "imageView": title = "查看图片"; text = value["path"] as? String ?? "[图片附件]"
        case "webSearch": kind = .tool; title = "搜索"; text = value["query"] as? String ?? ""
        default: return nil
        }
        return SessionItem(id: id, kind: kind, title: title, text: text, timestamp: date)
    }
}
