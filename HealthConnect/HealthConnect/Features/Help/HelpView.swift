import os
import SwiftUI
import UniformTypeIdentifiers

struct HelpView: View {
    var address: String
    var verificationCode: String

    @State private var copied: CopyTarget?

    private enum CopyTarget {
        case configuration
        case command
    }

    var body: some View {
        Form {
            Section {
                Text("服务器提供只读 MCP。连接地址使用你填写的服务器地址，并加上 /mcp。验证码会作为 Bearer 令牌。请先上传数据，客户端才能查到内容。")
            }

            if let material {
                Section("连接地址") {
                    Text(material.endpointURL)
                        .font(.footnote.monospaced())
                        .textSelection(.enabled)
                }

                Section {
                    snippet(material.configuration)
                    Button("复制配置") {
                        copy(material.configuration, as: .configuration)
                    }
                    if copied == .configuration {
                        Text("已复制配置。剪贴板内容会在 5 分钟后失效。")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                } header: {
                    Text("配置")
                } footer: {
                    Text("把 JSON 放进支持 HTTP MCP 的客户端配置，例如 Cursor 的 mcp.json。")
                }

                Section {
                    snippet(material.command)
                    Button("复制命令") {
                        copy(material.command, as: .command)
                    }
                    if copied == .command {
                        Text("已复制命令。剪贴板内容会在 5 分钟后失效。")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                } header: {
                    Text("命令")
                } footer: {
                    Text("在终端运行这条命令，可以把同一个地址加到 Claude Code。命令和配置都包含验证码。不要把它们提交到仓库，也不要发给别人。")
                }
            } else if let message {
                Section {
                    Text(message)
                        .foregroundStyle(.red)
                } footer: {
                    Text("填写有效的服务器地址和验证码后，可以复制命令和配置。")
                }
            }

            Section {
                ForEach(MCPClientMaterial.tools, id: \.name) { tool in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(tool.name)
                            .font(.body.monospaced())
                        Text(tool.summary)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
            } header: {
                Text("可查询的内容")
            } footer: {
                Text("时间窗包含开始时间，不包含结束时间。这些结果描述已存储的数字，不是诊断或治疗建议。")
            }
        }
        .navigationTitle("帮助")
    }

    private var parsed: Result<MCPClientMaterial.Payload, EndpointValidationError> {
        MCPClientMaterial.make(address: address, verificationCode: verificationCode)
    }

    private var material: MCPClientMaterial.Payload? {
        try? parsed.get()
    }

    private var message: String? {
        if case .failure(let error) = parsed {
            return error.message
        }
        return nil
    }

    private func snippet(_ text: String) -> some View {
        Text(text)
            .font(.footnote.monospaced())
            .textSelection(.enabled)
    }

    private func copy(_ text: String, as target: CopyTarget) {
        UIPasteboard.general.setItems(
            [[UTType.utf8PlainText.identifier: text]],
            options: [.expirationDate: Date().addingTimeInterval(5 * 60)]
        )
        copied = target
        switch target {
        case .configuration:
            AppLog.setup.info("Copied MCP configuration")
        case .command:
            AppLog.setup.info("Copied MCP command")
        }
    }
}

extension View {
    func mcpHelp(address: String, verificationCode: String) -> some View {
        toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                NavigationLink("帮助") {
                    HelpView(address: address, verificationCode: verificationCode)
                }
            }
        }
    }
}
