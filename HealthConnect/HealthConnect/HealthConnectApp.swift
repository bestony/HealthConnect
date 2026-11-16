//
//  HealthConnectApp.swift
//  HealthConnect
//
//  Created by Bestony on 2026/11/16.
//

import SwiftUI

@main
struct HealthConnectApp: App {
    @State private var model = AppModel.live()

    var body: some Scene {
        WindowGroup {
            NavigationStack {
                RootView(model: model)
            }
        }
    }
}
