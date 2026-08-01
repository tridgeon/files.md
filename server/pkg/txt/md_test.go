package txt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkdownToHTML(t *testing.T) {
	r := require.New(t)

	md := "### DisplayName\n**bold**\n*italic*"
	html := MarkdownToHTML(md)

	r.Equal("<b>DisplayName</b>\n<b>bold</b>\n<i>italic</i>", html)
}

func TestMarkdownToHTMLHeaderAndText(t *testing.T) {
	r := require.New(t)

	md := "# DisplayName\nText"
	html := MarkdownToHTML(md)

	r.Equal("<b>DisplayName</b>\nText", html)
}

func TestMarkdownToHTMLBold(t *testing.T) {
	r := require.New(t)

	md := "**bold**"
	html := MarkdownToHTML(md)

	r.Equal("<b>bold</b>", html)
}

func TestMarkdownToHTMLMultilineBold(t *testing.T) {
	r := require.New(t)

	md := "**bold\nstill bold**"
	html := MarkdownToHTML(md)

	r.Equal("<b>bold\nstill bold</b>", html)
}

func TestMarkdownToHTMLEmptyBoldAndItalic(t *testing.T) {
	r := require.New(t)

	md := "**"
	r.Equal("**", MarkdownToHTML(md))
	md = "*"
	r.Equal("*", MarkdownToHTML(md))

	md = "__"
	r.Equal("__", MarkdownToHTML(md))
	md = "_"
	r.Equal("_", MarkdownToHTML(md))
}

func TestMarkdownToHTMLNewLineChar(t *testing.T) {
	r := require.New(t)

	bold := "**\n**"
	r.Equal("<b>\n</b>", MarkdownToHTML(bold))

	italic := "*\n*"
	r.Equal("<i>\n</i>", MarkdownToHTML(italic))
}

func TestMarkdownToHTMLCharAndNewLineChar(t *testing.T) {
	r := require.New(t)

	bold := "**a\n**"
	r.Equal("<b>a\n</b>", MarkdownToHTML(bold))

	italic := "*a\n*"
	r.Equal("<i>a\n</i>", MarkdownToHTML(italic))
}

func TestMarkdownToHTMLNewLineAndChar(t *testing.T) {
	r := require.New(t)

	bold := "**\na**"
	r.Equal("<b>\na</b>", MarkdownToHTML(bold))

	italic := "*\na*"
	r.Equal("<i>\na</i>", MarkdownToHTML(italic))
}

func TestMarkdownToHTMLTwoNewlinesBreakFormatting(t *testing.T) {
	r := require.New(t)

	bold := "**no bold\n\nno bold**"
	r.Equal("**no bold\n\nno bold**", MarkdownToHTML(bold))

	italic := "*no italic\n\nno italic*"
	r.Equal("*no italic\n\nno italic*", MarkdownToHTML(italic))
}

func TestMarkdownToHTMLMultilineBoldAndItalic(t *testing.T) {
	r := require.New(t)

	md := "Some _italic text\nin two lines_, **bold text\nin two lines**, and ***bold italic text\nin two lines***."
	html := MarkdownToHTML(md)

	r.Equal("Some <i>italic text\nin two lines</i>, <b>bold text\nin two lines</b>, and <b><i>bold italic text\nin two lines</i></b>.", html)
}

func TestMarkdownToHTMLHtmlInsideCode(t *testing.T) {
	r := require.New(t)

	md := "```some code a > b```"
	html := MarkdownToHTML(md)

	r.Equal("<pre>some code a &gt; b</pre>", html)
}

func TestMarkdownToHTMLInlineCodeAndCodeBlock(t *testing.T) {
	r := require.New(t)

	md := "`inline code` and ```code block```"
	html := MarkdownToHTML(md)

	r.Equal("<code>inline code</code> and <pre>code block</pre>", html)
}

func TestMarkdownToHTMLInlineCodeBlockInsideCodeBlock(t *testing.T) {
	r := require.New(t)

	md := "```code block with `inline code` inside```"
	html := MarkdownToHTML(md)

	r.Equal("<pre>code block with <code>inline code</code> inside</pre>", html)
}

func TestMarkdownToHTMLItalic(t *testing.T) {
	r := require.New(t)

	md := "*italic*"
	html := MarkdownToHTML(md)

	r.Equal("<i>italic</i>", html)
}

