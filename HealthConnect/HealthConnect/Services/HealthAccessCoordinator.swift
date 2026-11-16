import Foundation
import os

nonisolated enum HealthAuthorizationRequestStatus: Equatable, Sendable {
    case shouldRequest
    case unnecessary
    case unknown
}

nonisolated enum HealthAccessState: Equatable, Sendable {
    case unavailable
    case determined(limitedSince: Date?)
    case failed(String)
}

enum HealthAuthorizationError: Error {
    case canceled
    case failed
}

protocol HealthAuthorizationClient: AnyObject {
    func isHealthDataAvailable() -> Bool
    func authorizationRequestStatus() async throws -> HealthAuthorizationRequestStatus
    func requestReadAuthorization() async throws
    func earliestLimitedReadDate() async throws -> Date?
}

/// HealthKit does not reveal whether read permission was granted.
/// This coordinator only checks whether the system sheet is still required.
final class HealthAccessCoordinator {
    private let client: HealthAuthorizationClient

    init(client: HealthAuthorizationClient) {
        self.client = client
    }

    func prepare() async -> HealthAccessState {
        guard client.isHealthDataAvailable() else {
            AppLog.health.info("Health data is unavailable on this device")
            return .unavailable
        }
        do {
            let status = try await client.authorizationRequestStatus()
            AppLog.health.info("Authorization request status is \(String(describing: status), privacy: .public)")
            if status == .shouldRequest || status == .unknown {
                try await client.requestReadAuthorization()
                AppLog.health.info("Finished the Health read authorization request")
            }
            let limitedSince = try await client.earliestLimitedReadDate()
            if limitedSince != nil {
                AppLog.health.info("Read access has a limited start date")
            }
            return .determined(limitedSince: limitedSince)
        } catch HealthAuthorizationError.canceled {
            AppLog.health.info("Health authorization sheet was canceled")
            return .failed("授权未完成。")
        } catch {
            AppLog.health.error("Health authorization failed")
            return .failed("无法请求健康数据授权。")
        }
    }
}
