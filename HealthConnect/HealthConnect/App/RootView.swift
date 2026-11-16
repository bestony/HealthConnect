import SwiftUI

struct RootView: View {
    @Bindable var model: AppModel

    var body: some View {
        switch model.route {
        case .setup:
            NavigationStack {
                SetupView(model: model)
            }
        case .upload:
            TabView {
                NavigationStack {
                    UploadView(model: model)
                }
                .tabItem {
                    Label("上传", systemImage: "square.and.arrow.up")
                }
                NavigationStack {
                    HistoryView(model: model)
                }
                .tabItem {
                    Label("历史", systemImage: "clock")
                }
                NavigationStack {
                    SettingsView(model: model)
                }
                .tabItem {
                    Label("设置", systemImage: "gearshape")
                }
            }
        }
    }
}
