import AppKit
import Foundation

/// Manages security-scoped bookmarks for sandbox-safe directory access.
/// Bookmarks persist across app restarts so Friday agents can access
/// user-approved directories without re-prompting.
@MainActor
final class BookmarkManager: ObservableObject {
    @Published private(set) var directories: [URL] = []
    @Published private(set) var files: [URL] = []

    private static let bookmarksKey = "securityScopedBookmarks"
    private static let fileBookmarksKey = "securityScopedFileBookmarks"

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

        // Restore file bookmarks.
        restoreFileBookmarks()
    }

    private func restoreFileBookmarks() {
        guard let datas = UserDefaults.standard.array(forKey: Self.fileBookmarksKey) as? [Data] else {
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

        UserDefaults.standard.set(validBookmarks, forKey: Self.fileBookmarksKey)
        files = restored
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

    // MARK: - File Bookmarks

    /// Present an open panel to let the user pick a file, then create a
    /// security-scoped bookmark for it.
    func addFile() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = false
        panel.canChooseFiles = true
        panel.allowsMultipleSelection = false
        panel.showsHiddenFiles = true
        panel.message = L10n.permAddFileMessage
        panel.prompt = L10n.permGrant

        guard panel.runModal() == .OK, let url = panel.url else { return }

        if files.contains(where: { $0.path == url.path }) { return }

        guard let bookmarkData = try? url.bookmarkData(
            options: .withSecurityScope,
            includingResourceValuesForKeys: nil,
            relativeTo: nil
        ) else { return }

        var stored = (UserDefaults.standard.array(forKey: Self.fileBookmarksKey) as? [Data]) ?? []
        stored.append(bookmarkData)
        UserDefaults.standard.set(stored, forKey: Self.fileBookmarksKey)

        if url.startAccessingSecurityScopedResource() {
            files.append(url)
        }
    }

    /// Bookmark a specific file URL (triggered by "suggested file" buttons).
    func addFile(url: URL) {
        if files.contains(where: { $0.path == url.path }) { return }

        let panel = NSOpenPanel()
        panel.canChooseDirectories = false
        panel.canChooseFiles = true
        panel.allowsMultipleSelection = false
        panel.showsHiddenFiles = true
        panel.directoryURL = url.deletingLastPathComponent()
        panel.message = L10n.permAddFileMessage
        panel.prompt = L10n.permGrant

        guard panel.runModal() == .OK, let selected = panel.url else { return }

        guard let bookmarkData = try? selected.bookmarkData(
            options: .withSecurityScope,
            includingResourceValuesForKeys: nil,
            relativeTo: nil
        ) else { return }

        var stored = (UserDefaults.standard.array(forKey: Self.fileBookmarksKey) as? [Data]) ?? []
        stored.append(bookmarkData)
        UserDefaults.standard.set(stored, forKey: Self.fileBookmarksKey)

        if selected.startAccessingSecurityScopedResource() {
            files.append(selected)
        }
    }

    /// Remove a file bookmark by index.
    func removeFile(at index: Int) {
        guard files.indices.contains(index) else { return }
        let url = files[index]
        url.stopAccessingSecurityScopedResource()
        files.remove(at: index)

        var stored = (UserDefaults.standard.array(forKey: Self.fileBookmarksKey) as? [Data]) ?? []
        if stored.indices.contains(index) {
            stored.remove(at: index)
        }
        UserDefaults.standard.set(stored, forKey: Self.fileBookmarksKey)
    }

    // MARK: - Allowed Paths

    /// All accessible paths as strings (for injecting into env).
    var allowedPathStrings: [String] {
        (directories + files).map(\.path)
    }

    /// Detect common developer directories that exist but are not yet bookmarked.
    var suggestedPaths: [URL] {
        let home = FileManager.default.homeDirectoryForCurrentUser
        let candidates = [
            URL(fileURLWithPath: "/opt/homebrew"),
            URL(fileURLWithPath: "/usr/local"),
            home,
        ]

        let bookmarked = Set(directories.map(\.path))
        return candidates.filter { url in
            FileManager.default.fileExists(atPath: url.path) && !bookmarked.contains(url.path)
        }
    }

    /// Detect common developer config files that exist but are not yet bookmarked.
    var suggestedFiles: [URL] {
        let home = FileManager.default.homeDirectoryForCurrentUser
        let candidates = [
            home.appendingPathComponent(".gitconfig"),
        ]

        let bookmarked = Set(files.map(\.path))
        return candidates.filter { url in
            FileManager.default.fileExists(atPath: url.path) && !bookmarked.contains(url.path)
        }
    }

    /// Bookmark a specific URL programmatically (triggered by "suggested path" buttons).
    func addDirectory(url: URL) {
        if directories.contains(where: { $0.path == url.path }) { return }

        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.directoryURL = url
        panel.message = L10n.permAddMessage
        panel.prompt = L10n.permGrant

        guard panel.runModal() == .OK, let selected = panel.url else { return }

        guard let bookmarkData = try? selected.bookmarkData(
            options: .withSecurityScope,
            includingResourceValuesForKeys: nil,
            relativeTo: nil
        ) else { return }

        var stored = (UserDefaults.standard.array(forKey: Self.bookmarksKey) as? [Data]) ?? []
        stored.append(bookmarkData)
        UserDefaults.standard.set(stored, forKey: Self.bookmarksKey)

        if selected.startAccessingSecurityScopedResource() {
            directories.append(selected)
        }
    }
}
