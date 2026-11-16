import Foundation
import os

protocol ExportTransport: AnyObject {
    func createExport(endpoint: ServerEndpoint) async throws -> String
    func uploadBatch(endpoint: ServerEndpoint, exportID: String, body: Data) async throws
    func completeExport(
        endpoint: ServerEndpoint,
        exportID: String,
        sampleCount: Int,
        batchCount: Int
    ) async throws
}

struct ExportUploader {
    var transport: ExportTransport
    var onProgress: (@MainActor (UploadProgress) -> Void)?

    func upload(endpoint: ServerEndpoint, exporter: HealthExporting) async throws -> UploadSummary {
        let exportID = try await transport.createExport(endpoint: endpoint)
        AppLog.upload.info("Created export \(exportID, privacy: .public)")
        var sampleCount = 0
        var byteCount = 0
        var batchCount = 0

        while let page = try await exporter.nextPage() {
            try Task.checkCancellation()
            let groups = try ExportBatchSizer.groups(records: page.records)
            for body in groups {
                try Task.checkCancellation()
                try await transport.uploadBatch(endpoint: endpoint, exportID: exportID, body: body)
                batchCount += 1
                byteCount += body.count
            }
            sampleCount += page.records.count
            let progress = UploadProgress(
                currentTypeIdentifier: exporter.currentTypeIdentifier,
                completedUnitCount: exporter.completedUnitCount,
                totalUnitCount: exporter.totalUnitCount,
                sampleCount: sampleCount
            )
            onProgress?(progress)
            AppLog.upload.info("Sent \(page.records.count, privacy: .public) records for \(page.typeIdentifier, privacy: .public)")
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
        return UploadSummary(sampleCount: sampleCount, byteCount: byteCount, batchCount: batchCount)
    }
}
