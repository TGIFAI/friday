import SwiftUI

// MARK: - Color Palette

extension Color {
    /// Friday brand accent — warm amber, evokes "TGIF" energy.
    static let fridayAccent = Color(red: 0.93, green: 0.65, blue: 0.18)

    /// Status colors with guaranteed visibility in both appearances.
    static let statusActive = Color(red: 0.30, green: 0.78, blue: 0.40)
    static let statusStopped = Color(red: 0.78, green: 0.30, blue: 0.30)
    static let statusWarning = Color(red: 0.92, green: 0.58, blue: 0.16)
}

// MARK: - Typography

/// Consistent type scale for the menu bar and settings.
enum FridayFont {
    /// App name in header — 14pt semibold.
    static let title = Font.system(size: 14, weight: .semibold, design: .default)

    /// Section headers and menu row labels — 13pt medium.
    static let body = Font.system(size: 13, weight: .medium, design: .default)

    /// Status text, descriptions — 12pt regular.
    static let caption = Font.system(size: 12, weight: .regular, design: .default)

    /// Version badge, tiny labels — 10pt medium.
    static let badge = Font.system(size: 10, weight: .medium, design: .rounded)

    /// Monospaced for paths, addresses, logs — 12pt mono.
    static let mono = Font.system(size: 12, weight: .regular, design: .monospaced)

    /// Smaller mono for log output — 11pt mono.
    static let monoSmall = Font.system(size: 11, weight: .regular, design: .monospaced)

    /// Control button labels — 12pt semibold.
    static let control = Font.system(size: 12, weight: .semibold, design: .default)

    /// Menu row icon size.
    static let icon = Font.system(size: 13, weight: .medium)
}
