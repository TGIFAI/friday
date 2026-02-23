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

    /// Ensure directory structure exists and write default config if missing.
    func initializeIfNeeded() throws {
        try bootstrap()
        if !configExists {
            try Self.defaultConfigYAML.write(to: configURL, atomically: true, encoding: .utf8)
        }
    }

    /// Ensure the FRIDAY_HOME directory structure exists and sync bundled skills.
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
        syncBundledSkills()
    }

    /// Copy built-in skills from the app bundle's Resources/skills/ into FRIDAY_HOME/skills/.
    /// Overwrites existing files to keep skills up-to-date with the app version.
    private func syncBundledSkills() {
        guard let bundledSkills = Bundle.main.resourceURL?.appendingPathComponent("skills"),
              FileManager.default.fileExists(atPath: bundledSkills.path) else { return }

        let fm = FileManager.default
        let destSkills = fridayHome.appendingPathComponent("skills")

        guard let entries = try? fm.contentsOfDirectory(
            at: bundledSkills, includingPropertiesForKeys: nil
        ) else { return }

        for src in entries {
            let dst = destSkills.appendingPathComponent(src.lastPathComponent)
            // Remove old version then copy fresh
            try? fm.removeItem(at: dst)
            try? fm.copyItem(at: src, to: dst)
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

    // MARK: - Default Config Template

    static let defaultConfigYAML = """
    # Friday Configuration
    # Full guide: https://tgif.sh/pilot

    gateway:
      bind: "127.0.0.1:8080"
      max_concurrent_sessions: 100
      request_timeout: 300

    logging:
      level: "info"
      format: "text"
      output: "stdout"

    providers:
      openai:
        type: "openai"
        config:
          api_key: "${OPENAI_API_KEY}"
          default_model: "gpt-4o-mini"
          timeout: 60
          max_retries: 3

    channels:
      http:
        type: "http"
        enabled: true
        config:
          api_key: "${HTTP_API_KEY}"

    agents:
      default:
        name: "Default"
        workspace: "workspaces/default"
        channels:
          - "http"
        models:
          primary: "openai:gpt-4o-mini"
        config:
          max_iterations: 10
          max_tokens: 4000
          temperature: 0.7
    """
}

// MARK: - Config model (subset, for reading gateway.bind)

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
