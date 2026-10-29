import Foundation

extension SessionReader {
    func fileCatalog(search: String, offset: Int, limit: Int) throws -> SessionCatalog {
        let fm = FileManager.default
        guard fm.fileExists(atPath: root.path) else { return SessionCatalog(sessions: [], hasMore: false, notice: "尚未找到 \(agent.rawValue) 的本地目录。") }
        let directories = agent == .claude ? ["projects"] : ["sessions", "archived_sessions"]
        var files: [(URL, Date)] = []; var capped = false
        for dir in directories {
            guard let walker = fm.enumerator(at: root.appendingPathComponent(dir), includingPropertiesForKeys: [.contentModificationDateKey, .isRegularFileKey, .isSymbolicLinkKey], options: [.skipsHiddenFiles]) else { continue }
            for case let file as URL in walker {
                if Task.isCancelled { throw CancellationError() }
                guard file.pathExtension == "jsonl" else { continue }
                let values = try file.resourceValues(forKeys: [.contentModificationDateKey, .isRegularFileKey, .isSymbolicLinkKey])
                guard values.isRegularFile == true, values.isSymbolicLink != true else { continue }
                files.append((file, values.contentModificationDate ?? .distantPast))
                if files.count >= 5000 { capped = true; break }
            }
        }
        var sessions: [LocalSession] = []
        for (file, updated) in files.sorted(by: { $0.1 > $1.1 }) {
            if Task.isCancelled { throw CancellationError() }
            let handle = try FileHandle(forReadingFrom: file); defer { try? handle.close() }
            let head = try handle.read(upToCount: 65_536) ?? Data()
            let headLines = lines(head, offset: 0, dropFirst: false)
            let end = try handle.seekToEnd(); try handle.seek(toOffset: end > 65_536 ? end - 65_536 : 0)
            let tail = try handle.read(upToCount: 65_536) ?? Data()
            let tailLines = lines(tail, offset: Int64(end > 65_536 ? end - 65_536 : 0), dropFirst: end > 65_536)
            let records = (headLines + tailLines).compactMap { SessionText.json($0.1) }
            var key = file.deletingPathExtension().lastPathComponent, title = "未命名会话", cwd = "", provider: String?, model: String?, tokens: Int64?
            for record in records {
                if agent == .codex {
                    let payload = record["payload"] as? [String: Any] ?? [:]
                    if record["type"] as? String == "session_meta" {
                        key = payload["id"] as? String ?? key; cwd = payload["cwd"] as? String ?? cwd
                        provider = payload["model_provider"] as? String
                    }
                    if record["type"] as? String == "turn_context" { model = payload["model"] as? String ?? model }
                } else {
                    key = record["sessionId"] as? String ?? key; cwd = record["cwd"] as? String ?? cwd
                    model = (record["message"] as? [String: Any])?["model"] as? String ?? model
                    if let named = record["customTitle"] as? String ?? record["aiTitle"] as? String { title = named }
                    if record["type"] as? String == "cost-state", let usage = record["modelUsage"] as? [String: [String: Any]] {
                        tokens = usage.values.reduce(Int64(0)) { sum, model in
                            sum + ["inputTokens", "outputTokens", "cacheReadInputTokens", "cacheCreationInputTokens"].reduce(Int64(0)) { $0 + ((model[$1] as? NSNumber)?.int64Value ?? 0) }
                        }
                    }
                }
                if title == "未命名会话", let item = Self.fileItem(record, agent: agent, offset: 0), item.kind == .user, !item.text.isEmpty {
                    title = String(item.text.prefix(120)).replacingOccurrences(of: "\n", with: " ")
                }
            }
            // A subagent has its own file, but may share the parent's sessionId.
            if file.path.contains("/subagents/") { key += ":" + file.deletingPathExtension().lastPathComponent }
            let session = LocalSession(key: key, agent: agent, title: SessionText.clean(title, limit: 180), cwd: SessionText.clean(cwd, limit: 500),
                provider: provider, model: model, updated: updated, archived: file.path.contains("/archived_sessions/"), file: file, tokens: tokens)
            if search.isEmpty || (session.title + " " + session.cwd).localizedCaseInsensitiveContains(search) { sessions.append(session) }
            if sessions.count > offset + limit { break }
        }
        return SessionCatalog(sessions: Array(sessions.dropFirst(offset).prefix(limit)), hasMore: sessions.count > offset + limit,
            notice: capped ? "目录超过 5,000 个历史文件，仅列出本次扫描范围。" : nil)
    }

