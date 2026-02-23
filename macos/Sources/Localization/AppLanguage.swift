import SwiftUI

/// Supported app languages with in-app switching.
enum AppLanguage: String, CaseIterable, Identifiable {
    case english = "en"
    case chinese = "zh-Hans"

    var id: String { rawValue }

    /// Display name in its own language.
    var displayName: String {
        switch self {
        case .english: return "English"
        case .chinese: return "简体中文"
        }
    }
}

// MARK: - Global language state

/// Observable language setting persisted via @AppStorage.
@MainActor
final class LanguageManager: ObservableObject {
    static let shared = LanguageManager()

    @AppStorage("appLanguage") var current: AppLanguage = .english {
        didSet { objectWillChange.send() }
    }

    private init() {}
}
