import os
import UIKit

protocol BackgroundTaskControlling: AnyObject {
    func begin(expiration: @escaping () -> Void)
    func end()
}

final class ApplicationBackgroundTasks: BackgroundTaskControlling {
    private var identifier: UIBackgroundTaskIdentifier = .invalid

    func begin(expiration: @escaping () -> Void) {
        guard identifier == .invalid else {
            return
        }
        identifier = UIApplication.shared.beginBackgroundTask(withName: "HealthConnectUpload") { [weak self] in
            AppLog.upload.info("Background time expired")
            expiration()
            self?.end()
        }
    }

    func end() {
        guard identifier != .invalid else {
            return
        }
        UIApplication.shared.endBackgroundTask(identifier)
        identifier = .invalid
    }
}

final class NoopBackgroundTasks: BackgroundTaskControlling {
    func begin(expiration: @escaping () -> Void) {}
    func end() {}
}
