import AppKit
import Foundation
import Yams

/// Reads and manages the Friday config.yaml inside the container.
final class ConfigManager {

    /// FRIDAY_HOME inside the macOS app sandbox container.
    let fridayHome: URL = {
        let appSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        return appSupport.appendingPathComponent("Friday")
    }()

    var configURL: URL {
        fridayHome.appendingPathComponent("config.yaml")
    }

    /// The gateway bind address parsed from config.yaml (e.g. "127.0.0.1:8080").
    var bindAddress: String {
        (try? load().gateway.bind) ?? "127.0.0.1:8080"
    }

    /// Read and parse config.yaml.
    func load() throws -> FridayConfig {
        let data = try Data(contentsOf: configURL)
        let decoder = YAMLDecoder()
        return try decoder.decode(FridayConfig.self, from: data)
    }

    /// Check whether config.yaml exists.
    var configExists: Bool {
        FileManager.default.fileExists(atPath: configURL.path)
    }

    /// Ensure the FRIDAY_HOME directory structure exists.
    func bootstrap() throws {
        let fm = FileManager.default
        let dirs = [
            fridayHome,
            fridayHome.appendingPathComponent("logs"),
            fridayHome.appendingPathComponent("skills"),
            fridayHome.appendingPathComponent("workspaces/default"),
        ]
        for dir in dirs {
            if !fm.fileExists(atPath: dir.path) {
                try fm.createDirectory(at: dir, withIntermediateDirectories: true)
            }
        }
    }

    /// Open config.yaml in the user's default editor.
    func openInEditor() {
        NSWorkspace.shared.open(configURL)
    }

    /// Reveal FRIDAY_HOME in Finder.
    func revealInFinder() {
        NSWorkspace.shared.selectFile(configURL.path, inFileViewerRootedAtPath: fridayHome.path)
    }
}

// MARK: - Config model (subset, for reading gateway.bind and display)

struct FridayConfig: Codable {
    var gateway: GatewayConfig
    var agents: [String: AgentConfig]?
    var channels: [String: ChannelConfig]?
    var providers: [String: ProviderConfig]?

    struct GatewayConfig: Codable {
        var bind: String
    }

    struct AgentConfig: Codable {
        var name: String?
        var workspace: String?
        var channels: [String]?
    }

    struct ChannelConfig: Codable {
        var type: String
        var enabled: Bool?
    }

    struct ProviderConfig: Codable {
        var type: String
    }
}
