import SwiftUI

struct RootView: View {
    @Bindable var model: AppModel

    var body: some View {
        switch model.route {
        case .setup:
            SetupView(model: model)
        case .upload:
            UploadView(model: model)
        }
    }
}
