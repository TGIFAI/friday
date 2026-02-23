import SwiftUI

@main
struct FridayApp: App {
    @StateObject private var runtime = FridayRuntime()

    var body: some Scene {
        MenuBarExtra {
            MenuBarView(runtime: runtime)
        } label: {
            Label("Friday", systemImage: runtime.isRunning ? "sparkle" : "sparkle.magnifyingglass")
        }
        .menuBarExtraStyle(.window)

        Settings {
            SettingsView(runtime: runtime)
        }
    }
}
