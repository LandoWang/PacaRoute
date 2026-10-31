import Foundation
import CSQLite

/// Disposable local metadata index. Raw request/response bodies and credentials
/// never enter this database; the daily JSONL files remain the source of truth.
final class ActivityIndex {
    private var db: OpaquePointer?
    private let file: URL
    private struct Failure: Error {}
    private enum Value {
        case text(String), integer(Int64), real(Double), blob(Data), null
        static func text(_ value: String?) -> Value { value.map(Value.text) ?? .null }
        static func int(_ value: Int?) -> Value { value.map { .integer(Int64($0)) } ?? .null }
    }

    static func open(in directory: URL) throws -> (ActivityIndex, [String: ActivityLogReader.Entry]) {
        let file = directory.appendingPathComponent(".agentlink-index.sqlite")
        for attempt in 0..<2 {
            let index = ActivityIndex(file: file)
            do {
                try index.connect()
                return (index, try index.load())
            } catch {
                index.invalidate()
                if attempt == 1 { throw error }
            }
        }
        throw Failure()
    }

    private init(file: URL) { self.file = file }
    deinit { if let db { sqlite3_close(db) } }

    private func connect() throws {
        if !FileManager.default.fileExists(atPath: file.path) {
            guard FileManager.default.createFile(atPath: file.path, contents: nil, attributes: [.posixPermissions: 0o600]) else { throw Failure() }
        }
        guard sqlite3_open_v2(file.path, &db, SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE | SQLITE_OPEN_NOMUTEX, nil) == SQLITE_OK else { throw Failure() }
        sqlite3_busy_timeout(db, 500)
        var version = 0
        try query("PRAGMA user_version") { version = Int(sqlite3_column_int($0, 0)) }
        guard version == 0 || version == 2 else { throw Failure() }
        try exec("""
            PRAGMA journal_mode=WAL;
            PRAGMA synchronous=NORMAL;
            CREATE TABLE IF NOT EXISTS files (
                path TEXT PRIMARY KEY, device INTEGER NOT NULL, inode INTEGER NOT NULL,
                size INTEGER NOT NULL, modified REAL NOT NULL, offset INTEGER NOT NULL,
                tail BLOB NOT NULL, pending BLOB NOT NULL
            );
            CREATE TABLE IF NOT EXISTS activity (
                file_path TEXT NOT NULL, ordinal INTEGER NOT NULL, id TEXT NOT NULL,
                source TEXT, timestamp REAL NOT NULL, kind TEXT NOT NULL, model TEXT, client TEXT,
                input_tokens INTEGER, output_tokens INTEGER, cache_read_tokens INTEGER,
                cache_write_tokens INTEGER, status INTEGER, duration_ms INTEGER, error TEXT,
                PRIMARY KEY (file_path, ordinal)
            );
            CREATE INDEX IF NOT EXISTS activity_time ON activity(timestamp);
            CREATE INDEX IF NOT EXISTS activity_source_time ON activity(source, timestamp);
            PRAGMA user_version=2;
            """)
    }

    private func load() throws -> [String: ActivityLogReader.Entry] {
        var result: [String: ActivityLogReader.Entry] = [:]
        try query("SELECT path,device,inode,size,modified,offset,tail,pending FROM files") { row in
            guard let path = text(row, 0) else { throw Failure() }
            let stamp = ActivityLogReader.Stamp(device: UInt64(bitPattern: sqlite3_column_int64(row, 1)),
                inode: UInt64(bitPattern: sqlite3_column_int64(row, 2)), size: UInt64(bitPattern: sqlite3_column_int64(row, 3)),
                modified: Date(timeIntervalSinceReferenceDate: sqlite3_column_double(row, 4)))
            let offset = UInt64(bitPattern: sqlite3_column_int64(row, 5))
            let tail = blob(row, 6), pending = blob(row, 7)
            guard offset <= stamp.size, tail.count <= 64, UInt64(tail.count) <= offset,
                  pending.count <= 1_048_576 else { throw Failure() }
            result[path] = ActivityLogReader.Entry(stamp: stamp, offset: offset,
                tail: tail, lines: JSONLineBuffer(pendingBytes: pending))
        }
        try query("""
            SELECT file_path,id,source,timestamp,kind,model,client,input_tokens,output_tokens,
                cache_read_tokens,cache_write_tokens,status,duration_ms,error
            FROM activity ORDER BY file_path,ordinal
            """) { row in
            guard let path = text(row, 0), let id = text(row, 1), let kind = text(row, 4), result[path] != nil else { throw Failure() }
            result[path]!.records.append(Activity(source: text(row, 2), id: id,
                timestamp: Date(timeIntervalSince1970: sqlite3_column_double(row, 3)), kind: kind,
                model: text(row, 5), client: text(row, 6), inputTokens: int(row, 7), outputTokens: int(row, 8),
                cacheReadTokens: int(row, 9), cacheWriteTokens: int(row, 10), status: int(row, 11),
                durationMs: int(row, 12), error: text(row, 13)))
        }
        return result
    }