func TestMarkdownToHTMLInvalidMD(t *testing.T) {
	r := require.New(t)

	md := "__valid__**invalid"
	r.Equal("<b>valid</b>**invalid", MarkdownToHTML(md))

	r.Equal("*invalid_markdown", MarkdownToHTML("*invalid_markdown"))
	r.Equal("*``invalid_markdown", MarkdownToHTML("*``invalid_markdown"))
	r.Equal("*```invalid_markdown", MarkdownToHTML("*```invalid_markdown"))
}

func TestMarkdownToHTMLMultiline(t *testing.T) {
	r := require.New(t)

	md := "line1\n**line2**\nline3"
	html := MarkdownToHTML(md)

	r.Equal("line1\n<b>line2</b>\nline3", html)
}

func TestMarkdownToHTMLBoldInsideItalic(t *testing.T) {
	r := require.New(t)

	md := "*italic and __bold__*"
	r.Equal("<i>italic and <b>bold</b></i>", MarkdownToHTML(md))

	md = "*italic and **bold***"
	r.Equal("<i>italic and <b>bold</b></i>", MarkdownToHTML(md))
}

func TestMarkdownToHTMLItalicInsideBold(t *testing.T) {
	r := require.New(t)

	md := "__bold and _italic___"
	r.Equal("<b>bold and <i>italic</i></b>", MarkdownToHTML(md))

	md = "**bold and *italic***"
	r.Equal("<b>bold and <i>italic</i></b>", MarkdownToHTML(md))
}

func TestMarkdownToHTMLNoLists(t *testing.T) {
	r := require.New(t)

	md := "list\n1) item1\n2) item2"
	html := MarkdownToHTML(md)

	r.Equal("list\n1) item1\n2) item2", html)
}

func TestMarkdownToHTMLEscapeHtml(t *testing.T) {
	r := require.New(t)

	html := MarkdownToHTML("<a> &b")

	r.Equal("&lt;a&gt; &amp;b", html)
}

func TestMarkdownToHTMLHeader(t *testing.T) {
	r := require.New(t)

	md := "Multiline\n# DisplayName"
	html := MarkdownToHTML(md)

	r.Equal("Multiline\n<b>DisplayName</b>", html)
}

func TestMarkdownToHTMLMultipleHeaders(t *testing.T) {
	r := require.New(t)

	md := "# Header1\n## Header2"
	html := MarkdownToHTML(md)

	r.Equal("<b>Header1</b>\n<b>Header2</b>", html)
}

func TestMarkdownToHTMLInlineCode(t *testing.T) {
	r := require.New(t)

	md := "`inline code`"
	html := MarkdownToHTML(md)

	r.Equal("<code>inline code</code>", html)
}

func TestMarkdownToHTMLMultilineCodeBlock(t *testing.T) {
	r := require.New(t)

	md := "```\ncode line 1\ncode line 2\n```"
	html := MarkdownToHTML(md)

	r.Equal("<pre>code line 1\ncode line 2</pre>", html)
}

func TestMarkdownToHTMLCodeWithBold(t *testing.T) {
	r := require.New(t)

	md := "`code` **bold**"
	html := MarkdownToHTML(md)

	r.Equal("<code>code</code> <b>bold</b>", html)
}

func TestMarkdownToHTMLHeaderWithInlineCode(t *testing.T) {
	r := require.New(t)

	md := "# DisplayName\n`inline code`"
	html := MarkdownToHTML(md)

	r.Equal("<b>DisplayName</b>\n<code>inline code</code>", html)
}

func TestChecklistItems(t *testing.T) {
	r := require.New(t)

	md := "- [ ] unchecked item\n- [x] checked item\n- [ ] another unchecked"
	items, isChecked := ChecklistItems(md)

	r.Equal([]string{"unchecked item", "checked item", "another unchecked"}, items)

	expected := map[string]bool{
		"unchecked item":    false,
		"checked item":      true,
		"another unchecked": false,
	}
	r.Equal(expected, isChecked)
}

func TestChecklistItemsEmpty(t *testing.T) {
	r := require.New(t)

	md := ""
	items, isChecked := ChecklistItems(md)

	r.Len(items, 0)
	r.Equal(map[string]bool{}, isChecked)
}

