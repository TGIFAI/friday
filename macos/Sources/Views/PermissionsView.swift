import SwiftUI

struct PermissionsView: View {
    @ObservedObject var bookmarks: BookmarkManager
    @State private var selectedTab = 0

    var body: some View {
        TabView(selection: $selectedTab) {
            directoryTab
                .tabItem {
                    Label(L10n.permDirectories, systemImage: "folder.badge.gearshape")
                }
                .tag(0)
        }
        .frame(minWidth: 520, minHeight: 380)
    }

    // MARK: - Directory Access Tab

    private var directoryTab: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header
            VStack(alignment: .leading, spacing: 6) {
                Text(L10n.permDirectories)
                    .font(FridayFont.title)
                Text(L10n.permDirectoriesDesc)
                    .font(FridayFont.caption)
                    .foregroundStyle(.secondary)
            }
            .padding(.horizontal, 20)
            .padding(.top, 20)
            .padding(.bottom, 14)

            Divider().padding(.horizontal, 16)

            // Directory list
            if bookmarks.directories.isEmpty {
                Spacer()
                VStack(spacing: 10) {
                    Image(systemName: "folder.badge.plus")
                        .font(.system(size: 32))
                        .foregroundStyle(.tertiary)
                    Text(L10n.permEmpty)
                        .font(FridayFont.body)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity)
                Spacer()
            } else {
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(Array(bookmarks.directories.enumerated()), id: \.offset) { index, url in
                            directoryRow(url: url, index: index)
                            if index < bookmarks.directories.count - 1 {
                                Divider().padding(.leading, 48)
                            }
                        }
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                }
            }

            Divider().padding(.horizontal, 16)

            // Footer: Add button + hint
            HStack {
                Button {
                    bookmarks.addDirectory()
                } label: {
                    HStack(spacing: 5) {
                        Image(systemName: "plus")
                            .font(.system(size: 11, weight: .bold))
                        Text(L10n.permAdd)
                            .font(FridayFont.control)
                    }
                }
                .buttonStyle(.borderedProminent)
                .tint(Color.fridayAccent)

                Spacer()

                Text(L10n.permHint)
                    .font(FridayFont.caption)
                    .foregroundStyle(.tertiary)
                    .lineLimit(2)
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
    }

    private func directoryRow(url: URL, index: Int) -> some View {
        HStack(spacing: 10) {
            Image(systemName: "folder.fill")
                .font(.system(size: 16))
                .foregroundStyle(Color.fridayAccent)
                .frame(width: 28, alignment: .center)

            VStack(alignment: .leading, spacing: 2) {
                Text(url.lastPathComponent)
                    .font(FridayFont.body)
                    .lineLimit(1)
                Text(url.path)
                    .font(FridayFont.mono)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }

            Spacer()

            Button {
                bookmarks.removeDirectory(at: index)
            } label: {
                Image(systemName: "xmark.circle.fill")
                    .font(.system(size: 14))
                    .foregroundStyle(.tertiary)
            }
            .buttonStyle(.plain)
            .help(L10n.permRemove)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 8)
        .contentShape(Rectangle())
    }
}
