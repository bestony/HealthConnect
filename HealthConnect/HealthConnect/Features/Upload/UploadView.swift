import SwiftUI

struct UploadView: View {
    var model: AppModel
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        Form {
            Section("服务器") {
                Text(model.savedServerText)
            }

            Section("健康数据") {
                Text(statusText)
                if let limitText {
                    Text(limitText)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                if case .uploading(let progress) = model.session.phase {
                    ProgressView(
                        value: Double(progress.completedUnitCount),
                        total: Double(max(progress.totalUnitCount, 1))
                    )
                    Text(progress.currentTypeIdentifier)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    Text("已上传 \(progress.sampleCount) 条")
                }
                if case .succeeded(let receipt) = model.session.phase {
                    Text("已上传 \(receipt.sampleCount) 条")
                }
            }

            Section {
                Button(buttonTitle) {
                    if model.session.canRequestAuthorization {
                        Task { await model.session.reload() }
                    } else {
                        model.session.startUpload()
                    }
                }
                .disabled(!model.session.canUpload && !model.session.canRequestAuthorization)
            } footer: {
                Text("请保持 HealthConnect 在前台，直到上传完成。成功后，这个按钮会锁定。更换服务器或验证码后可以再次上传。")
            }

            Section {
                Button("修改设置") {
                    model.editSettings()
                }
            }
        }
        .navigationTitle("上传数据")
        .task {
            await model.session.reload()
        }
        .onChange(of: scenePhase) { _, phase in
            if phase != .active {
                model.session.cancelForBackground()
            }
        }
    }

    private var statusText: String {
        switch model.session.phase {
        case .checking:
            "正在检查健康数据授权。"
        case .unavailable:
            "这台设备不能使用 Apple 健康数据。"
        case .authorizationFailed(let message):
            message
        case .ready:
            "可以上传当前健康数据。"
        case .uploading:
            "正在上传。请不要离开此应用。"
        case .succeeded:
            "上传已完成。"
        case .uploadFailed(let message):
            message
        }
    }

    private var limitText: String? {
        guard case .ready(let limitedSince) = model.session.phase, let limitedSince else {
            return nil
        }
        return "系统只允许读取部分历史。上传内容从 \(limitedSince.formatted(date: .abbreviated, time: .omitted)) 开始。"
    }

    private var buttonTitle: String {
        switch model.session.phase {
        case .authorizationFailed:
            "授权健康数据"
        case .uploading:
            "正在上传"
        case .succeeded:
            "已上传"
        case .checking:
            "正在检查"
        case .unavailable:
            "无法上传"
        case .ready, .uploadFailed:
            "上传全部数据"
        }
    }
}
