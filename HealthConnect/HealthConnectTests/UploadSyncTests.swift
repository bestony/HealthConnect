import Foundation
import HealthKit
import Testing
@testable import HealthConnect

struct AnchorCommitTests {
    @Test func failureKeepsThePreviousMapAndFullReplacesIt() {
        let previous = ["steps": Data("old".utf8)]
        let finished = ["steps": Data("new".utf8), "heart": Data("h".utf8)]
        #expect(AnchorCommit.map(kind: .full, previous: previous, finished: finished, succeeded: false) == nil)
        #expect(AnchorCommit.map(kind: .incremental, previous: previous, finished: finished, succeeded: false) == nil)
        #expect(AnchorCommit.map(kind: .full, previous: previous, finished: finished, succeeded: true) == finished)
    }

    @Test func incrementalKeepsTypesThatDidNotFinish() {
        let previous = ["steps": Data("old".utf8), "heart": Data("h".utf8)]
        let finished = ["steps": Data("new".utf8)]
        #expect(AnchorCommit.map(kind: .incremental, previous: previous, finished: finished, succeeded: true) == [
            "steps": Data("new".utf8),
            "heart": Data("h".utf8),
        ])
    }
}

struct SupplementalDigestTests {
    @Test func equalRecordsHaveTheSameDigest() {
        let record = ExportRecord(kind: "activitySummary", typeIdentifier: "HKActivitySummaryTypeIdentifier", fields: ["day": .int(2)])
        #expect(SupplementalDigest.hash([record]) == SupplementalDigest.hash([record]))
        #expect(SupplementalDigest.hash([record]) != SupplementalDigest.hash([]))
    }
}

@MainActor
struct IncrementalUploadTests {
    @Test func aWakeBeforeSetupDoesNotWriteHistory() async {
        let settings = makeSettings()
        let transport = RecordingTransport()
        let session = makeSyncSession(settings: settings, transport: transport, finished: [:], records: [])
        await session.performUpload(kind: .incremental)
        #expect(transport.events.isEmpty)
        #expect(session.history.isEmpty)
    }

