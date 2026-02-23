import SwiftUI

struct ConfigEditorView: View {
    @ObservedObject var runtime: FridayRuntime
    @ObservedObject private var lang = LanguageManager.shared

    @State private var content: String = ""
    @State private var originalContent: String = ""
    @State private var saveState: SaveState = .idle
    @State private var errorMessage: String?

    private enum SaveState { case idle, saved }

    private let configManager = ConfigManager()
    private let guideURL = URL(string: "https://tgif.sh/pilot")!

    var body: some View {
        VStack(spacing: 0) {
            toolbar
            Divider()
            editor
        }
        .frame(minWidth: 600, minHeight: 450)
        .onAppear { loadConfig() }
    }

    // MARK: - Toolbar

    private var toolbar: some View {
        HStack(spacing: 12) {
            HStack(spacing: 6) {
                Image(systemName: "doc.text.fill")
                    .font(.system(size: 12))
                    .foregroundStyle(Color.fridayAccent)
                Text(configManager.configURL.lastPathComponent)
                    .font(FridayFont.body)
            }

            if hasChanges {
                Text(L10n.configUnsaved)
                    .font(FridayFont.badge)
                    .foregroundStyle(.white)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 3)
                    .background(Color.statusWarning)
                    .clipShape(Capsule())
            }

            if saveState == .saved {
                HStack(spacing: 4) {
                    Image(systemName: "checkmark.circle.fill")
                    Text(L10n.saved)
                }
                .font(FridayFont.caption)
                .foregroundStyle(Color.statusActive)
                .transition(.opacity)
            }

            Spacer()

            // Config guide link
            Button {
                NSWorkspace.shared.open(guideURL)
            } label: {
                HStack(spacing: 4) {
                    Image(systemName: "questionmark.circle")
                    Text(L10n.configGuide)
                }
            }
            .controlSize(.small)

            Button(action: loadConfig) {
                Label(L10n.configReload, systemImage: "arrow.clockwise")
            }
            .controlSize(.small)

            Button(action: { configManager.openInEditor() }) {
                Label(L10n.configOpenExternal, systemImage: "arrow.up.forward.square")
            }
            .controlSize(.small)

            Button(action: saveConfig) {
                Label(L10n.save, systemImage: "square.and.arrow.down")
            }
            .controlSize(.small)
            .disabled(!hasChanges)
            .keyboardShortcut("s", modifiers: .command)

            if runtime.isRunning && hasChanges {
                Button(action: saveAndRestart) {
                    Label(L10n.configSaveRestart, systemImage: "arrow.clockwise")
                }
                .controlSize(.small)
                .tint(Color.statusWarning)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(Color.primary.opacity(0.02))
    }

    // MARK: - Editor

    private var editor: some View {
        ZStack(alignment: .topLeading) {
            TextEditor(text: $content)
                .font(.system(size: 13, weight: .regular, design: .monospaced))
                .scrollContentBackground(.hidden)
                .padding(8)

            // Error banner
            if let errorMessage {
                HStack(spacing: 8) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(Color.statusStopped)
                    Text(errorMessage)
                        .font(FridayFont.caption)
                    Spacer()
                    Button {
                        withAnimation { self.errorMessage = nil }
                    } label: {
                        Image(systemName: "xmark")
                            .font(.system(size: 10, weight: .bold))
                    }
                    .buttonStyle(.plain)
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .background(.ultraThinMaterial)
                .clipShape(RoundedRectangle(cornerRadius: 6))
                .padding(12)
                .transition(.move(edge: .top).combined(with: .opacity))
            }
        }
    }

    // MARK: - State

    private var hasChanges: Bool { content != originalContent }

    // MARK: - Actions

    private func loadConfig() {
        do {
            try configManager.initializeIfNeeded()
            let text = try String(contentsOf: configManager.configURL, encoding: .utf8)
            content = text
            originalContent = text
            withAnimation { errorMessage = nil }
        } catch {
            withAnimation { errorMessage = error.localizedDescription }
        }
    }

    private func saveConfig() {
        do {
            try content.write(to: configManager.configURL, atomically: true, encoding: .utf8)
            originalContent = content
            withAnimation(.easeInOut(duration: 0.2)) {
                saveState = .saved
                errorMessage = nil
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) {
                withAnimation { saveState = .idle }
            }
        } catch {
            withAnimation { errorMessage = error.localizedDescription }
        }
    }

    private func saveAndRestart() {
        saveConfig()
        runtime.restart()
    }
}
