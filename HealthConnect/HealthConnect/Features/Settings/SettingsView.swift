import SwiftUI

struct SettingsView: View {
    var model: AppModel
    @State private var showsTips = false

    var body: some View {
        Form {
            Section("连接") {
                LabeledContent("服务器", value: model.savedServerText)
                Button("修改服务器") {
                    model.editSettings()
                }
            }

            Section("关于") {
                LabeledContent("版本", value: versionText)
                Text("HealthConnect 把 Apple 健康数据上传到你自己的服务器。")
                Button("打赏") {
                    showsTips = true
                }
            }
        }
        .navigationTitle("设置")
        .sheet(isPresented: $showsTips) {
            TipSheet(store: model.tips)
        }
    }

    private var versionText: String {
        let short = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "1.0"
        let build = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "1"
        return "\(short) (\(build))"
    }
}

private struct TipSheet: View {
    var store: TipStore
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text("打赏是自愿的一次性付款，不会解锁功能。同一档可以再次购买。")
                }
                Section {
                    if store.isLoading {
                        ProgressView()
                    }
                    ForEach(store.offers) { offer in
                        Button("打赏 \(offer.priceText)") {
                            Task { await store.purchase(offer.id) }
                        }
                        .disabled(!store.canPurchase(offer.id))
                    }
                    if store.isPurchasing {
                        ProgressView()
                    }
                    if let message = store.message {
                        Text(message)
                    }
                }
            }
            .navigationTitle("打赏")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("关闭") {
                        dismiss()
                    }
                }
            }
            .task {
                await store.load()
            }
        }
    }
}