    func filePage(_ session: LocalSession, before: Int64?, limit: Int) throws -> SessionPage {
        guard let file = session.file, FileManager.default.fileExists(atPath: file.path) else {
            throw SessionReadError("该会话的原始记录已移走或清理，无法读取历史。")
        }
        // Never follow a rollout pointer outside the selected client directory.
        let base = root.resolvingSymlinksInPath().standardizedFileURL.path + "/"
        guard file.resolvingSymlinksInPath().standardizedFileURL.path.hasPrefix(base) else {
            throw SessionReadError("会话文件不在所选客户端目录内。")
        }
        let handle = try FileHandle(forReadingFrom: file); defer { try? handle.close() }
        let length = Int64(try handle.seekToEnd()), end = min(max(before ?? length, 0), length)
        let start = max(0, end - 4 * 1024 * 1024)
        try handle.seek(toOffset: UInt64(start))
        let data = try handle.read(upToCount: Int(end - start)) ?? Data()
        let completeLines = lines(data, offset: start, dropFirst: start > 0)
        var items: [(Int64, SessionItem)] = [], seen = Set<String>(), malformed = 0
        // Later copies of the same visible item win (e.g. a completed streaming message).
        for (position, bytes) in completeLines.reversed() {
            if Task.isCancelled { throw CancellationError() }
            guard let record = SessionText.json(bytes) else { malformed += 1; continue }
            if let item = Self.fileItem(record, agent: agent, offset: position), seen.insert(item.id).inserted { items.append((position, item)) }
        }
        items.reverse()
        let selected = Array(items.suffix(limit))
        // Include the skipped boundary fragment in the previous page, so normal-sized
        // records crossing the 4 MB read boundary are not lost. An oversized line may
        // still be omitted explicitly, while every cursor must make progress.
        let boundary = completeLines.first?.0 ?? start
        let older = items.count > limit ? selected.first?.0 : (start > 0 ? min(boundary, end - 1) : nil)
        var notes = [String]()
        if start > 0 { notes.append("按片段读取；尚有更早历史。超过 4 MB 的单条记录可能被省略。") }
        if malformed > 0 { notes.append("跳过了 \(malformed) 条不完整记录。") }
        return SessionPage(items: selected.map(\.1), olderCursor: older.map { .file($0) }, notice: notes.isEmpty ? nil : notes.joined(separator: " "))
    }

    /// Only complete newline-terminated records are visible during an append.
    func lines(_ data: Data, offset: Int64, dropFirst: Bool) -> [(Int64, Data)] {
        var result: [(Int64, Data)] = [], start = data.startIndex, skip = dropFirst
        for i in data.indices where data[i] == 10 {
            if !skip && i > start { result.append((offset + Int64(start), Data(data[start..<i]))) }
            skip = false; start = i + 1
        }
        return result
    }

    static func fileItem(_ record: [String: Any], agent: SessionAgent, offset: Int64) -> SessionItem? {
        let date = SessionText.date(record["timestamp"] as? String)
        if agent == .codex {
            let payload = record["payload"] as? [String: Any] ?? [:]
            // New Codex emits canonical items and response copies. Read only canonical items.
            if record["type"] as? String == "event_msg", payload["type"] as? String == "item_completed",
                let item = payload["item"] as? [String: Any] { return codexItem(item, date: date, fallbackID: String(offset)) }
            // Legacy event messages are the user-visible transcript; response_item copies are ignored.
            if record["type"] as? String == "event_msg" {
                let type = payload["type"] as? String
                if type == "user_message" || type == "agent_message" {
                    return SessionItem(id: "line-\(offset)", kind: type == "user_message" ? .user : .assistant,
                        title: type == "user_message" ? "你" : "Assistant", text: payload["message"] as? String ?? "", timestamp: date)
                }
                if type == "error" { return SessionItem(id: "line-\(offset)", kind: .error, title: "错误", text: payload["message"] as? String ?? "", timestamp: date) }
                if type == "exec_command_end" {
                    let failed = (payload["exit_code"] as? Int ?? 0) != 0
                    return SessionItem(id: payload["call_id"] as? String ?? "line-\(offset)", kind: failed ? .error : .tool, title: failed ? "命令失败" : "命令",
                        text: (payload["command"] as? [String] ?? []).joined(separator: " ") + "\n" + (payload["aggregated_output"] as? String ?? ""), timestamp: date)
                }
            }
            if record["type"] as? String == "compacted" {
                return SessionItem(id: "line-\(offset)", kind: .context, title: "上下文压缩", text: "客户端在这里压缩了上下文。", timestamp: date)
            }
            return nil
        }
        let type = record["type"] as? String, id = record["uuid"] as? String ?? "line-\(offset)"
        if type == "system", let error = record["error"] {
            return SessionItem(id: id, kind: .error, title: "客户端错误", text: SessionText.jsonText(error), timestamp: date)
        }
        guard type == "user" || type == "assistant", let message = record["message"] as? [String: Any] else { return nil }
        let blocks = message["content"] as? [[String: Any]] ?? []
        let isResult = blocks.contains { $0["type"] as? String == "tool_result" }
        let isTool = blocks.contains { $0["type"] as? String == "tool_use" }
        let isError = record["isApiErrorMessage"] as? Bool == true || blocks.contains { $0["is_error"] as? Bool == true }
        let kind: SessionItem.Kind = isError ? .error : isResult ? .result : isTool ? .tool : type == "user" ? .user : .assistant
        let title = isError ? "错误" : isResult ? "工具结果" : isTool ? "工具调用" : type == "user" ? "你" : "Assistant"
        let content = SessionText.content(message["content"])
        guard !content.isEmpty else { return nil }
        return SessionItem(id: id, kind: kind, title: title, text: content, timestamp: date, model: message["model"] as? String)
    }
}
