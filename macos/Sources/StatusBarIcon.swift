import AppKit
import SwiftUI

/// Generates a custom "F" template icon for the macOS status bar.
enum StatusBarIcon {
    /// Creates a template NSImage with a stylized "F" glyph.
    /// macOS automatically renders template images in the correct menu bar color.
    static func make(active: Bool) -> NSImage {
        let size = NSSize(width: 18, height: 18)
        let image = NSImage(size: size, flipped: false) { rect in
            NSGraphicsContext.current?.cgContext.setAllowsAntialiasing(true)

            let inset = rect.insetBy(dx: 1.5, dy: 1.5)

            if active {
                drawActive(in: inset)
            } else {
                drawInactive(in: inset)
            }
            return true
        }
        image.isTemplate = true
        return image
    }

    // MARK: - Active: filled rounded rect + "F"

    private static func drawActive(in rect: NSRect) {
        let bg = NSBezierPath(roundedRect: rect, xRadius: 3.5, yRadius: 3.5)
        NSColor.black.setFill()
        bg.fill()

        // Draw "F" letter in white (knockout)
        let attrs: [NSAttributedString.Key: Any] = [
            .font: NSFont.systemFont(ofSize: 11.5, weight: .bold),
            .foregroundColor: NSColor.white,
        ]
        let str = NSAttributedString(string: "F", attributes: attrs)
        let strSize = str.size()
        let origin = NSPoint(
            x: rect.midX - strSize.width / 2 + 0.5,
            y: rect.midY - strSize.height / 2
        )
        str.draw(at: origin)
    }

    // MARK: - Inactive: outlined rounded rect + "F"

    private static func drawInactive(in rect: NSRect) {
        let bg = NSBezierPath(roundedRect: rect.insetBy(dx: 0.5, dy: 0.5), xRadius: 3.5, yRadius: 3.5)
        NSColor.black.setStroke()
        bg.lineWidth = 1.2
        bg.stroke()

        let attrs: [NSAttributedString.Key: Any] = [
            .font: NSFont.systemFont(ofSize: 11.5, weight: .semibold),
            .foregroundColor: NSColor.black,
        ]
        let str = NSAttributedString(string: "F", attributes: attrs)
        let strSize = str.size()
        let origin = NSPoint(
            x: rect.midX - strSize.width / 2 + 0.5,
            y: rect.midY - strSize.height / 2
        )
        str.draw(at: origin)
    }
}
