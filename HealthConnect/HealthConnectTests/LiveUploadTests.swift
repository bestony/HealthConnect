import Foundation
import Testing
@testable import HealthConnect

/// Drives the shipped upload client against a running healthd.
/// The test skips when healthd is down or the live token is not configured.
enum LiveUploadGate {
    /// Skip the live test when healthd is down or the token is not configured.
    static func isReady() async -> Bool {
        guard LiveToken.load() != nil else {
            return false
        }
        let base = ProcessInfo.processInfo.environment["HEALTHCONNECT_LIVE_BASE_URL"] ?? "http://127.0.0.1:8080"
        let root = base.hasSuffix("/") ? String(base.dropLast()) : base
        guard let url = URL(string: root + "/v1/healthz"),
              let health = await LiveHTTP.get(url: url, token: nil) else {
            return false
        }
        return health.status == 200 && health.bodyText.contains("\"status\":\"ok\"")
    }
}

@MainActor
struct LiveUploadTests {
    @Test(.enabled("Runs when healthd is up and a live token is configured.", LiveUploadGate.isReady))
    func shippedUploaderStoresAQuantitySample() async throws {
        let base = ProcessInfo.processInfo.environment["HEALTHCONNECT_LIVE_BASE_URL"] ?? "http://127.0.0.1:8080"
        let token = try #require(LiveToken.load())
        let health = try #require(await LiveHTTP.get(url: url(base, path: "v1/healthz"), token: nil))
        try #require(health.status == 200)
        try #require(health.bodyText.contains("\"status\":\"ok\""))

        let marker = LiveMarker()
        let endpoint = try ServerEndpoint.parse(address: base, verificationCode: token).get()
        #expect(endpoint.url.host == "127.0.0.1" || endpoint.url.host == "localhost")

        LiveExchangeRecorder.reset()
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [LiveExchangeRecorder.self]
        configuration.timeoutIntervalForRequest = 120
        configuration.timeoutIntervalForResource = 300
        let session = URLSession(configuration: configuration)
        let transport = URLSessionExportTransport(session: session)
        let exporter = LivePageExporter(record: marker.record)
        let summary = try await ExportUploader(transport: transport).upload(endpoint: endpoint, exporter: exporter)
        #expect(summary.sampleCount == 1)
        #expect(summary.batchCount == 1)

        let exchanges = LiveExchangeRecorder.snapshot()
        try #require(exchanges.count == 3)
        try expectCreate(exchanges[0])
        expectBatch(exchanges[1])
        try expectComplete(exchanges[2])

        let exportID = try exportID(from: exchanges[0].body)
        let read = try await readRecords(base: base, token: token, marker: marker)
        #expect(read.contains { $0.type == marker.typeIdentifier && $0.value == marker.value && $0.sourceName == marker.source })
        #expect(exportID.allSatisfy { $0.isHexDigit })
    }

    private func expectCreate(_ exchange: LiveExchange) throws {
        #expect(exchange.status == 201)
        #expect(exchange.path == "/v1/exports")
        let id = try exportID(from: exchange.body)
        #expect(id.count == 32)
        #expect(id.allSatisfy { $0.isHexDigit && !$0.isUppercase })
    }

    private func expectBatch(_ exchange: LiveExchange) {
        #expect(exchange.status == 204)
        #expect(exchange.path.hasSuffix("/batches"))
        #expect(exchange.body.isEmpty)
    }

    private func expectComplete(_ exchange: LiveExchange) throws {
        #expect(exchange.status == 200)
        #expect(exchange.path.hasSuffix("/complete"))
        let body = try JSONDecoder().decode(LiveCompleteBody.self, from: exchange.body)
        #expect(body.sampleCount == 1)
        #expect(body.batchCount == 1)
        #expect(body.receivedBatchCount == 1)
        #expect(!body.completedAt.isEmpty)
    }

    private func exportID(from body: Data) throws -> String {
        let decoded = try JSONDecoder().decode(LiveCreateBody.self, from: body)
        return decoded.exportId
    }

    private func readRecords(base: String, token: String, marker: LiveMarker) async throws -> [LiveRecord] {
        var parts = URLComponents(string: url(base, path: "v1/records").absoluteString)
        parts?.queryItems = [
            URLQueryItem(name: "type", value: marker.typeIdentifier),
            URLQueryItem(name: "from", value: marker.queryFrom),
            URLQueryItem(name: "to", value: marker.queryTo),
            URLQueryItem(name: "limit", value: "50"),
        ]
        let target = try #require(parts?.url)
        let response = try #require(await LiveHTTP.get(url: target, token: token))
        try #require(response.status == 200)
        return try JSONDecoder().decode(LiveRecordsBody.self, from: response.body).records
    }

    private func url(_ base: String, path: String) -> URL {
        let root = base.hasSuffix("/") ? String(base.dropLast()) : base
        return URL(string: root + "/" + path)!
    }
}

