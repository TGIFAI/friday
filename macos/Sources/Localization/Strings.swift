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

    static var editConfig: String { localized("Edit Config", "编辑配置") }
    static var showInFinder: String { localized("Show in Finder", "在 Finder 中显示") }
    static var quitApp: String { localized("Quit", "退出") }

    static var language: String { localized("Language", "语言") }
    static var save: String { localized("Save", "保存") }
    static var saved: String { localized("Saved", "已保存") }
    static var clear: String { localized("Clear", "清空") }

    // MARK: - Config Editor

    static var configEditor: String { localized("Config", "配置编辑") }
    static var configReload: String { localized("Reload", "重新加载") }
    static var configOpenExternal: String { localized("Open in Editor", "用编辑器打开") }
    static var configUnsaved: String { localized("Unsaved", "未保存") }
    static var configNotFound: String { localized("config.yaml not found", "未找到 config.yaml") }
    static var viewConfig: String { localized("Config", "配置编辑") }
    static var configGuide: String { localized("Config Guide", "配置指南") }
    static var configSaveRestart: String { localized("Save & Restart", "保存并重启") }

    // MARK: - Log Viewer

    static var runtimeLogs: String { localized("Runtime Logs", "运行日志") }
    static var logViewer: String { localized("Log Viewer", "日志查看") }
    static var viewLogs: String { localized("Log Viewer", "日志查看") }
    static var logSearch: String { localized("Search...", "搜索...") }
    static var logAutoScroll: String { localized("Auto-scroll", "自动滚动") }
    static var logCopy: String { localized("Copy All", "全部复制") }
    static var logEmpty: String { localized("No logs yet", "暂无日志") }

    static func logFilteredOf(_ shown: Int, _ total: Int) -> String {
        localized("\(shown) of \(total)", "\(shown) / \(total)")
    }

    // MARK: - Power

    static var launchAtLogin: String { localized("Launch at Login", "开机自动启动") }
    static var preventSleep: String { localized("Prevent Sleep", "阻止休眠") }
    static var preventLidSleep: String { localized("Prevent Lid Sleep", "阻止合盖休眠") }

    // MARK: - Permissions

    static var permissions: String { localized("Permissions", "权限管理") }
    static var permDirectories: String { localized("Directory Access", "目录访问") }
    static var permDirectoriesDesc: String {
        localized(
            "Grant Friday access to files and directories outside its sandbox. Bookmarks persist across restarts.",
            "授予 Friday 访问沙箱外文件和目录的权限，授权在重启后仍然有效。"
        )
    }
    static var permEmpty: String { localized("No bookmarks added", "暂无授权条目") }
    static var permAdd: String { localized("Add Directory", "添加目录") }
    static var permAddFile: String { localized("Add File", "添加文件") }
    static var permAddMessage: String {
        localized(
            "Choose a directory to grant Friday access",
            "选择一个目录以授予 Friday 访问权限"
        )
    }
    static var permAddFileMessage: String {
        localized(
            "Choose a file to grant Friday access",
            "选择一个文件以授予 Friday 访问权限"
        )
    }
    static var permGrant: String { localized("Grant Access", "授权访问") }
    static var permPathPlaceholder: String {
        localized(
            "Enter path, e.g. ~/.local or /opt/homebrew",
            "输入路径，如 ~/.local 或 /opt/homebrew"
        )
    }
    static var permRemove: String { localized("Revoke Access", "撤销访问") }
    static var permHint: String {
        localized(
            "Bookmarks persist across app restarts.",
            "授权在应用重启后仍然有效。"
        )
    }
    static var noSandbox: String { localized("No Sandbox", "非沙箱模式") }
    static var permSuggested: String { localized("Suggested", "推荐授权") }
    static var permSuggestedDesc: String {
        localized(
            "Common developer directories detected on your system. Grant access so Friday can use tools like brew and git.",
            "检测到系统中的常用开发目录，授权后 Friday 可正常使用 brew、git 等工具。"
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
