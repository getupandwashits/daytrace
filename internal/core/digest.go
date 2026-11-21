package core

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const DigestMask = "〔已遮蔽〕"

var defaultSkipApps = map[string]bool{"Clash Party": true}

var passwordManagerApps = map[string]bool{
	"Bitwarden":       true,
	"1Password":       true,
	"1Password 7":     true,
	"KeePassXC":       true,
	"Passwords":       true,
	"Keychain Access": true,
}

type AddedLine struct {
	Stamp time.Time
	App   string
	Title string
	Line  string
	Part  int
}

type DigestStats struct {
	DigestChars     int
	SourceChars     int
	ExcerptChars    int
	Blocks          int
	TruncatedBlocks int
	Dropped         map[string]int
	MaskedSecrets   int
}

type digestBlock struct {
	First       time.Time
	Last        time.Time
	Lines       []digestLine
	Parts       map[int]bool
	SourceChars int
}

type digestLine struct {
	Text string
	Part int
}

type digestSelection struct {
	Kept    []digestLine
	Omitted []digestLine
	Used    int
}

var (
	wordlikeRE   = regexp.MustCompile(`[0-9A-Za-z㐀-鿿가-힯぀-ヿ]`)
	vaultTitleRE = regexp.MustCompile(`(?i)vaultwarden|bitwarden|1password|keepass|密码库|password manager`)
	vaultLineRE  = regexp.MustCompile(`^View item - |^查看项目 - `)
	whitespaceRE = regexp.MustCompile(`\s+`)
	digitsRE     = regexp.MustCompile(`\d+`)

	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bsk-(?:ant-|proj-)?[A-Za-z0-9_-]{16,}`),
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\btvly-[A-Za-z0-9_-]{16,}`),
		regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{30,}`),
		regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`),
		regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
	}
	base32LikeRE = regexp.MustCompile(`\b[A-Z2-7]{16,64}\b`)

	passwordValueRE = regexp.MustCompile(`(?i)((?:主密码|确认主密码|密码|口令|passcode|password|PIN)\s*[*＊]?\s*(?:[(（]必填[)）])?\s*(?:[:：=]|·)\s*)([^\s·]{6,})`)
	punctuationSet  = "。！？；，：.,;:?!=()[]{}/"
)

