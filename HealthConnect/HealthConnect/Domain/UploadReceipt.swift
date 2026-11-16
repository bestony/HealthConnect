import Foundation

nonisolated struct UploadReceipt: Codable, Equatable, Sendable {
    var identity: String
    var finishedAt: Date
    var sampleCount: Int
    var byteCount: Int
    var batchCount: Int
}

nonisolated struct UploadProgress: Equatable, Sendable {
    var currentTypeIdentifier: String
    var completedUnitCount: Int
    var totalUnitCount: Int
    var sampleCount: Int
}

nonisolated struct UploadSummary: Equatable, Sendable {
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
}

nonisolated enum UploadError: Error, Equatable {
    case rejectedCode
    case server(status: Int)
    case incomplete(skipped: Int)
}
