import SwiftUI

struct SettingsView: View {
    @ObservedObject var runtime: FridayRuntime
    @ObservedObject private var lang = LanguageManager.shared

    private enum Tab: Hashable {
        case general, apiKeys, logs, language
    }

    @State private var selectedTab: Tab = .general

    var body: some View {
        TabView(selection: $selectedTab) {
            GeneralTab(runtime: runtime)
                .tabItem { Label(L10n.general, systemImage: "gearshape") }
                .tag(Tab.general)

            APIKeysTab()
                .tabItem { Label(L10n.apiKeys, systemImage: "key") }
                .tag(Tab.apiKeys)

            LogsTab(runtime: runtime)
                .tabItem { Label(L10n.logs, systemImage: "doc.text") }
                .tag(Tab.logs)

            LanguageTab()
                .tabItem { Label(L10n.language, systemImage: "globe") }
                .tag(Tab.language)
        }
        .frame(width: 560, height: 420)
    }
}

// MARK: - General

private struct GeneralTab: View {
    @ObservedObject var runtime: FridayRuntime

    var body: some View {
        Form {
            Section(L10n.runtime) {
                LabeledContent(L10n.status) {
                    HStack(spacing: 8) {
                        Circle()
                            .fill(runtime.isRunning ? Color.statusActive : Color.secondary.opacity(0.25))
                            .frame(width: 9, height: 9)
                        Text(runtime.statusText)
                            .font(FridayFont.body)
                            .foregroundStyle(runtime.isRunning ? Color.statusActive : .primary)
                    }
                }
                LabeledContent(L10n.bindAddress) {
                    Text(runtime.bindAddress)
                        .font(FridayFont.mono)
                        .textSelection(.enabled)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(Color.primary.opacity(0.04))
                        .clipShape(RoundedRectangle(cornerRadius: 4))
                }
                LabeledContent("FRIDAY_HOME") {
                    Text(runtime.fridayHome.path)
                        .font(FridayFont.mono)
                        .textSelection(.enabled)
                        .lineLimit(1)
                        .truncationMode(.middle)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(Color.primary.opacity(0.04))
                        .clipShape(RoundedRectangle(cornerRadius: 4))
                }
            }

            Section(L10n.config) {
                HStack(spacing: 10) {
                    Button(L10n.editConfigYaml) {
                        ConfigManager().openInEditor()
                    }
                    Button(L10n.revealInFinder) {
                        ConfigManager().revealInFinder()
                    }
                }
            }

            Section {
                HStack(spacing: 10) {
                    if runtime.isRunning {
                        Button(L10n.restartRuntime) { runtime.restart() }
                        Button(L10n.stopRuntime) { runtime.stop() }
                            .tint(Color.statusStopped)
                    } else {
                        Button(L10n.startRuntime) { runtime.start() }
                            .tint(Color.statusActive)
                    }
                }
            }
        }
        .formStyle(.grouped)
        .padding()
    }
}

// MARK: - API Keys

private struct APIKeysTab: View {
    private let keys = [
        ("OPENAI_API_KEY", "OpenAI"),
        ("ANTHROPIC_API_KEY", "Anthropic"),
        ("GEMINI_API_KEY", "Gemini"),
        ("BRAVE_API_KEY", "Brave Search"),
        ("TELEGRAM_BOT_TOKEN", "Telegram Bot"),
    ]

    @State private var values: [String: String] = [:]
    @State private var showSaved = false

    var body: some View {
        Form {
            Section {
                Text(L10n.apiKeysHint)
                    .font(FridayFont.caption)
                    .foregroundStyle(.secondary)
                    .padding(.bottom, 4)

                ForEach(keys, id: \.0) { key, label in
                    LabeledContent(label) {
                        SecureField("", text: binding(for: key))
                            .font(FridayFont.mono)
                            .textFieldStyle(.roundedBorder)
                            .frame(maxWidth: 300)
                    }
                }
            } header: {
                Text(L10n.providerAPIKeys)
            }

            Section {
                HStack(spacing: 10) {
                    Button(action: saveAll) {
                        HStack(spacing: 4) {
                            if showSaved {
                                Image(systemName: "checkmark")
                                    .foregroundStyle(Color.statusActive)
                            }
                            Text(showSaved ? L10n.saved : L10n.save)
                        }
                    }
                    .disabled(showSaved)
                }
            }
        }
        .formStyle(.grouped)
        .padding()
        .onAppear { loadAll() }
    }

    private func binding(for key: String) -> Binding<String> {
        Binding(
            get: { values[key] ?? "" },
            set: { values[key] = $0 }
        )
    }

    private func loadAll() {
        for (key, _) in keys {
            values[key] = KeychainHelper.get(key) ?? ""
        }
    }

    private func saveAll() {
        for (key, _) in keys {
            let value = values[key] ?? ""
            if value.isEmpty {
                KeychainHelper.delete(key)
            } else {
                KeychainHelper.set(key, value: value)
            }
        }
        withAnimation(.easeInOut(duration: 0.25)) {
            showSaved = true
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
            withAnimation(.easeInOut(duration: 0.25)) {
                showSaved = false
            }
        }
    }
}

// MARK: - Logs

private struct LogsTab: View {
    @ObservedObject var runtime: FridayRuntime

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                Text(L10n.runtimeLogs)
                    .font(FridayFont.body)
                Spacer()
                Text("\(runtime.logs.count)")
                    .font(FridayFont.badge)
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(Color.primary.opacity(0.06))
                    .clipShape(Capsule())
                Button(L10n.clear) {
                    runtime.logs.removeAll()
                }
                .controlSize(.small)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)

            Divider()

            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 0) {
                        ForEach(Array(runtime.logs.enumerated()), id: \.offset) { idx, line in
                            Text(line)
                                .font(FridayFont.monoSmall)
                                .foregroundStyle(.primary)
                                .textSelection(.enabled)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .padding(.horizontal, 16)
                                .padding(.vertical, 3)
                                .background(idx % 2 == 0 ? Color.clear : Color.primary.opacity(0.02))
                                .id(idx)
                        }
                    }
                }
                .onChange(of: runtime.logs.count) { _, newCount in
                    if newCount > 0 {
                        proxy.scrollTo(newCount - 1, anchor: .bottom)
                    }
                }
            }
        }
    }
}

// MARK: - Language

private struct LanguageTab: View {
    @ObservedObject private var lang = LanguageManager.shared

    var body: some View {
        Form {
            Section {
                Text(L10n.languageHint)
                    .font(FridayFont.caption)
                    .foregroundStyle(.secondary)
                    .padding(.bottom, 4)

                Picker(L10n.selectLanguage, selection: $lang.current) {
                    ForEach(AppLanguage.allCases) { language in
                        Text(language.displayName).tag(language)
                    }
                }
                .pickerStyle(.radioGroup)
            } header: {
                Text(L10n.languageSettings)
            }
        }
        .formStyle(.grouped)
        .padding()
    }
}
