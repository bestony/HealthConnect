import Foundation
import os
import Security

struct SavedSettings: Equatable {
    var url: URL
    var verificationCode: String

    var endpoint: ServerEndpoint {
        ServerEndpoint(url: url, verificationCode: verificationCode)
    }
}

protocol SettingsStoring: AnyObject {
    func load() -> SavedSettings?
    func save(_ endpoint: ServerEndpoint) throws
    func receipt() -> UploadReceipt?
    func saveReceipt(_ receipt: UploadReceipt)
    func clearReceipt()
}

protocol CredentialStoring: AnyObject {
    func saveCode(_ code: String) throws
    func loadCode() -> String?
    func deleteCode() throws
}

final class SettingsStore: SettingsStoring {
    private let defaults: UserDefaults
    private let credentials: CredentialStoring
    private let encoder: JSONEncoder
    private let decoder: JSONDecoder

    init(defaults: UserDefaults, credentials: CredentialStoring) {
        self.defaults = defaults
        self.credentials = credentials
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        self.encoder = encoder
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        self.decoder = decoder
    }

    func load() -> SavedSettings? {
        guard let raw = defaults.string(forKey: Key.serverURL), let url = URL(string: raw) else {
            return nil
        }
        guard let code = credentials.loadCode(), !code.isEmpty else {
            AppLog.setup.error("Saved server address has no verification code")
            return nil
        }
        return SavedSettings(url: url, verificationCode: code)
    }

    func save(_ endpoint: ServerEndpoint) throws {
        let previous = load()?.endpoint.identity
        if previous != endpoint.identity {
            clearReceipt()
        }
        defaults.set(endpoint.url.absoluteString, forKey: Key.serverURL)
        do {
            try credentials.saveCode(endpoint.verificationCode)
        } catch {
            defaults.removeObject(forKey: Key.serverURL)
            throw error
        }
    }

    func receipt() -> UploadReceipt? {
        guard let data = defaults.data(forKey: Key.receipt) else {
            return nil
        }
        do {
            return try decoder.decode(UploadReceipt.self, from: data)
        } catch {
            AppLog.upload.error("Stored upload receipt could not be read")
            return nil
        }
    }

    func saveReceipt(_ receipt: UploadReceipt) {
        do {
            let data = try encoder.encode(receipt)
            defaults.set(data, forKey: Key.receipt)
        } catch {
            AppLog.upload.error("Upload receipt could not be stored")
        }
    }

    func clearReceipt() {
        defaults.removeObject(forKey: Key.receipt)
    }

    private enum Key {
        static let serverURL = "serverURL"
        static let receipt = "uploadReceipt"
    }
}

final class MemoryCredentialStore: CredentialStoring {
    private var code: String?

    func saveCode(_ code: String) throws {
        self.code = code
    }

    func loadCode() -> String? {
        code
    }

    func deleteCode() throws {
        code = nil
    }
}

final class KeychainCredentialStore: CredentialStoring {
    private let service = "com.bestony.HealthConnect"
    private let account = "verificationCode"

    func saveCode(_ code: String) throws {
        try deleteCode()
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecValueData as String: Data(code.utf8),
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        let status = SecItemAdd(query as CFDictionary, nil)
        guard status == errSecSuccess else {
            AppLog.setup.error("Keychain save failed with status \(status, privacy: .public)")
            throw SettingsSaveError()
        }
    }

    func loadCode() -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        guard status == errSecSuccess, let data = item as? Data else {
            if status != errSecItemNotFound {
                AppLog.setup.error("Keychain read failed with status \(status, privacy: .public)")
            }
            return nil
        }
        return String(data: data, encoding: .utf8)
    }

    func deleteCode() throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            AppLog.setup.error("Keychain delete failed with status \(status, privacy: .public)")
            throw SettingsSaveError()
        }
    }
}

struct SettingsSaveError: Error {}
