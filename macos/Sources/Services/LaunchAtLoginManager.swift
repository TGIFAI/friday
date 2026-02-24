import Foundation
import ServiceManagement

/// Manages "Launch at Login" via SMAppService (macOS 13+).
@MainActor
final class LaunchAtLoginManager: ObservableObject {
    @Published var isEnabled: Bool {
        didSet { update() }
    }

    init() {
        self.isEnabled = SMAppService.mainApp.status == .enabled
    }

    private func update() {
        do {
            if isEnabled {
                try SMAppService.mainApp.register()
            } else {
                try SMAppService.mainApp.unregister()
            }
        } catch {
            // Revert to actual state on failure.
            let actual = SMAppService.mainApp.status == .enabled
            if self.isEnabled != actual {
                self.isEnabled = actual
            }
        }
    }
}
