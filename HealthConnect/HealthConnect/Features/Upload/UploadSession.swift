import Foundation
import Observation
import os

@MainActor
@Observable
final class UploadSession {
    private let settings: SettingsStoring
    private let access: HealthAccessCoordinator
    private let anchors: AnchorStoring
    private let historyStore: UploadHistoryStoring
    private let background: BackgroundTaskControlling
    private let makeExporter: (UploadKind) -> HealthExporting
    private let makeUploader: () -> ExportUploader
    private var reloadTask: Task<Void, Never>?
    private var uploadTask: Task<Void, Never>?
    private var observerCompletions: [() -> Void] = []
    private var limitedSince: Date?

    private(set) var phase: UploadPhase = .checking
    private(set) var history: [UploadHistoryEntry] = []

    init(
        settings: SettingsStoring,
        access: HealthAccessCoordinator,
        anchors: AnchorStoring,
        history: UploadHistoryStoring,
        background: BackgroundTaskControlling? = nil,
        makeExporter: @escaping (UploadKind) -> HealthExporting,
        makeUploader: @escaping () -> ExportUploader
    ) {
        self.settings = settings
        self.access = access
        self.anchors = anchors
        self.historyStore = history
        self.background = background ?? NoopBackgroundTasks()
        self.makeExporter = makeExporter
        self.makeUploader = makeUploader
        self.history = history.entries().reversed()
    }

    var canUpload: Bool {
        switch phase {
        case .ready, .uploadFailed, .succeeded, .interrupted:
            settings.load() != nil
        default:
            false
        }
    }

    var canRequestAuthorization: Bool {
        if case .authorizationFailed = phase {
            return true
        }
        return false
    }

    var hasCommittedAnchors: Bool {
        guard let identity = settings.load()?.endpoint.identity else {
            return false
        }
        return anchors.load(identity: identity)?.anchors.isEmpty == false
    }

    func reload() async {
        if case .uploading = phase {
            return
        }
        if let reloadTask {
            await reloadTask.value
            return
        }
        let task = Task { @MainActor in
            await self.prepare()
        }
        reloadTask = task
        await task.value
        reloadTask = nil
    }

    func startUpload() {
        guard canUpload, uploadTask == nil else {
            return
        }
        uploadTask = Task {
            await performUpload(kind: .full)
            uploadTask = nil
            finishObservers()
        }
    }

    func noteHealthUpdate(completion: @escaping () -> Void) {
        observerCompletions.append(completion)
        guard uploadTask == nil else {
            AppLog.upload.info("Health update is waiting for the current upload")
            return
        }
        uploadTask = Task {
            await performUpload(kind: .incremental)
            uploadTask = nil
            finishObservers()
        }
    }

    func sceneLeftForeground() {
        guard case .uploading = phase else {
            return
        }
        background.begin { [weak self] in
            self?.expireBackgroundUpload()
        }
    }

    func expireBackgroundUpload() {
        guard case .uploading = phase else {
            return
        }
        AppLog.upload.info("Upload stopped because background time expired")
        uploadTask?.cancel()
    }

