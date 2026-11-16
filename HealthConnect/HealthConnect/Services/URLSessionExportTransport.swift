import Foundation
import os

final class URLSessionExportTransport: ExportTransport {
    private let session: URLSession
    private let encoder: JSONEncoder

    init(session: URLSession = URLSessionExportTransport.makeSession()) {
        self.session = session
        self.encoder = ExportJSON.encoder()
    }

    func createExport(endpoint: ServerEndpoint) async throws -> String {
        let body = try encoder.encode(CreateExportBody(startedAt: Date()))
        let (status, data) = try await send(
            endpoint: endpoint,
            path: "v1/exports",
            method: "POST",
            body: body,
            contentType: "application/json"
        )
        guard status == 201 else {
            throw statusError(status)
        }
        do {
            return try JSONDecoder().decode(CreateExportResponse.self, from: data).exportId
        } catch {
            AppLog.upload.error("Create export response was not valid")
            throw UploadError.server(status: status)
        }
    }

    func uploadBatch(endpoint: ServerEndpoint, exportID: String, body: Data) async throws {
        let (status, _) = try await send(
            endpoint: endpoint,
            path: "v1/exports/\(exportID)/batches",
            method: "POST",
            body: body,
            contentType: "application/x-ndjson"
        )
        guard status == 204 else {
            throw statusError(status)
        }
    }

    func completeExport(
        endpoint: ServerEndpoint,
        exportID: String,
        sampleCount: Int,
        batchCount: Int
    ) async throws {
        let body = try encoder.encode(CompleteExportBody(sampleCount: sampleCount, batchCount: batchCount))
        let (status, _) = try await send(
            endpoint: endpoint,
            path: "v1/exports/\(exportID)/complete",
            method: "POST",
            body: body,
            contentType: "application/json"
        )
        guard status == 200 else {
            throw statusError(status)
        }
    }

    private func send(
        endpoint: ServerEndpoint,
        path: String,
        method: String,
        body: Data?,
        contentType: String?
    ) async throws -> (Int, Data) {
        var request = URLRequest(url: endpoint.url(for: path))
        request.httpMethod = method
        request.httpBody = body
        request.setValue("Bearer \(endpoint.verificationCode)", forHTTPHeaderField: "Authorization")
        if let contentType {
            request.setValue(contentType, forHTTPHeaderField: "Content-Type")
        }
        do {
            let (data, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse else {
                throw UploadError.server(status: -1)
            }
            AppLog.upload.info("Server responded \(http.statusCode, privacy: .public) for \(path, privacy: .public)")
            return (http.statusCode, data)
        } catch let error as UploadError {
            throw error
        } catch {
            AppLog.upload.error("Upload request failed for \(path, privacy: .public)")
            throw error
        }
    }

    private func statusError(_ status: Int) -> UploadError {
        if status == 401 {
            return .rejectedCode
        }
        return .server(status: status)
    }

    private static func makeSession() -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 120
        configuration.timeoutIntervalForResource = 300
        configuration.waitsForConnectivity = true
        return URLSession(configuration: configuration)
    }
}

enum ExportJSON {
    static func encoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        encoder.dateEncodingStrategy = .custom { date, encoder in
            var container = encoder.singleValueContainer()
            try container.encode(ExportDate.string(from: date))
        }
        return encoder
    }
}

private struct CreateExportBody: Encodable {
    var startedAt: Date
}

private struct CreateExportResponse: Decodable {
    var exportId: String
}

private struct CompleteExportBody: Encodable {
    var sampleCount: Int
    var batchCount: Int
}
