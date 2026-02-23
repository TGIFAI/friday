import Foundation
import Security

/// Simple Keychain wrapper for storing API keys.
enum KeychainHelper {
    private static let service = "sh.tgif.friday"

    /// Known API key environment variable names.
    private static let knownKeys = [
        "OPENAI_API_KEY",
        "ANTHROPIC_API_KEY",
        "GEMINI_API_KEY",
        "BRAVE_API_KEY",
        "TELEGRAM_BOT_TOKEN",
        "LARK_APP_ID",
        "LARK_APP_SECRET",
        "LARK_VERIFICATION_TOKEN",
        "LARK_ENCRYPT_KEY",
        "HTTP_API_KEY",
    ]

    static func get(_ key: String) -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]

        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        guard status == errSecSuccess, let data = item as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    static func set(_ key: String, value: String) {
        let data = Data(value.utf8)
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
        ]

        // Delete existing, then add.
        SecItemDelete(query as CFDictionary)

        var add = query
        add[kSecValueData as String] = data
        SecItemAdd(add as CFDictionary, nil)
    }

    static func delete(_ key: String) {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
        ]
        SecItemDelete(query as CFDictionary)
    }

    /// Returns all stored API keys as an environment dictionary.
    static func allAPIKeys() -> [String: String] {
        var result: [String: String] = [:]
        for key in knownKeys {
            if let value = get(key), !value.isEmpty {
                result[key] = value
            }
        }
        return result
    }
}
