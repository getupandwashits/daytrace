import Foundation

public struct AddedLine {
    public var stamp: Date
    public var app: String
    public var title: String
    public var line: String
    public var part: Int

    public init(stamp: Date, app: String, title: String, line: String, part: Int) {
        self.stamp = stamp
        self.app = app
        self.title = title
        self.line = line
        self.part = part
    }
}

public struct DigestStats {
    public var digestChars = 0
    public var sourceChars = 0
    public var excerptChars = 0
    public var blocks = 0
    public var truncatedBlocks = 0
    public var dropped: [String: Int] = [:]
    public var maskedSecrets = 0
}

public enum DigestEngine {
    public static let mask = "〔已遮蔽〕"
    public static let defaultSkipApps: Set<String> = ["Clash Party"]
    public static let passwordManagers: Set<String> = [
        "Bitwarden", "1Password", "1Password 7", "KeePassXC", "Passwords", "Keychain Access",
    ]

    private static let wordlike = try! NSRegularExpression(pattern: "[0-9A-Za-z\\u3400-\\u9fff\\uac00-\\ud7af\\u3040-\\u30ff]")
    private static let vaultTitle = try! NSRegularExpression(
        pattern: "vaultwarden|bitwarden|1password|keepass|密码库|password manager",
        options: [.caseInsensitive]
    )
    private static let vaultLine = try! NSRegularExpression(pattern: "^View item - |^查看项目 - ")
    private static let whitespace = try! NSRegularExpression(pattern: "\\s+")
    private static let digits = try! NSRegularExpression(pattern: "\\d+")
    private static let punctuation = CharacterSet(charactersIn: "。！？；，：.,;:?!=()[]{}/")

    private static let secretPatterns: [NSRegularExpression] = [
        try! NSRegularExpression(pattern: "\\bsk-(?:ant-|proj-)?[A-Za-z0-9_-]{16,}"),
        try! NSRegularExpression(pattern: "\\bgh[pousr]_[A-Za-z0-9]{20,}"),
        try! NSRegularExpression(pattern: "\\b(?:AKIA|ASIA)[0-9A-Z]{16}\\b"),
        try! NSRegularExpression(pattern: "\\btvly-[A-Za-z0-9_-]{16,}"),
        try! NSRegularExpression(pattern: "\\bAIza[0-9A-Za-z_-]{30,}"),
        try! NSRegularExpression(pattern: "\\bxox[abprs]-[A-Za-z0-9-]{10,}"),
        try! NSRegularExpression(pattern: "\\beyJ[A-Za-z0-9_-]{10,}\\.[A-Za-z0-9_-]{10,}\\.[A-Za-z0-9_-]{10,}"),
        try! NSRegularExpression(pattern: "\\b(?=[A-Z2-7]*[2-7])(?=[A-Z2-7]*[A-Z])[A-Z2-7]{16,64}\\b"),
    ]

    private static let passwordValue = try! NSRegularExpression(
        pattern: "((?:主密码|确认主密码|密码|口令|passcode|password|PIN)\\s*[*＊]?\\s*(?:[(（]必填[)）])?\\s*(?:[:：=]|·)\\s*)(?!加载中|Loading)([^\\s·]{6,})",
        options: [.caseInsensitive]
    )

    private struct Block {
        var first: Date
        var last: Date
        var lines: [(String, Int)]
        var parts: Set<Int>
        var sourceChars: Int
    }

