import Foundation

/// Owns the offsets, partial lines and decoder on one background executor.
/// A nil result means callers should keep their current UI snapshot.
public actor ActivityLogReader {
    struct Stamp: Equatable {
        let device: UInt64
        let inode: UInt64
        let size: UInt64
        let modified: Date

        init(device: UInt64, inode: UInt64, size: UInt64, modified: Date) {
            self.device = device; self.inode = inode; self.size = size; self.modified = modified
        }

        init?(_ file: URL) {
            guard let attributes = try? FileManager.default.attributesOfItem(atPath: file.path),
                  attributes[.type] as? FileAttributeType == .typeRegular,
                  let device = attributes[.systemNumber] as? NSNumber,
                  let inode = attributes[.systemFileNumber] as? NSNumber,
                  let size = attributes[.size] as? NSNumber,
                  let modified = attributes[.modificationDate] as? Date else { return nil }
            self.device = device.uint64Value; self.inode = inode.uint64Value
            self.size = size.uint64Value; self.modified = modified
        }
    }

    struct Entry {
        var stamp: Stamp
        var offset: UInt64 = 0
        var tail = Data()
        var lines = JSONLineBuffer()
        var records: [Activity] = []
    }

    private var entries: [String: Entry] = [:]
    private let decoder = ActivityReader.makeDecoder()
    private var published = false
    private var index: ActivityIndex?
    private var openedIndex = false
    public init() {}

    public func refresh(directory: URL) -> [Activity]? {
        let fm = FileManager.default
        let files: [URL]
        do {
            files = try fm.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil)
                .filter { $0.pathExtension == "jsonl" }.sorted { $0.path > $1.path }.prefix(366).map { $0 }
        } catch {
            // A transient read failure must not erase visible history.
            if fm.fileExists(atPath: directory.path) { return nil }
            let changed = !entries.isEmpty || !published
            entries.removeAll(); published = true
            return changed ? [] : nil
        }
        if !openedIndex {
            openedIndex = true
            // Disposable cache: failures must not prevent reading the source logs.
            if let restored = try? ActivityIndex.open(in: directory) {
                index = restored.0; entries = restored.1
            }
        }
        let paths = Set(files.map(\.path))
        var changed = !published
        for path in Array(entries.keys) where !paths.contains(path) {
            do { try index?.remove(path: path) } catch { discardIndex() }
            if entries.removeValue(forKey: path)?.records.isEmpty == false { changed = true }
        }
        for file in files {
            guard let stamp = Stamp(file) else { continue }
            let previous = entries[file.path]
            if let previous, previous.stamp == stamp, previous.offset == stamp.size { continue }
            do {
                let handle = try FileHandle(forReadingFrom: file)
                defer { try? handle.close() }
                var entry = previous ?? Entry(stamp: stamp)
                var reset = previous == nil || entry.stamp.device != stamp.device || entry.stamp.inode != stamp.inode
                    || stamp.size < entry.offset || (stamp.size == entry.offset && stamp.modified != entry.stamp.modified)
                // Catch an in-place rewrite that also grew, without scanning old records.
                if !reset && !entry.tail.isEmpty {
                    try handle.seek(toOffset: entry.offset - UInt64(entry.tail.count))
                    reset = try handle.read(upToCount: entry.tail.count) != entry.tail
                }
                if reset { entry = Entry(stamp: stamp) }
                try handle.seek(toOffset: entry.offset)
                let oldCount = entry.records.count
                // Stop at the observed size, even if the writer keeps appending.
                while entry.offset < stamp.size {
                    let length = Int(min(65_536, stamp.size - entry.offset))
                    guard let data = try handle.read(upToCount: length), !data.isEmpty else { break }
                    entry.offset += UInt64(data.count)
                    entry.tail = Data((entry.tail + data).suffix(64))
                    for line in entry.lines.append(data) {
                        if let record = try? decoder.decode(Activity.self, from: line) { entry.records.append(record) }
                    }
                }
                entry.stamp = stamp
                do { try index?.save(path: file.path, entry: entry, from: oldCount, reset: reset) }
                catch { discardIndex() }
                if entry.records.count != oldCount || (reset && previous?.records.isEmpty == false) { changed = true }
                entries[file.path] = entry
            } catch {
                // Keep the last good offset so the next poll can retry the same bytes.
                continue
            }
        }
        guard changed else { return nil }
        published = true
        return entries.values.flatMap(\.records).sorted {
            $0.timestamp == $1.timestamp ? $0.id < $1.id : $0.timestamp > $1.timestamp
        }
    }

    private func discardIndex() {
        index?.invalidate()
        index = nil
    }
}
