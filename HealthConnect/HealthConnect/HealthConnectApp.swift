//
//  HealthConnectApp.swift
//  HealthConnect
//
//  Created by Bestony on 2026/11/16.
//

import SwiftUI

@main
struct HealthConnectApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @State private var model = AppRuntime.model

    var body: some Scene {
        WindowGroup {
            RootView(model: model)
        }
    }
}