    public static func build(
        added: [AddedLine],
        blockSeconds: [BlockKey: TimeInterval],
        notice: String,
        skipApps: Set<String> = defaultSkipApps
    ) -> (String, DigestStats) {
        let calendar = DayTracePaths.beijingCalendar()
        let skip = skipApps.union(passwordManagers)
        let superseded = supersededLines(added)
        var seenNormalized: [String: Set<String>] = [:]
        var dropped: [String: Int] = [:]
        var blocks: [BlockKey: Block] = [:]
        var totalSource = 0

        for item in added {
            totalSource += item.line.count
            let title = normalizedTitle(item.title)
            let hour = calendar.dateInterval(of: .hour, for: item.stamp)?.start ?? item.stamp
            let key = BlockKey(hour: hour, app: item.app, title: title)
            var block = blocks[key] ?? Block(first: item.stamp, last: item.stamp, lines: [], parts: [], sourceChars: 0)
            block.first = min(block.first, item.stamp)
            block.last = max(block.last, item.stamp)
            block.parts.insert(item.part)
            block.sourceChars += item.line.count
            blocks[key] = block

            guard !skip.contains(item.app) else {
                dropped["skipped_app", default: 0] += item.line.count
                continue
            }
            if matches(vaultTitle, title) || matches(vaultLine, item.line) {
                dropped["password_manager_page", default: 0] += item.line.count
                continue
            }
            if superseded.contains("\(item.app)\u{0}\(item.title)\u{0}\(item.line)") {
                dropped["streaming_draft", default: 0] += item.line.count
                continue
            }
            if !matches(wordlike, item.line) {
                dropped["decoration", default: 0] += item.line.count
                continue
            }
            let rawTitle = item.title.trimmingCharacters(in: .whitespacesAndNewlines)
            if !rawTitle.isEmpty,
               item.line.trimmingCharacters(in: .whitespaces) == rawTitle
                || item.line.trimmingCharacters(in: .whitespaces).hasPrefix(rawTitle + " - ") {
                dropped["title_echo", default: 0] += item.line.count
                continue
            }
            let normalized = normalize(item.line)
            if seenNormalized[item.app, default: []].contains(normalized) {
                dropped["digit_or_space_variant", default: 0] += item.line.count
                continue
            }
            seenNormalized[item.app, default: []].insert(normalized)

            var updated = blocks[key]!
            updated.lines.append((item.line, item.part))
            blocks[key] = updated
        }

        var secondsByBlock: [BlockKey: TimeInterval] = [:]
        for (key, seconds) in blockSeconds {
            let normalized = BlockKey(hour: key.hour, app: key.app, title: normalizedTitle(key.title))
            secondsByBlock[normalized, default: 0] += seconds
        }

        var out = "# 活动摘要（机械压缩，不是模型总结）\n\n\(notice)\n\n"
        out += "按「小时｜应用｜窗口」分块。已去掉同一窗口里后来被补全的流式草稿、只差数字或空白的变体、"
        out += "纯装饰线、重复窗口标题的行、状态刷新文字和密码管理器文字；"
        out += "每块按活跃用时分配字数，超出时优先保留正文，省略处标注段号。"
        out += "除 API Key 形状的字符串和密码字段值（显示为\(mask)）外，没有改写文字。\n\n"

        var excerpt = 0
        var truncatedBlocks = 0
        var masked = 0
        let grouped = Dictionary(grouping: blocks.keys) { $0.hour }

        for hour in grouped.keys.sorted() {
            let hourBlocks = grouped[hour]!.sorted { blocks[$0]!.first < blocks[$1]!.first }
            var entries: [String] = []
            var minor: [String] = []
            for key in hourBlocks {
                let block = blocks[key]!
                let seconds = secondsByBlock[key] ?? 0
                let ranges = partRanges(block.parts)
                let name = key.app + (key.title.isEmpty ? "" : "｜" + String(key.title.prefix(60)))
                if seconds < 60 {
                    minor.append("\(name)（\(duration(seconds))，段 \(ranges)）")
                    continue
                }

                var lines = joinFragments(block.lines)
                let budget = min(3000, 120 + Int(2.5 * seconds))
                var labelCount = 0
                let textChars = lines.reduce(0) { $0 + $1.0.count }
                if textChars > budget {
                    let content = lines.filter { $0.0.count >= 16 }
                    labelCount = lines.count - content.count
                    lines = content
                }
                let selection = select(lines: lines, budget: budget)
                if selection.omitted.isEmpty {
                    // no truncation
                } else {
                    truncatedBlocks += 1
                }
                excerpt += selection.used

                var body: [String] = []
                var omittedRun: [(String, Int)] = []
                // Rebuild in original order: kept lines stay in place, omitted runs collapse.
                let keptParts = Set(selection.kept.map { "\($0.0)\u{0}\($0.1)" })
                for (line, part) in lines {
                    let identifier = "\(line)\u{0}\(part)"
                    if keptParts.contains(identifier) {
                        if !omittedRun.isEmpty {
                            body.append(omissionMarker(omittedRun))
                            omittedRun.removeAll()
                        }
                        let maskedLine = maskLine(line)
                        masked += maskedLine.1
                        body.append(maskedLine.0)
                    } else {
                        omittedRun.append((line, part))
                    }
                }
                if !omittedRun.isEmpty, !selection.kept.isEmpty {
                    body.append(omissionMarker(omittedRun))
                }
                if body.isEmpty {
                    minor.append("\(name)（\(duration(seconds))，无新增正文，段 \(ranges)）")
                    continue
                }
                if labelCount > 0 {
                    body.append("（另略去 \(labelCount) 个短界面标签）")
                }

                let longestFence = body.map { longestBackticks($0) }.max() ?? 0
                let fence = String(repeating: "`", count: max(3, longestFence + 1))
                let header = "### \(time(block.first))–\(time(block.last))｜\(name)｜活跃 \(duration(seconds))｜"
                    + "原文 段 \(ranges)（新增 \(block.sourceChars) 字，摘录 \(selection.used) 字）\n\n"
                entries.append(header + "\(fence)text\n" + body.joined(separator: "\n") + "\n\(fence)\n\n")
            }
            if entries.isEmpty && minor.isEmpty { continue }
            out += "## \(dayHour(hour))\n\n"
            out += entries.joined()
            if !minor.isEmpty {
                out += "- 短暂停留或无新增正文：" + minor.joined(separator: "；") + "\n\n"
            }
        }

        var stats = DigestStats()
        stats.digestChars = out.count
        stats.sourceChars = totalSource
        stats.excerptChars = excerpt
        stats.blocks = blocks.count
        stats.truncatedBlocks = truncatedBlocks
        stats.dropped = dropped
        stats.maskedSecrets = masked
        return (out, stats)
    }

