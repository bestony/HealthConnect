import Foundation
import HealthKit
import os

protocol HealthExporting: AnyObject {
    var completedUnitCount: Int { get }
    var totalUnitCount: Int { get }
    var currentTypeIdentifier: String { get }
    var skippedTypeIdentifiers: [String] { get }
    func nextPage() async throws -> ExportPage?
}

enum ExportFailure: Error {
    case anchorDidNotAdvance(String)
}

/// Pages every readable HealthKit type.
///
/// CDA document bodies stay empty here. A sample query does not return them, and a document query asks for each document.
/// Vision prescriptions can also require a separate per-object choice. This session uploads the samples the first authorization allows.
final class HealthKitExportSession: HealthExporting {
    private let store: HKHealthStore
    private let readableTypes: ReadableTypeFilter
    private let pageLimit: Int
    private var didReadCharacteristics = false
    private var didReadActivity = false
    private var didReadMedications = false
    private var sampleTypes: [HKSampleType] = []
    private var typeIndex = 0
    private var anchor: HKQueryAnchor?
    private var preparedSamples = false

    private(set) var completedUnitCount = 0
    private(set) var currentTypeIdentifier = ""
    private(set) var skippedTypeIdentifiers: [String] = []

    var totalUnitCount: Int {
        sampleTypes.count + 3
    }

    init(
        store: HKHealthStore = HKHealthStore(),
        readableTypes: ReadableTypeFilter? = nil,
        pageLimit: Int = 500
    ) {
        self.store = store
        self.readableTypes = readableTypes ?? ReadableTypeFilter()
        self.pageLimit = pageLimit
    }

    func nextPage() async throws -> ExportPage? {
        if !didReadCharacteristics {
            didReadCharacteristics = true
            currentTypeIdentifier = "characteristics"
            let records = HealthSampleEncoder.characteristicRecords(store: store)
            completedUnitCount = 1
            if !records.isEmpty {
                AppLog.health.info("Read \(records.count, privacy: .public) characteristic records")
                return ExportPage(typeIdentifier: "characteristics", records: records)
            }
        }

        if !preparedSamples {
            let available = HealthTypeCatalog.sampleTypes.filter { readableTypes.allows($0.identifier) }
            let omitted = HealthTypeCatalog.sampleTypes.count - available.count
            sampleTypes = available
            preparedSamples = true
            if omitted > 0 {
                AppLog.health.info("Omitted \(omitted, privacy: .public) sample types that HealthKit will not authorize")
            }
            AppLog.health.info("Export will read \(self.sampleTypes.count, privacy: .public) sample types")
        }

        while typeIndex < sampleTypes.count {
            let type = sampleTypes[typeIndex]
            currentTypeIdentifier = type.identifier
            do {
                if let page = try await page(for: type) {
                    AppLog.health.info("Read page \(self.typeIndex, privacy: .public) \(type.identifier, privacy: .public) count \(page.records.count, privacy: .public)")
                    return page
                }
                typeIndex += 1
                anchor = nil
                completedUnitCount = 1 + typeIndex
            } catch {
                AppLog.health.error("Skipped sample type \(type.identifier, privacy: .public)")
                skippedTypeIdentifiers.append(type.identifier)
                typeIndex += 1
                anchor = nil
                completedUnitCount = 1 + typeIndex
            }
        }

        if !didReadActivity {
            didReadActivity = true
            currentTypeIdentifier = HKObjectType.activitySummaryType().identifier
            guard readableTypes.allows(currentTypeIdentifier) else {
                completedUnitCount = sampleTypes.count + 2
                AppLog.health.info("Omitted activity summaries because HealthKit will not authorize them")
                return try await nextPage()
            }
            do {
                let summaries = try await HKActivitySummaryQueryDescriptor(predicate: nil).result(for: store)
                let records = HealthSampleEncoder.activityRecords(summaries)
                completedUnitCount = sampleTypes.count + 2
                if !records.isEmpty {
                    AppLog.health.info("Read \(records.count, privacy: .public) activity summaries")
                    return ExportPage(typeIdentifier: currentTypeIdentifier, records: records)
                }
            } catch {
                AppLog.health.error("Skipped activity summaries")
                skippedTypeIdentifiers.append(currentTypeIdentifier)
            }
        }

        if !didReadMedications {
            didReadMedications = true
            currentTypeIdentifier = HKObjectType.userAnnotatedMedicationType().identifier
            guard readableTypes.allows(currentTypeIdentifier) else {
                completedUnitCount = sampleTypes.count + 3
                AppLog.health.info("Omitted annotated medications because HealthKit will not authorize them")
                return nil
            }
            do {
                let medications = try await HKUserAnnotatedMedicationQueryDescriptor().result(for: store)
                let records = HealthSampleEncoder.medicationRecords(medications)
                completedUnitCount = sampleTypes.count + 3
                if !records.isEmpty {
                    AppLog.health.info("Read \(records.count, privacy: .public) annotated medications")
                    return ExportPage(typeIdentifier: currentTypeIdentifier, records: records)
                }
            } catch {
                AppLog.health.error("Skipped annotated medications")
                skippedTypeIdentifiers.append(currentTypeIdentifier)
            }
        }

        return nil
    }

    private func page(for type: HKSampleType) async throws -> ExportPage? {
        let limit = seriesPageLimit(for: type)
        let descriptor = HKAnchoredObjectQueryDescriptor<HKSample>(
            predicates: [.sample(type: type, predicate: nil)],
            anchor: anchor,
            limit: limit
        )
        let result = try await descriptor.result(for: store)
        if result.addedSamples.isEmpty {
            return nil
        }
        if let anchor, result.newAnchor.isEqual(anchor) {
            throw ExportFailure.anchorDidNotAdvance(type.identifier)
        }
        anchor = result.newAnchor
        var records: [ExportRecord] = []
        records.reserveCapacity(result.addedSamples.count)
        for sample in result.addedSamples {
            records.append(await HealthSampleEncoder.record(for: sample, store: store))
        }
        return ExportPage(typeIdentifier: type.identifier, records: records)
    }

    private func seriesPageLimit(for type: HKSampleType) -> Int {
        if type is HKElectrocardiogramType || type.identifier == HKDataTypeIdentifierHeartbeatSeries {
            return min(pageLimit, 20)
        }
        return pageLimit
    }
}
