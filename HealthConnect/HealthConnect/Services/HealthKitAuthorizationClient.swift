import Foundation
import HealthKit
import os

final class HealthKitAuthorizationClient: HealthAuthorizationClient {
    private let store = HKHealthStore()
    private let readableTypes: ReadableTypeFilter
    private var cachedReadTypes: Set<HKObjectType>?

    init(readableTypes: ReadableTypeFilter? = nil) {
        self.readableTypes = readableTypes ?? ReadableTypeFilter()
    }

    func isHealthDataAvailable() -> Bool {
        HKHealthStore.isHealthDataAvailable()
    }

    func authorizationRequestStatus() async throws -> HealthAuthorizationRequestStatus {
        let types = try await resolvedReadTypes()
        let outcome = await HealthAuthorizationProbe.readStatus(store: store, types: types)
        try throwIfNeeded(outcome)
        switch HKAuthorizationRequestStatus(rawValue: outcome.status) {
        case .shouldRequest:
            return .shouldRequest
        case .unnecessary:
            return .unnecessary
        case .unknown:
            return .unknown
        default:
            return .unknown
        }
    }

    func requestReadAuthorization() async throws {
        let types = try await resolvedReadTypes()
        let outcome = await HealthAuthorizationProbe.readRequest(store: store, types: types)
        AppLog.health.info("Read authorization request finished with status \(outcome.status, privacy: .public)")
        try throwIfNeeded(outcome)
        if outcome.status == 0 {
            throw HealthAuthorizationError.canceled
        }
    }

    func earliestLimitedReadDate() async throws -> Date? {
        let types = try await resolvedReadTypes()
        let outcome = await HealthAuthorizationProbe.earliestDate(store: store, types: types)
        if outcome.exceptionReason != nil || outcome.errorDomain != nil {
            AppLog.health.error("Earliest authorized read date is unavailable")
            return nil
        }
        return outcome.date
    }

    private func resolvedReadTypes() async throws -> Set<HKObjectType> {
        if let cachedReadTypes {
            return cachedReadTypes
        }
        var remaining = HealthTypeCatalog.readTypes
        var removed: Set<HKObjectType> = []
        var usedFallback = false
        while true {
            let outcome = await HealthAuthorizationProbe.readStatus(store: store, types: remaining)
            guard let reason = outcome.exceptionReason else {
                try throwIfNeeded(outcome)
                break
            }
            var drop = remaining.filter { type in
                let names = AuthorizationDenial.identifiers(in: reason)
                return names.contains(type.identifier) || reason.contains(type.identifier)
            }
            if drop.isEmpty && !usedFallback {
                usedFallback = true
                drop = remaining.filter(Self.isBulkDisallowed)
            }
            guard !drop.isEmpty else {
                AppLog.health.error("Authorization probe failed and no type could be removed")
                throw HealthAuthorizationError.failed
            }
            let list = drop.map(\.identifier).sorted().joined(separator: ",")
            AppLog.health.error("HealthKit disallowed read authorization for \(list, privacy: .public)")
            removed.formUnion(drop)
            remaining.subtract(drop)
        }
        readableTypes.setDisallowed(Set(removed.map(\.identifier)))
        AppLog.health.info("Read authorization uses \(remaining.count, privacy: .public) types")
        cachedReadTypes = remaining
        return remaining
    }

    private func throwIfNeeded(_ outcome: AuthorizationProbeOutcome) throws {
        guard outcome.exceptionReason == nil else {
            throw HealthAuthorizationError.failed
        }
        guard let domain = outcome.errorDomain else {
            return
        }
        if domain == HKError.errorDomain, HKError.Code(rawValue: outcome.errorCode) == .errorUserCanceled {
            throw HealthAuthorizationError.canceled
        }
        throw HealthAuthorizationError.failed
    }

    private static func isBulkDisallowed(_ type: HKObjectType) -> Bool {
        if type is HKCorrelationType {
            return true
        }
        let identifier = type.identifier
        if identifier == HKObjectType.visionPrescriptionType().identifier {
            return true
        }
        if identifier == HKObjectType.medicationDoseEventType().identifier {
            return true
        }
        if identifier == HKObjectType.userAnnotatedMedicationType().identifier {
            return true
        }
        return false
    }
}

/// HealthKit calls these completions on an arbitrary queue.
/// The callbacks stay off the main actor so a sheet dismissal cannot deadlock.
nonisolated private enum HealthAuthorizationProbe {
    static func readStatus(store: HKHealthStore, types: Set<HKObjectType>) async -> AuthorizationProbeOutcome {
        await withCheckedContinuation { continuation in
            HCProbeReadAuthorizationStatus(store, types) { status, reason, domain, code in
                continuation.resume(returning: AuthorizationProbeOutcome(
                    status: status,
                    exceptionReason: reason,
                    errorDomain: domain,
                    errorCode: code,
                    date: nil
                ))
            }
        }
    }

    static func readRequest(store: HKHealthStore, types: Set<HKObjectType>) async -> AuthorizationProbeOutcome {
        await withCheckedContinuation { continuation in
            HCProbeRequestReadAuthorization(store, types) { status, reason, domain, code in
                continuation.resume(returning: AuthorizationProbeOutcome(
                    status: status,
                    exceptionReason: reason,
                    errorDomain: domain,
                    errorCode: code,
                    date: nil
                ))
            }
        }
    }

    static func earliestDate(store: HKHealthStore, types: Set<HKObjectType>) async -> AuthorizationProbeOutcome {
        await withCheckedContinuation { continuation in
            HCProbeEarliestReadDate(store, types) { date, reason, domain, code in
                continuation.resume(returning: AuthorizationProbeOutcome(
                    status: 0,
                    exceptionReason: reason,
                    errorDomain: domain,
                    errorCode: code,
                    date: date
                ))
            }
        }
    }
}

private struct AuthorizationProbeOutcome: Sendable {
    var status: Int
    var exceptionReason: String?
    var errorDomain: String?
    var errorCode: Int
    var date: Date?
}