    // MARK: - Rules

    private static func supersededLines(_ added: [AddedLine]) -> Set<String> {
        var byWindow: [String: Set<String>] = [:]
        for item in added {
            byWindow["\(item.app)\u{0}\(item.title)", default: []].insert(item.line)
        }
        var result = Set<String>()
        for (window, lines) in byWindow {
            let ordered = lines.sorted()
            guard ordered.count > 1 else { continue }
            for index in 0..<(ordered.count - 1) {
                let current = ordered[index]
                let next = ordered[index + 1]
                if current.count >= 8, next.hasPrefix(current) {
                    result.insert("\(window)\u{0}\(current)")
                }
            }
        }
        return result
    }

    private static func normalize(_ line: String) -> String {
        let range = NSRange(line.startIndex..<line.endIndex, in: line)
        let collapsed = whitespace.stringByReplacingMatches(in: line, range: range, withTemplate: " ")
        let collapsedRange = NSRange(collapsed.startIndex..<collapsed.endIndex, in: collapsed)
        return digits.stringByReplacingMatches(in: collapsed, range: collapsedRange, withTemplate: "#")
            .trimmingCharacters(in: .whitespaces)
    }

    private static func normalizedTitle(_ title: String) -> String {
        let flat = title.replacingOccurrences(of: "\n", with: " ").trimmingCharacters(in: .whitespaces)
        guard !flat.isEmpty else { return "" }
        let scalars = Array(flat.unicodeScalars)
        var start = 0
        while start < scalars.count {
            let scalar = scalars[start]
            if CharacterSet.alphanumerics.contains(scalar) || isCJK(scalar) || "~/.(（[".unicodeScalars.contains(scalar) {
                break
            }
            start += 1
        }
        let result = String(String.UnicodeScalarView(scalars[start...])).trimmingCharacters(in: .whitespaces)
        return result.isEmpty ? flat : result
    }

    private static func joinFragments(_ lines: [(String, Int)]) -> [(String, Int)] {
        var out: [(String, Int, Bool)] = []
        for (line, part) in lines {
            if let last = out.last, line.count < 24, last.0.count < 200, last.2 {
                out[out.count - 1] = (last.0 + " · " + line, last.1, true)
            } else {
                out.append((line, part, line.count < 24))
            }
        }
        return out.map { ($0.0, $0.1) }
    }

    private struct Selection {
        var kept: [(String, Int)]
        var omitted: [(String, Int)]
        var used: Int
    }

