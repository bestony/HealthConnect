import CryptoKit
import Foundation

nonisolated struct AnchorCheckpoint: Codable, Equatable, Sendable {
    var anchors: [String: Data]
    var supplementalHash: String?
}

nonisolated struct UploadHistoryEntry: Codable, Equatable, Identifiable, Sendable {
    var id: UUID
    var finishedAt: Date
    var host: String
    var kind: UploadKind
    var status: UploadHistoryStatus
    var sampleCount: Int
    var byteCount: Int
    var batchCount: Int
    var exportID: String?
    var message: String?

    var kindLabel: String {
        kind == .full ? "全量" : "增量"
    }

    var statusLabel: String {
        switch status {
        case .succeeded:
            "成功"
        case .failed:
            "失败"
        case .interrupted:
            "已中断"
        }
    }
}

nonisolated enum UploadHistoryStatus: String, Codable, Equatable, Sendable {
    case succeeded
    case failed
    case interrupted
}

enum AnchorCommit {
    /// Returns the map to store after a successful complete. A failed upload returns nil and the caller keeps the previous map.
    static func map(kind: UploadKind, previous: [String: Data], finished: [String: Data], succeeded: Bool) -> [String: Data]? {
        guard succeeded else {
            return nil
        }
        if kind == .full {
            return finished
        }
        var merged = previous
        for (type, anchor) in finished {
            merged[type] = anchor
        }
        return merged
    }
}

enum SupplementalDigest {
    static func hash(_ records: [ExportRecord]) -> String {
        let data = (try? ExportJSON.encoder().encode(records)) ?? Data()
        let digest = SHA256.hash(data: data)
        return digest.map { String(format: "%02x", $0) }.joined()
    }
}
