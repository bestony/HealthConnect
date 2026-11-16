import Foundation

nonisolated struct UploadReceipt: Codable, Equatable, Sendable {
    var identity: String
    var finishedAt: Date
    var sampleCount: Int
    var byteCount: Int
    var batchCount: Int
}

nonisolated enum UploadKind: String, Codable, Equatable, Sendable {
    case full
    case incremental
}

nonisolated enum ExportMode: String, Equatable, Sendable {
    case replace
    case merge

    static func mode(for kind: UploadKind) -> ExportMode {
        kind == .full ? .replace : .merge
    }
}

nonisolated struct UploadProgress: Equatable, Sendable {
    var kind: UploadKind
    var currentTypeIdentifier: String
    var completedUnitCount: Int
    var totalUnitCount: Int
    var sampleCount: Int
    var byteCount: Int
}

nonisolated struct UploadSummary: Equatable, Sendable {
    var exportID: String
    var sampleCount: Int
    var byteCount: Int
    var batchCount: Int
}

nonisolated enum UploadPhase: Equatable, Sendable {
    case checking
    case unavailable
    case authorizationFailed(String)
    case ready(limitedSince: Date?)
    case uploading(UploadProgress)
    case succeeded(UploadReceipt)
    case uploadFailed(String)
    case interrupted(String)
}

nonisolated enum UploadError: Error, Equatable {
    case rejectedCode
    case server(status: Int)
    case incomplete(skipped: Int)
}
