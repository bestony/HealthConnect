import Foundation
import HealthKit
import os

protocol HealthExporting: AnyObject {
    var completedUnitCount: Int { get }
    var totalUnitCount: Int { get }
    var currentTypeIdentifier: String { get }
    var skippedTypeIdentifiers: [String] { get }
    var finishedAnchors: [String: Data] { get }
    var supplementalHash: String? { get }
    func nextPage() async throws -> ExportPage?
}

enum HealthExportScope: Equatable {
    case full
    case incremental(anchors: [String: Data], supplementalHash: String?)
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
    private var sampleTypes: [HKSampleType] = []
    private var typeIndex = 0
    private let scope: HealthExportScope
    private var cursor: HKQueryAnchor?
    private var preparedSamples = false
    private var producedChanges = false
    private var supplementalPages: [ExportPage] = []
    private var didQueueSupplemental = false

    private(set) var completedUnitCount = 0
    private(set) var currentTypeIdentifier = ""
    private(set) var skippedTypeIdentifiers: [String] = []
    private(set) var finishedAnchors: [String: Data] = [:]
    private(set) var supplementalHash: String?

    var totalUnitCount: Int {
        sampleTypes.count + 3
    }

    init(
        store: HKHealthStore = HKHealthStore(),
        readableTypes: ReadableTypeFilter? = nil,
        pageLimit: Int = 500,
        scope: HealthExportScope = .full
    ) {
        self.store = store
        self.readableTypes = readableTypes ?? ReadableTypeFilter()
        self.pageLimit = pageLimit
        self.scope = scope
    }

    func nextPage() async throws -> ExportPage? {
        if !didReadCharacteristics {
            didReadCharacteristics = true
            currentTypeIdentifier = "characteristics"
            completedUnitCount = 1
            if case .full = scope {
                let records = HealthSampleEncoder.characteristicRecords(store: store)
                if !records.isEmpty {
                    AppLog.health.info("Read \(records.count, privacy: .public) characteristic records")
                    return ExportPage(typeIdentifier: "characteristics", records: records)
                }
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
                    if !page.records.isEmpty {
                        producedChanges = true
                    }
                    AppLog.health.info("Read page \(self.typeIndex, privacy: .public) \(type.identifier, privacy: .public) count \(page.records.count, privacy: .public)")
                    return page
                }
                commitCursor(for: type.identifier)
                typeIndex += 1
                cursor = nil
                completedUnitCount = 1 + typeIndex
            } catch {
                AppLog.health.error("Skipped sample type \(type.identifier, privacy: .public)")
                skippedTypeIdentifiers.append(type.identifier)
                typeIndex += 1
                cursor = nil
                completedUnitCount = 1 + typeIndex
            }
        }

        if !didQueueSupplemental {
            didQueueSupplemental = true
            supplementalPages = await supplementalPagesToSend()
        }
        if !supplementalPages.isEmpty {
            return supplementalPages.removeFirst()
        }