func TestChecklistItemsWithRegularText(t *testing.T) {
	r := require.New(t)

	md := "# DisplayName\n- [ ] task one\nregular text\n- [x] task two"
	items, isChecked := ChecklistItems(md)
	r.Equal([]string{"task one", "task two"}, items)
	expected := map[string]bool{
		"task one": false,
		"task two": true,
	}
	r.Equal(expected, isChecked)
}

func TestChecklistItemsWithWhitespace(t *testing.T) {
	r := require.New(t)

	md := "   - [ ] spaced task   \n\t- [x] tabbed task\t"
	items, isChecked := ChecklistItems(md)
	r.Equal([]string{"spaced task", "tabbed task"}, items)

	expected := map[string]bool{
		"spaced task": false,
		"tabbed task": true,
	}
	r.Equal(expected, isChecked)
}

func TestAddChecklistItemUnchecked(t *testing.T) {
	r := require.New(t)

	md := "existing text"
	result := AddChecklistItem(md, "new task", false)

	r.Equal("existing text\n- [ ] new task", result)
}

func TestAddChecklistItemChecked(t *testing.T) {
	r := require.New(t)

	md := "existing text"
	result := AddChecklistItem(md, "completed task", true)

	r.Equal("existing text\n- [x] completed task", result)
}

func TestAddChecklistItemToEmpty(t *testing.T) {
	r := require.New(t)

	md := ""
	result := AddChecklistItem(md, "first task", false)

	r.Equal("- [ ] first task", result)
}

func TestAddChecklistItemWithNewlines(t *testing.T) {
	r := require.New(t)

	md := "text"
	result := AddChecklistItem(md, "task with\nnewlines", false)

	r.Equal("text\n- [ ] task with newlines", result)
}

func TestAddChecklistItemRemovesDuplicate(t *testing.T) {
	r := require.New(t)

	md := "- [ ] existing task\nother text"
	result := AddChecklistItem(md, "existing task", true)

	r.Equal("other text\n- [x] existing task", result)
}

func TestContainsIncompleteChecklistItemPresent(t *testing.T) {
	r := require.New(t)

	md := "- [ ] Water plants\n- [ ] Read book"

	r.True(ContainsIncompleteChecklistItem(md, "Water plants"))
}

func TestContainsIncompleteChecklistItemWithChatTimestamp(t *testing.T) {
	r := require.New(t)

	md := "#### 28 Jul, Monday\n- [ ] `09:00` Water plants"

	r.True(ContainsIncompleteChecklistItem(md, "Water plants"))
}

func TestContainsIncompleteChecklistItemCompletedIsNotPresent(t *testing.T) {
	r := require.New(t)

	md := "- [x] `09:00` Water plants"

	r.False(ContainsIncompleteChecklistItem(md, "Water plants"))
}

func TestContainsIncompleteChecklistItemAbsent(t *testing.T) {
	r := require.New(t)

	md := "- [ ] Read book"

	r.False(ContainsIncompleteChecklistItem(md, "Water plants"))
}

func TestCompleteChecklistItem(t *testing.T) {
	r := require.New(t)

	md := "- [ ] task one\n- [ ] task two"
	itemHash := Hash("task one")
	result, foundItem := CompleteChecklistItem(md, itemHash)

	r.Equal("- [x] task one\n- [ ] task two", result)
	r.Equal("task one", foundItem)
}

func TestCompleteChecklistItemNotFound(t *testing.T) {
	r := require.New(t)

	md := "- [ ] task one\n- [ ] task two"
	result, foundItem := CompleteChecklistItem(md, "nonexistent")

	r.Equal(md, result)
	r.Equal("", foundItem)
}

func TestCompleteChecklistItemWithWhitespace(t *testing.T) {
	r := require.New(t)

	md := "   - [ ] spaced task   \nother text"
	itemHash := Hash("spaced task")
	result, foundItem := CompleteChecklistItem(md, itemHash)

	r.Equal("- [x] spaced task\nother text", result)
	r.Equal("spaced task", foundItem)
}

