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
}

enum UploadOutcome: Equatable, Sendable {
    case noChanges
    case uploaded(UploadSummary)
}

struct ExportUploader {
    var transport: ExportTransport
    var maxBatchBytes: Int = ExportBatchSizer.maxBytes
    var onProgress: (@MainActor (UploadProgress) -> Void)?

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
        var sampleCount = 0
        var byteCount = 0
        var batchCount = 0

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

        let skipped = exporter.skippedTypeIdentifiers.count
        if skipped > 0 {
            AppLog.upload.error("Export skipped \(skipped, privacy: .public) types")
            throw UploadError.incomplete(skipped: skipped)
        }

        try await transport.completeExport(
            endpoint: endpoint,
            exportID: exportID,
            sampleCount: sampleCount,
            batchCount: batchCount
        )
        AppLog.upload.info("Completed export records \(sampleCount, privacy: .public) batches \(batchCount, privacy: .public)")
        return .uploaded(UploadSummary(exportID: exportID, sampleCount: sampleCount, byteCount: byteCount, batchCount: batchCount))
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
        var sampleCount = 0
        var byteCount = 0
        var batchCount = 0
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
        let skipped = exporter.skippedTypeIdentifiers.count
        if skipped > 0 {
            AppLog.upload.error("Export skipped \(skipped, privacy: .public) types")
            throw UploadError.incomplete(skipped: skipped)
        }
        try await transport.completeExport(
            endpoint: endpoint,
            exportID: exportID,
            sampleCount: sampleCount,
            batchCount: batchCount
        )
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
