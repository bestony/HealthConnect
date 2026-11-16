import Foundation
import os

protocol UploadHistoryStoring: AnyObject {
    func entries() -> [UploadHistoryEntry]
    func append(_ entry: UploadHistoryEntry)
}

final class UploadHistoryStore: UploadHistoryStoring {
    static let limit = 200

    private let fileURL: URL
    private let encoder: JSONEncoder
    private let decoder: JSONDecoder

    init(fileURL: URL) {
        self.fileURL = fileURL
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        encoder.outputFormatting = [.sortedKeys]
        self.encoder = encoder
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        self.decoder = decoder
    }

    func entries() -> [UploadHistoryEntry] {
        guard let data = try? Data(contentsOf: fileURL) else {
            return []
        }
        do {
            return try decoder.decode([UploadHistoryEntry].self, from: data)
        } catch {
            AppLog.upload.error("Upload history could not be read")
            return []
        }
    }

    func append(_ entry: UploadHistoryEntry) {
        var items = entries()
        items.append(entry)
        if items.count > Self.limit {
            items.removeFirst(items.count - Self.limit)
        }
        do {
            let directory = fileURL.deletingLastPathComponent()
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
            let data = try encoder.encode(items)
            try data.write(to: fileURL, options: .atomic)
            AppLog.upload.info("Recorded upload history \(entry.kind.rawValue, privacy: .public) \(entry.status.rawValue, privacy: .public)")
        } catch {
            AppLog.upload.error("Upload history could not be stored")
        }
    }
}

final class MemoryHistoryStore: UploadHistoryStoring {
    private(set) var items: [UploadHistoryEntry] = []

    func entries() -> [UploadHistoryEntry] {
        items
    }

    func append(_ entry: UploadHistoryEntry) {
        items.append(entry)
        if items.count > UploadHistoryStore.limit {
            items.removeFirst(items.count - UploadHistoryStore.limit)
        }
    }
}