// BuildDigest mechanically compresses captured text. It never summarizes with
// a model; it only removes drafts, decoration and variants, and budgets the
// remaining lines by observed active time.
func BuildDigest(added []AddedLine, blockSeconds map[BlockKey]float64, notice string, skipApps map[string]bool) (string, DigestStats) {
	skip := map[string]bool{}
	for app := range defaultSkipApps {
		skip[app] = true
	}
	for app := range passwordManagerApps {
		skip[app] = true
	}
	for app := range skipApps {
		skip[app] = true
	}
	superseded := supersededLines(added)
	seenNormalized := map[string]map[string]bool{}
	dropped := map[string]int{}
	blocks := map[BlockKey]*digestBlock{}
	totalSource := 0

	for _, item := range added {
		length := runeLen(item.Line)
		totalSource += length
		title := normalizedTitle(item.Title)
		hour := item.Stamp.In(Beijing).Truncate(time.Hour)
		key := BlockKey{Hour: hour, App: item.App, Title: title}
		block := blocks[key]
		if block == nil {
			block = &digestBlock{First: item.Stamp, Last: item.Stamp, Parts: map[int]bool{}}
			blocks[key] = block
		}
		if item.Stamp.Before(block.First) {
			block.First = item.Stamp
		}
		if item.Stamp.After(block.Last) {
			block.Last = item.Stamp
		}
		block.Parts[item.Part] = true
		block.SourceChars += length

		if skip[item.App] {
			dropped["skipped_app"] += length
			continue
		}
		if vaultTitleRE.MatchString(title) || vaultLineRE.MatchString(item.Line) {
			dropped["password_manager_page"] += length
			continue
		}
		if superseded[item.App+"\x00"+item.Title+"\x00"+item.Line] {
			dropped["streaming_draft"] += length
			continue
		}
		if !wordlikeRE.MatchString(item.Line) {
			dropped["decoration"] += length
			continue
		}
		rawTitle := strings.TrimSpace(item.Title)
		flatLine := strings.TrimSpace(item.Line)
		if rawTitle != "" && (flatLine == rawTitle || strings.HasPrefix(flatLine, rawTitle+" - ")) {
			dropped["title_echo"] += length
			continue
		}
		normalized := normalizeLine(item.Line)
		if seenNormalized[item.App] == nil {
			seenNormalized[item.App] = map[string]bool{}
		}
		if seenNormalized[item.App][normalized] {
			dropped["digit_or_space_variant"] += length
			continue
		}
		seenNormalized[item.App][normalized] = true

		block.Lines = append(block.Lines, digestLine{Text: item.Line, Part: item.Part})
	}

	secondsByBlock := map[BlockKey]float64{}
	for key, seconds := range blockSeconds {
		normalized := BlockKey{Hour: key.Hour, App: key.App, Title: normalizedTitle(key.Title)}
		secondsByBlock[normalized] += seconds
	}

	var out strings.Builder
	out.WriteString("# 活动摘要（机械压缩，不是模型总结）\n\n")
	out.WriteString(notice)
	out.WriteString("\n\n")
	out.WriteString("按「小时｜应用｜窗口」分块。已去掉同一窗口里后来被补全的流式草稿、只差数字或空白的变体、")
	out.WriteString("纯装饰线、重复窗口标题的行、状态刷新文字和密码管理器文字；")
	out.WriteString("每块按活跃用时分配字数，超出时优先保留正文，省略处标注段号。")
	out.WriteString("摘要会移除网址的查询参数和片段，并遮蔽能识别的密钥和密码（显示为")
	out.WriteString(DigestMask)
	out.WriteString("）。分享前仍请检查内容。\n\n")

	excerpt := 0
	truncatedBlocks := 0
	masked := 0

	byHour := map[time.Time][]BlockKey{}
	for key := range blocks {
		byHour[key.Hour] = append(byHour[key.Hour], key)
	}
	hours := make([]time.Time, 0, len(byHour))
	for hour := range byHour {
		hours = append(hours, hour)
	}
	sort.Slice(hours, func(a, b int) bool { return hours[a].Before(hours[b]) })

	for _, hour := range hours {
		hourBlocks := byHour[hour]
		sort.Slice(hourBlocks, func(a, b int) bool {
			left, right := blocks[hourBlocks[a]], blocks[hourBlocks[b]]
			if left.First.Equal(right.First) {
				if hourBlocks[a].App != hourBlocks[b].App {
					return hourBlocks[a].App < hourBlocks[b].App
				}
				return hourBlocks[a].Title < hourBlocks[b].Title
			}
			return left.First.Before(right.First)
		})
		var entries []string
		var minor []string
		for _, key := range hourBlocks {
			block := blocks[key]
			seconds := secondsByBlock[key]
			ranges := PartRanges(block.Parts)
			name, hits := maskLine(sanitizeURLs(key.App))
			masked += hits
			name = markdownText(name)
			if key.Title != "" {
				title, hits := maskLine(sanitizeURLs(key.Title))
				masked += hits
				name += "｜" + markdownText(firstRunes(title, 60))
			}
			if seconds < 60 {
				minor = append(minor, fmt.Sprintf("%s（%s，段 %s）", name, FormatDuration(seconds), ranges))
				continue
			}

			lines := joinFragments(block.Lines)
			budget := min(3000, 120+int(2.5*seconds))
			labelCount := 0
			textChars := 0
			for _, line := range lines {
				textChars += runeLen(line.Text)
			}
			if textChars > budget {
				content := lines[:0]
				for _, line := range lines {
					if runeLen(line.Text) >= 16 {
						content = append(content, line)
					}
				}
				labelCount = len(lines) - len(content)
				lines = content
			}
			selection := selectDigestLines(lines, budget)
			if len(selection.Omitted) > 0 {
				truncatedBlocks++
			}
			excerpt += selection.Used

			var body []string
			var omittedRun []digestLine
			kept := map[string]bool{}
			for _, line := range selection.Kept {
				kept[line.Text+"\x00"+fmt.Sprint(line.Part)] = true
			}
			for _, line := range lines {
				if kept[line.Text+"\x00"+fmt.Sprint(line.Part)] {
					if len(omittedRun) > 0 {
						body = append(body, omissionMarker(omittedRun))
						omittedRun = nil
					}
					maskedLine, count := maskLine(sanitizeURLs(line.Text))
					masked += count
					body = append(body, maskedLine)
				} else {
					omittedRun = append(omittedRun, line)
				}
			}
			if len(omittedRun) > 0 && len(selection.Kept) > 0 {
				body = append(body, omissionMarker(omittedRun))
			}
			if len(body) == 0 {
				minor = append(minor, fmt.Sprintf("%s（%s，无新增正文，段 %s）", name, FormatDuration(seconds), ranges))
				continue
			}
			if labelCount > 0 {
				body = append(body, fmt.Sprintf("（另略去 %d 个短界面标签）", labelCount))
			}

			longestFence := 0
			for _, line := range body {
				longestFence = max(longestFence, longestBackticks(line))
			}
			fence := strings.Repeat("`", max(3, longestFence+1))
			header := fmt.Sprintf("### %s–%s｜%s｜活跃 %s｜原文 段 %s（新增 %d 字，摘录 %d 字）\n\n",
				clockTime(block.First), clockTime(block.Last), name, FormatDuration(seconds),
				ranges, block.SourceChars, selection.Used)
			entries = append(entries, header+fence+"text\n"+strings.Join(body, "\n")+"\n"+fence+"\n\n")
		}
		if len(entries) == 0 && len(minor) == 0 {
			continue
		}
		out.WriteString("## " + dayHour(hour) + "\n\n")
		for _, entry := range entries {
			out.WriteString(entry)
		}
		if len(minor) > 0 {
			out.WriteString("- 短暂停留或无新增正文：" + strings.Join(minor, "；") + "\n\n")
		}
	}

	text := out.String()
	return text, DigestStats{
		DigestChars:     runeLen(text),
		SourceChars:     totalSource,
		ExcerptChars:    excerpt,
		Blocks:          len(blocks),
		TruncatedBlocks: truncatedBlocks,
		Dropped:         dropped,
		MaskedSecrets:   masked,
	}
}

