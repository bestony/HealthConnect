import CryptoKit
import Foundation

nonisolated struct ServerEndpoint: Equatable, Sendable {
    var url: URL
    var verificationCode: String

    /// Stable id for the saved server and code. This is not the code.
    var identity: String {
        let material = Data("\(url.absoluteString)\n\(verificationCode)".utf8)
        let digest = SHA256.hash(data: material)
        return digest.map { String(format: "%02x", $0) }.joined()
    }

    func url(for path: String) -> URL {
        url.appending(path: path)
    }

    static func parse(address: String, verificationCode: String) -> Result<ServerEndpoint, EndpointValidationError> {
        let code = verificationCode.trimmingCharacters(in: .whitespacesAndNewlines)
        if code.isEmpty {
            return .failure(.emptyCode)
        }
        if code.count > 256 || code.contains(where: \.isNewline) {
            return .failure(.invalidCode)
        }

        let trimmed = address.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty {
            return .failure(.emptyAddress)
        }
        guard let components = URLComponents(string: trimmed), let url = components.url else {
            return .failure(.invalidURL)
        }
        guard components.user == nil, components.password == nil else {
            return .failure(.invalidURL)
        }
        guard let scheme = components.scheme?.lowercased(), let host = components.host, !host.isEmpty else {
            return .failure(.invalidURL)
        }
        if scheme != "https" && scheme != "http" {
            return .failure(.unsupportedScheme)
        }
        if scheme == "http" && !isLocalHost(host) {
            return .failure(.publicHTTP)
        }
        return .success(ServerEndpoint(url: url, verificationCode: code))
    }

    private static func isLocalHost(_ host: String) -> Bool {
        let name = host.lowercased()
        if name == "localhost" || name == "::1" || name.hasSuffix(".local") {
            return true
        }
        let parts = name.split(separator: ".")
        guard parts.count == 4, parts.allSatisfy({ $0.allSatisfy(\.isNumber) }) else {
            return false
        }
        guard let first = Int(parts[0]), let second = Int(parts[1]),
              (0...255).contains(first), (0...255).contains(second) else {
            return false
        }
        if first == 10 || first == 127 {
            return true
        }
        if first == 192 && second == 168 {
            return true
        }
        if first == 172 && (16...31).contains(second) {
            return true
        }
        return false
    }
}

nonisolated enum EndpointValidationError: Equatable, Error {
    case emptyAddress
    case emptyCode
    case invalidCode
    case invalidURL
    case unsupportedScheme
    case publicHTTP

    var message: String {
        switch self {
        case .emptyAddress:
            "请输入服务器地址。"
        case .emptyCode:
            "请输入验证码。"
        case .invalidCode:
            "验证码无效。"
        case .invalidURL:
            "服务器地址无效。"
        case .unsupportedScheme:
            "请输入以 http:// 或 https:// 开头的地址。"
        case .publicHTTP:
            "公网地址请使用 https。"
        }
    }
}
