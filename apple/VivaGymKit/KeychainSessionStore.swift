import Foundation
import Security

/// Stores and reads the member `Session` in a Keychain access group shared
/// between the iOS companion app and the watchOS app.
///
/// The access group identifier is `$(DEVELOPMENT_TEAM).` + `vivagym.session`,
/// the same value both targets declare in `keychain-access-groups`
/// entitlements. The team prefix is injected into Info.plist at build time as
/// `AppIdentifierPrefix`; when it is empty (e.g. unsigned simulator builds) the
/// default per-app keychain is used instead, which still works for local runs.
public enum KeychainSessionStore {
    private static let accessGroupSuffix = "vivagym.session"
    private static let service = "com.vivagym.session"
    private static let account = "member"

    public static var accessGroup: String? {
        guard let prefix = Bundle.main.object(forInfoDictionaryKey: "AppIdentifierPrefix") as? String else {
            return nil
        }
        let trimmed = prefix.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, trimmed != "." else { return nil }
        return trimmed + accessGroupSuffix
    }

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
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        if let accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }
        return query
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