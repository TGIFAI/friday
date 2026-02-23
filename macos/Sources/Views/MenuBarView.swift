import SwiftUI

struct MenuBarView: View {
    @ObservedObject var runtime: FridayRuntime
    @ObservedObject private var lang = LanguageManager.shared
    @Environment(\.openWindow) private var openWindow
    @State private var languageExpanded = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            headerSection
            sectionDivider
            controlSection
            sectionDivider
            actionSection
            sectionDivider
            languageSection
            sectionDivider
            footerSection
        }
        .frame(width: 280)
        .padding(.vertical, 6)
    }

    // MARK: - Divider

    private var sectionDivider: some View {
        Divider()
            .padding(.horizontal, 14)
            .opacity(0.5)
    }

    // MARK: - Header

    private var headerSection: some View {
        HStack(spacing: 12) {
            ZStack {
                if runtime.isRunning {
                    Circle()
                        .fill(Color.statusActive.opacity(0.2))
                        .frame(width: 36, height: 36)
                        .modifier(PulseModifier())
                }
                Circle()
                    .fill(runtime.isRunning ? Color.statusActive : Color.secondary.opacity(0.25))
                    .frame(width: 28, height: 28)
                Text("F")
                    .font(.system(size: 15, weight: .bold, design: .rounded))
                    .foregroundStyle(runtime.isRunning ? .white : .secondary)
            }
            .frame(width: 36, height: 36)

            VStack(alignment: .leading, spacing: 3) {
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Text(L10n.appName)
                        .font(FridayFont.title)
                        .foregroundStyle(.primary)
                    Text("v\(Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "0.0.0")")
                        .font(FridayFont.badge)
                        .foregroundStyle(.white.opacity(0.7))
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color.fridayAccent.opacity(0.65))
                        .clipShape(Capsule())
                }
                Text(runtime.statusText)
                    .font(FridayFont.caption)
                    .foregroundStyle(statusColor)
            }

            Spacer()
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
    }

    private var statusColor: Color {
        if runtime.isRunning {
            return .statusActive
        }
        if runtime.statusText.contains("Error") || runtime.statusText.contains("错误") {
            return .statusStopped
        }
        return .secondary
    }

    // MARK: - Controls

    private var controlSection: some View {
        HStack(spacing: 8) {
            if runtime.isRunning {
                controlButton(L10n.restartRuntime, icon: "arrow.clockwise", color: .statusWarning) {
                    runtime.restart()
                }
                controlButton(L10n.stopRuntime, icon: "stop.fill", color: .statusStopped) {
                    runtime.stop()
                }
            } else {
                controlButton(L10n.startRuntime, icon: "play.fill", color: .statusActive) {
                    runtime.start()
                }
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }

    private func controlButton(
        _ title: String,
        icon: String,
        color: Color,
        action: @escaping () -> Void
    ) -> some View {
        Button(action: action) {
            HStack(spacing: 5) {
                Image(systemName: icon)
                    .font(.system(size: 11, weight: .bold))
                Text(title)
                    .font(FridayFont.control)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 7)
            .foregroundStyle(color)
            .background(color.opacity(0.12))
            .overlay(
                RoundedRectangle(cornerRadius: 6)
                    .strokeBorder(color.opacity(0.2), lineWidth: 0.5)
            )
            .clipShape(RoundedRectangle(cornerRadius: 6))
        }
        .buttonStyle(.plain)
    }

    // MARK: - Actions

    private var actionSection: some View {
        VStack(spacing: 0) {
            menuRow(L10n.viewConfig, icon: "doc.text") {
                openWindow(id: "config-editor")
                NSApp.activate(ignoringOtherApps: true)
            }
            menuRow(L10n.viewLogs, icon: "text.line.last.and.arrowtriangle.forward") {
                openWindow(id: "log-viewer")
                NSApp.activate(ignoringOtherApps: true)
            }
            menuRow(L10n.showInFinder, icon: "folder") {
                ConfigManager().revealInFinder()
            }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
    }

    // MARK: - Language (expandable submenu)

    private var languageSection: some View {
        VStack(spacing: 0) {
            // Parent row: click to expand/collapse
            Button {
                withAnimation(.easeInOut(duration: 0.15)) {
                    languageExpanded.toggle()
                }
            } label: {
                HStack(spacing: 10) {
                    Image(systemName: "globe")
                        .font(FridayFont.icon)
                        .frame(width: 20, alignment: .center)
                    Text(L10n.language)
                        .font(FridayFont.body)
                    Spacer()
                    Text(lang.current.displayName)
                        .font(FridayFont.caption)
                        .foregroundStyle(.secondary)
                    Image(systemName: "chevron.right")
                        .font(.system(size: 10, weight: .semibold))
                        .foregroundStyle(.tertiary)
                        .rotationEffect(.degrees(languageExpanded ? 90 : 0))
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
                .contentShape(Rectangle())
            }
            .buttonStyle(MenuRowButtonStyle())

            // Submenu items
            if languageExpanded {
                VStack(spacing: 0) {
                    ForEach(AppLanguage.allCases) { language in
                        languageOption(language)
                    }
                }
                .padding(.leading, 30)
                .transition(.opacity.combined(with: .move(edge: .top)))
            }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
    }

    private func languageOption(_ language: AppLanguage) -> some View {
        let isSelected = lang.current == language
        return Button {
            withAnimation(.easeInOut(duration: 0.15)) {
                lang.current = language
            }
        } label: {
            HStack(spacing: 8) {
                Image(systemName: isSelected ? "checkmark" : "")
                    .font(.system(size: 11, weight: .bold))
                    .frame(width: 14, alignment: .center)
                    .foregroundStyle(Color.fridayAccent)
                Text(language.displayName)
                    .font(FridayFont.body)
                    .foregroundStyle(isSelected ? Color.fridayAccent : .primary)
                Spacer()
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
            .contentShape(Rectangle())
        }
        .buttonStyle(MenuRowButtonStyle())
    }

    // MARK: - Footer

    private var footerSection: some View {
        VStack(spacing: 0) {
            menuRow(L10n.quitApp, icon: "power", tint: .statusStopped) {
                runtime.stop()
                NSApplication.shared.terminate(nil)
            }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
    }

    // MARK: - Menu Row

    private func menuRow(
        _ title: String,
        icon: String,
        tint: Color? = nil,
        action: @escaping () -> Void
    ) -> some View {
        Button(action: action) {
            HStack(spacing: 10) {
                Image(systemName: icon)
                    .font(FridayFont.icon)
                    .frame(width: 20, alignment: .center)
                    .foregroundStyle(tint ?? .primary)
                Text(title)
                    .font(FridayFont.body)
                    .foregroundStyle(tint ?? .primary)
                Spacer()
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 7)
            .contentShape(Rectangle())
        }
        .buttonStyle(MenuRowButtonStyle())
    }

}

// MARK: - Pulse Animation

private struct PulseModifier: ViewModifier {
    @State private var isPulsing = false

    func body(content: Content) -> some View {
        content
            .scaleEffect(isPulsing ? 1.15 : 1.0)
            .opacity(isPulsing ? 0.0 : 0.6)
            .animation(
                .easeInOut(duration: 1.8).repeatForever(autoreverses: false),
                value: isPulsing
            )
            .onAppear { isPulsing = true }
    }
}

// MARK: - Menu Row Hover Style

private struct MenuRowButtonStyle: ButtonStyle {
    @State private var isHovering = false

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(
                RoundedRectangle(cornerRadius: 5)
                    .fill(backgroundColor(isPressed: configuration.isPressed))
            )
            .onHover { isHovering = $0 }
    }

    private func backgroundColor(isPressed: Bool) -> Color {
        if isPressed {
            return Color.primary.opacity(0.12)
        }
        if isHovering {
            return Color.primary.opacity(0.06)
        }
        return Color.clear
    }
}
