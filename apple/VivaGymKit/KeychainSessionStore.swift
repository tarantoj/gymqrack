import Foundation
import Security

/// Stores and reads the member `Session` in this app's own Keychain.
///
/// The companion and watch do not share keychain items (cross-app keychain
/// access groups are unreliable for standalone watch apps on free teams), so
/// each app keeps its own copy and `SessionSync` transfers the session between
/// them over WatchConnectivity.
public enum KeychainSessionStore {
    private static let service = "com.vivagym.session"
    private static let account = "member"

    public static func load() -> Session? {
        var query = baseQuery()
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        guard status == errSecSuccess, let data = item as? Data else { return nil }
        return try? JSONDecoder().decode(Session.self, from: data)
    }

    public static func save(_ session: Session) throws {
        let data = try JSONEncoder().encode(session)
        var item = baseQuery()
        item[kSecValueData as String] = data
        // AfterFirstUnlock so the watch can read/refresh the token in the
        // background or while locked; ThisDeviceOnly keeps it off other devices.
        item[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        try delete(throwOnAbsent: false)
        let status = SecItemAdd(item as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw KeychainError.osStatus(status)
        }
    }

    public static func clear() {
        try? delete(throwOnAbsent: true)
    }

    private static func delete(throwOnAbsent: Bool) throws {
        let status = SecItemDelete(baseQuery() as CFDictionary)
        if status != errSecSuccess && !(status == errSecItemNotFound && !throwOnAbsent) {
            throw KeychainError.osStatus(status)
        }
    }

    private static func baseQuery() -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
    }
}

public enum KeychainError: Error, LocalizedError {
    case osStatus(OSStatus)

    public var errorDescription: String? {
        switch self {
        case .osStatus(let status):
            return "Keychain error \(status)"
        }
    }
}