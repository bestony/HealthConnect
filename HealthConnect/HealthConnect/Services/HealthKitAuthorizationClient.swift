import Foundation
import HealthKit

final class HealthKitAuthorizationClient: HealthAuthorizationClient {
    private let store = HKHealthStore()

    func isHealthDataAvailable() -> Bool {
        HKHealthStore.isHealthDataAvailable()
    }

    func authorizationRequestStatus() async throws -> HealthAuthorizationRequestStatus {
        let status = try await store.statusForAuthorizationRequest(toShare: [], read: HealthTypeCatalog.readTypes)
        switch status {
        case .shouldRequest:
            return .shouldRequest
        case .unnecessary:
            return .unnecessary
        case .unknown:
            return .unknown
        @unknown default:
            return .unknown
        }
    }

    func requestReadAuthorization() async throws {
        do {
            try await store.requestAuthorization(toShare: [], read: HealthTypeCatalog.readTypes)
        } catch let error as HKError where error.code == .errorUserCanceled {
            throw HealthAuthorizationError.canceled
        }
    }

    func earliestLimitedReadDate() async throws -> Date? {
        let dates = try await store.earliestAuthorizedSampleDate(for: HealthTypeCatalog.readTypes)
        return dates.values.min()
    }
}