    private static func select(lines: [(String, Int)], budget: Int) -> Selection {
        let total = lines.reduce(0) { $0 + $1.0.count }
        guard total > budget else { return Selection(kept: lines, omitted: [], used: total) }

        let scored = lines.enumerated().map { index, item in (index: index, score: score(item.0), item: item) }
        let ordered = scored.sorted { $0.score > $1.score }
        var chosen = Set<Int>()
        var used = 0
        for candidate in ordered {
            let length = candidate.item.0.count
            if used + length > budget && !chosen.isEmpty { continue }
            if used + length > Int(Double(budget) * 1.5) { continue }
            chosen.insert(candidate.index)
            used += length
        }

        var kept: [(String, Int)] = []
        var omitted: [(String, Int)] = []
        for (index, item) in lines.enumerated() {
            if chosen.contains(index) { kept.append(item) } else { omitted.append(item) }
        }
        return Selection(kept: kept, omitted: omitted, used: used)
    }

    private static func score(_ line: String) -> Int {
        var value = min(line.count, 200)
        if line.unicodeScalars.contains(where: isCJK) { value += 60 }
        if line.rangeOfCharacter(from: punctuation) != nil {
            value += 30
        } else if line.split(whereSeparator: \.isWhitespace).count <= 8 {
            value -= 120
        }
        return value
    }

    private static func maskLine(_ line: String) -> (String, Int) {
        var result = line
        var count = 0
        for pattern in secretPatterns {
            let range = NSRange(result.startIndex..<result.endIndex, in: result)
            count += pattern.numberOfMatches(in: result, range: range)
            result = pattern.stringByReplacingMatches(in: result, range: range, withTemplate: mask)
        }
        let range = NSRange(result.startIndex..<result.endIndex, in: result)
        count += passwordValue.numberOfMatches(in: result, range: range)
        result = passwordValue.stringByReplacingMatches(in: result, range: range, withTemplate: "$1\(mask)")
        return (result, count)
    }

    private static func matches(_ regex: NSRegularExpression, _ value: String) -> Bool {
        regex.firstMatch(in: value, range: NSRange(value.startIndex..<value.endIndex, in: value)) != nil
    }

    private static func isCJK(_ scalar: Unicode.Scalar) -> Bool {
        let value = scalar.value
        return (0x3400...0x9FFF).contains(value)
            || (0xAC00...0xD7AF).contains(value)
            || (0x3040...0x30FF).contains(value)
    }

    private static func longestBackticks(_ line: String) -> Int {
        var longest = 0
        var current = 0
        for character in line {
            if character == "`" {
                current += 1
                longest = max(longest, current)
            } else {
                current = 0
            }
        }
        return longest
    }

    private static func omissionMarker(_ lines: [(String, Int)]) -> String {
        let chars = lines.reduce(0) { $0 + $1.0.count }
        return "……（省略 \(lines.count) 行，约 \(chars) 字，见 段 \(partRanges(Set(lines.map { $0.1 })))）……"
    }

    // MARK: - Formatting

    public static func duration(_ seconds: TimeInterval) -> String {
        let minutes = Int((seconds / 60).rounded())
        if minutes < 1 { return "\(Int(seconds.rounded()))s" }
        let hours = minutes / 60
        let remaining = minutes % 60
        return hours > 0 ? "\(hours)h \(String(format: "%02d", remaining))m" : "\(minutes)m"
    }

    public static func partRanges(_ values: Set<Int>) -> String {
        guard !values.isEmpty else { return "—" }
        var groups: [(Int, Int)] = []
        for value in values.sorted() {
            if let last = groups.last, value == last.1 + 1 {
                groups[groups.count - 1].1 = value
            } else {
                groups.append((value, value))
            }
        }
        return groups.map { $0.0 == $0.1 ? "\($0.0)" : "\($0.0)–\($0.1)" }.joined(separator: "、")
    }

    private static func time(_ date: Date) -> String {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = DayTracePaths.beijing
        formatter.dateFormat = "HH:mm"
        return formatter.string(from: date)
    }

    private static func dayHour(_ date: Date) -> String {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = DayTracePaths.beijing
        formatter.dateFormat = "MM-dd H"
        return formatter.string(from: date) + ":00"
    }
}
