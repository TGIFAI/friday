import SwiftUI

@main
struct FridayApp: App {
    @StateObject private var runtime = FridayRuntime()

    var body: some Scene {
        MenuBarExtra {
            MenuBarView(runtime: runtime)
                .onAppear { runtime.bootstrap() }
        } label: {
            Image(nsImage: StatusBarIcon.make(active: runtime.isRunning))
        }
        .menuBarExtraStyle(.window)

        Window(L10n.configEditor, id: "config-editor") {
            ConfigEditorView(runtime: runtime)
        }
        .defaultSize(width: 700, height: 500)

        Window(L10n.logViewer, id: "log-viewer") {
            LogViewerView(runtime: runtime)
        }
        .defaultSize(width: 750, height: 500)

        Window(L10n.permissions, id: "permissions") {
            PermissionsView(bookmarks: runtime.bookmarks)
        }
        .defaultSize(width: 560, height: 420)
    }
}
