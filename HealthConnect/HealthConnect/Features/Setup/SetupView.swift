import SwiftUI

struct SetupView: View {
    @Bindable var model: AppModel

    var body: some View {
        Form {
            Section {
                TextField("服务器地址", text: $model.draftAddress)
                    .textInputAutocapitalization(.never)
                    .keyboardType(.URL)
                    .autocorrectionDisabled()
                SecureField("验证码", text: $model.draftCode)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
            } footer: {
                Text("确认后会保存地址和验证码，并检查 Apple 健康数据授权。")
            }

            if let setupMessage = model.setupMessage {
                Section {
                    Text(setupMessage)
                        .foregroundStyle(.red)
                }
            }

            Section {
                Button {
                    Task { await model.confirm() }
                } label: {
                    if model.isSaving {
                        ProgressView()
                    } else {
                        Text("确认")
                    }
                }
                .disabled(model.isSaving)
            }
        }
        .navigationTitle("连接设置")
        .mcpHelp(address: model.draftAddress, verificationCode: model.draftCode)
    }
}