    @Test func aWakeWithoutAnchorsDoesNotCreateAnExport() async throws {
        let settings = makeSettings()
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "code").get()
        try settings.save(endpoint)
        let transport = RecordingTransport()
        let session = makeSyncSession(settings: settings, transport: transport, finished: ["steps": Data("new".utf8)], records: [
            ExportRecord(kind: "sample", typeIdentifier: "HKQuantityTypeIdentifierStepCount", fields: ["value": .int(1)]),
        ])
        await session.reload()
        await session.performUpload(kind: .incremental)
        #expect(transport.events.isEmpty)
        #expect(session.history.isEmpty)
    }

    @Test func aWakeWithAnchorsUploadsAMergeAndCommitsOnlyAfterSuccess() async throws {
        let settings = makeSettings()
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "secret-code").get()
        try settings.save(endpoint)
        let anchors = MemoryAnchorStore()
        try anchors.save(AnchorCheckpoint(anchors: ["steps": Data("old".utf8)], supplementalHash: nil), identity: endpoint.identity)
        let transport = RecordingTransport()
        let session = makeSyncSession(
            settings: settings,
            transport: transport,
            anchorStore: anchors,
            finished: ["steps": Data("new".utf8)],
            records: [
                ExportRecord(kind: "deleted", typeIdentifier: "HKQuantityTypeIdentifierStepCount", uuid: "E621E1F8-C36C-495A-93FC-0C247A3E6E5F"),
            ]
        )
        await session.reload()
        await session.performUpload(kind: .incremental)
        #expect(transport.events == ["create:merge", "batch", "complete"])
        #expect(anchors.load(identity: endpoint.identity)?.anchors["steps"] == Data("new".utf8))
        #expect(session.history.count == 1)
        #expect(session.history[0].kind == .incremental)
        #expect(session.history[0].host == "example.com")
        let encoded = String(data: try JSONEncoder().encode(session.history), encoding: .utf8) ?? ""
        #expect(!encoded.contains("secret-code"))

        let failed = RecordingTransport()
        failed.error = .rejectedCode
        let failedSession = makeSyncSession(
            settings: settings,
            transport: failed,
            anchorStore: anchors,
            finished: ["steps": Data("newer".utf8)],
            records: [ExportRecord(kind: "sample", typeIdentifier: "HKQuantityTypeIdentifierStepCount")]
        )
        await failedSession.reload()
        await failedSession.performUpload(kind: .incremental)
        #expect(anchors.load(identity: endpoint.identity)?.anchors["steps"] == Data("new".utf8))
        #expect(failedSession.history.contains { $0.status == .failed })
    }

    @Test func anEmptyIncrementalDoesNotCreateAnExport() async throws {
        let settings = makeSettings()
        let endpoint = try ServerEndpoint.parse(address: "https://example.com", verificationCode: "code").get()
        try settings.save(endpoint)
        let anchors = MemoryAnchorStore()
        try anchors.save(AnchorCheckpoint(anchors: ["steps": Data("old".utf8)], supplementalHash: nil), identity: endpoint.identity)
        let transport = RecordingTransport()
        let session = makeSyncSession(settings: settings, transport: transport, anchorStore: anchors, finished: [:], records: [])
        await session.reload()
        await session.performUpload(kind: .incremental)
        #expect(transport.events.isEmpty)
        #expect(session.history.isEmpty)
        #expect(anchors.load(identity: endpoint.identity)?.anchors["steps"] == Data("old".utf8))
    }

    @Test func historyDropsTheOldestEntryPastTheLimit() {
        let store = MemoryHistoryStore()
        for index in 0..<(UploadHistoryStore.limit + 2) {
            store.append(UploadHistoryEntry(
                id: UUID(),
                finishedAt: Date(timeIntervalSince1970: TimeInterval(index)),
                host: "example.com",
                kind: .full,
                status: .succeeded,
                sampleCount: index,
                byteCount: 0,
                batchCount: 0,
                exportID: nil,
                message: nil
            ))
        }
        let entries = store.entries()
        #expect(entries.count == UploadHistoryStore.limit)
        #expect(entries.first?.sampleCount == 2)
        #expect(entries.last?.sampleCount == UploadHistoryStore.limit + 1)
    }

    private func makeSettings() -> SettingsStore {
        let suite = "HealthConnectTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        return SettingsStore(defaults: defaults, credentials: MemoryCredentialStore())
    }

    private func makeSyncSession(
        settings: SettingsStore,
        transport: RecordingTransport,
        anchorStore: MemoryAnchorStore = MemoryAnchorStore(),
        finished: [String: Data],
        records: [ExportRecord]
    ) -> UploadSession {
        let client = ScriptedAuthorizationClient()
        client.status = .unnecessary
        return UploadSession(
            settings: settings,
            access: HealthAccessCoordinator(client: client),
            anchors: anchorStore,
            history: MemoryHistoryStore(),
            makeExporter: { _ in
                ScriptedExporter(skipped: [], records: records, finishedAnchors: finished)
            },
            makeUploader: { ExportUploader(transport: transport) }
        )
    }
}

@MainActor
struct ObserverQuerySupportTests {
    @Test func stepCountCanBeObservedAndTheCatalogProbeDoesNotThrow() {
        let steps = HKQuantityType(HKQuantityTypeIdentifier(rawValue: "HKQuantityTypeIdentifierStepCount"))
        #expect(ObserverQuerySupport.allows(steps))
        let sampleTypes = HealthTypeCatalog.sampleTypes
        let supported = sampleTypes.filter { ObserverQuerySupport.allows($0) }
        #expect(!supported.isEmpty)
        #expect(supported.count <= sampleTypes.count)
    }
}