func supersededLines(added []AddedLine) map[string]bool {
	byWindow := map[string]map[string]bool{}
	for _, item := range added {
		window := item.App + "\x00" + item.Title
		if byWindow[window] == nil {
			byWindow[window] = map[string]bool{}
		}
		byWindow[window][item.Line] = true
	}
	result := map[string]bool{}
	for window, set := range byWindow {
		if len(set) < 2 {
			continue
		}
		ordered := make([]string, 0, len(set))
		for line := range set {
			ordered = append(ordered, line)
		}
		sort.Strings(ordered)
		for index := 0; index+1 < len(ordered); index++ {
			current, next := ordered[index], ordered[index+1]
			if runeLen(current) >= 8 && strings.HasPrefix(next, current) {
				result[window+"\x00"+current] = true
			}
		}
	}
	return result
}

func normalizeLine(value string) string {
	collapsed := whitespaceRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(digitsRE.ReplaceAllString(collapsed, "#"))
}

func normalizedTitle(title string) string {
	flat := strings.TrimSpace(strings.ReplaceAll(title, "\n", " "))
	if flat == "" {
		return ""
	}
	runes := []rune(flat)
	start := 0
	for start < len(runes) {
		r := runes[start]
		if unicode.IsLetter(r) || unicode.IsDigit(r) || isCJK(r) || strings.ContainsRune("~/.(（[", r) {
			break
		}
		start++
	}
	if start >= len(runes) {
		return flat
	}
	result := strings.TrimSpace(string(runes[start:]))
	if result == "" {
		return flat
	}
	return result
}

func joinFragments(lines []digestLine) []digestLine {
	type fragment struct {
		line digestLine
		tail bool
	}
	var out []fragment
	for _, line := range lines {
		if len(out) > 0 && runeLen(line.Text) < 24 && runeLen(out[len(out)-1].line.Text) < 200 && out[len(out)-1].tail {
			last := out[len(out)-1]
			last.line.Text = last.line.Text + " · " + line.Text
			last.tail = true
			out[len(out)-1] = last
		} else {
			out = append(out, fragment{line: line, tail: runeLen(line.Text) < 24})
		}
	}
	result := make([]digestLine, 0, len(out))
	for _, item := range out {
		result = append(result, item.line)
	}
	return result
}

func selectDigestLines(lines []digestLine, budget int) digestSelection {
	total := 0
	for _, line := range lines {
		total += runeLen(line.Text)
	}
	if total <= budget {
		return digestSelection{Kept: lines, Used: total}
	}

	type scored struct {
		index int
		score int
		line  digestLine
	}
	ordered := make([]scored, len(lines))
	for index, line := range lines {
		ordered[index] = scored{index: index, score: lineScore(line.Text), line: line}
	}
	sort.SliceStable(ordered, func(a, b int) bool { return ordered[a].score > ordered[b].score })

	chosen := map[int]bool{}
	used := 0
	for _, candidate := range ordered {
		length := runeLen(candidate.line.Text)
		if used+length > budget && len(chosen) > 0 {
			continue
		}
		if used+length > int(float64(budget)*1.5) {
			continue
		}
		chosen[candidate.index] = true
		used += length
	}

	selection := digestSelection{Used: used}
	for index, line := range lines {
		if chosen[index] {
			selection.Kept = append(selection.Kept, line)
		} else {
			selection.Omitted = append(selection.Omitted, line)
		}
	}
	return selection
}

