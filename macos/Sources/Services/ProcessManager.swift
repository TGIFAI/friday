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

    /// Runs the user's login shell to capture the full environment (PATH, etc.).
    /// macOS apps launched from Finder/Dock inherit a minimal launchd environment;
    /// this ensures we get the user's profile-sourced variables.
    private func loadUserShellEnvironment() -> [String: String] {
        let shell = ProcessInfo.processInfo.environment["SHELL"] ?? "/bin/zsh"
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: shell)
        proc.arguments = ["-l", "-c", "env"]

        let pipe = Pipe()
        proc.standardOutput = pipe
        proc.standardError = FileHandle.nullDevice

        do {
            try proc.run()
            proc.waitUntilExit()
        } catch {
            return [:]
        }

        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        guard let output = String(data: data, encoding: .utf8) else { return [:] }

        var env: [String: String] = [:]
        for line in output.components(separatedBy: "\n") {
            guard let eqIndex = line.firstIndex(of: "=") else { continue }
            let key = String(line[line.startIndex..<eqIndex])
            let value = String(line[line.index(after: eqIndex)...])
            env[key] = value
        }
        return env
    }

    func start(fridayHome: URL, config: FridayConfig, allowedPaths: [String] = []) throws {
        guard let binary = binaryURL else {
            throw FridayError.binaryNotFound
        }

        let proc = Process()
        proc.executableURL = binary
        proc.arguments = ["gateway", "run"]
        proc.currentDirectoryURL = fridayHome

        // Start with the user's full login-shell environment so child processes
        // (and the Go shellx tool) see the complete PATH and other variables.
        var env = loadUserShellEnvironment()
        // Merge in process environment as fallback (for any launchd-specific vars).
        for (key, value) in ProcessInfo.processInfo.environment {
            if env[key] == nil {
                env[key] = value
            }
        }
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