        return nil
    }

    private func supplementalPagesToSend() async -> [ExportPage] {
        var pages: [ExportPage] = []
        let activityIdentifier = HKObjectType.activitySummaryType().identifier
        currentTypeIdentifier = activityIdentifier
        if readableTypes.allows(activityIdentifier) {
            do {
                let summaries = try await HKActivitySummaryQueryDescriptor(predicate: activityPredicate()).result(for: store)
                let records = HealthSampleEncoder.activityRecords(summaries)
                completedUnitCount = sampleTypes.count + 2
                if !records.isEmpty {
                    AppLog.health.info("Read \(records.count, privacy: .public) activity summaries")
                    pages.append(ExportPage(typeIdentifier: activityIdentifier, records: records))
                }
            } catch {
                AppLog.health.error("Skipped activity summaries")
                skippedTypeIdentifiers.append(activityIdentifier)
            }
        } else {
            completedUnitCount = sampleTypes.count + 2
            AppLog.health.info("Omitted activity summaries because HealthKit will not authorize them")
        }

        let medicationIdentifier = HKObjectType.userAnnotatedMedicationType().identifier
        currentTypeIdentifier = medicationIdentifier
        if readableTypes.allows(medicationIdentifier) {
            do {
                let medications = try await HKUserAnnotatedMedicationQueryDescriptor().result(for: store)
                let records = HealthSampleEncoder.medicationRecords(medications)
                completedUnitCount = sampleTypes.count + 3
                if !records.isEmpty {
                    AppLog.health.info("Read \(records.count, privacy: .public) annotated medications")
                    pages.append(ExportPage(typeIdentifier: medicationIdentifier, records: records))
                }
            } catch {
                AppLog.health.error("Skipped annotated medications")
                skippedTypeIdentifiers.append(medicationIdentifier)
            }
        } else {
            completedUnitCount = sampleTypes.count + 3
            AppLog.health.info("Omitted annotated medications because HealthKit will not authorize them")
        }

        let records = pages.flatMap(\.records)
        let digest = SupplementalDigest.hash(records)
        supplementalHash = digest
        if case .incremental(_, let previous) = scope, !producedChanges, previous == digest {
            return []
        }
        return pages
    }

    private func activityPredicate() -> NSPredicate? {
        guard case .incremental = scope else {
            return nil
        }
        let calendar = Calendar.current
        let end = Date()
        let start = calendar.date(byAdding: .day, value: -14, to: end) ?? end
        return HKQuery.predicate(
            forActivitySummariesBetweenStart: calendar.dateComponents([.year, .month, .day], from: start),
            end: calendar.dateComponents([.year, .month, .day], from: end)
        )
    }

    private func page(for type: HKSampleType) async throws -> ExportPage? {
        let limit = seriesPageLimit(for: type)
        let descriptor = HKAnchoredObjectQueryDescriptor<HKSample>(
            predicates: [.sample(type: type, predicate: nil)],
            anchor: cursor ?? storedAnchor(for: type),
            limit: limit
        )
        let result = try await descriptor.result(for: store)
        let hasChanges = !result.addedSamples.isEmpty || !result.deletedObjects.isEmpty
        if hasChanges, let cursor, result.newAnchor.isEqual(cursor) {
            throw ExportFailure.anchorDidNotAdvance(type.identifier)
        }
        cursor = result.newAnchor
        if !hasChanges {
            return nil
        }
        var records: [ExportRecord] = []
        records.reserveCapacity(result.addedSamples.count + result.deletedObjects.count)
        for sample in result.addedSamples {
            records.append(await HealthSampleEncoder.record(for: sample, store: store))
        }
        for deleted in result.deletedObjects {
            records.append(ExportRecord(kind: "deleted", typeIdentifier: type.identifier, uuid: deleted.uuid.uuidString))
        }
        return ExportPage(typeIdentifier: type.identifier, records: records)
    }

    private func storedAnchor(for type: HKSampleType) -> HKQueryAnchor? {
        guard case .incremental(let anchors, _) = scope, let data = anchors[type.identifier] else {
            return nil
        }
        do {
            return try NSKeyedUnarchiver.unarchivedObject(ofClass: HKQueryAnchor.self, from: data)
        } catch {
            AppLog.health.error("Stored anchor could not be read for \(type.identifier, privacy: .public)")
            return nil
        }
    }

    private func commitCursor(for identifier: String) {
        guard let cursor else {
            return
        }
        do {
            finishedAnchors[identifier] = try NSKeyedArchiver.archivedData(withRootObject: cursor, requiringSecureCoding: true)
        } catch {
            AppLog.health.error("Anchor could not be archived for \(identifier, privacy: .public)")
        }
    }

    private func seriesPageLimit(for type: HKSampleType) -> Int {
        if type is HKElectrocardiogramType || type.identifier == HKDataTypeIdentifierHeartbeatSeries {
            return min(pageLimit, 20)
        }
        return pageLimit
    }
}
