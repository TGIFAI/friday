import Foundation

/// Centralized localized strings with runtime language switching.
@MainActor
enum L10n {
    // MARK: - Menu Bar

    static var appName: String { localized("Friday", "Friday") }
    static var running: String { localized("Running", "运行中") }
    static var stopped: String { localized("Stopped", "已停止") }
    static var starting: String { localized("Starting...", "启动中...") }

    static var startRuntime: String { localized("Start", "启动") }
    static var stopRuntime: String { localized("Stop", "停止") }
    static var restartRuntime: String { localized("Restart", "重启") }

    static var editConfig: String { localized("Edit Config...", "编辑配置...") }
    static var showInFinder: String { localized("Show in Finder", "在 Finder 中显示") }
    static var settings: String { localized("Settings...", "设置...") }
    static var quitApp: String { localized("Quit Friday", "退出 Friday") }

    // MARK: - Settings Tabs

    static var general: String { localized("General", "通用") }
    static var apiKeys: String { localized("API Keys", "API 密钥") }
    static var logs: String { localized("Logs", "日志") }
    static var language: String { localized("Language", "语言") }

    // MARK: - General Tab

    static var runtime: String { localized("Runtime", "运行时") }
    static var status: String { localized("Status", "状态") }
    static var bindAddress: String { localized("Bind Address", "绑定地址") }
    static var config: String { localized("Config", "配置") }
    static var editConfigYaml: String { localized("Edit config.yaml", "编辑 config.yaml") }
    static var revealInFinder: String { localized("Reveal in Finder", "在 Finder 中显示") }

    // MARK: - API Keys Tab

    static var providerAPIKeys: String { localized("Provider API Keys", "服务商 API 密钥") }
    static var apiKeysHint: String {
        localized(
            "Keys are stored in macOS Keychain and injected as environment variables.",
            "密钥存储在 macOS 钥匙串中，并作为环境变量注入。"
        )
    }
    static var save: String { localized("Save", "保存") }
    static var saved: String { localized("Saved", "已保存") }

    // MARK: - Logs Tab

    static var runtimeLogs: String { localized("Runtime Logs", "运行日志") }
    static var clear: String { localized("Clear", "清空") }

    // MARK: - Language Tab

    static var languageSettings: String { localized("Language Settings", "语言设置") }
    static var selectLanguage: String { localized("Display Language", "显示语言") }
    static var languageHint: String {
        localized(
            "Choose the display language for the app interface.",
            "选择应用界面的显示语言。"
        )
    }

    // MARK: - Errors

    static func exitCode(_ code: Int32) -> String {
        localized("Exited (\(code))", "已退出 (\(code))")
    }

    static func startFailed(_ error: String) -> String {
        localized("Error: \(error)", "错误：\(error)")
    }

    // MARK: - Private

    private static func localized(_ en: String, _ zh: String) -> String {
        switch LanguageManager.shared.current {
        case .english: return en
        case .chinese: return zh
        }
    }
}
