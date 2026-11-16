import Foundation
import HealthKit
import Testing
@testable import HealthConnect

struct ServerEndpointTests {
    @Test func acceptsHTTPSAndPrivateHTTP() throws {
        let secure = try ServerEndpoint.parse(address: " https://example.com/base ", verificationCode: " code ").get()
        #expect(secure.url.absoluteString == "https://example.com/base")
        #expect(secure.verificationCode == "code")
        #expect(secure.url(for: "v1/exports").absoluteString == "https://example.com/base/v1/exports")

        let local = try ServerEndpoint.parse(address: "http://192.168.1.20:8080", verificationCode: "secret").get()
        #expect(local.url.host == "192.168.1.20")
        let loopback = try ServerEndpoint.parse(address: "http://127.0.0.1:8080", verificationCode: "secret").get()
        #expect(loopback.url.host == "127.0.0.1")
        let localName = try ServerEndpoint.parse(address: "http://health.local:8080", verificationCode: "secret").get()
        #expect(localName.url.host == "health.local")
        let ipv6Loopback = try ServerEndpoint.parse(address: "http://[::1]:8080", verificationCode: "secret").get()
        #expect(ipv6Loopback.url.host == "::1")
        #expect(ipv6Loopback.url(for: "v1/exports").absoluteString == "http://[::1]:8080/v1/exports")
        let expanded = try ServerEndpoint.parse(address: "http://[0:0:0:0:0:0:0:1]:8080", verificationCode: "secret").get()
        #expect(expanded.url.host == "0:0:0:0:0:0:0:1")
    }

    @Test func rejectsMissingHostBadSchemeAndPublicHTTP() {
        #expect(ServerEndpoint.parse(address: "", verificationCode: "code") == .failure(.emptyAddress))
        #expect(ServerEndpoint.parse(address: "https://example.com", verificationCode: " ") == .failure(.emptyCode))
        #expect(ServerEndpoint.parse(address: "not a url", verificationCode: "code") == .failure(.invalidURL))
        #expect(ServerEndpoint.parse(address: "ftp://example.com", verificationCode: "code") == .failure(.unsupportedScheme))
        #expect(ServerEndpoint.parse(address: "http://example.com", verificationCode: "code") == .failure(.publicHTTP))
        #expect(ServerEndpoint.parse(address: "https://user:pass@example.com", verificationCode: "code") == .failure(.invalidURL))
        #expect(ServerEndpoint.parse(address: "", verificationCode: "") == .failure(.emptyAddress))
    }
}

@MainActor
struct SettingsStoreTests {
    @Test func saveKeepsTheCodeOutOfDefaultsAndClearsTheReceiptWhenTheIdentityChanges() throws {
        let suite = "HealthConnectTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        let credentials = MemoryCredentialStore()
        let store = SettingsStore(defaults: defaults, credentials: credentials)
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "secret-code-xyz").get()

        try store.save(endpoint)
        let loaded = store.load()
        #expect(loaded?.url == endpoint.url)
        #expect(loaded?.verificationCode == "secret-code-xyz")

        let snapshot = defaults.dictionaryRepresentation().description
        #expect(!snapshot.contains("secret-code-xyz"))

        let receipt = UploadReceipt(identity: endpoint.identity, finishedAt: Date(), sampleCount: 3, byteCount: 10, batchCount: 1)
        store.saveReceipt(receipt)
        #expect(store.receipt()?.identity == endpoint.identity)

        let changed = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "other-code").get()
        try store.save(changed)
        #expect(store.receipt() == nil)
        defaults.removePersistentDomain(forName: suite)
    }
}

@MainActor
struct HealthAccessTests {
    @Test func shouldRequestAsksOnceAndUnnecessaryDoesNotAsk() async {
        let client = ScriptedAuthorizationClient()
        client.status = .shouldRequest
        let coordinator = HealthAccessCoordinator(client: client)
        let requested = await coordinator.prepare()
        #expect(client.requestCount == 1)
        #expect(requested == .determined(limitedSince: nil))

        client.status = .unnecessary
        client.limited = Date(timeIntervalSince1970: 1_700_000_000)
        let ready = await coordinator.prepare()
        #expect(client.requestCount == 1)
        #expect(ready == .determined(limitedSince: client.limited))
    }

    @Test func unavailableDoesNotAsk() async {
        let client = ScriptedAuthorizationClient()
        client.available = false
        let state = await HealthAccessCoordinator(client: client).prepare()
        #expect(state == .unavailable)
        #expect(client.requestCount == 0)
    }
}

