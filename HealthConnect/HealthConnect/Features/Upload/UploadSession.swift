import Foundation
import Observation
import os

@MainActor
@Observable
final class UploadSession {
    private let settings: SettingsStoring
    private let access: HealthAccessCoordinator
    private let makeExporter: () -> HealthExporting
    private let makeUploader: () -> ExportUploader
    private var reloadTask: Task<Void, Never>?
    private var uploadTask: Task<Void, Never>?

    private(set) var phase: UploadPhase = .checking

    init(
        settings: SettingsStoring,
        access: HealthAccessCoordinator,
        makeExporter: @escaping () -> HealthExporting,
        makeUploader: @escaping () -> ExportUploader
    ) {
        self.settings = settings
        self.access = access
        self.makeExporter = makeExporter
        self.makeUploader = makeUploader
    }

    var canUpload: Bool {
        switch phase {
        case .ready, .uploadFailed:
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
            await performUpload()
            uploadTask = nil
        }
    }

    func cancelForBackground() {
        guard case .uploading = phase else {
            return
        }
        uploadTask?.cancel()
    }

    func performUpload() async {
        guard let saved = settings.load() else {
            phase = .uploadFailed("请先保存服务器地址和验证码。")
            return
        }
        let exporter = makeExporter()
        var uploader = makeUploader()
        uploader.onProgress = { [weak self] progress in
            self?.phase = .uploading(progress)
        }
        phase = .uploading(UploadProgress(
            currentTypeIdentifier: "",
            completedUnitCount: 0,
            totalUnitCount: max(exporter.totalUnitCount, 1),
            sampleCount: 0
        ))
        AppLog.upload.info("Upload started")
        do {
            let summary = try await uploader.upload(endpoint: saved.endpoint, exporter: exporter)
            let receipt = UploadReceipt(
                identity: saved.endpoint.identity,
                finishedAt: Date(),
                sampleCount: summary.sampleCount,
                byteCount: summary.byteCount,
                batchCount: summary.batchCount
            )
            settings.saveReceipt(receipt)
            phase = .succeeded(receipt)
        } catch is CancellationError {
            AppLog.upload.info("Upload canceled")
            phase = .uploadFailed("上传已停止。请保持应用在前台后重试。")
        } catch UploadError.rejectedCode {
            AppLog.upload.error("Server rejected the verification code")
            phase = .uploadFailed("验证码被服务器拒绝。")
        } catch UploadError.incomplete(let skipped) {
            phase = .uploadFailed("有 \(skipped) 类数据未能读取。上传未完成，可以重试。")
        } catch {
            AppLog.upload.error("Upload failed")
            phase = .uploadFailed("上传失败。可以重试。")
        }
    }

    private func prepare() async {
        guard let saved = settings.load() else {
            phase = .authorizationFailed("请先保存服务器地址和验证码。")
            return
        }
        if let receipt = settings.receipt(), receipt.identity == saved.endpoint.identity {
            phase = .succeeded(receipt)
            AppLog.upload.info("Upload is already complete for the saved server")
            return
        }
        phase = .checking
        switch await access.prepare() {
        case .unavailable:
            phase = .unavailable
        case .failed(let message):
            phase = .authorizationFailed(message)
        case .determined(let limitedSince):
            phase = .ready(limitedSince: limitedSince)
        }
    }
}
