import Foundation
import os

protocol ExportTransport: AnyObject {
    func createExport(endpoint: ServerEndpoint, mode: ExportMode) async throws -> String
    func uploadBatch(endpoint: ServerEndpoint, exportID: String, body: Data) async throws
    func completeExport(
        endpoint: ServerEndpoint,
        exportID: String,
        sampleCount: Int,
        batchCount: Int
    ) async throws
    func abortExport(endpoint: ServerEndpoint, exportID: String) async throws
}

enum UploadOutcome: Equatable, Sendable {
    case noChanges
    case uploaded(UploadSummary)
}

struct ExportUploader {
    var transport: ExportTransport
    var openExports: OpenExportStoring?
    var maxBatchBytes: Int = ExportBatchSizer.maxBytes
    var onProgress: (@MainActor (UploadProgress) -> Void)?

    /// Completes an export whose batches are already stored.
    /// The open-export record stays in place when the request fails.
    func completeStored(
        endpoint: ServerEndpoint,
        exportID: String,
        sampleCount: Int,
        batchCount: Int
    ) async throws -> UploadSummary {
        try await transport.completeExport(
            endpoint: endpoint,
            exportID: exportID,
            sampleCount: sampleCount,
            batchCount: batchCount
        )
        openExports?.delete(identity: endpoint.identity)
        AppLog.upload.info("Completed stored export records \(sampleCount, privacy: .public) batches \(batchCount, privacy: .public)")
        return UploadSummary(exportID: exportID, sampleCount: sampleCount, byteCount: 0, batchCount: batchCount)
    }

    /// Asks the server to drop an unfinished export. A transport error is logged.
    func abortOpen(endpoint: ServerEndpoint, exportID: String) async {
        do {
            try await transport.abortExport(endpoint: endpoint, exportID: exportID)
            AppLog.upload.info("Aborted export \(exportID, privacy: .public)")
        } catch {
            AppLog.upload.error("Abort export failed")
        }
        openExports?.delete(identity: endpoint.identity)
    }

    func upload(endpoint: ServerEndpoint, exporter: HealthExporting, kind: UploadKind) async throws -> UploadOutcome {
        if kind == .incremental {
            var pages: [ExportPage] = []
            while let page = try await exporter.nextPage() {
                try Task.checkCancellation()
                if !page.records.isEmpty {
                    pages.append(page)
                }
            }
            if pages.isEmpty {
                if !exporter.skippedTypeIdentifiers.isEmpty {
                    AppLog.upload.error("Export skipped \(exporter.skippedTypeIdentifiers.count, privacy: .public) types")
                    throw UploadError.incomplete(skipped: exporter.skippedTypeIdentifiers.count)
                }
                AppLog.upload.info("Incremental upload had no changes")
                return .noChanges
            }
            let summary = try await send(endpoint: endpoint, kind: kind, pages: pages, exporter: exporter)
            return .uploaded(summary)
        }

        let exportID = try await transport.createExport(endpoint: endpoint, mode: .replace)
        AppLog.upload.info("Created export \(exportID, privacy: .public) mode replace")
        try await rememberSending(endpoint: endpoint, exportID: exportID)
        var sampleCount = 0
        var byteCount = 0
        var batchCount = 0

        do {
            while let page = try await exporter.nextPage() {
                try Task.checkCancellation()
                let sent = try await sendPage(
                    endpoint: endpoint,
                    exportID: exportID,
                    page: page,
                    sampleCount: &sampleCount,
                    byteCount: &byteCount,
                    batchCount: &batchCount
                )
                if sent {
                    report(kind: kind, exporter: exporter, sampleCount: sampleCount, byteCount: byteCount, completed: exporter.completedUnitCount, total: exporter.totalUnitCount)
                }
            }
        } catch {
            await abortOpen(endpoint: endpoint, exportID: exportID)
            throw error
        }

        let summary = try await finishExport(
            endpoint: endpoint,
            exportID: exportID,
            sampleCount: sampleCount,
            byteCount: byteCount,
            batchCount: batchCount,
            skipped: exporter.skippedTypeIdentifiers.count
        )
        return .uploaded(summary)
    }