@MainActor
struct AuthorizationDenialTests {
    @Test func readsDisallowedTypeNames() {
        let reason = "Authorization to read the following types is disallowed: HKVisionPrescriptionTypeIdentifier, HKCorrelationTypeIdentifierFood"
        let names = AuthorizationDenial.identifiers(in: reason)
        #expect(names == Set(["HKVisionPrescriptionTypeIdentifier", "HKCorrelationTypeIdentifierFood"]))
    }
}

@MainActor
struct CatalogTests {
    @Test func catalogIncludesSupportedTypesAndExcludesClinicalRecordsAndRoutes() {
        let names = HealthTypeCatalog.declaredIdentifiers
        #expect(Set(names).count == names.count)
        #expect(names.contains("HKQuantityTypeIdentifierStepCount"))
        #expect(names.allSatisfy { !$0.localizedCaseInsensitiveContains("Clinical") })
        #expect(names.allSatisfy { !$0.localizedCaseInsensitiveContains("WorkoutRoute") })

        #expect(HealthTypeCatalog.readTypes.count > HealthTypeCatalog.quantityIdentifiers.count)
        #expect(HealthTypeCatalog.readTypes.contains { $0.identifier.contains("StepCount") })
        let identifiers = HealthTypeCatalog.readTypes.map(\.identifier)
        #expect(identifiers.allSatisfy { !$0.localizedCaseInsensitiveContains("Clinical") })
        #expect(identifiers.allSatisfy { !$0.localizedCaseInsensitiveContains("WorkoutRoute") })
    }
}

@MainActor
struct ExportEncodingTests {
    @Test func quantityPageKeepsMetadata() async throws {
        let type = HKQuantityType(HKQuantityTypeIdentifier(rawValue: "HKQuantityTypeIdentifierStepCount"))
        let start = Date(timeIntervalSince1970: 1_700_000_000)
        let sample = HKQuantitySample(
            type: type,
            quantity: HKQuantity(unit: .count(), doubleValue: 42),
            start: start,
            end: start.addingTimeInterval(60),
            metadata: ["note": "keep-me"]
        )
        let record = await HealthSampleEncoder.record(for: sample, store: HKHealthStore())
        #expect(record.fields["value"] == .number(42))
        #expect(record.fields["unit"] == .string(HKUnit.count().unitString))
        #expect(record.metadata["note"] == .string("keep-me"))

        let body = try ExportPageEncoder.encode([record])
        let text = try #require(String(data: body, encoding: .utf8))
        #expect(text.contains("\"note\":\"keep-me\""))
        #expect(text.hasSuffix("\n"))
    }

    @Test func recordKindStaysSampleWhenAFieldUsesTheSameName() throws {
        let record = ExportRecord(
            kind: "sample",
            typeIdentifier: "HKStateOfMindType",
            fields: ["kind": .string("momentaryEmotion"), "valence": .number(0.2)]
        )
        let text = try #require(String(data: ExportPageEncoder.encode([record]), encoding: .utf8))
        #expect(text.contains("\"kind\":\"sample\""))
        #expect(text.contains("\"field_kind\":\"momentaryEmotion\""))
        #expect(!text.contains("\"kind\":\"momentaryEmotion\""))
    }

    @Test func startedAtUsesFractionalSeconds() throws {
        struct Body: Encodable { var startedAt: Date }
        let data = try ExportJSON.encoder().encode(Body(startedAt: Date(timeIntervalSince1970: 1_767_225_600)))
        let text = try #require(String(data: data, encoding: .utf8))
        #expect(text.contains("2026-01-01T00:00:00.000Z"))
    }

    @Test func oneLargeArrayStaysWithinTheBatchLimit() throws {
        let items = (0..<20).map { _ in ExportValue.string(String(repeating: "a", count: 40)) }
        let record = ExportRecord(
            kind: "sample",
            typeIdentifier: "HKElectrocardiogramType",
            fields: ["voltages": .array(items)]
        )
        let groups = try ExportBatchSizer.groups(records: [record], maxBytes: 350)
        #expect(groups.count > 1)
        #expect(groups.allSatisfy { $0.count <= 350 })
    }

    @Test func oneUnsplittableRecordIsRejected() {
        let record = ExportRecord(
            kind: "sample",
            typeIdentifier: "HKQuantityTypeIdentifierStepCount",
            fields: ["note": .string(String(repeating: "a", count: 200))]
        )
        do {
            _ = try ExportBatchSizer.groups(records: [record], maxBytes: 80)
            Issue.record("Expected an oversized record to fail")
        } catch ExportBatchError.recordTooLarge {
        } catch {
            Issue.record("Unexpected error \(error)")
        }
    }
}

@MainActor
struct UploadFlowTests {
    @Test func successLocksTheButtonAndFailureDoesNot() async throws {
        let settings = makeSettings()
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "code").get()
        try settings.save(endpoint)
        let transport = RecordingTransport()
        let session = makeSession(settings: settings, transport: transport, skipped: [])

