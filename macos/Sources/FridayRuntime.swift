import Foundation
import Combine

/// FridayRuntime is the central state object coordinating the Go subprocess,
/// config reading, and health monitoring.
@MainActor
final class FridayRuntime: ObservableObject {
    @Published var isRunning = false
    @Published var statusText = L10n.stopped
    @Published var logs: [String] = []

    private let processManager = ProcessManager()
    private let configManager = ConfigManager()
    private var healthTimer: Timer?

    var fridayHome: URL {
        configManager.fridayHome
    }

    var bindAddress: String {
        configManager.bindAddress
    }

    // MARK: - Lifecycle

    func start() {
        guard !isRunning else { return }
        do {
            let config = try configManager.load()
            try processManager.start(fridayHome: fridayHome, config: config)

            processManager.onLog = { [weak self] line in
                Task { @MainActor in
                    self?.appendLog(line)
                }
            }
            processManager.onExit = { [weak self] code in
                Task { @MainActor in
                    self?.isRunning = false
                    self?.statusText = L10n.exitCode(code)
                    self?.stopHealthCheck()
                }
            }

            isRunning = true
            statusText = L10n.starting
            startHealthCheck()
        } catch {
            statusText = L10n.startFailed(error.localizedDescription)
            appendLog("[app] start failed: \(error)")
        }
    }

    func stop() {
        processManager.stop()
        isRunning = false
        statusText = L10n.stopped
        stopHealthCheck()
    }

    func restart() {
        stop()
        // Give the process a moment to clean up.
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) {
            self.start()
        }
    }

    // MARK: - Health

    private func startHealthCheck() {
        healthTimer = Timer.scheduledTimer(withTimeInterval: 5.0, repeats: true) { [weak self] _ in
            Task { [weak self] in
                await self?.checkHealth()
            }
        }
    }

    private func stopHealthCheck() {
        healthTimer?.invalidate()
        healthTimer = nil
    }

    private func checkHealth() async {
        let addr = configManager.bindAddress
        guard let url = URL(string: "http://\(addr)/health") else { return }

        do {
            let (data, _) = try await URLSession.shared.data(from: url)
            if let body = String(data: data, encoding: .utf8), body.contains("ok") {
                await MainActor.run {
                    if self.statusText != L10n.running {
                        self.statusText = L10n.running
                    }
                }
            }
        } catch {
            // Health check failed — process might still be starting.
        }
    }

    // MARK: - Logs

    private func appendLog(_ line: String) {
        logs.append(line)
        if logs.count > 500 {
            logs.removeFirst(logs.count - 500)
        }
    }
}
