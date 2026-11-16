import Foundation
import os

/// One in-flight server export for one saved server identity.
/// `sending` means batches may still be missing. The next upload aborts it.
/// `readyToComplete` means every batch is stored. The next upload calls complete only.
struct OpenExport: Codable, Equatable, Sendable {
    var exportID: String
    var phase: Phase
    var sampleCount: Int
    var batchCount: Int

    enum Phase: String, Codable, Sendable {
        case sending
        case readyToComplete
    }
}

protocol OpenExportStoring: AnyObject {
    func load(identity: String) -> OpenExport?
    func save(_ record: OpenExport, identity: String) throws
    func delete(identity: String)
}

/// Persists at most one open export for each server identity.
final class OpenExportStore: OpenExportStoring {
    private let fileURL: URL
    private let encoder: JSONEncoder
    private let decoder: JSONDecoder

    init(fileURL: URL) {
        self.fileURL = fileURL
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        self.encoder = encoder
        self.decoder = JSONDecoder()
    }

    func load(identity: String) -> OpenExport? {
        records()[identity]
    }

    func save(_ record: OpenExport, identity: String) throws {
        var items = records()
        items[identity] = record
        try write(items)
    }

    func delete(identity: String) {
        var items = records()
        guard items.removeValue(forKey: identity) != nil else {
            return
        }
        do {
            try write(items)
        } catch {
            AppLog.upload.error("Open export record could not be removed")
        }
    }

    private func records() -> [String: OpenExport] {
        guard let data = try? Data(contentsOf: fileURL) else {
            return [:]
        }
        do {
            return try decoder.decode([String: OpenExport].self, from: data)
        } catch {
            AppLog.upload.error("Open export record could not be read")
            return [:]
        }
    }

    private func write(_ items: [String: OpenExport]) throws {
        let directory = fileURL.deletingLastPathComponent()
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let data = try encoder.encode(items)
        try data.write(to: fileURL, options: .atomic)
    }
}

final class MemoryOpenExportStore: OpenExportStoring {
    private var items: [String: OpenExport] = [:]

    func load(identity: String) -> OpenExport? {
        items[identity]
    }

    func save(_ record: OpenExport, identity: String) throws {
        items[identity] = record
    }

    func delete(identity: String) {
        items.removeValue(forKey: identity)
    }
}