        await session.reload()
        #expect(session.canUpload)
        await session.performUpload()
        guard case .succeeded = session.phase else {
            Issue.record("Expected success, got \(session.phase)")
            return
        }
        #expect(session.canUpload)
        #expect(transport.events == ["create:replace", "batch", "complete"])
        #expect(session.history.count == 1)
        #expect(session.history[0].status == .succeeded)
        #expect(session.history[0].kind == .full)

        let reloaded = makeSession(settings: settings, transport: RecordingTransport(), skipped: [])
        await reloaded.reload()
        #expect(reloaded.canUpload)

        let changed = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "next").get()
        try settings.save(changed)
        let again = makeSession(settings: settings, transport: RecordingTransport(), skipped: [])
        await again.reload()
        #expect(again.canUpload)
    }

    @Test func rejectedCodeAndSkippedTypesLeaveTheButtonEnabled() async throws {
        let settings = makeSettings()
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "code").get()
        try settings.save(endpoint)

        let rejected = RecordingTransport()
        rejected.error = UploadError.rejectedCode
        let rejectedSession = makeSession(settings: settings, transport: rejected, skipped: [])
        await rejectedSession.reload()
        await rejectedSession.performUpload()
        #expect(rejectedSession.phase == .uploadFailed("验证码被服务器拒绝。"))
        #expect(rejectedSession.canUpload)
        #expect(settings.receipt() == nil)
        #expect(rejected.events == ["create:replace"])

        let skipped = RecordingTransport()
        let skippedSession = makeSession(settings: settings, transport: skipped, skipped: ["HKQuantityTypeIdentifierStepCount"])
        await skippedSession.reload()
        await skippedSession.performUpload()
        #expect(skippedSession.canUpload)
        #expect(settings.receipt() == nil)
        #expect(skipped.events == ["create:replace", "batch", "abort"])
    }

    @Test func cancellationAbortsTheOpenExport() async throws {
        let settings = makeSettings()
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "code").get()
        try settings.save(endpoint)
        let transport = RecordingTransport()
        transport.cancelBatch = true
        let session = makeSession(settings: settings, transport: transport, skipped: [])
        await session.reload()
        await session.performUpload()
        #expect(session.phase == .interrupted("全量上传未完成。回到应用后可以重新上传。"))
        #expect(session.canUpload)
        #expect(transport.events == ["create:replace", "batch", "abort"])
        #expect(transport.abortedIDs == ["abc"])
    }

    @Test func aStoredSendingExportIsAbortedBeforeCreate() async throws {
        let settings = makeSettings()
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "code").get()
        try settings.save(endpoint)
        let store = MemoryOpenExportStore()
        try store.save(
            OpenExport(exportID: "old-export", phase: .sending, sampleCount: 1, batchCount: 1),
            identity: endpoint.identity
        )
        let transport = RecordingTransport()
        let session = makeSession(settings: settings, transport: transport, skipped: [], openExports: store)
        await session.reload()
        await session.performUpload()
        guard case .succeeded = session.phase else {
            Issue.record("Expected success, got \(session.phase)")
            return
        }
        #expect(transport.events == ["abort", "create:replace", "batch", "complete"])
        #expect(transport.abortedIDs == ["old-export"])
        #expect(store.load(identity: endpoint.identity) == nil)
    }

    @Test func aStoredCompleteIsRetriedWithoutANewExport() async throws {
        let settings = makeSettings()
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "code").get()
        try settings.save(endpoint)
        let store = MemoryOpenExportStore()
        let transport = RecordingTransport()
        transport.completeError = UploadError.server(status: 500)
        let session = makeSession(settings: settings, transport: transport, skipped: [], openExports: store)
        await session.reload()
        await session.performUpload()
        #expect(session.phase == .uploadFailed("上传失败。可以重试。"))
        #expect(transport.events == ["create:replace", "batch", "complete"])
        #expect(store.load(identity: endpoint.identity)?.phase == .readyToComplete)
        #expect(store.load(identity: endpoint.identity)?.sampleCount == 1)
        #expect(store.load(identity: endpoint.identity)?.batchCount == 1)

        transport.completeError = nil
        await session.performUpload()
        guard case .succeeded = session.phase else {
            Issue.record("Expected success, got \(session.phase)")
            return
        }
        #expect(transport.events == ["create:replace", "batch", "complete", "complete"])
        #expect(transport.completedSampleCount == 1)
        #expect(transport.completedBatchCount == 1)
        #expect(store.load(identity: endpoint.identity) == nil)
    }

    @Test func anOversizedRecordDoesNotCompleteOrLock() async throws {
        let settings = makeSettings()
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "code").get()
        try settings.save(endpoint)
        let transport = RecordingTransport()
        let client = ScriptedAuthorizationClient()
        client.status = .unnecessary
        let openExports = MemoryOpenExportStore()
        let session = UploadSession(
            settings: settings,
            access: HealthAccessCoordinator(client: client),
            anchors: MemoryAnchorStore(),
            history: MemoryHistoryStore(),
            openExports: openExports,
            makeExporter: { _ in
                ScriptedExporter(skipped: [], records: [
                    ExportRecord(
                        kind: "sample",
                        typeIdentifier: "HKQuantityTypeIdentifierStepCount",
                        fields: ["note": .string(String(repeating: "a", count: 200))]
                    ),
                ])
            },
            makeUploader: {
                var uploader = ExportUploader(transport: transport, openExports: openExports)
                uploader.maxBatchBytes = 80
                return uploader
            }
        )
        await session.reload()
        await session.performUpload()
        #expect(session.phase == .uploadFailed("有一条记录超过上传大小限制。上传未完成。"))
        #expect(session.canUpload)
        #expect(settings.receipt() == nil)
        #expect(transport.events == ["create:replace", "abort"])
    }

    private func makeSettings() -> SettingsStore {
        let suite = "HealthConnectTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        return SettingsStore(defaults: defaults, credentials: MemoryCredentialStore())
    }

    private func makeSession(
        settings: SettingsStore,
        transport: RecordingTransport,
        skipped: [String],
        openExports: OpenExportStoring? = nil
    ) -> UploadSession {
        let client = ScriptedAuthorizationClient()
        client.status = .unnecessary
        let store = openExports ?? MemoryOpenExportStore()
        return UploadSession(
            settings: settings,
            access: HealthAccessCoordinator(client: client),
            anchors: MemoryAnchorStore(),
            history: MemoryHistoryStore(),
            openExports: store,
            makeExporter: { _ in ScriptedExporter(skipped: skipped) },
            makeUploader: { ExportUploader(transport: transport, openExports: store) }
        )
    }
}

