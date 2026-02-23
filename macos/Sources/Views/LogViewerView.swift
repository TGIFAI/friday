import SwiftUI

struct LogViewerView: View {
    @ObservedObject var runtime: FridayRuntime
    @ObservedObject private var lang = LanguageManager.shared

    @State private var searchText: String = ""
    @State private var autoScroll: Bool = true

    var body: some View {
        VStack(spacing: 0) {
            toolbar
            Divider()
            logContent
        }
        .frame(minWidth: 650, minHeight: 400)
    }

    // MARK: - Filtered logs

    private var filteredLogs: [(index: Int, line: String)] {
        let enumerated = Array(runtime.logs.enumerated())
        if searchText.isEmpty {
            return enumerated.map { (index: $0.offset, line: $0.element) }
        }
        return enumerated.compactMap { idx, line in
            line.localizedCaseInsensitiveContains(searchText) ? (index: idx, line: line) : nil
        }
    }

    // MARK: - Toolbar

    private var toolbar: some View {
        HStack(spacing: 12) {
            // Status
            HStack(spacing: 6) {
                Circle()
                    .fill(runtime.isRunning ? Color.statusActive : Color.secondary.opacity(0.25))
                    .frame(width: 8, height: 8)
                Text(L10n.runtimeLogs)
                    .font(FridayFont.body)
            }

            // Line count badge
            Text("\(filteredLogs.count)")
                .font(FridayFont.badge)
                .foregroundStyle(.secondary)
                .padding(.horizontal, 7)
                .padding(.vertical, 2)
                .background(Color.primary.opacity(0.06))
                .clipShape(Capsule())

            if !searchText.isEmpty && filteredLogs.count != runtime.logs.count {
                Text(L10n.logFilteredOf(filteredLogs.count, runtime.logs.count))
                    .font(FridayFont.caption)
                    .foregroundStyle(.tertiary)
            }

            Spacer()

            // Search
            HStack(spacing: 6) {
                Image(systemName: "magnifyingglass")
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
                TextField(L10n.logSearch, text: $searchText)
                    .textFieldStyle(.plain)
                    .font(FridayFont.caption)
                    .frame(width: 140)
                if !searchText.isEmpty {
                    Button {
                        searchText = ""
                    } label: {
                        Image(systemName: "xmark.circle.fill")
                            .font(.system(size: 11))
                            .foregroundStyle(.secondary)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 5)
            .background(Color.primary.opacity(0.04))
            .clipShape(RoundedRectangle(cornerRadius: 6))

            // Auto-scroll toggle
            Button {
                autoScroll.toggle()
            } label: {
                Image(systemName: autoScroll ? "arrow.down.to.line.compact" : "arrow.down.to.line")
                    .font(.system(size: 12))
                    .foregroundStyle(autoScroll ? Color.fridayAccent : .secondary)
            }
            .buttonStyle(.plain)
            .help(L10n.logAutoScroll)

            // Clear
            Button(action: { runtime.logs.removeAll() }) {
                Label(L10n.clear, systemImage: "trash")
            }
            .controlSize(.small)

            // Copy all
            Button(action: copyLogs) {
                Label(L10n.logCopy, systemImage: "doc.on.doc")
            }
            .controlSize(.small)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(Color.primary.opacity(0.02))
    }

    // MARK: - Log Content

    private var logContent: some View {
        Group {
            if runtime.logs.isEmpty {
                VStack(spacing: 12) {
                    Image(systemName: "text.alignleft")
                        .font(.system(size: 32))
                        .foregroundStyle(.tertiary)
                    Text(L10n.logEmpty)
                        .font(FridayFont.body)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                ScrollViewReader { proxy in
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: 0) {
                            ForEach(filteredLogs, id: \.index) { item in
                                logLine(item.line, index: item.index)
                            }
                        }
                    }
                    .onChange(of: runtime.logs.count) { _, newCount in
                        if autoScroll, newCount > 0, let last = filteredLogs.last {
                            proxy.scrollTo(last.index, anchor: .bottom)
                        }
                    }
                }
            }
        }
    }

    private func logLine(_ line: String, index: Int) -> some View {
        HStack(alignment: .top, spacing: 0) {
            // Line number
            Text("\(index + 1)")
                .font(FridayFont.monoSmall)
                .foregroundStyle(.tertiary)
                .frame(width: 44, alignment: .trailing)
                .padding(.trailing, 10)

            // Content with highlight
            if searchText.isEmpty {
                Text(line)
                    .font(FridayFont.monoSmall)
                    .foregroundStyle(logColor(for: line))
                    .textSelection(.enabled)
            } else {
                highlightedText(line)
                    .textSelection(.enabled)
            }

            Spacer(minLength: 0)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 2)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(index % 2 == 0 ? Color.clear : Color.primary.opacity(0.02))
        .id(index)
    }

    private func logColor(for line: String) -> Color {
        let lower = line.lowercased()
        if lower.contains("error") || lower.contains("fatal") || lower.contains("panic") {
            return Color.statusStopped
        }
        if lower.contains("warn") {
            return Color.statusWarning
        }
        return .primary
    }

    private func highlightedText(_ line: String) -> Text {
        guard !searchText.isEmpty else {
            return Text(line).font(FridayFont.monoSmall)
        }

        var result = Text("")
        var remaining = line[line.startIndex...]

        while let range = remaining.range(of: searchText, options: .caseInsensitive) {
            // Text before match
            if remaining.startIndex < range.lowerBound {
                let before = String(remaining[remaining.startIndex..<range.lowerBound])
                result = result + Text(before).font(FridayFont.monoSmall)
            }
            // Match itself — bold + underline (Text concatenation requires Text return)
            let match = String(remaining[range])
            result = result + Text(match)
                .font(FridayFont.monoSmall)
                .bold()
                .foregroundColor(Color.fridayAccent)
                .underline()
            remaining = remaining[range.upperBound...]
        }

        // Remaining text after last match
        if !remaining.isEmpty {
            result = result + Text(String(remaining)).font(FridayFont.monoSmall)
        }

        return result
    }

    // MARK: - Actions

    private func copyLogs() {
        let text = runtime.logs.joined(separator: "\n")
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
    }
}