// The Chat.md flow hashes the FIRST line of a multi-line block (see
// todayBlockHash), and CompleteChecklistItem walks the file line-by-line
// hashing the same first-line content - so the box flips on the matched
// line and the continuation stays right where the user wrote it.
func TestCompleteChecklistItem_FlipsBox_WithContinuationLines(t *testing.T) {
	r := require.New(t)

	md := "- [ ] `19:44` Five\nsix"
	result, foundItem := CompleteChecklistItem(md, Hash("`19:44` Five"))

	r.Equal("- [x] `19:44` Five\nsix", result)
	r.Equal("`19:44` Five", foundItem)
}

func TestRemoveChecklistItemByText(t *testing.T) {
	r := require.New(t)

	md := "- [ ] task one\n- [x] task two\n- [ ] task three"
	result, removedItem := RemoveChecklistItem(md, "task two")

	r.Equal("- [ ] task one\n- [ ] task three", result)
	r.Equal("task two", removedItem)
}

func TestRemoveChecklistItemByHash(t *testing.T) {
	r := require.New(t)

	md := "- [ ] task one\n- [x] task two"
	itemHash := Hash("task one")
	result, removedItem := RemoveChecklistItem(md, itemHash)

	r.Equal("- [x] task two", result)
	r.Equal("task one", removedItem)
}

func TestRemoveChecklistItemNotFound(t *testing.T) {
	r := require.New(t)

	md := "- [ ] task one\n- [x] task two"
	result, removedItem := RemoveChecklistItem(md, "nonexistent")

	r.Equal(md, result)
	r.Equal("", removedItem)
}

func TestRemoveChecklistItemWithRegularText(t *testing.T) {
	r := require.New(t)

	md := "# DisplayName\n- [ ] task one\nregular text\n- [x] task two"
	result, removedItem := RemoveChecklistItem(md, "task one")

	r.Equal("# DisplayName\nregular text\n- [x] task two", result)
	r.Equal("task one", removedItem)
}

func TestRemoveCompletedChecklistItems(t *testing.T) {
	r := require.New(t)

	md := "- [ ] unchecked\n- [x] completed one\n- [ ] another unchecked\n- [x] completed two"
	result, removedMD := RemoveCompletedChecklistItems(md)

	r.Equal("- [ ] unchecked\n- [ ] another unchecked", result)
	r.Equal("- [x] completed one\n- [x] completed two\n", removedMD)
}

func TestRemoveCompletedChecklistItemsNoneCompleted(t *testing.T) {
	r := require.New(t)

	md := "- [ ] unchecked one\n- [ ] unchecked two"
	result, removedMD := RemoveCompletedChecklistItems(md)

	r.Equal(md, result)
	r.Equal("", removedMD)
}

func TestRemoveCompletedChecklistItemsAllCompleted(t *testing.T) {
	r := require.New(t)

	md := "- [x] completed one\n- [x] completed two"
	result, removedMD := RemoveCompletedChecklistItems(md)

	r.Equal("", result)
	r.Equal("- [x] completed one\n- [x] completed two\n", removedMD)
}

func TestRemoveCompletedChecklistItemsWithRegularText(t *testing.T) {
	r := require.New(t)

	md := "# DisplayName\n- [ ] unchecked\nregular text\n- [x] completed"
	result, removedMD := RemoveCompletedChecklistItems(md)

	r.Equal("# DisplayName\n- [ ] unchecked\nregular text", result)
	r.Equal("- [x] completed\n", removedMD)
}

func TestChecklistItem(t *testing.T) {
	r := require.New(t)

	md := "- [ ] task one\n- [x] task two"
	result := ChecklistItem(md, "task one")

	r.Equal("task one", result)
}

func TestChecklistItemByHash(t *testing.T) {
	r := require.New(t)

	md := "- [ ] task one\n- [x] task two"
	itemHash := Hash("task two")
	result := ChecklistItem(md, itemHash)

	r.Equal("task two", result)
}

func TestChecklistItemNotFound(t *testing.T) {
	r := require.New(t)

	md := "- [ ] task one\n- [x] task two"
	result := ChecklistItem(md, "nonexistent")

	r.Equal("", result)
}

func TestChecklistItemWithWhitespace(t *testing.T) {
	r := require.New(t)

	md := "   - [ ] spaced task   \nother text"
	result := ChecklistItem(md, "spaced task")

	r.Equal("spaced task", result)
}