    func save(path: String, entry: ActivityLogReader.Entry, from start: Int, reset: Bool) throws {
        try exec("BEGIN IMMEDIATE")
        do {
            if reset { try run("DELETE FROM activity WHERE file_path=?", [.text(path)]) }
            let insert = try prepare("INSERT INTO activity VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
            defer { sqlite3_finalize(insert) }
            for ordinal in start..<entry.records.count {
                let record = entry.records[ordinal]
                try run(insert, [.text(path), .integer(Int64(ordinal)), .text(record.id), .text(record.source),
                    .real(record.timestamp.timeIntervalSince1970), .text(record.kind), .text(record.model), .text(record.client),
                    .int(record.inputTokens), .int(record.outputTokens), .int(record.cacheReadTokens), .int(record.cacheWriteTokens),
                    .int(record.status), .int(record.durationMs), .text(record.error)])
            }
            let stamp = entry.stamp
            // Preserve Date's native reference interval exactly. An epoch conversion
            // can round file mtimes and falsely invalidate a checkpoint on reopen.
            try run("INSERT OR REPLACE INTO files VALUES (?,?,?,?,?,?,?,?)", [.text(path),
                .integer(Int64(bitPattern: stamp.device)), .integer(Int64(bitPattern: stamp.inode)),
                .integer(Int64(bitPattern: stamp.size)), .real(stamp.modified.timeIntervalSinceReferenceDate),
                .integer(Int64(bitPattern: entry.offset)), .blob(entry.tail), .blob(entry.lines.pendingBytes)])
            try exec("COMMIT")
        } catch { try? exec("ROLLBACK"); throw error }
    }

    func remove(path: String) throws {
        try exec("BEGIN IMMEDIATE")
        do {
            try run("DELETE FROM activity WHERE file_path=?", [.text(path)])
            try run("DELETE FROM files WHERE path=?", [.text(path)])
            try exec("COMMIT")
        } catch { try? exec("ROLLBACK"); throw error }
    }

    func invalidate() {
        if let db { sqlite3_close(db); self.db = nil }
        for suffix in ["", "-wal", "-shm"] { try? FileManager.default.removeItem(atPath: file.path + suffix) }
    }

    private func prepare(_ sql: String) throws -> OpaquePointer {
        var statement: OpaquePointer?
        guard sqlite3_prepare_v2(db, sql, -1, &statement, nil) == SQLITE_OK, let statement else { throw Failure() }
        return statement
    }
    private func exec(_ sql: String) throws {
        guard sqlite3_exec(db, sql, nil, nil, nil) == SQLITE_OK else { throw Failure() }
    }
    private func run(_ sql: String, _ values: [Value]) throws {
        let statement = try prepare(sql)
        defer { sqlite3_finalize(statement) }
        try run(statement, values)
    }
    private func run(_ statement: OpaquePointer, _ values: [Value]) throws {
        sqlite3_reset(statement); sqlite3_clear_bindings(statement)
        let transient = unsafeBitCast(-1, to: sqlite3_destructor_type.self)
        for (index, value) in values.enumerated() {
            let i = Int32(index + 1)
            let status: Int32
            switch value {
            case .text(let value): status = value.withCString { sqlite3_bind_text(statement, i, $0, -1, transient) }
            case .integer(let value): status = sqlite3_bind_int64(statement, i, value)
            case .real(let value): status = sqlite3_bind_double(statement, i, value)
            case .blob(let value):
                if value.isEmpty { status = sqlite3_bind_zeroblob(statement, i, 0) }
                else { status = value.withUnsafeBytes { sqlite3_bind_blob(statement, i, $0.baseAddress, Int32($0.count), transient) } }
            case .null: status = sqlite3_bind_null(statement, i)
            }
            guard status == SQLITE_OK else { throw Failure() }
        }
        guard sqlite3_step(statement) == SQLITE_DONE else { throw Failure() }
    }
    private func query(_ sql: String, row: (OpaquePointer) throws -> Void) throws {
        let statement = try prepare(sql)
        defer { sqlite3_finalize(statement) }
        while true {
            switch sqlite3_step(statement) {
            case SQLITE_ROW: try row(statement)
            case SQLITE_DONE: return
            default: throw Failure()
            }
        }
    }
    private func text(_ row: OpaquePointer, _ column: Int32) -> String? {
        sqlite3_column_text(row, column).map { String(cString: $0) }
    }
    private func int(_ row: OpaquePointer, _ column: Int32) -> Int? {
        sqlite3_column_type(row, column) == SQLITE_NULL ? nil : Int(sqlite3_column_int64(row, column))
    }
    private func blob(_ row: OpaquePointer, _ column: Int32) -> Data {
        guard let pointer = sqlite3_column_blob(row, column) else { return Data() }
        return Data(bytes: pointer, count: Int(sqlite3_column_bytes(row, column)))
    }
}
