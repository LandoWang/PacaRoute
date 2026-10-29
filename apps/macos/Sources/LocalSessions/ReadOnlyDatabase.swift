import Foundation
import CSQLite

final class ReadOnlyDatabase {
    private var db: OpaquePointer?
    init(_ path: URL) throws {
        guard sqlite3_open_v2(path.path, &db, SQLITE_OPEN_READONLY | SQLITE_OPEN_NOMUTEX, nil) == SQLITE_OK else {
            if let db { sqlite3_close(db) }; db = nil
            throw SessionReadError("无法只读打开会话数据库：\(path.lastPathComponent)")
        }
        sqlite3_busy_timeout(db, 1000)
        try rows("PRAGMA query_only = ON") { _ in }
    }
    deinit { sqlite3_close(db) }
    func columns(_ table: String) throws -> Set<String> {
        var result = Set<String>()
        // Table names are compile-time adapter constants, never transcript values.
        try rows("PRAGMA table_info(\(table))") { row in if let name = row.string(1) { result.insert(name) } }
        return result
    }
    func rows(_ sql: String, bindings: [String] = [], _ read: (Row) throws -> Void) throws {
        var stmt: OpaquePointer?
        guard sqlite3_prepare_v2(db, sql, -1, &stmt, nil) == SQLITE_OK else {
            throw SessionReadError("会话数据库格式暂不支持，请检查客户端版本。")
        }
        defer { sqlite3_finalize(stmt) }
        for (i, value) in bindings.enumerated() {
            _ = value.withCString { sqlite3_bind_text(stmt, Int32(i + 1), $0, -1, unsafeBitCast(-1, to: sqlite3_destructor_type.self)) }
        }
        while true {
            if Task.isCancelled { throw CancellationError() }
            let status = sqlite3_step(stmt)
            if status == SQLITE_DONE { return }
            guard status == SQLITE_ROW else { throw SessionReadError("读取会话数据库失败或正忙，请稍后刷新。") }
            try read(Row(statement: stmt))
        }
    }
    struct Row {
        let statement: OpaquePointer?
        func string(_ i: Int32) -> String? {
            guard let ptr = sqlite3_column_text(statement, i) else { return nil }
            return String(cString: ptr)
        }
        func int(_ i: Int32) -> Int64 { sqlite3_column_int64(statement, i) }
    }
}
