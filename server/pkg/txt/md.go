package txt

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

type (
	parser func(input string) []result
	result struct {
		consumed string
		left     string
	}
)

var openTags = map[string]string{
	"*":  "<i>",
	"**": "<b>",
	"_":  "<i>",
	"__": "<b>",
}

// chatTimestampRE matches a leading “ `HH:MM` “ token used by chat
// entries. Used to strip it before re-recording the body as a
// journal/completion entry that already gets its own fresh timestamp.
var chatTimestampRE = regexp.MustCompile("^`\\d{2}:\\d{2}` ")

// StripChatTimestamp drops the leading “ `HH:MM` “ token from a chat
// entry body. Returns the input unchanged if no timestamp prefix is found.
func StripChatTimestamp(s string) string {
	return chatTimestampRE.ReplaceAllString(s, "")
}

var closeTags = map[string]string{
	"*":  "</i>",
	"**": "</b>",
	"_":  "</i>",
	"__": "</b>",
}

func AddHeaderAndText(existingContent, header, newContent string) string {
	if !strings.Contains(existingContent, header) {
		if existingContent == "" {
			return fmt.Sprintf("%s\n%s", header, newContent)
		} else {
			return fmt.Sprintf("%s\n%s\n\n%s", header, newContent, existingContent)
		}
	}

	lines := strings.Split(existingContent, "\n")
	headerIndex := -1

	// Find the header line
	for i, line := range lines {
		if line == header {
			headerIndex = i
			break
		}
	}

	if headerIndex == -1 {
		return fmt.Sprintf("%s\n%s\n\n%s", header, newContent, existingContent)
	}

	// Find where to insert (after the last line belonging to this header)
	insertIndex := headerIndex + 1

	// Look for the next header or end of content
	for i := headerIndex + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "###") {
			insertIndex = i
			break
		}
		insertIndex = i + 1
	}

	// Insert the new content
	newLines := make([]string, 0, len(lines)+2)
	newLines = append(newLines, lines[:insertIndex]...)
	newLines = append(newLines, newContent)

	// Add empty line after new content if there's content following and it's not empty
	if insertIndex < len(lines) && strings.TrimSpace(lines[insertIndex]) != "" {
		newLines = append(newLines, "")
	}

	newLines = append(newLines, lines[insertIndex:]...)

	return strings.Join(newLines, "\n")
}

func IncompleteChecklistItems(md string) []string {
	items, isCompleted := ChecklistItems(md)
	var incomplete []string
	for _, item := range items {
		if !isCompleted[item] {
			incomplete = append(incomplete, item)
		}
	}

	return incomplete
}

func ChecklistItems(md string) ([]string, map[string]bool) {
	var items []string
	isCompleted := make(map[string]bool)
	lines := strings.Split(md, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- [ ] ") {
			item := strings.TrimPrefix(line, "- [ ] ")
			items = append(items, item)
			isCompleted[item] = false
		} else if strings.HasPrefix(line, "- [x] ") {
			item := strings.TrimPrefix(line, "- [x] ")
			items = append(items, item)
			isCompleted[item] = true
		}
	}

	return items, isCompleted
}

func AddChecklistItem(md, item string, checked bool) string {
	item = strings.ReplaceAll(NormNewLines(item), "\n", " ")

	md, _ = RemoveChecklistItem(md, item)
	lines := strings.Split(md, "\n")

	if checked {
		lines = append(lines, "- [x] "+item)
	} else {
		// Find the last incomplete item and insert before it
		insertIndex := len(lines)
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if strings.HasPrefix(line, "- [ ] ") {
				insertIndex = i
			}
		}

		// Insert the new incomplete item
		if insertIndex == len(lines) {
			lines = append(lines, "- [ ] "+item)
		} else {
			lines = append(lines[:insertIndex], append([]string{"- [ ] " + item}, lines[insertIndex:]...)...)
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// CompleteChecklistItem marks the matching item as completed in place.
// Returns newMarkdown and modifiedItem.
//
// The marker stays where it was instead of being relocated to the bottom.
// Moving it broke multi-line records on Chat.md: only the marker line
// would migrate, leaving the continuation lines stranded above it.
func CompleteChecklistItem(md, itemHash string) (string, string) {
	foundItem := ""
	lines := strings.Split(md, "\n")
	foundIndex := -1
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 6 {
			continue
		}

		if strings.HasPrefix(line, "- [ ] ") && Hash(line[6:]) == itemHash {
			foundItem = line[6:]
			foundIndex = i
			break
		}
	}

	if foundIndex != -1 {
		lines[foundIndex] = "- [x] " + foundItem
	}

	return strings.Join(lines, "\n"), foundItem
}

// RemoveChecklistItem removes given item from checklist.
// Returns newMarkdown, removedItem
func RemoveChecklistItem(md, itemOrHash string) (string, string) {
	removedItem := ""
	lines := strings.Split(md, "\n")
	var newLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Preserve invalid lines
		if len(line) < 6 {
			newLines = append(newLines, line)
			continue
		}

		if Hash(line[6:]) == itemOrHash || line[6:] == itemOrHash {
			removedItem = line[6:]
			continue
		}
		newLines = append(newLines, line)
	}
	return strings.Join(newLines, "\n"), removedItem
}

