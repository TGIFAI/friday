import Foundation

/// Manages the lifecycle of the friday Go binary subprocess.
final class ProcessManager: @unchecked Sendable {
    private var process: Process?

    var onLog: (@Sendable (String) -> Void)?
    var onExit: (@Sendable (Int32) -> Void)?

    /// Locates the friday-core binary bundled in the app's Resources.
    private var binaryURL: URL? {
        Bundle.main.url(forResource: "friday-core", withExtension: nil)
    }

    func start(fridayHome: URL, config: FridayConfig, allowedPaths: [String] = []) throws {
        guard let binary = binaryURL else {
            throw FridayError.binaryNotFound
        }

        let proc = Process()
        proc.executableURL = binary
        proc.arguments = ["gateway", "run"]
        proc.currentDirectoryURL = fridayHome

        // Build environment: runtime marker + FRIDAY_HOME + user API keys from config.
        var env = ProcessInfo.processInfo.environment
        env["FRIDAY_RUNTIME"] = "macos-app"
        env["FRIDAY_HOME"] = fridayHome.path
        // Pass API keys from Keychain into environment so config.yaml can use ${VAR} syntax.
        for (key, value) in KeychainHelper.allAPIKeys() {
            env[key] = value
        }
        // Inject security-scoped bookmark paths so agents can access user-approved directories.
        if !allowedPaths.isEmpty {
            env["FRIDAY_ALLOWED_PATHS"] = allowedPaths.joined(separator: ":")
        }
        proc.environment = env

        // Capture stdout + stderr.
        let pipe = Pipe()
        proc.standardOutput = pipe
        proc.standardError = pipe
        pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            guard !data.isEmpty, let line = String(data: data, encoding: .utf8) else { return }
            self?.onLog?(line)
        }

        proc.terminationHandler = { [weak self] p in
            self?.onExit?(p.terminationStatus)
        }

        try proc.run()
        process = proc
    }

    func stop() {
        guard let proc = process, proc.isRunning else { return }
        proc.terminate() // sends SIGTERM → Go handles graceful shutdown
        process = nil
    }

    var isRunning: Bool {
        process?.isRunning ?? false
    }
}

enum FridayError: LocalizedError {
    case binaryNotFound
    case configNotFound

    var errorDescription: String? {
        switch self {
        case .binaryNotFound:
            return "friday-core binary not found in app bundle"
        case .configNotFound:
            return "config.yaml not found"
        }
    }
}