private struct LiveMarker {
    let typeIdentifier = "HKQuantityTypeIdentifierStepCount"
    let value = 135791357.0
    let source = "com.bestony.HealthConnect.live-test"
    let start = "2026-11-16T04:00:00.000Z"
    let end = "2026-11-16T04:01:00.000Z"
    let queryFrom = "2026-11-16T03:00:00Z"
    let queryTo = "2026-11-16T05:00:00Z"

    var record: ExportRecord {
        ExportRecord(
            kind: "sample",
            typeIdentifier: typeIdentifier,
            uuid: "live-upload-135791357",
            start: start,
            end: end,
            source: source,
            fields: [
                "value": .number(value),
                "unit": .string("count"),
            ]
        )
    }
}

private struct LiveCreateBody: Decodable {
    var exportId: String
}

private struct LiveCompleteBody: Decodable {
    var sampleCount: Int
    var batchCount: Int
    var receivedBatchCount: Int
    var completedAt: String
}

private struct LiveRecordsBody: Decodable {
    var records: [LiveRecord]
}

private struct LiveRecord: Decodable {
    var type: String
    var value: Double?
    var sourceName: String?

    enum CodingKeys: String, CodingKey {
        case type
        case value
        case sourceName = "source_name"
    }
}

private enum LiveToken {
    static func load() -> String? {
        if let value = ProcessInfo.processInfo.environment["HEALTHCONNECT_LIVE_TOKEN"] {
            let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmed.isEmpty {
                return trimmed
            }
        }
        let bundle = Bundle(for: LiveTokenAnchor.self)
        guard let url = bundle.url(forResource: "LiveSecret", withExtension: "txt"),
              let text = try? String(contentsOf: url, encoding: .utf8) else {
            return nil
        }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}

private final class LiveTokenAnchor: NSObject {}

@MainActor
private final class LivePageExporter: HealthExporting {
    let record: ExportRecord
    private var served = false

    init(record: ExportRecord) {
        self.record = record
    }

    var completedUnitCount = 1
    var totalUnitCount = 1
    var currentTypeIdentifier: String { record.typeIdentifier }
    var skippedTypeIdentifiers: [String] { [] }

    func nextPage() async throws -> ExportPage? {
        if served {
            return nil
        }
        served = true
        return ExportPage(typeIdentifier: record.typeIdentifier, records: [record])
    }
}

private struct LiveHTTPResponse {
    var status: Int
    var body: Data

    var bodyText: String {
        String(data: body, encoding: .utf8) ?? ""
    }
}

private enum LiveHTTP {
    static func get(url: URL, token: String?) async -> LiveHTTPResponse? {
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = 10
        if let token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 20
        configuration.timeoutIntervalForResource = 30
        let session = URLSession(configuration: configuration)
        do {
            let (data, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse else {
                return nil
            }
            return LiveHTTPResponse(status: http.statusCode, body: data)
        } catch {
            return nil
        }
    }
}

struct LiveExchange {
    var path: String
    var status: Int
    var body: Data
}

/// Records the real HTTP status and body, then returns that response to the shipped transport.
final class LiveExchangeRecorder: URLProtocol, @unchecked Sendable {
    static let marker = "com.bestony.HealthConnect.LiveExchangeRecorder"
    private static let lock = NSLock()
    private static var exchanges: [LiveExchange] = []
    private var forwardSession: URLSession?
    private var forwardedTask: URLSessionDataTask?

    static func reset() {
        lock.lock()
        exchanges = []
        lock.unlock()
    }

    static func snapshot() -> [LiveExchange] {
        lock.lock()
        defer { lock.unlock() }
        return exchanges
    }

    override class func canInit(with request: URLRequest) -> Bool {
        URLProtocol.property(forKey: marker, in: request) == nil
    }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        let mutable = (request as NSURLRequest).mutableCopy() as! NSMutableURLRequest
        URLProtocol.setProperty(true, forKey: Self.marker, in: mutable)
        let forwarded = mutable as URLRequest
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = []
        configuration.timeoutIntervalForRequest = 120
        let session = URLSession(configuration: configuration)
        self.forwardSession = session
        let task = session.dataTask(with: forwarded) { data, response, error in
            let body = data ?? Data()
            if let error {
                self.client?.urlProtocol(self, didFailWithError: error)
                session.finishTasksAndInvalidate()
                return
            }
            guard let response else {
                self.client?.urlProtocol(self, didFailWithError: URLError(.badServerResponse))
                session.finishTasksAndInvalidate()
                return
            }
            let status = (response as? HTTPURLResponse)?.statusCode ?? -1
            let path = forwarded.url?.path ?? ""
            Self.lock.lock()
            Self.exchanges.append(LiveExchange(path: path, status: status, body: body))
            Self.lock.unlock()
            self.client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            if !body.isEmpty {
                self.client?.urlProtocol(self, didLoad: body)
            }
            self.client?.urlProtocolDidFinishLoading(self)
            session.finishTasksAndInvalidate()
        }
        self.forwardedTask = task
        task.resume()
    }

    override func stopLoading() {
        forwardedTask?.cancel()
        forwardSession?.invalidateAndCancel()
    }
}