@MainActor
final class ScriptedAuthorizationClient: HealthAuthorizationClient {
    var available = true
    var status: HealthAuthorizationRequestStatus = .shouldRequest
    var requestCount = 0
    var limited: Date?

    func isHealthDataAvailable() -> Bool {
        available
    }

    func authorizationRequestStatus() async throws -> HealthAuthorizationRequestStatus {
        status
    }

    func requestReadAuthorization() async throws {
        requestCount += 1
    }

    func earliestLimitedReadDate() async throws -> Date? {
        limited
    }
}

@MainActor
final class ScriptedExporter: HealthExporting {
    var completedUnitCount = 1
    var totalUnitCount = 1
    var currentTypeIdentifier = "HKQuantityTypeIdentifierStepCount"
    var skippedTypeIdentifiers: [String]
    var finishedAnchors: [String: Data]
    var supplementalHash: String?
    private let records: [ExportRecord]
    private var served = false

    init(skipped: [String], records: [ExportRecord]? = nil, finishedAnchors: [String: Data] = [:]) {
        skippedTypeIdentifiers = skipped
        self.finishedAnchors = finishedAnchors
        self.records = records ?? [
            ExportRecord(kind: "sample", typeIdentifier: "HKQuantityTypeIdentifierStepCount", fields: ["value": .int(1)]),
        ]
    }

    func nextPage() async throws -> ExportPage? {
        if served {
            return nil
        }
        served = true
        return ExportPage(typeIdentifier: currentTypeIdentifier, records: records)
    }
}

@MainActor
final class RecordingTransport: ExportTransport {
    var events: [String] = []
    var abortedIDs: [String] = []
    var error: UploadError?
    var completeError: UploadError?
    var cancelBatch = false
    var completedSampleCount = 0
    var completedBatchCount = 0

    func createExport(endpoint: ServerEndpoint, mode: ExportMode) async throws -> String {
        events.append("create:\(mode.rawValue)")
        if let error {
            throw error
        }
        return "abc"
    }

    func uploadBatch(endpoint: ServerEndpoint, exportID: String, body: Data) async throws {
        events.append("batch")
        if cancelBatch {
            throw CancellationError()
        }
    }

    func completeExport(
        endpoint: ServerEndpoint,
        exportID: String,
        sampleCount: Int,
        batchCount: Int
    ) async throws {
        events.append("complete")
        completedSampleCount = sampleCount
        completedBatchCount = batchCount
        if let completeError {
            throw completeError
        }
    }

    func abortExport(endpoint: ServerEndpoint, exportID: String) async throws {
        events.append("abort")
        abortedIDs.append(exportID)
    }
}
