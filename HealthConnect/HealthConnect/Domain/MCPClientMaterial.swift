import Foundation

/// Copyable MCP client setup for the read-only `/mcp` endpoint.
///
/// The URL is the server address that the user enters, with `mcp` appended.
/// The verification code is the bearer token. It is not a tool argument.
nonisolated enum MCPClientMaterial {
    struct Tool: Equatable, Sendable {
        var name: String
        var summary: String
    }

    struct Payload: Equatable, Sendable {
        var endpointURL: String
        var configuration: String
        var command: String
    }

    /// Names match `mcpapi.register` in the server.
    static let tools: [Tool] = [
        Tool(name: "list_catalog", summary: "列出已存储的类型，以及锻炼、活动日和临床记录的数量。"),
        Tool(name: "get_profile", summary: "读取已存储的个人资料。这些值可以识别本人。"),
        Tool(name: "query_series", summary: "按小时或按天汇总一个数值类型。小时最多 14 天，按天最多 400 天。"),
        Tool(name: "query_records", summary: "分页读取一个类型的原始样本。默认 50 条，最多 200 条。"),
        Tool(name: "query_workouts", summary: "分页读取锻炼。默认 20 条，最多 100 条。"),
        Tool(name: "query_activity", summary: "读取每日活动摘要。日期跨度最多 400 天。"),
        Tool(name: "query_health_records", summary: "读取临床记录。服务器不解释内容。默认 5 条，最多 20 条。"),
        Tool(name: "summarize_range", summary: "汇总最多 366 天的存储数据，并和等长的前一段比较。这不是诊断。"),
        Tool(name: "compare_periods", summary: "比较一个数值类型在两个时间窗里的结果。每个窗口最多 366 天。"),
    ]

    static func make(address: String, verificationCode: String) -> Result<Payload, EndpointValidationError> {
        switch ServerEndpoint.parse(address: address, verificationCode: verificationCode) {
        case .failure(let error):
            return .failure(error)
        case .success(let endpoint):
            let endpointURL = endpoint.url(for: "mcp").absoluteString
            let token = endpoint.verificationCode
            return .success(Payload(
                endpointURL: endpointURL,
                configuration: configuration(url: endpointURL, token: token),
                command: command(url: endpointURL, token: token)
            ))
        }
    }

    private static func configuration(url: String, token: String) -> String {
        """
        {
          "mcpServers": {
            "health-connect": {
              "url": \(jsonString(url)),
              "headers": {
                "Authorization": \(jsonString("Bearer \(token)"))
              }
            }
          }
        }
        """
    }

    private static func command(url: String, token: String) -> String {
        let header = "Authorization: Bearer \(token)"
        return "claude mcp add --transport http health-connect \(shellQuote(url)) --header \(shellQuote(header))"
    }

    private static func jsonString(_ value: String) -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.withoutEscapingSlashes]
        guard let data = try? encoder.encode(value) else {
            return "\"\""
        }
        return String(decoding: data, as: UTF8.self)
    }

    private static func shellQuote(_ value: String) -> String {
        "'" + value.replacingOccurrences(of: "'", with: "'\\''") + "'"
    }
}
