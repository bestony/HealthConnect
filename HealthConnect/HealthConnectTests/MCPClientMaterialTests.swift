import Foundation
import Testing
@testable import HealthConnect

struct MCPClientMaterialTests {
    @Test func usesTheAddressTheUserEnters() throws {
        let material = try MCPClientMaterial.make(
            address: " http://192.168.1.20:8080/base ",
            verificationCode: " secret "
        ).get()

        #expect(material.endpointURL == "http://192.168.1.20:8080/base/mcp")
        let configuration = try JSONSerialization.jsonObject(with: Data(material.configuration.utf8)) as? [String: Any]
        let servers = configuration?["mcpServers"] as? [String: Any]
        let entry = servers?["health-connect"] as? [String: Any]
        let headers = entry?["headers"] as? [String: String]
        #expect(entry?["url"] as? String == "http://192.168.1.20:8080/base/mcp")
        #expect(headers?["Authorization"] == "Bearer secret")
        #expect(material.command == "claude mcp add --transport http health-connect 'http://192.168.1.20:8080/base/mcp' --header 'Authorization: Bearer secret'")
        #expect(!material.configuration.contains("127.0.0.1"))
        #expect(!material.command.contains("127.0.0.1"))
    }

    @Test func appendsMCPToATrailingSlashAndKeepsTheJSONShape() throws {
        let material = try MCPClientMaterial.make(
            address: "https://example.com/",
            verificationCode: "hc_token"
        ).get()

        #expect(material.endpointURL == "https://example.com/mcp")
        #expect(material.configuration == """
        {
          "mcpServers": {
            "health-connect": {
              "url": "https://example.com/mcp",
              "headers": {
                "Authorization": "Bearer hc_token"
              }
            }
          }
        }
        """)
    }

    @Test func escapesTheTokenInJSONAndTheShellCommand() throws {
        let material = try MCPClientMaterial.make(
            address: "https://example.com",
            verificationCode: "a\"b\\c'e"
        ).get()

        let configuration = try JSONSerialization.jsonObject(with: Data(material.configuration.utf8)) as? [String: Any]
        let servers = configuration?["mcpServers"] as? [String: Any]
        let entry = servers?["health-connect"] as? [String: Any]
        let headers = entry?["headers"] as? [String: String]
        #expect(headers?["Authorization"] == "Bearer a\"b\\c'e")
        #expect(material.command.contains("'Authorization: Bearer a\"b\\c'\\''e'"))
        #expect(!material.command.contains("a\"b\\c'e'"))
    }

    @Test func rejectsAnAddressThatTheServerSettingsReject() {
        #expect(MCPClientMaterial.make(address: "http://example.com", verificationCode: "code") == .failure(.publicHTTP))
        #expect(MCPClientMaterial.make(address: "https://example.com", verificationCode: " ") == .failure(.emptyCode))
    }

    @Test func listsTheReadOnlyServerTools() {
        #expect(MCPClientMaterial.tools.map(\.name) == [
            "list_catalog",
            "get_profile",
            "query_series",
            "query_records",
            "query_workouts",
            "query_activity",
            "query_health_records",
            "summarize_range",
            "compare_periods",
        ])
    }
}