// RemoveCompletedChecklistItems returns newMarkdown, removedMarkdown
func RemoveCompletedChecklistItems(md string) (string, string) {
	removedMD := ""
	lines := strings.Split(md, "\n")
	var newLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 6 {
			newLines = append(newLines, line)
			continue
		}

		if strings.HasPrefix(line, "- [x] ") {
			removedMD += line + "\n"
			continue
		}
		newLines = append(newLines, line)
	}
	return strings.Join(newLines, "\n"), removedMD
}

func ChecklistItem(md, itemOrHash string) string {
	lines := strings.Split(md, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 6 {
			continue
		}

		if Hash(line[6:]) == itemOrHash || line[6:] == itemOrHash {
			return line[6:]
		}
	}
	return ""
}

// ContainsIncompleteChecklistItem reports whether md has an unchecked item
// whose content (ignoring a leading chat timestamp) equals item.
func ContainsIncompleteChecklistItem(md, item string) bool {
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [ ] ") {
			continue
		}
		content := StripChatTimestamp(strings.TrimPrefix(line, "- [ ] "))
		if strings.TrimSpace(content) == item {
			return true
		}
	}
	return false
}

// MarkdownToHTML naively converts user's markdown to Telegram-supported subset of HTML.
// We don't need to implement full-blown AST parser because TG only supports a few HTML tags.
// Telegram supports the following HTML tags:
// <b>bold</b>, <strong>bold</strong>
// <i>italic</i>, <em>italic</em>
// <u>underline</u>, <ins>underline</ins>
// <s>strikethrough</s>, <strike>strikethrough</strike>, <del>strikethrough</del>
// <span class="tg-spoiler">spoiler</span>, <tg-spoiler>spoiler</tg-spoiler>
// <b>bold <i>italic bold <s>italic bold strikethrough <span class="tg-spoiler">italic bold strikethrough spoiler</span></s> <u>underline italic bold</u></i> bold</b>
// <a href="http://www.example.com/">inline Path</a>
// <a href="tg://user?id=123456789">inline mention of a user</a>
// <tg-emoji emoji-id="5368324170671202286">👍</tg-emoji>
// <code>inline fixed-width code</code>
// <pre>pre-formatted fixed-width code block</pre>
// <pre><code class="language-python">pre-formatted fixed-width code block written in the Python programming language</code></pre>
// <blockquote>Block quotation started\nBlock quotation continued\nThe last line of the block quotation</blockquote>
// <blockquote expandable>Expandable block quotation started\nExpandable block quotation continued\nExpandable block quotation continued\nHidden by default part of the block quotation started\nExpandable block quotation continued\nThe last line of the block quotation</blockquote>
func MarkdownToHTML(md string) string {
	mdWithoutCode := EscapeHTML(md)
	mdWithoutCode, codePlaceholders := ReplaceWithPlaceholders(mdWithoutCode, "(?s)```.*?```", "c0debl0ck")
	mdWithoutCode, inlinePlaceholders := ReplaceWithPlaceholders(mdWithoutCode, "`[^`]+`", "inl1ne")
	// By this point our markdown is safe to send as HTML via Telegram.
	// There won't be any issues like "missing closing HTML tag",
	// for the cases when our markdown has some html tags.
	// We try to convert as much markdown as possible to Telegram HTML.

	// We split by \n\n+, because markdown context is broken by \n\n (excluding code inside ```)
	// TODO test splitting by \n\n+
	reNewLines := regexp.MustCompile(`\n{2,}`)
	segments := reNewLines.Split(mdWithoutCode, -1)
	processedSegments := make([]string, len(segments))
	for i, segment := range segments {
		// Process each segment separately
		docs := markdown()(segment)
		if len(docs) > 0 {
			segment = docs[0].consumed + docs[0].left
		}
		processedSegments[i] = segment
	}
	mdWithoutCode = strings.Join(processedSegments, "\n\n")

	mdWithCode := RestoreFromPlaceholders(mdWithoutCode, codePlaceholders)
	mdWithCode = RestoreFromPlaceholders(mdWithCode, inlinePlaceholders)

	// We do dirty but simple md -> html conversion.
	// Covert ` and ``` to <pre> and <code> HTML tags
	reCodeBlock := regexp.MustCompile("(?s)```(.+?)```")
	mdWithCode = reCodeBlock.ReplaceAllStringFunc(mdWithCode, func(s string) string {
		return "<pre>" + strings.TrimSpace(reCodeBlock.FindStringSubmatch(s)[1]) + "</pre>"
	})
	reInlineCode := regexp.MustCompile("`([^`]+?)`")
	mdWithCode = reInlineCode.ReplaceAllString(mdWithCode, "<code>$1</code>")

	// Convert #+ DisplayName to <b>DisplayName</b>
	reHeader := regexp.MustCompile(`(?m)^#+\s*(.+)`)
	mdWithCode = reHeader.ReplaceAllString(mdWithCode, "<b>$1</b>")

	return mdWithCode
}

