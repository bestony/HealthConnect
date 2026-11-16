import Foundation
import os

protocol AnchorStoring: AnyObject {
    func load(identity: String) -> AnchorCheckpoint?
    func save(_ checkpoint: AnchorCheckpoint, identity: String) throws
}

final class AnchorStore: AnchorStoring {
    private let directory: URL
    private let encoder: JSONEncoder
    private let decoder: JSONDecoder

    init(directory: URL) {
        self.directory = directory
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        encoder.outputFormatting = [.sortedKeys]
        self.encoder = encoder
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        self.decoder = decoder
    }

    func load(identity: String) -> AnchorCheckpoint? {
        let url = fileURL(identity: identity)
        guard let data = try? Data(contentsOf: url) else {
            return nil
        }
        do {
            return try decoder.decode(AnchorCheckpoint.self, from: data)
        } catch {
            AppLog.upload.error("Stored anchors could not be read")
            return nil
        }
    }

    func save(_ checkpoint: AnchorCheckpoint, identity: String) throws {
        let url = fileURL(identity: identity)
        do {
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
            let data = try encoder.encode(checkpoint)
            try data.write(to: url, options: .atomic)
            AppLog.upload.info("Committed anchors for \(checkpoint.anchors.count, privacy: .public) types")
        } catch {
            AppLog.upload.error("Anchors could not be stored")
            throw error
        }
    }

    private func fileURL(identity: String) -> URL {
        directory.appending(path: "\(identity).json")
    }
}

final class MemoryAnchorStore: AnchorStoring {
    private var checkpoints: [String: AnchorCheckpoint] = [:]

    func load(identity: String) -> AnchorCheckpoint? {
        checkpoints[identity]
    }

    func save(_ checkpoint: AnchorCheckpoint, identity: String) throws {
        checkpoints[identity] = checkpoint
    }
}
