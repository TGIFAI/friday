import AppKit
import Foundation

/// Manages security-scoped bookmarks for sandbox-safe directory access.
/// Bookmarks persist across app restarts so Friday agents can access
/// user-approved directories without re-prompting.
@MainActor
final class BookmarkManager: ObservableObject {
    @Published private(set) var directories: [URL] = []

    private static let bookmarksKey = "securityScopedBookmarks"

    /// Restore all previously saved bookmarks on launch.
    func restoreBookmarks() {
        guard let datas = UserDefaults.standard.array(forKey: Self.bookmarksKey) as? [Data] else {
            return
        }

        var restored: [URL] = []
        var validBookmarks: [Data] = []

        for data in datas {
            var isStale = false
            guard let url = try? URL(
                resolvingBookmarkData: data,
                options: .withSecurityScope,
                relativeTo: nil,
                bookmarkDataIsStale: &isStale
            ) else { continue }

            if isStale {
                // Re-create the bookmark if it became stale.
                if let refreshed = try? url.bookmarkData(
                    options: .withSecurityScope,
                    includingResourceValuesForKeys: nil,
                    relativeTo: nil
                ) {
                    validBookmarks.append(refreshed)
                }
            } else {
                validBookmarks.append(data)
            }

            if url.startAccessingSecurityScopedResource() {
                restored.append(url)
            }
        }

        UserDefaults.standard.set(validBookmarks, forKey: Self.bookmarksKey)
        directories = restored
    }

    /// Present an open panel to let the user pick a directory, then create a
    /// security-scoped bookmark for it.
    func addDirectory() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.message = L10n.permAddMessage
        panel.prompt = L10n.permGrant

        guard panel.runModal() == .OK, let url = panel.url else { return }

        // Avoid duplicates.
        if directories.contains(where: { $0.path == url.path }) { return }

        guard let bookmarkData = try? url.bookmarkData(
            options: .withSecurityScope,
            includingResourceValuesForKeys: nil,
            relativeTo: nil
        ) else { return }

        // Persist bookmark data.
        var stored = (UserDefaults.standard.array(forKey: Self.bookmarksKey) as? [Data]) ?? []
        stored.append(bookmarkData)
        UserDefaults.standard.set(stored, forKey: Self.bookmarksKey)

        // Start accessing.
        if url.startAccessingSecurityScopedResource() {
            directories.append(url)
        }
    }

    /// Remove a directory bookmark by index.
    func removeDirectory(at index: Int) {
        guard directories.indices.contains(index) else { return }
        let url = directories[index]
        url.stopAccessingSecurityScopedResource()
        directories.remove(at: index)

        // Update persisted bookmarks.
        var stored = (UserDefaults.standard.array(forKey: Self.bookmarksKey) as? [Data]) ?? []
        if stored.indices.contains(index) {
            stored.remove(at: index)
        }
        UserDefaults.standard.set(stored, forKey: Self.bookmarksKey)
    }

    /// All accessible directory paths as strings (for injecting into env).
    var allowedPathStrings: [String] {
        directories.map(\.path)
    }
}
