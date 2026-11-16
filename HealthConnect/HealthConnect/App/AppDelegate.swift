import Foundation
import os
import UIKit

final class AppDelegate: NSObject, UIApplicationDelegate {
    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        if ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil {
            AppRuntime.model.startBackgroundDelivery()
        } else {
            AppLog.setup.info("Background delivery skipped for the test host")
        }
        AppLog.setup.info("Application finished launching")
        return true
    }
}

enum AppRuntime {
    @MainActor static let model = AppModel.live()
}