    func performUpload(kind: UploadKind = .full) async {
        guard let saved = settings.load() else {
            if kind == .incremental {
                AppLog.upload.info("Incremental upload skipped because settings are missing")
                return
            }
            phase = .uploadFailed("请先保存服务器地址和验证码。")
            record(kind: kind, status: .failed, summary: nil, exportID: nil, message: "请先保存服务器地址和验证码。", host: "")
            return
        }
        if kind == .incremental && !hasCommittedAnchors {
            AppLog.upload.info("Incremental upload skipped because no anchor is committed")
            return
        }
        let exporter = makeExporter(kind)
        var uploader = makeUploader()
        uploader.onProgress = { [weak self] progress in
            self?.phase = .uploading(progress)
        }
        phase = .uploading(UploadProgress(
            kind: kind,
            currentTypeIdentifier: "",
            completedUnitCount: 0,
            totalUnitCount: max(exporter.totalUnitCount, 1),
            sampleCount: 0,
            byteCount: 0
        ))
        AppLog.upload.info("Upload started \(kind.rawValue, privacy: .public)")
        let identity = saved.endpoint.identity
        let host = saved.url.host ?? ""
        do {
            let outcome = try await uploader.upload(endpoint: saved.endpoint, exporter: exporter, kind: kind)
            switch outcome {
            case .noChanges:
                AppLog.upload.info("Incremental upload made no request")
                phase = .ready(limitedSince: limitedSince)
            case .uploaded(let summary):
                commitAnchors(kind: kind, identity: identity, exporter: exporter)
                let receipt = UploadReceipt(
                    identity: identity,
                    finishedAt: Date(),
                    sampleCount: summary.sampleCount,
                    byteCount: summary.byteCount,
                    batchCount: summary.batchCount
                )
                phase = .succeeded(receipt)
                record(kind: kind, status: .succeeded, summary: summary, exportID: summary.exportID, message: nil, host: host)
            }
        } catch is CancellationError {
            AppLog.upload.info("Upload canceled")
            let message = kind == .full ? "全量上传未完成。回到应用后可以重新上传。" : "增量上传已中断。系统会在下次有新数据时再试。"
            phase = .interrupted(message)
            record(kind: kind, status: .interrupted, summary: nil, exportID: nil, message: message, host: host)
        } catch UploadError.rejectedCode {
            AppLog.upload.error("Server rejected the verification code")
            let message = "验证码被服务器拒绝。"
            phase = .uploadFailed(message)
            record(kind: kind, status: .failed, summary: nil, exportID: nil, message: message, host: host)
        } catch UploadError.incomplete(let skipped) {
            let message = "有 \(skipped) 类数据未能读取。上传未完成，可以重试。"
            phase = .uploadFailed(message)
            record(kind: kind, status: .failed, summary: nil, exportID: nil, message: message, host: host)
        } catch ExportBatchError.recordTooLarge {
            AppLog.upload.error("One record is larger than the upload limit")
            let message = "有一条记录超过上传大小限制。上传未完成。"
            phase = .uploadFailed(message)
            record(kind: kind, status: .failed, summary: nil, exportID: nil, message: message, host: host)
        } catch {
            AppLog.upload.error("Upload failed")
            let message = "上传失败。可以重试。"
            phase = .uploadFailed(message)
            record(kind: kind, status: .failed, summary: nil, exportID: nil, message: message, host: host)
        }
        background.end()
    }

    private func commitAnchors(kind: UploadKind, identity: String, exporter: HealthExporting) {
        let previous = anchors.load(identity: identity)?.anchors ?? [:]
        guard let committed = AnchorCommit.map(kind: kind, previous: previous, finished: exporter.finishedAnchors, succeeded: true) else {
            return
        }
        let checkpoint = AnchorCheckpoint(anchors: committed, supplementalHash: exporter.supplementalHash)
        do {
            try anchors.save(checkpoint, identity: identity)
        } catch {
            AppLog.upload.error("Committed upload could not store anchors")
        }
    }

    private func record(
        kind: UploadKind,
        status: UploadHistoryStatus,
        summary: UploadSummary?,
        exportID: String?,
        message: String?,
        host: String
    ) {
        let entry = UploadHistoryEntry(
            id: UUID(),
            finishedAt: Date(),
            host: host,
            kind: kind,
            status: status,
            sampleCount: summary?.sampleCount ?? 0,
            byteCount: summary?.byteCount ?? 0,
            batchCount: summary?.batchCount ?? 0,
            exportID: exportID,
            message: message
        )
        historyStore.append(entry)
        history = historyStore.entries().reversed()
    }

    private func finishObservers() {
        let pending = observerCompletions
        observerCompletions = []
        pending.forEach { $0() }
    }

    private func prepare() async {
        guard settings.load() != nil else {
            phase = .authorizationFailed("请先保存服务器地址和验证码。")
            return
        }
        phase = .checking
        switch await access.prepare() {
        case .unavailable:
            phase = .unavailable
        case .failed(let message):
            phase = .authorizationFailed(message)
        case .determined(let limitedSince):
            self.limitedSince = limitedSince
            phase = .ready(limitedSince: limitedSince)
        }
    }
}