func lineScore(line string) int {
	value := min(runeLen(line), 200)
	if strings.ContainsFunc(line, isCJK) {
		value += 60
	}
	if strings.ContainsAny(line, punctuationSet) {
		value += 30
	} else if len(strings.Fields(line)) <= 8 {
		value -= 120
	}
	return value
}

func maskLine(line string) (string, int) {
	result := line
	count := 0
	for _, pattern := range secretPatterns {
		count += len(pattern.FindAllStringIndex(result, -1))
		result = pattern.ReplaceAllString(result, DigestMask)
	}
	result, base32Count := maskBase32(result)
	count += base32Count
	result, passwordCount := maskPasswordValues(result)
	count += passwordCount
	return result, count
}

func maskBase32(line string) (string, int) {
	matches := base32LikeRE.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return line, 0
	}
	var builder strings.Builder
	last := 0
	count := 0
	for _, match := range matches {
		value := line[match[0]:match[1]]
		if !strings.ContainsAny(value, "234567") || !strings.ContainsAny(value, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			continue
		}
		builder.WriteString(line[last:match[0]])
		builder.WriteString(DigestMask)
		last = match[1]
		count++
	}
	if count == 0 {
		return line, 0
	}
	builder.WriteString(line[last:])
	return builder.String(), count
}

func maskPasswordValues(line string) (string, int) {
	indices := passwordValueRE.FindAllStringSubmatchIndex(line, -1)
	if len(indices) == 0 {
		return line, 0
	}
	var builder strings.Builder
	last := 0
	count := 0
	for _, index := range indices {
		if index[4] < 0 || index[5] < 0 {
			continue
		}
		value := line[index[4]:index[5]]
		if strings.HasPrefix(value, "加载中") || strings.HasPrefix(value, "Loading") {
			continue
		}
		builder.WriteString(line[last:index[0]])
		builder.WriteString(line[index[2]:index[3]])
		builder.WriteString(DigestMask)
		last = index[1]
		count++
	}
	if count == 0 {
		return line, 0
	}
	builder.WriteString(line[last:])
	return builder.String(), count
}

func longestBackticks(line string) int {
	longest, current := 0, 0
	for _, r := range line {
		if r == '`' {
			current++
			longest = max(longest, current)
		} else {
			current = 0
		}
	}
	return longest
}

func omissionMarker(lines []digestLine) string {
	chars := 0
	parts := map[int]bool{}
	for _, line := range lines {
		chars += runeLen(line.Text)
		parts[line.Part] = true
	}
	return fmt.Sprintf("……（省略 %d 行，约 %d 字，见 段 %s）……", len(lines), chars, PartRanges(parts))
}

func FormatDuration(seconds float64) string {
	minutes := int(math.Round(seconds / 60))
	if minutes < 1 {
		return fmt.Sprintf("%ds", int(math.Round(seconds)))
	}
	hours := minutes / 60
	remaining := minutes % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, remaining)
	}
	return fmt.Sprintf("%dm", minutes)
}

func PartRanges(values map[int]bool) string {
	if len(values) == 0 {
		return "—"
	}
	ordered := make([]int, 0, len(values))
	for value := range values {
		ordered = append(ordered, value)
	}
	sort.Ints(ordered)
	type span struct{ first, last int }
	var groups []span
	for _, value := range ordered {
		if count := len(groups); count > 0 && value == groups[count-1].last+1 {
			groups[count-1].last = value
		} else {
			groups = append(groups, span{value, value})
		}
	}
	parts := make([]string, len(groups))
	for index, group := range groups {
		if group.first == group.last {
			parts[index] = fmt.Sprint(group.first)
		} else {
			parts[index] = fmt.Sprintf("%d–%d", group.first, group.last)
		}
	}
	return strings.Join(parts, "、")
}

func clockTime(value time.Time) string { return value.In(Beijing).Format("15:04") }

func dayHour(value time.Time) string {
	return value.In(Beijing).Format("01-02 15") + ":00"
}

func runeLen(value string) int { return len([]rune(value)) }

func firstRunes(value string, count int) string {
	runes := []rune(value)
	if len(runes) <= count {
		return value
	}
	return string(runes[:count])
}

func isCJK(r rune) bool {
	return (r >= 0x3400 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0x3040 && r <= 0x30FF)
}
