import SwiftUI

struct HistoryView: View {
    var model: AppModel

    var body: some View {
        Group {
            if model.session.history.isEmpty {
                ContentUnavailableView("还没有上传记录。", systemImage: "clock")
            } else {
                List(model.session.history) { entry in
                    VStack(alignment: .leading, spacing: 4) {
                        Text("\(entry.kindLabel) · \(entry.statusLabel)")
                        Text(entry.host.isEmpty ? "服务器" : entry.host)
                            .font(.subheadline)
                        Text(entry.finishedAt.formatted(date: .abbreviated, time: .shortened))
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                        Text("\(entry.sampleCount) 条 · \(entry.byteCount) 字节 · \(entry.batchCount) 批")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                        if let exportID = entry.exportID {
                            Text(exportID)
                                .font(.footnote.monospaced())
                                .foregroundStyle(.secondary)
                                .textSelection(.enabled)
                        }
                        if let message = entry.message {
                            Text(message)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .accessibilityElement(children: .combine)
                }
            }
        }
        .navigationTitle("上传历史")
    }
}
