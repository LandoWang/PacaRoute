import Foundation
import Security

/// A successful Keychain read lasts for this app session, not just one child process.
/// Failed reads/writes never replace a usable cached key. Nothing is persisted here.
final class SourceCredentialCache {
    private var values: [String: String] = [:]
    private let readKey: (String) throws -> String
    private let saveKey: (String, String) throws -> Void
    private let deleteKey: (String) throws -> Void

    init(read: @escaping (String) throws -> String = SourceKeychain.read,
         save: @escaping (String, String) throws -> Void = { try SourceKeychain.save($0, for: $1) },
         delete: @escaping (String) throws -> Void = SourceKeychain.delete) {
        readKey = read; saveKey = save; deleteKey = delete
    }
    func read(_ id: String) throws -> String {
        if let value = values[id] { return value }
        let value = try readKey(id)
        values[id] = value
        return value
    }
    func save(_ value: String, for id: String) throws {
        try saveKey(value, id)
        values[id] = value
    }
    func delete(_ id: String) throws {
        try deleteKey(id)
        values[id] = nil
    }
    func clear() { values.removeAll() }
}

/// No credential readback is exposed to views. Keys only travel to the owned bridge pipe.
enum SourceKeychain {
    private static let service = "com.lando.copilot-bridge.azure"
    private static func query(_ id: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: service, kSecAttrAccount as String: id]
    }
    static func save(_ value: String, for id: String) throws {
        let bytes = Data(value.utf8)
        let status = SecItemUpdate(query(id) as CFDictionary, [kSecValueData as String: bytes] as CFDictionary)
        if status == errSecItemNotFound {
            var item = query(id); item[kSecValueData as String] = bytes
            item[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
            try check(SecItemAdd(item as CFDictionary, nil))
        } else { try check(status) }
    }
    static func read(_ id: String) throws -> String {
        var item = query(id); item[kSecReturnData as String] = true; item[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        try check(SecItemCopyMatching(item as CFDictionary, &result))
        guard let data = result as? Data, let value = String(data: data, encoding: .utf8) else {
            throw AppFailure(message: "无法读取此来源的 Keychain 凭据。")
        }
        return value
    }
    static func delete(_ id: String) throws {
        let status = SecItemDelete(query(id) as CFDictionary)
        if status != errSecItemNotFound { try check(status) }
    }
    private static func check(_ status: OSStatus) throws {
        guard status == errSecSuccess else { throw AppFailure(message: "Keychain 操作失败（\(status)）。请解锁登录钥匙串后重试。") }
    }
}