// parser Combinators. Watch an amazing video here: https://youtu.be/dDtZLm7HIJs.
// We only support one level of nesting for bold and italic.
func markdown() parser {
	text := notMarkdown()
	italicNoBold := or(
		and(open("*"), text, close("*")),
		and(open("_"), text, close("_")),
	)
	bold := or(
		and(open("**"), some(or(text, italicNoBold)), close("**")),
		and(open("__"), some(or(text, italicNoBold)), close("__")),
	)
	italic := or(
		and(open("*"), some(or(text, bold)), close("*")),
		and(open("_"), some(or(text, bold)), close("_")),
	)
	span := or(bold, italic, text)

	return some(span)
}

// open opens the tag
func open(t string) parser {
	return func(input string) []result {
		if strings.HasPrefix(input, t) {
			return []result{{openTags[t], input[len(t):]}}
		}
		return nil
	}
}

// close closes the tag
func close(t string) parser {
	return func(input string) []result {
		if strings.HasPrefix(input, t) {
			return []result{{closeTags[t], input[len(t):]}}
		}
		return nil
	}
}

// or applies multiple parsers and returns the result of the first successful parser.
// If no parser succeeds, it returns an empty result.
func or(parsers ...parser) parser {
	return func(input string) []result {
		var results []result
		for _, p := range parsers {
			results = append(results, p(input)...)
		}
		return results
	}
}

// and applies multiple parsers in sequence.
// Each parser must succeed in consuming part of the input.
// If any parser fails, the whole parse fails.
func and(parsers ...parser) parser {
	return func(input string) []result {
		results := []result{{"", input}}

		for _, p := range parsers {
			var newResults []result
			for _, r := range results {
				for _, parsed := range p(r.left) {
					if parsed.consumed != "" {
						newResults = append(newResults, result{r.consumed + parsed.consumed, parsed.left})
					}
				}
			}
			if len(newResults) == 0 {
				return nil
			}
			results = newResults
		}

		return results
	}
}

// some applies the parser one or more times. Each parse result is combined with the previous result.
// And each parse can generate multiple results.
func some(parser parser) parser {
	return func(input string) []result {
		return recursive(input, parser, 0)
	}
}

func recursive(input string, parser parser, depth int) []result {
	var results []result
	empty := true
	for _, item := range parser(input) {
		if item.consumed == "" {
			continue
		}
		empty = false
		for _, child := range recursive(item.left, parser, depth+1) {
			results = append(results, result{item.consumed + child.consumed, child.left})
		}
	}
	if empty && depth != 0 {
		results = append(results, result{"", input})
	}

	return results
}

// notMarkdown incrementally yields when it encounters a *, **, _, __
func notMarkdown() parser {
	return func(input string) []result {
		for i, ch := range input {
			if ch == '*' || ch == '_' {
				return []result{{input[:i], input[i:]}}
			}
		}
		if len(input) > 0 {
			return []result{{input, ""}}
		}
		return nil
	}
}

func Hash(filename string) string {
	hash := md5.Sum([]byte(filename))
	return hex.EncodeToString(hash[:])[:11]
}