    private func send(
        endpoint: ServerEndpoint,
        kind: UploadKind,
        pages: [ExportPage],
        exporter: HealthExporting
    ) async throws -> UploadSummary {
        let mode = ExportMode.mode(for: kind)
        let exportID = try await transport.createExport(endpoint: endpoint, mode: mode)
        AppLog.upload.info("Created export \(exportID, privacy: .public) mode \(mode.rawValue, privacy: .public)")
        try await rememberSending(endpoint: endpoint, exportID: exportID)
        var sampleCount = 0
        var byteCount = 0
        var batchCount = 0
        do {
            for (index, page) in pages.enumerated() {
                try Task.checkCancellation()
                _ = try await sendPage(
                    endpoint: endpoint,
                    exportID: exportID,
                    page: page,
                    sampleCount: &sampleCount,
                    byteCount: &byteCount,
                    batchCount: &batchCount
                )
                report(
                    kind: kind,
                    exporter: exporter,
                    sampleCount: sampleCount,
                    byteCount: byteCount,
                    completed: index + 1,
                    total: pages.count,
                    typeIdentifier: page.typeIdentifier
                )
            }
        } catch {
            await abortOpen(endpoint: endpoint, exportID: exportID)
            throw error
        }
        return try await finishExport(
            endpoint: endpoint,
            exportID: exportID,
            sampleCount: sampleCount,
            byteCount: byteCount,
            batchCount: batchCount,
            skipped: exporter.skippedTypeIdentifiers.count
        )
    }

    private func rememberSending(endpoint: ServerEndpoint, exportID: String) async throws {
        guard let openExports else {
            return
        }
        do {
            try openExports.save(
                OpenExport(exportID: exportID, phase: .sending, sampleCount: 0, batchCount: 0),
                identity: endpoint.identity
            )
        } catch {
            AppLog.upload.error("Open export could not be stored")
            await abortOpen(endpoint: endpoint, exportID: exportID)
            throw error
        }
    }

    /// Completes the export, or aborts it when HealthKit skipped a type.
    /// A failed complete request leaves the record at `readyToComplete`.
    private func finishExport(
        endpoint: ServerEndpoint,
        exportID: String,
        sampleCount: Int,
        byteCount: Int,
        batchCount: Int,
        skipped: Int
    ) async throws -> UploadSummary {
        if skipped > 0 {
            AppLog.upload.error("Export skipped \(skipped, privacy: .public) types")
            await abortOpen(endpoint: endpoint, exportID: exportID)
            throw UploadError.incomplete(skipped: skipped)
        }
        do {
            try openExports?.save(
                OpenExport(
                    exportID: exportID,
                    phase: .readyToComplete,
                    sampleCount: sampleCount,
                    batchCount: batchCount
                ),
                identity: endpoint.identity
            )
        } catch {
            AppLog.upload.error("Open export could not be stored")
        }
        try await transport.completeExport(
            endpoint: endpoint,
            exportID: exportID,
            sampleCount: sampleCount,
            batchCount: batchCount
        )
        openExports?.delete(identity: endpoint.identity)
        AppLog.upload.info("Completed export records \(sampleCount, privacy: .public) batches \(batchCount, privacy: .public)")
        return UploadSummary(exportID: exportID, sampleCount: sampleCount, byteCount: byteCount, batchCount: batchCount)
    }

    private func sendPage(
        endpoint: ServerEndpoint,
        exportID: String,
        page: ExportPage,
        sampleCount: inout Int,
        byteCount: inout Int,
        batchCount: inout Int
    ) async throws -> Bool {
        let groups = try ExportBatchSizer.groups(records: page.records, maxBytes: maxBatchBytes)
        if groups.isEmpty {
            return false
        }
        for body in groups {
            try Task.checkCancellation()
            try await transport.uploadBatch(endpoint: endpoint, exportID: exportID, body: body)
            batchCount += 1
            byteCount += body.count
            sampleCount += body.filter { $0 == 0x0A }.count
        }
        AppLog.upload.info("Sent \(page.records.count, privacy: .public) records for \(page.typeIdentifier, privacy: .public)")
        return true
    }

    private func report(
        kind: UploadKind,
        exporter: HealthExporting,
        sampleCount: Int,
        byteCount: Int,
        completed: Int,
        total: Int,
        typeIdentifier: String? = nil
    ) {
        let progress = UploadProgress(
            kind: kind,
            currentTypeIdentifier: typeIdentifier ?? exporter.currentTypeIdentifier,
            completedUnitCount: completed,
            totalUnitCount: max(total, 1),
            sampleCount: sampleCount,
            byteCount: byteCount
        )
        onProgress?(progress)
    }
}
