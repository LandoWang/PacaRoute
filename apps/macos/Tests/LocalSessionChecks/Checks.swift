import Foundation
import LocalSessions
import CSQLite

func require(_ condition: @autoclosure () throws -> Bool, _ message: String) {
    do { guard try condition() else { fatalError(message) } }
    catch { fatalError("\(message): \(error)") }
}
func writeDB(_ url: URL, sql: String) throws {
    var db: OpaquePointer?
    guard sqlite3_open(url.path, &db) == SQLITE_OK else { fatalError("fixture database") }
    defer { sqlite3_close(db) }
    guard sqlite3_exec(db, sql, nil, nil, nil) == SQLITE_OK else { fatalError(String(cString: sqlite3_errmsg(db))) }
}
func quote(_ value: String) -> String { "'" + value.replacingOccurrences(of: "'", with: "''") + "'" }
func json(_ value: [String: Any]) throws -> String { String(decoding: try JSONSerialization.data(withJSONObject: value, options: [.sortedKeys]), as: UTF8.self) }

@main struct Checks {
    static func main() throws {
        if CommandLine.arguments.count > 1 { try inspectLocal(); return }
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("agentlink-history-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try checkCodex(root.appendingPathComponent("codex"))
        try checkFiles(root.appendingPathComponent("claude"))
        try checkLargeFile(root.appendingPathComponent("large"))
        try checkLegacy(root.appendingPathComponent("legacy"))
        print("PASS: read-only DB, pagination, unknown schemas, missing history, Claude copies/results, partial writes, legacy fallback, redaction, bounded analysis")
    }

    static func checkCodex(_ root: URL) throws {
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        let state = root.appendingPathComponent("state_5.sqlite"), history = root.appendingPathComponent("thread_history_1.sqlite")
        try writeDB(state, sql: """
            CREATE TABLE threads(id TEXT, title TEXT, cwd TEXT, model_provider TEXT, model TEXT, updated_at INTEGER, tokens_used INTEGER, rollout_path TEXT);
            INSERT INTO threads VALUES('test', 'Invoice 修复', '/project', 'copilot-bridge', 'test-model', 100, 123, '/missing');
            """)
        var sql = "CREATE TABLE thread_items(thread_id TEXT, rollout_ordinal INTEGER, item_type TEXT, item_json TEXT, created_at_ms INTEGER);"
        sql += "CREATE TABLE thread_turns(thread_id TEXT, rollout_ordinal INTEGER, status TEXT, error_json TEXT); INSERT INTO thread_turns VALUES('test',1,'completed',NULL);"
        let values: [[String: Any]] = [
            ["id": "u", "type": "userMessage", "content": [["type": "text", "text": "Fix invoice math"]]],
            ["id": "r", "type": "reasoning", "summary": ["not shown"]],
            ["id": "cmd", "type": "commandExecution", "command": "run-tests", "aggregatedOutput": "one assertion failed", "exitCode": 1],
            ["id": "tool", "type": "mcpToolCall", "tool": "read-file", "arguments": ["path": "invoice.py"], "result": ["content": [["type": "text", "text": "api-key=should-never-appear"]]]],
            ["id": "a", "type": "agentMessage", "text": "Patched; not yet re-tested."],
        ]
        for (i, value) in values.enumerated() {
            sql += "INSERT INTO thread_items VALUES('test',\(i + 1),\(quote(value["type"] as! String)),\(quote(try json(value))),100000);"
        }
        try writeDB(history, sql: sql)
        let originalState = try Data(contentsOf: state), originalHistory = try Data(contentsOf: history)
        let reader = SessionReader(root: root, agent: .codex), catalog = try reader.catalog(search: "Invoice")
        require(catalog.sessions.count == 1, "Search must match stored title")
        let selected = catalog.sessions[0]
        require(selected.providerLabel.contains("未记录"), "Bridge is not an upstream provider")
        require(selected.tokens == 123, "Use recorded usage, never fabricate zero")
        let first = try reader.page(selected, limit: 2), older = try reader.page(selected, before: first.olderCursor, limit: 2)
        require(first.items.map(\.id) == ["tool", "a"], "Newest page must be chronological")
        require(older.items.map(\.id) == ["u", "cmd"] && older.olderCursor == nil, "Pagination must not repeat/omit visible records")
        require(older.items.last?.kind == .error, "Nonzero exit is evidence of failure")
        require(!first.items[0].text.contains("should-never-appear"), "Credentials must be redacted before display")
        let full = older.items + first.items
        let analysis = SessionAnalysisInput(session: selected, items: full, hasOlder: false)
        let request = try JSONSerialization.jsonObject(with: analysis.request(model: "selected-model", question: "Next step?")) as! [String: Any]
        require(request["model"] as? String == "selected-model", "Analysis must use the chosen model")
        require(request["store"] as? Bool == false && (request["tools"] as? [Any])?.isEmpty == true, "Analysis must be stateless and tool-free")
        require(analysis.evidence.contains("E3") && !analysis.evidence.contains("should-never-appear"), "Citations and sanitized evidence required")
        let small = SessionAnalysisInput(session: selected, items: full, hasOlder: true, characterLimit: 160)
        require(small.itemIDs.count < full.count && small.coverage.contains("未发送"), "Partial analysis must disclose omitted evidence")
        let reply = try json(["output": [["type": "reasoning", "summary": ["ignored"]], ["type": "message", "content": [["type": "output_text", "text": "尚待复测 [E4]"]]]]])
        require(try SessionAnalysisInput.response(Data(reply.utf8)) == "尚待复测 [E4]", "Only model text is displayed")
        require(try Data(contentsOf: state) == originalState && Data(contentsOf: history) == originalHistory, "Browsing must not mutate client databases")
        try writeDB(history, sql: "INSERT INTO thread_items VALUES('test',6,'agentMessage','broken',100000);")
        require(try reader.page(selected).items.contains { $0.title == "记录未展开" }, "Malformed data must be visible as omitted")
        try writeDB(root.appendingPathComponent("state_99.sqlite"), sql: "CREATE TABLE unsupported(id TEXT);")
        do { _ = try reader.catalog(); fatalError("Unsupported schema must not look like no sessions") } catch is SessionReadError { }
    }

    static func checkFiles(_ root: URL) throws {
        let project = root.appendingPathComponent("projects/example")
        try FileManager.default.createDirectory(at: project, withIntermediateDirectories: true)
        let file = project.appendingPathComponent("session.jsonl")
        let messages: [[String: Any]] = [
            ["type": "user", "uuid": "u", "sessionId": "claude-test", "cwd": "/project", "message": ["content": "修复账单"]],
            ["type": "api-request-blob", "message": ["content": "duplicate must not display"]],
            ["type": "assistant", "uuid": "call", "message": ["model": "claude-test", "content": [["type": "tool_use", "name": "Bash", "input": ["command": "test"]]]]],
            ["type": "user", "uuid": "result", "message": ["content": [["type": "tool_result", "content": "failed", "is_error": true]]]],
            ["type": "assistant", "uuid": "a", "message": ["model": "claude-test", "content": [["type": "text", "text": "需要修复"]]]],
            ["type": "cost-state", "modelUsage": ["claude-test": ["inputTokens": 5,"outputTokens": 6,"cacheReadInputTokens": 10,"thinkingTokens": 3]]],
        ]
        let content = try messages.map(json).joined(separator: "\n") + "\n{\"type\":\"user\",\"uuid\":\"partial\",\"message\":{\"content\":\"尚未完成\"}}"
        try content.write(to: file, atomically: true, encoding: .utf8)
        let reader = SessionReader(root: root, agent: .claude), selected = try reader.catalog().sessions[0]
        require(selected.tokens == 21, "Claude cost snapshot is cumulative; do not sum duplicate snapshots/thinking")
        let recent = try reader.page(selected, limit: 2), older = try reader.page(selected, before: recent.olderCursor, limit: 2)
        require(recent.items.map(\.id) == ["result", "a"] && recent.items[0].kind == .error, "Tool errors are not human messages")
        require(older.items.map(\.id) == ["u", "call"], "Ignore request copies; maintain separate tool calls")
        let original = try Data(contentsOf: file)
        require(try reader.page(selected).items.count == 4, "Do not expose a trailing incomplete append")
        require(try Data(contentsOf: file) == original, "File histories must not be modified")
        let handle = try FileHandle(forWritingTo: file); try handle.seekToEnd(); try handle.write(contentsOf: Data("\n".utf8)); try handle.close()
        require(try reader.page(selected).items.last?.id == "partial", "Refresh must pick up completed appends")
        let clean = SessionText.clean("Bearer token-value api-key=credential ghp_123456789 sk-secret123456 https://name:pass@example.com -----BEGIN PRIVATE KEY-----\nprivate\n-----END PRIVATE KEY-----")
        for forbidden in ["token-value", "credential", "123456789", "secret123456", "name:pass", "\nprivate\n"] { require(!clean.contains(forbidden), "Redaction contract") }
    }

    static func checkLegacy(_ root: URL) throws {
        let dir = root.appendingPathComponent("sessions")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let file = dir.appendingPathComponent("rollout.jsonl")
        let records: [[String: Any]] = [
            ["type": "session_meta", "payload": ["id": "legacy", "cwd": "/project", "model_provider": "azure"]],
            ["type": "event_msg", "payload": ["type": "user_message", "message": "Hello"]],
            ["type": "response_item", "payload": ["type": "message", "role": "user", "content": [["type": "input_text", "text": "Hello"]]]],
            ["type": "event_msg", "payload": ["type": "exec_command_end", "call_id": "c", "command": ["echo", "hello"], "exit_code": 0, "aggregated_output": "hello"]],
            ["type": "event_msg", "payload": ["type": "agent_message", "message": "Done"]],
        ]
        try (records.map(json).joined(separator: "\n") + "\n").write(to: file, atomically: true, encoding: .utf8)
        // A DB exists but this older session has no projection: byte cursor must survive paging.
        try writeDB(root.appendingPathComponent("thread_history_1.sqlite"), sql: "CREATE TABLE thread_items(thread_id TEXT, rollout_ordinal INTEGER, item_type TEXT, item_json TEXT);")
        let reader = SessionReader(root: root, agent: .codex), session = try reader.catalog().sessions[0]
        let page = try reader.page(session, limit: 1), older = try reader.page(session, before: page.olderCursor, limit: 1)
        require(page.items.first?.text == "Done" && older.items.first?.text.contains("hello") == true, "Legacy paging must keep byte offsets")
        require(try reader.page(session).items.count == 3, "Legacy response copies must not duplicate conversations")
    }

    static func checkLargeFile(_ root: URL) throws {
        let project = root.appendingPathComponent("projects/example")
        try FileManager.default.createDirectory(at: project, withIntermediateDirectories: true)
        let file = project.appendingPathComponent("large.jsonl")
        var records = [String]()
        for i in 0..<12 {
            records.append(try json(["type": "user", "uuid": "item-\(i)", "sessionId": "large", "message": ["content": "message \(i) " + String(repeating: "x", count: 450_000)]]))
        }
        try (records.joined(separator: "\n") + "\n").write(to: file, atomically: true, encoding: .utf8)
        let reader = SessionReader(root: root, agent: .claude), selected = try reader.catalog().sessions[0]
        let page = try reader.page(selected, limit: 100)
        require(page.olderCursor != nil, "Large histories must expose a continuation")
        let older = try reader.page(selected, before: page.olderCursor, limit: 100)
        require((older.items + page.items).map(\.id) == (0..<12).map { "item-\($0)" }, "A record across a read boundary must not be lost or repeated")
        require(page.items.allSatisfy { $0.text.count < 17_000 }, "Huge tool/message text must be bounded")
    }

    static func inspectLocal() throws {
        for agent in SessionAgent.allCases {
            let root = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(agent == .codex ? ".codex" : ".claude")
            let reader = SessionReader(root: root, agent: agent), catalog = try reader.catalog()
            var count = 0, tools = 0, errors = 0
            for selected in catalog.sessions.prefix(3) {
                let page = try reader.page(selected)
                count += page.items.count; tools += page.items.filter { $0.kind == .tool || $0.kind == .result }.count
                errors += page.items.filter { $0.kind == .error }.count
            }
            // Deliberately print no transcript text, source endpoints, project paths or credentials.
            print("\(agent.rawValue): catalog=\(catalog.sessions.count), more=\(catalog.hasMore), sampledItems=\(count), tools=\(tools), errors=\(errors)")
        }
    }
}
