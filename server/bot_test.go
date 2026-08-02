package server

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/zakirullin/files.md/server/db"
	"github.com/zakirullin/files.md/server/fs"
	"github.com/zakirullin/files.md/server/journal"
	"github.com/zakirullin/files.md/server/pkg/tg"
	"github.com/zakirullin/files.md/server/pkg/txt"
	"github.com/zakirullin/files.md/server/userconfig"
)

func init() {
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 0
	}
}

// inboxMsgHash returns the hash of the nth non-header block in Chat.md, or
// a placeholder string if the inbox doesn't yet exist or has fewer entries.
// Tests use this to resolve the stable inbox-entry identifier the bot now
// carries in its callback params (previously a positional int index).
func inboxMsgHash(t *testing.T, userFS *fs.FS, nth int) string {
	t.Helper()
	content, err := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	if err != nil {
		return "nohash"
	}
	blocks := readChatMsgs(content)
	i := 0
	for _, block := range blocks {
		if strings.HasPrefix(block, "#### ") {
			continue
		}
		if i == nth {
			return chatBlockHash(block)
		}
		i++
	}
	return "nohash"
}

func TestSaveFromTextMsg(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2025, 6, 29, 12, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, "New task"))
	r.NoError(err)

	chat, err := bot.fs.Read("/", "Chat.md")
	r.NoError(err)

	r.Equal("#### 29 June, Sunday\n- [ ] `12:00` New task\n", chat)
}

// A message containing a slash-token in the middle (e.g. "Check the /token
// please") used to be intercepted as a `/token` command invocation and never
// reached the chat. With the cmd-extraction now requiring the slashed token
// to be the entire message, this kind of note must save normally with the
// `/token` text intact.
func TestSaveFromTextMsgWithSlashCmdInTheMiddle(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2025, 6, 29, 12, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	const text = "Check the /token please"
	rawUpdate := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: -1},
			Text: text,
			Entities: []tgbotapi.MessageEntity{{
				Type:   "bot_command",
				Offset: 10, // index of '/' in text
				Length: 6,  // length of "/token"
			}},
		},
	}
	err = bot.Reply(tg.NewTGUpd(rawUpdate))
	r.NoError(err)

	chat, err := bot.fs.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 29 June, Sunday\n- [ ] `12:00` Check the /token please\n", chat)
}

// TODO Chat.md
func TestSaveFromLongTextMsg(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, strings.Repeat("a", 34)))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)
	r.Len(tasks, 1)

	filename := "A" + strings.Repeat("a", 33) + ".md"
	r.Equal(filename, tasks[0].Name)

	content, err := bot.fs.Read("notes", filename)
	r.NoError(err)
	r.Equal("", content)
}

// TODO Chat.md
//
//	func TestSaveFromTextMsgWithSanitize(t *testing.T) {
//		r := require.New(t)
//
//		mode := userconfig.DefaultConfig.Mode
//		userconfig.DefaultConfig.Mode = userconfig.ModeTasks
//		defer func() {
//			userconfig.DefaultConfig.Mode = mode
//		}()
//
//		userFS, err := fs.NewFS("/", afero.NewMemMapFs())
//		r.NoError(err)
//		err = userFS.CreateSystemDirs()
//		r.NoError(err)
//
//		tgram := tg.NewFakeTG()
//
//		bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
//		err = bot.Reply(tg.NewUpd(-1, "New task/"))
//		r.NoError(err)
//
//		err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"c5e7dfaf771", inboxMsgHash(t, userFS, 0)})))
//		r.NoError(err)
//
//		tasks, err := bot.fs.FilesAndDirs("home")
//		r.NoError(err)
//
//		r.Len(tasks, 1)
//		r.Equal("New task／.md", tasks[0].Name)
//
//		content, err := bot.fs.Read("home", "New task／.md")
//		r.NoError(err)
//		r.Equal("New task/", content)
//
//		err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("home", nil)))
//		r.NoError(err)
//
//		err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("add_item", []string{"db0a776589b", inboxMsgHash(t, userFS, 0)})))
//		r.NoError(err)
//
//		r.Equal("<b>1</b> items"+wideSpacer, tgram.LastSentText)
//		r.Equal(tg.NewKeyboard([]tg.Row{
//			tg.NewBtn("👀 New task/", tg.NewCmd("task", []string{"home", "24e70ffbf48"})),
//		},
//		), tgram.LastSentKeyboard)
//	}
func TestAddMultilineNoteToNotes(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, "New task\nContent"))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
	r.True(tasks[0].IsMultiline)

	content, err := bot.fs.Read("notes", "New task.md")
	r.NoError(err)
	r.Equal("Content", content)
}

func TestAddNoteWithSpecCharsToNotes(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	userFS.CreateSystemDirs()
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, "New task\nUrl! https://g.com (Also_text] ##header\n-item1\n-item2\n1+1=2"))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)

	r.Len(tasks, 1)
	r.Equal("New task.md", tasks[0].Name)
	r.True(tasks[0].IsMultiline)

	content, err := bot.fs.Read("notes", "New task.md")
	r.NoError(err)
	r.Equal("Url! https://g.com (Also_text] ##header\n-item1\n-item2\n1+1=2", content)
}

func TestAddNoteWithOnlyWhitespace(t *testing.T) {
	// Test adding a task that contains only whitespace characters
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	err = bot.Reply(tg.NewUpd(-1, "   \t\n"))
	r.EqualError(err, "save: empty message")

	tasks, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)
	r.Len(tasks, 0)
}

func TestAddNoteWithLeadingAndTrailingSpaces(t *testing.T) {
	// Test adding a task with leading and trailing spaces in the name
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	userFS.CreateSystemDirs()
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	err = bot.Reply(tg.NewUpd(-1, "   Task with spaces   "))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)
	r.Len(tasks, 1)
	r.Equal("Task with spaces.md", tasks[0].Name)
}

func TestShowEmptyTodayList(t *testing.T) {
	// Test displaying today's tasks when there are none
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("home", nil)))
	r.NoError(err)
	r.Equal("🌴 Nothing here yet - send me something!", tgram.LastSentText)
}

func TestSaveFromTextMsgWithUnicodeCharacters(t *testing.T) {
	// Test handling of text messages containing Unicode characters
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	unicodeText := "测试含有Unicode字符的文本🚀🌟"
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, unicodeText))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	tasks, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)
	r.Len(tasks, 1)
	r.Equal("测试含有Unicode字符的文本🚀🌟.md", tasks[0].Name)
}

func TestSaveFromEmptyTextMsg(t *testing.T) {
	// Test handling of empty text messages
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, ""))
	r.EqualError(err, "save: empty message")

	tasks, err := bot.fs.FilesAndDirs("home")
	r.NoError(err)
	r.Len(tasks, 0)
}

func TestSaveFromRegularReply(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("home", "Existing file.md", "Existing content")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	database.SetHashOrPathByMsgID(255, "home/Existing file.md")

	bot := NewBot(-1, tgram, userFS, database, fakeConfig())

	upd := tg.NewUpd(-1, "Line")
	upd.ReplyToMessageID = 255
	err = bot.Reply(upd)
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("home")
	r.NoError(err)
	r.Len(files, 1)

	content, err := bot.fs.Read("home", "Existing file.md")
	r.NoError(err)
	r.Equal("Existing content\n\nLine\n", content)
}

func TestSaveFromPhotoWithCaption(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = "Caption"
	err = bot.Reply(upd)
	r.NoError(err)

	content, err := userFS.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 11 August, Sunday\n- [ ] `09:54` ![](media/tg_PHOTO_ID)\nCaption\n", content)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)
	r.Len(files, 1)
	r.Equal("Caption.md", files[0].Name)
	r.True(files[0].IsMultiline)

	content, err = bot.fs.Read("notes", "Caption.md")
	r.NoError(err)
	r.Equal("![](media/tg_PHOTO_ID)\nCaption", content)
}

func TestSaveFromPhotoWithLongCaption(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = strings.Repeat("a", 34)
	err = bot.Reply(upd)
	r.NoError(err)

	content, err := userFS.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 11 August, Sunday\n- [ ] `09:54` ![](media/tg_PHOTO_ID)\nAaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n", content)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	content, err = bot.fs.Read("notes", "A"+strings.Repeat("a", 33)+".md")
	r.NoError(err)
	r.Equal(fmt.Sprintf("![](media/tg_PHOTO_ID)\nA%s", strings.Repeat("a", 33)), content)
}

func TestSaveFromPhotoWithSanitizedCaption(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = "Caption/"
	err = bot.Reply(upd)
	r.NoError(err)

	content, err := userFS.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 11 August, Sunday\n- [ ] `09:54` ![](media/tg_PHOTO_ID)\nCaption/\n", content)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)

	r.Len(files, 1)
	r.Equal("Caption／.md", files[0].Name)
	r.True(files[0].IsMultiline)

	content, err = bot.fs.Read("notes", "Caption／.md")
	r.NoError(err)
	r.Equal("![](media/tg_PHOTO_ID)\nCaption/", content)
}

func TestSaveFromPhotoWithoutCaption(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)
	userFS.CreateDirsIfNotExist("notes")

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	err = bot.Reply(upd)
	r.NoError(err)

	content, err := userFS.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 11 August, Sunday\n- [ ] `09:54` ![](media/tg_PHOTO_ID)\n", content)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)

	r.Len(files, 1)
	// Be aware that it's not regular ꞉
	r.Equal("Img 11.08.24 09꞉54.md", files[0].Name)
	r.True(files[0].IsMultiline)

	// Be aware that it's not regular ꞉
	content, err = bot.fs.Read("notes", "Img 11.08.24 09꞉54.md")
	r.NoError(err)
	r.Equal("![](media/tg_PHOTO_ID)", content)
}

func TestSaveFromReplyPhotoWithCaption(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("home", "Existing file.md", "Existing content")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	database := db.NewFakeDB()
	database.SetHashOrPathByMsgID(255, "home/Existing file.md")

	bot := NewBot(-1, tgram, userFS, database, fakeConfig())

	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = "Caption"
	upd.ReplyToMessageID = 255
	err = bot.Reply(upd)
	r.NoError(err)

	content, err := bot.fs.Read("home", "Existing file.md")
	r.NoError(err)
	r.Equal("Existing content\n\n![](media/tg_PHOTO_ID)\nCaption\n", content)
}

func TestAddTaskToLater(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.CreateSystemDirs())
	r.NoError(userFS.Write(fs.DirUserRoot, fs.ChatFilename, "- [ ] First task"))

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	r.NoError(bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv_later", []string{chatBlockHash("- [ ] First task")}))))

	todayMD, err := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	r.NoError(err)
	r.NotContains(todayMD, "First task")

	laterMD, err := userFS.Read(fs.DirUserRoot, fs.LaterFilename)
	r.NoError(err)
	r.Contains(laterMD, "- [ ] First task")
}

func completeTask(t *testing.T, initial, hashed string) string {
	t.Helper()

	savedNow := now
	t.Cleanup(func() { now = savedNow })
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	r := require.New(t)
	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write("/", "Chat.md", initial))

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())
	r.NoError(bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("c", []string{chatBlockHash(hashed)}))))

	got, err := userFS.Read("/", "Chat.md")
	r.NoError(err)
	return got
}

func TestCompleteTask_Incomplete_NoTimestamp_NoHeader(t *testing.T) {
	got := completeTask(t, "- [ ] New task", "- [ ] New task")
	require.Equal(t, "- [x] New task", got)
}

func TestCompleteTask_Incomplete_WithTimestamp_NoHeader(t *testing.T) {
	got := completeTask(t, "- [ ] `00:00` New task", "- [ ] `00:00` New task")
	require.Equal(t, "- [x] `00:00` New task", got)
}

func TestCompleteTask_Incomplete_NoTimestamp_WithHeader(t *testing.T) {
	got := completeTask(t,
		"#### 1 January, Thursday\n- [ ] New task",
		"- [ ] New task",
	)
	require.Equal(t, "#### 1 January, Thursday\n- [x] New task", got)
}

func TestCompleteTask_Incomplete_WithTimestamp_WithHeader(t *testing.T) {
	got := completeTask(t,
		"#### 1 January, Thursday\n- [ ] `09:30` New task",
		"- [ ] `09:30` New task",
	)
	require.Equal(t, "#### 1 January, Thursday\n- [x] `09:30` New task", got)
}

// Both todayBlockHash and CompleteChecklistItem hash the first line after
// the marker, so the box must still flip even when the block has
// continuation lines attached.
func TestCompleteTask_Multiline_FlipsBox(t *testing.T) {
	block := "- [ ] `00:00` First task\nsome continuation"
	got := completeTask(t, block, block)
	require.Contains(t, got, "- [x] `00:00` First task")
	require.NotContains(t, got, "- [ ] `00:00` First task")
}

func TestCompleteTask_AlreadyCompleted_IsNoOp(t *testing.T) {
	got := completeTask(t, "- [x] Done task", "- [x] Done task")
	require.Equal(t, "- [x] Done task", got)
}

func TestCompleteTask_Pomodoro_AddsSchedule(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	cfg := fakeConfig()
	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), cfg)

	r.NoError(bot.togglePomodoro(nil))

	pomodoroBlock := "- [ ] " + fs.PomodoroTask
	before := time.Now().Unix()
	r.NoError(bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("c", []string{chatBlockHash(pomodoroBlock)}))))
	after := time.Now().Unix()

	todayMD, err := userFS.Read("/", fs.ChatFilename)
	r.NoError(err)
	r.Contains(todayMD, "- [x] "+fs.PomodoroTask)

	schedules, err := cfg.Schedules()
	r.NoError(err)
	r.Len(schedules, 1)
	r.Equal(fs.PomodoroTask, schedules[0].Filename)

	durationSec := int64(cfg.PomodoroDuration().Seconds())
	r.GreaterOrEqual(schedules[0].ScheduledAt, before+durationSec)
	r.LessOrEqual(schedules[0].ScheduledAt, after+durationSec)
}

func TestCompleteTask_NonPomodoro_DoesNotSchedule(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write("/", fs.ChatFilename, "- [ ] Regular task"))

	cfg := fakeConfig()
	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), cfg)

	r.NoError(bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("c", []string{chatBlockHash("- [ ] Regular task")}))))

	schedules, err := cfg.Schedules()
	r.NoError(err)
	r.Empty(schedules)
}

func TestCompleteChecklistItem_Pomodoro_DoesNotSchedule(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write("/", fs.ChatFilename, "- [ ] "+fs.PomodoroTask))

	cfg := fakeConfig()
	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), cfg)

	r.NoError(bot.completeChecklistItem([]string{fs.Hash(fs.ChatFilename), fs.Hash(fs.PomodoroTask)}))

	schedules, err := cfg.Schedules()
	r.NoError(err)
	r.Empty(schedules)
}

func TestToday(t *testing.T) {
	r := require.New(t)

	savedCtime := fs.Ctime
	defer func() {
		fs.Ctime = savedCtime
	}()
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 0
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("/", "Chat.md", "- [ ] `00:00` First task\n- [ ] `00:00` Second task")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("home", nil)))
	r.NoError(err)

	r.Equal("<b>2</b> items"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd("c", []string{"060f6b7c9c8"})),
		tg.NewBtn("🥈 Second task", tg.NewCmd("c", []string{"083d6c37d07"})),
	},
	), tgram.LastSentKeyboard)
}

func TestTodayQuickMenuFilled(t *testing.T) {
	savedCtime := fs.Ctime
	defer func() {
		fs.Ctime = savedCtime
	}()
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 0
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	cfg := fakeConfig()
	_ = cfg.AddQuickCmd("files")
	_ = cfg.AddQuickCmd("checklists")
	_ = cfg.AddQuickCmd("postpone")
	bot, tgram, r := makeBot(t, cfg)
	err := bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("home", nil)))
	r.NoError(err)
	r.Equal("<b>1</b> item"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd("c", []string{"832a6c2a713"})),
		tg.NewRow(
			tg.NewBtn("📄", tg.NewCmd("files", nil)),
			tg.NewBtn("☑️", tg.NewCmd("checklists", nil)),
			tg.NewBtn("🦥", tg.NewCmd("postpone", nil)),
		),
	},
	), tgram.LastSentKeyboard)
}

func TestTodayMultilineTaskShownAsLong(t *testing.T) {
	r := require.New(t)

	savedCtime := fs.Ctime
	defer func() { fs.Ctime = savedCtime }()
	fs.Ctime = func(fi os.FileInfo) int64 { return 0 }

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() { userconfig.DefaultConfig.Mode = mode }()

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time { return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC) }

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("/", "Chat.md", "- [ ] `00:00` First task\nsome continuation")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("home", nil)))
	r.NoError(err)

	hash := inboxMsgHash(t, userFS, 0)
	r.Equal("<b>1</b> item"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("👀 First task", tg.NewCmd(CmdShowLongItem, []string{hash})),
	}), tgram.LastSentKeyboard)
}

func TestTodayMixedSingleAndMultilineTasks(t *testing.T) {
	r := require.New(t)

	savedCtime := fs.Ctime
	defer func() { fs.Ctime = savedCtime }()
	fs.Ctime = func(fi os.FileInfo) int64 { return 0 }

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() { userconfig.DefaultConfig.Mode = mode }()

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time { return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC) }

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	content := "- [ ] `00:00` First task\n" +
		"- [ ] `00:00` Second task\nextra detail line"
	err = userFS.Write("/", "Chat.md", content)
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("home", nil)))
	r.NoError(err)

	first := inboxMsgHash(t, userFS, 0)
	second := inboxMsgHash(t, userFS, 1)
	r.Equal("<b>2</b> items"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("First task", tg.NewCmd(CmdComplete, []string{first})),
		tg.NewBtn("👀 Second task", tg.NewCmd(CmdShowLongItem, []string{second})),
	}), tgram.LastSentKeyboard)
}

// TODO Chat.md
//func TestTodayWithMultilineTasks(t *testing.T) {
//	r := require.New(t)
//
//	savedCtime := fs.Ctime
//	defer func() {
//		fs.Ctime = savedCtime
//	}()
//	fs.Ctime = func(fi os.FileInfo) int64 {
//		return 0
//	}
//
//	savedNow := now
//	defer func() {
//		now = savedNow
//	}()
//	now = func() time.Time {
//		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
//	}
//
//	mode := userconfig.DefaultConfig.Mode
//	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
//	defer func() {
//		userconfig.DefaultConfig.Mode = mode
//	}()
//
//	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
//	r.NoError(err)
//	err = userFS.Write("home", "First task.md", "content")
//	r.NoError(err)
//	err = userFS.Write("home", "Second task.md", "")
//	r.NoError(err)
//
//	tgram := tg.NewFakeTG()
//
//	upd := tg.NewUpdCmd(-1, tg.NewCmd("home", nil))
//	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
//	err = bot.Reply(upd)
//	r.NoError(err)
//
//	r.Equal("<b>2</b> items"+wideSpacer, tgram.LastSentText)
//	r.Equal(tg.NewKeyboard([]tg.Row{
//		tg.NewBtn("👀 First task", tg.NewCmd("task", []string{"home", "0824149b387"})),
//		tg.NewBtn("🥈 Second task", tg.NewCmd("c", []string{"home", "4eb62f93b3e"})),
//	},
//	), tgram.LastSentKeyboard)
//}

func TestFiles(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("", "Doc1.md", "")
	r.NoError(err)
	err = userFS.Write("", "Doc2.md", "")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("files", nil)))
	r.NoError(err)

	r.Equal("📄 Your files:"+wideSpacer, tgram.SentTexts[0])
	r.Equal(tg.NewKeyboard([]tg.Row{
		[]tg.Btn{
			tg.NewBtn("Doc1", tg.NewCmd("file", []string{fs.DirUserRoot, "c1253521ac7"})),
			tg.NewBtn("Doc2", tg.NewCmd("file", []string{fs.DirUserRoot, "64572c3093f"})),
		},
		[]tg.Btn{
			tg.NewBtn("🔎 Search", tg.NewCustomCmd("search", nil, "iq")),
			tg.NewBtn("🏠 Home", tg.NewCmd("home", nil)),
		},
	}), tgram.LastSentKeyboard)
}

func TestChecklists(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("/", "Checklist1_.md", "")
	r.NoError(err)
	err = userFS.Write("/", "Checklist2_.md", "")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("checklists", nil)))
	r.NoError(err)

	r.Equal("☑️ Checklists", tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Checklist1", tg.NewCmd("checklist", []string{"3755fd335a8"})),
		tg.NewBtn("Checklist2", tg.NewCmd("checklist", []string{"c54b67d1d3d"})),
		tg.NewBtn("🏠 Home", tg.NewCmd("home", nil)),
	},
	), tgram.LastSentKeyboard)
}

func TestAddSingleItemToChecklist(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.MakeDir("-checklist1-")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	err = bot.Reply(tg.NewUpd(-1, "Item"))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv_to_chk", []string{inboxMsgHash(t, userFS, 0), "-checklist1-"})))
	r.NoError(err)

	files, err := userFS.FilesAndDirs("-checklist1-")
	r.NoError(err)
	r.Len(files, 1)
	r.Equal("Item.md", files[0].Name)

	files, err = userFS.FilesAndDirs("home")
	r.NoError(err)
	r.Len(files, 0)
}

func TestAddMultipleItemsToChecklist(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.MakeDir("-checklist1-")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	err = bot.Reply(tg.NewUpd(-1, "Item\nItem2\nItem3"))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv_to_chk", []string{inboxMsgHash(t, userFS, 0), "-checklist1-"})))
	r.NoError(err)

	files, err := userFS.FilesAndDirs("-checklist1-")
	r.NoError(err)
	r.Len(files, 3)
	r.ElementsMatch([]string{"Item.md", "Item2.md", "Item3.md"}, []string{files[0].Name, files[1].Name, files[2].Name})
}

func TestShowChecklist(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("/", "Checklist1_.txt", "- [ ] Item")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("checklist", []string{"Checklist1_.txt"})))
	r.NoError(err)

	r.Equal("Checklist1"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Item", tg.NewCmd("check_item", []string{"Checklist1_.txt", "7d74f3b92b1"})),
		tg.NewBtn("🏠 Home", tg.NewCmd("home", nil)),
	},
	), tgram.LastSentKeyboard)
}

func TestCompleteItemInChecklist(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.MakeDir("_checklist1_")
	r.NoError(err)
	err = userFS.Write("_checklist1_", "Item.md", "")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("check_comp", []string{"49d872c025c", "7b72407ca70"})))
	r.NoError(err)

	r.Equal("Checklist1"+wideSpacer, tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("🏠 Home", tg.NewCmd("home", nil)),
	},
	), tgram.LastSentKeyboard)

	items, err := bot.fs.FilesAndDirs("_checklist1_")
	r.NoError(err)
	r.Empty(items)

	items, err = bot.fs.FilesAndDirs("archive")
	r.NoError(err)
	r.Len(items, 1)
	r.Equal("Item.md", items[0].Name)
}

func TestBotTodayLabelIcons(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	tgram := tg.NewFakeTG()
	b := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	// Pomodoro is the only task in today
	r.Nil(b.togglePomodoro(nil))
	label := b.homeLabel()
	r.Contains(label, "🌴")
	r.Contains(label, "🍅")

	// Pomodoro and another task in today
	todayMD, err := b.fs.Read("", "Chat.md")
	r.NoError(err)
	r.Nil(b.fs.Write("", "Chat.md", txt.AddChecklistItem(todayMD, "Task", false)))
	label = b.homeLabel(1)
	r.NotContains(label, "🌴")
	r.Contains(label, "🍅")

	// No pomodoro, but there is another task in today
	r.Nil(b.completeChecklistItem([]string{fs.Hash(fs.ChatFilename), fs.Hash(fs.PomodoroTask)}))
	label = b.homeLabel(1)
	r.NotContains(label, "🌴")
	r.NotContains(label, "🍅")

	// No pomodoro, no other tasks in today
	r.Nil(b.completeChecklistItem([]string{fs.Hash(fs.ChatFilename), fs.Hash("Task")}))
	label = b.homeLabel()
	r.NoError(err)
	r.Contains(label, "🌴")
	r.NotContains(label, "🍅")
}

func makeBot(t *testing.T, cfg *userconfig.Config) (*Bot, *tg.FakeTG, *require.Assertions) {
	r := require.New(t)
	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.Write("/", "Chat.md", "- [ ] First task")
	r.NoError(err)

	err = userFS.Write("/", "Later.md", "- [ ] Second task")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	return bot, tgram, r
}

func TestSettingsMainPanel(t *testing.T) {
	bot, tgram, r := makeBot(t, fakeConfig())
	err := bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("settings", nil)))
	r.NoError(err)
	r.Equal("Settings:", tgram.LastSentText)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("🧠 Full mode", tg.NewCmd("full", nil)),
		//tg.NewBtn("💬 Inbox mode", tg.NewCmd("chat", nil)),
		tg.NewBtn("📌 Notes mode", tg.NewCmd("notes_only", nil)),
		tg.NewBtn("✅ Tasks mode", tg.NewCmd("tasks_only", nil)),
		//tg.NewBtn("💚 Journal mode", tg.NewCmd("journal_only", nil)),
		tg.NewBtn("-", tg.NewCmd("nothing", nil)),
		tg.NewBtn("⚡️ Quick buttons", tg.NewCmd("c_quick_btns", nil)),
		tg.NewBtn("➡️ Move to buttons", tg.NewCmd("c_move_btns", nil)),
		tg.NewBtn("🌎 Timezone", tg.NewCmd("timezone", nil)),
		tg.NewBtn("🏠 Home", tg.NewCmd("home", nil)),
	},
	), tgram.LastSentKeyboard)
}

// Quick Panel Data-driven tests

var (
	btnFilesDel      = tg.NewBtn("📄 Files ➖", tg.NewCmd("del_quick", []string{"files"}))
	btnChecklistsDel = tg.NewBtn("☑️ Checklists ➖", tg.NewCmd("del_quick", []string{"checklists"}))
	btnPostponeDel   = tg.NewBtn("🦥 Postpone ➖", tg.NewCmd("del_quick", []string{"postpone"}))
)

var (
	delimiter = tg.NewBtn("-", tg.NewCmd("nothing", nil))
	homeBtn   = tg.NewBtn("🏠 Home", tg.NewCmd("home", nil))
)

var (
	btnLater          = tg.NewBtn("⏳ Later ➕", tg.NewCmd("add_quick", []string{"later"}))
	btnSearch         = tg.NewBtn("🔎 Search ➕", tg.NewCmd("add_quick", []string{"search"}))
	btnFilesAdd       = tg.NewBtn("📄 Files ➕", tg.NewCmd("add_quick", []string{"files"}))
	btnChecklistsAdd  = tg.NewBtn("☑️ Checklists ➕", tg.NewCmd("add_quick", []string{"checklists"}))
	btnPostponeAdd    = tg.NewBtn("🦥 Postpone ➕", tg.NewCmd("add_quick", []string{"postpone"}))
	btnReadChecklist  = tg.NewBtn("📚 Read ➕", tg.NewCmd("add_quick", []string{"read"}))
	btnWatchChecklist = tg.NewBtn("📺 Watch ➕", tg.NewCmd("add_quick", []string{"watch"}))
	btnShopChecklist  = tg.NewBtn("🛒 Shop ➕", tg.NewCmd("add_quick", []string{"shop"}))
	btnSchedule       = tg.NewBtn("🗓 Schedule ➕", tg.NewCmd("add_quick", []string{"schedule"}))
	btnHabits         = tg.NewBtn("🌱 Habits ➕", tg.NewCmd("add_quick", []string{"habits"}))
	btnRandom         = tg.NewBtn("🎲 Random ➕", tg.NewCmd("add_quick", []string{"random_note"}))
)

func TestConfigureQP_Empty_Default(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{""},
		tg.NewUpdCmd(-1, tg.NewCmd("c_quick_btns", nil)),
		[]tg.Row{
			delimiter,
			btnLater,
			btnSearch,
			btnFilesAdd,
			btnChecklistsAdd,
			btnPostponeAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnSchedule,
			btnHabits,
			btnRandom,
			homeBtn,
		},
	}, t)
}

func TestConfigureQP_Empty_AddFiles(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{""},
		tg.NewUpdCmd(-1, tg.NewCmd("add_quick", []string{"files"})),
		[]tg.Row{
			btnFilesDel,
			delimiter,
			btnLater,
			btnSearch,
			btnChecklistsAdd,
			btnPostponeAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnSchedule,
			btnHabits,
			btnRandom,
			homeBtn,
		},
	}, t)
}

func TestConfigureQP_Doc_AddCheckList(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files"},
		tg.NewUpdCmd(-1, tg.NewCmd("add_quick", []string{"checklists"})),
		[]tg.Row{
			btnFilesDel,
			btnChecklistsDel,
			delimiter,
			btnLater,
			btnSearch,
			btnPostponeAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnSchedule,
			btnHabits,
			btnRandom,
			homeBtn,
		},
	}, t)
}

func TestConfigureQP_DocChecklists_AddPostpone(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files", "checklists"},
		tg.NewUpdCmd(-1, tg.NewCmd("add_quick", []string{"postpone"})),
		[]tg.Row{
			btnFilesDel,
			btnChecklistsDel,
			btnPostponeDel,
			delimiter,
			btnLater,
			btnSearch,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnSchedule,
			btnHabits,
			btnRandom,
			homeBtn,
		},
	}, t)
}

func TestConfigureQP_DocChecklistsPostpone_Show(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files", "checklists", "postpone"},
		tg.NewUpdCmd(-1, tg.NewCmd("c_quick_btns", nil)),
		[]tg.Row{
			btnFilesDel,
			btnChecklistsDel,
			btnPostponeDel,
			delimiter,
			btnLater,
			btnSearch,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnSchedule,
			btnHabits,
			btnRandom,
			homeBtn,
		},
	}, t)
}

func TestConfigureQP_DocChecklistsPostpone_DelChecklists(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files", "checklists", "postpone"},
		tg.NewUpdCmd(-1, tg.NewCmd("del_quick", []string{"checklists"})),
		[]tg.Row{
			btnFilesDel,
			btnPostponeDel,
			delimiter,
			btnLater,
			btnSearch,
			btnChecklistsAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnSchedule,
			btnHabits,
			btnRandom,
			homeBtn,
		},
	}, t)
}

func TestConfigureQP_DocPostpone_DelDoc(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"files", "postpone"},
		tg.NewUpdCmd(-1, tg.NewCmd("del_quick", []string{"files"})),
		[]tg.Row{
			btnPostponeDel,
			delimiter,
			btnLater,
			btnSearch,
			btnFilesAdd,
			btnChecklistsAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnSchedule,
			btnHabits,
			btnRandom,
			homeBtn,
		},
	}, t)
}

func TestConfigureQP_Postpone_DelPostpone(t *testing.T) {
	RunQuickPanelTc(PrefTableTestCase{
		[]string{"postpone"},
		tg.NewUpdCmd(-1, tg.NewCmd("del_quick", []string{"postpone"})),
		[]tg.Row{
			delimiter,
			btnLater,
			btnSearch,
			btnFilesAdd,
			btnChecklistsAdd,
			btnPostponeAdd,
			btnReadChecklist,
			btnWatchChecklist,
			btnShopChecklist,
			btnSchedule,
			btnHabits,
			btnRandom,
			homeBtn,
		},
	}, t)
}

func TestConfigureQP_Empty_AddUnknown(t *testing.T) {
	RunquickpaneltcError(PrefTableTestCase{
		[]string{""},
		tg.NewUpdCmd(-1, tg.NewCmd("add_quick", []string{"wrong"})),
		[]tg.Row{},
	}, "unknown command: wrong", t)
}

func RunQuickPanelTc(tc PrefTableTestCase, t *testing.T) {
	cnf := fakeConfig()
	for _, cmd := range tc.existingCmds {
		_ = cnf.AddQuickCmd(cmd)
	}

	bot, tgram, r := makeBot(t, cnf)

	err := bot.Reply(tc.updToAnswer)
	r.NoError(err)
	r.Equal("Configure quick buttons (➕ = add to quick buttons, ➖ = to remove from quick buttons):", tgram.LastSentText)
	r.Equal(tg.NewKeyboard(tc.buttons), tgram.LastSentKeyboard)
}

func RunquickpaneltcError(tc PrefTableTestCase, expectedErr string, t *testing.T) {
	cnf := fakeConfig()
	for _, cmd := range tc.existingCmds {
		_ = cnf.AddQuickCmd(cmd)
	}
	bot, _, r := makeBot(t, cnf)
	actualErr := bot.Reply(tc.updToAnswer)
	r.EqualError(actualErr, expectedErr)
}

type PrefTableTestCase struct {
	existingCmds []string
	updToAnswer  *tg.Upd
	buttons      []tg.Row
}

func TestShowToFileNoDirs(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("/", "Note.md", "")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	const h = "fakehash"
	err = bot.showMoveToFileOrDir([]string{h})
	r.NoError(err)

	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewRow(tg.NewBtn("Note", tg.NewCmd("mf", []string{"345fb", h}))),
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{h}))),
	},
	), tgram.LastSentKeyboard)
}

func TestShowMoveToFile(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("notes", "Note.md", "")
	r.NoError(err)
	err = userFS.MakeDir("dir")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	const h = "fakehash"
	err = bot.showMoveToFileOrDir([]string{h})
	r.NoError(err)

	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(
			tg.NewBtn("🗂️ Dir", tg.NewCmd("mv", []string{"73600", h})),
			tg.NewBtn("🗂️ Notes", tg.NewCmd("mv", []string{"4358b", h})),
			tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{h})),
		),
	}), tgram.LastSentKeyboard)
}

func TestShow(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.showHTML("text", nil)
	r.NoError(err)

	r.Equal("text", tgram.LastSentText)
}

func TestShowMDLongMessage(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.showMD(strings.Repeat("a", 4096)+"b", nil)
	r.NoError(err)

	r.Len(tgram.SentTexts, 2)
	r.Equal("b", tgram.LastSentText)
}

// When utf8.RuneCountInString(textChunk) == 4096, tg sends the message (len(textChunk) => 7003)
// if I have 4095 chars and add 🟢, we have 4096 chars, and it is ok
// if I have 4095 chars and add ⚪️, we have 4097 chars, and we fail, so tg doesn't operate on glyph clusters
func TestShowMDLongMessageWithColoredEmojis(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.showMD(strings.Repeat("a", 4095)+"🟢", nil)
	r.NoError(err)

	r.Len(tgram.SentTexts, 1)
}

func TestShowMDLongMessageWithColoredEmoji(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.showMD(strings.Repeat("a", 4095)+"⚪️", nil)
	r.NoError(err)

	r.Len(tgram.SentTexts, 2)
}

func TestShowMDLongMessageSplitByNewLine(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.showMD(strings.Repeat("a", 4094)+"\nabc", nil)
	r.NoError(err)

	r.Len(tgram.SentTexts, 2)
	r.Equal("abc", tgram.LastSentText)
}

func TestShowMDLongMessageAttachKeyboardToTheLast(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.showMD(strings.Repeat("a", 4094)+"\nabc", tg.NewKeyboard([]tg.Row{tg.NewBtn("btn", tg.NewCmd("cmd", nil))}))
	r.NoError(err)

	r.Len(tgram.SentTexts, 2)
	r.Equal("abc", tgram.LastSentText)
	r.NotNil(tgram.LastSentKeyboard)
	r.Len(tgram.LastSentKeyboard.Btns, 1)
}

func TestMoveToExistingFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("/", "Chat.md", "#### 27 June, Friday\n`12:00` New message")
	r.NoError(err)
	err = userFS.Write("", "Existing file.md", "")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpdCmd(-1, tg.NewCmd("mf", []string{"1c8f819d075", inboxMsgHash(t, userFS, 0)}))
	err = bot.Reply(upd)
	r.NoError(err)

	content, err := userFS.Read("", "Existing file.md")
	r.NoError(err)
	r.Equal("#### 11 August 2024, Sunday\nNew message", content)
}

func TestMoveToExistingFileExistingRecord(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("/", "Chat.md", "#### 27 June, Friday\n`12:00` New message")
	r.NoError(err)
	err = userFS.Write("", "Existing file.md", "### 11.08.2024 Sunday\nContent")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpdCmd(-1, tg.NewCmd("mf", []string{"1c8f819d075", inboxMsgHash(t, userFS, 0)}))
	err = bot.Reply(upd)
	r.NoError(err)

	content, err := userFS.Read("", "Existing file.md")
	r.NoError(err)
	r.Equal("#### 11 August 2024, Sunday\nNew message\n\n### 11.08.2024 Sunday\nContent", content)
}

func TestShowMoveTo(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("", "file", "")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.Reply(tg.NewUpd(-1, "New task\nContent"))
	r.NoError(err)

	r.Equal("Saved!", tgram.SentTexts[0])

	h := inboxMsgHash(t, userFS, 0)
	kb := tg.NewKeyboard([]tg.Row{
		[]tg.Btn{
			{Name: "🌚 To tmrw", Cmd: tg.Cmd{Name: "sc_tmrw", Params: []string{h}, Type: "cmd"}},
			{Name: "⏳ To later", Cmd: tg.Cmd{Name: "mv_later", Params: []string{h}, Type: "cmd"}},
			{Name: "📆 To a day", Cmd: tg.Cmd{Name: "sc_day", Params: []string{h}, Type: "cmd"}},
		},
		[]tg.Btn{
			{Name: "📄 To File", Cmd: tg.Cmd{Name: "to_file", Params: []string{h}, Type: "cmd"}},
			{Name: "💚 To Journal", Cmd: tg.Cmd{Name: "mv_to_journal", Params: []string{h}, Type: "cmd"}},
		},
	},
	)
	r.Equal(kb, tgram.LastSentKeyboard)
}

func TestShowMoveFromTodayAndInbox(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2025, 6, 29, 9, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write(
		fs.DirUserRoot, fs.ChatFilename,
		"#### 29 June, Sunday\n- [ ] `09:00` Inbox body\n- [x] `09:05` Completed body\n",
	))

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("move", nil)))
	r.NoError(err)

	inboxHash := inboxMsgHash(t, userFS, 0)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("💬 Inbox body", tg.NewCmd("s_move", []string{inboxHash})),
		tg.NewRow(
			tg.NewBtn("Rename", tg.NewCmd("rename", []string{})),
			tg.NewBtn("OK", tg.NewCmd("home", []string{})),
		),
	}), tgram.LastSentKeyboard)
}

func TestMoveFromTodayAndInbox_ToLater(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2025, 6, 29, 9, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write(
		fs.DirUserRoot, fs.ChatFilename,
		"- [ ] Today task\n#### 29 June, Sunday\n- [ ] `09:00` Inbox body",
	))

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	// Click the inbox button → moveToLater ships the entry to Later.md.
	// Block 0 is the top-level "Today task"; the inbox body is block 1.
	inboxHash := inboxMsgHash(t, userFS, 1)
	r.NoError(bot.moveToLater([]string{inboxHash}))

	inboxMD, _ := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	r.NotContains(inboxMD, "Inbox body")

	laterMD, _ := userFS.Read(fs.DirUserRoot, fs.LaterFilename)
	r.Contains(laterMD, "- [ ] Inbox body")
}

func TestShowPostpone_WithInbox(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2025, 6, 29, 9, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write(
		fs.DirUserRoot, fs.ChatFilename,
		"#### 29 June, Sunday\n- [ ] `09:00` Inbox body\n- [x] `09:05` Completed body\n",
	))

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("postpone", nil)))
	r.NoError(err)

	inboxHash := inboxMsgHash(t, userFS, 0)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("💬 Inbox body", tg.NewCmd("post", []string{inboxHash})),
		tg.NewRow(
			tg.NewBtn("Rename", tg.NewCmd("rename", []string{})),
			tg.NewBtn("OK", tg.NewCmd("home", []string{})),
		),
	}), tgram.LastSentKeyboard)
}

func TestPostponeTodayTask(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write(fs.DirUserRoot, fs.ChatFilename, "- [ ] Today task"))
	r.NoError(userFS.Write(fs.DirUserRoot, fs.LaterFilename, ""))

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.postpone([]string{fs.Hash("Today task")})
	r.NoError(err)

	todayMD, _ := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	r.NotContains(todayMD, "Today task")

	laterMD, _ := userFS.Read(fs.DirUserRoot, fs.LaterFilename)
	r.Contains(laterMD, "- [ ] Today task")
}

func TestPostponeInboxEntry(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2025, 6, 29, 9, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write(
		fs.DirUserRoot, fs.ChatFilename,
		"#### 29 June, Sunday\n- [ ] `09:00` Inbox body",
	))

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	h := inboxMsgHash(t, userFS, 0)
	err = bot.postpone([]string{h})
	r.NoError(err)

	inboxMD, _ := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	r.NotContains(inboxMD, "Inbox body")

	laterMD, _ := userFS.Read(fs.DirUserRoot, fs.LaterFilename)
	r.Contains(laterMD, "- [ ] Inbox body")
}

func TestShowRename_WithInbox(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2025, 6, 29, 9, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write(
		fs.DirUserRoot, fs.ChatFilename,
		"#### 29 June, Sunday\n- [ ] `09:00` Inbox body\n- [x] `09:05` Completed body\n",
	))

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	r.NoError(bot.showRename(nil))

	inboxHash := inboxMsgHash(t, userFS, 0)
	r.Equal(tg.NewKeyboard([]tg.Row{
		tg.NewBtn("💬 Inbox body", tg.NewCmd("rename_file", []string{fs.ChatFilename, inboxHash})),
		tg.NewBtn("🏠 Home", tg.NewCmd("home", nil)),
	}), tgram.LastSentKeyboard)
}

func TestRenameInboxEntry(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2025, 6, 29, 9, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write(
		fs.DirUserRoot, fs.ChatFilename,
		"#### 29 June, Sunday\n- [ ] `09:00` Inbox body\n- [ ] `09:05` Second entry",
	))

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	h := inboxMsgHash(t, userFS, 0)
	r.NoError(bot.rename([]string{fs.ChatFilename, h, "Renamed body"}))

	inboxMD, _ := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	// Timestamp and marker preserved, body replaced.
	r.Contains(inboxMD, "- [ ] `09:00` Renamed body")
	r.NotContains(inboxMD, "Inbox body")
	// Untouched entry stays intact.
	r.Contains(inboxMD, "- [ ] `09:05` Second entry")
}

func TestRenameInboxEntry_PreservesCompletedMarker(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(userFS.Write(
		fs.DirUserRoot, fs.ChatFilename,
		"#### 29 June, Sunday\n- [x] `09:00` Done body",
	))

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	h := inboxMsgHash(t, userFS, 0)
	r.NoError(bot.rename([]string{fs.ChatFilename, h, "Renamed done"}))

	inboxMD, _ := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	r.Contains(inboxMD, "- [x] `09:00` Renamed done")
}

// Reproduces the "msgHash ... not found in inbox" error that users hit when
// forwarding multiple messages: message 1 produces a keyboard carrying
// msgHash1, message 2 is collapsed into the same inbox block as a
// continuation line, which shifts the block's hash. The original msgHash1
// from the keyboard then fails to resolve.
func TestForwardCollapse_HashStableAcrossContinuations(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2025, 6, 29, 14, 35, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	u1 := tg.NewUpd(-1, "one\ntwo\nthree")
	u1.TimeVal = 100
	u1.HasTimeVal = true
	r.NoError(bot.Reply(u1))

	msgHash1, ok := firstMsgHash(-1)
	r.True(ok, "firstMsgHash should be stored for msg 1")

	u2 := tg.NewUpd(-1, "two")
	u2.TimeVal = 100
	u2.HasTimeVal = true
	r.NoError(bot.Reply(u2))

	// The keyboard on msg 1 still carries msgHash1. After the collapsed append,
	// moveFromInbox must still resolve it to the first block.
	var gotContent string
	err = bot.moveFromChat(func(content string, _ time.Time) error {
		gotContent = content
		return nil
	}, false, msgHash1)
	r.NoError(err)
	r.Contains(gotContent, "One")
	r.Contains(gotContent, "two")
	r.Contains(gotContent, "three")
}

func TestShowScheduleEmpty(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("schedule", nil)))
	r.NoError(err)

	r.Equal("You don't have any scheduled tasks! 🌴", tgram.SentTexts[0])
}

func TestShowSchedule(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	savedSchedNow := now
	defer func() {
		now = savedSchedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 10, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	err = cfg.AddToSchedule("filename.md", 0, "")
	r.NoError(err)
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("schedule", nil)))
	r.NoError(err)

	r.Equal("<b>01 January, Thursday</b>\n- Filename", tgram.SentTexts[0])
}

func TestMoveToChecklistSplittable(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.MakeDir("-checklist-")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, "item1\nitem2"))
	r.NoError(err)

	content, err := userFS.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 11 August, Sunday\n- [ ] `09:54` Item1\nitem2\n", content)

	err = bot.moveToDirChecklist([]string{inboxMsgHash(t, userFS, 0), "-checklist-"})
	r.NoError(err)

	files, err := userFS.FilesAndDirs("-checklist-")
	r.NoError(err)

	items := fs.OnlyFilenames(files)
	r.ElementsMatch([]string{"Item1.md", "Item2.md"}, items)
}

func fakeConfig() *userconfig.Config {
	userFS, _ := fs.NewFS("/-1", afero.NewMemMapFs())
	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	_ = cfg.CreateDefaultIfNotExists()

	return cfg
}

func TestExtractCmd(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "jj journal record")
	cmd, err := bot.extractCmd(upd)
	r.NoError(err)

	r.NotNil(cmd)
	r.Equal("j", cmd.Name)
	r.Equal([]string{"Journal record"}, cmd.Params)
}

func TestExtractCmdRu(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "жж запись в журнал")
	cmd, err := bot.extractCmd(upd)
	r.NoError(err)

	r.NotNil(cmd)
	r.Equal("j", cmd.Name)
	r.Equal([]string{"Запись в журнал"}, cmd.Params)
}

func TestExtractCmdSkipsInTheBeginning(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "jjj task for tomorrow")
	cmd, err := bot.extractCmd(upd)
	r.NoError(err)

	r.Nil(cmd)
}

func TestExtractCmdSkipsAtTheMiddle(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "journal jj record")
	cmd, err := bot.extractCmd(upd)
	r.NoError(err)
	r.Nil(cmd)
}

func TestExtractCmdSkipsInTheEnd(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "task for tomorrow jjj")
	cmd, err := bot.extractCmd(upd)
	r.NoError(err)
	r.Nil(cmd)
}

func TestExtractCmdAtTheEnd(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, tg.NewFakeTG(), userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "journal record jj")
	cmd, err := bot.extractCmd(upd)
	r.NoError(err)

	r.NotNil(cmd)
	r.Equal("j", cmd.Name)
	r.Equal([]string{"Journal record"}, cmd.Params)
}

func TestMoveToJournal(t *testing.T) {
	r := require.New(t)

	savedNow := journal.Now
	defer func() {
		journal.Now = savedNow
	}()
	journal.Now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	err = userFS.Write("/", "Chat.md", "#### 27 June, Friday\n`01:01` Multiline\ncontent")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	files, err := userFS.FilesAndDirs("journal")
	r.NoError(err)
	r.Len(files, 1)

	content, err := userFS.Read("journal", files[0].Name)
	r.NoError(err)
	r.Equal("## 1 January, Thursday\n`00:00` Multiline\ncontent\n", content)

	content, err = userFS.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 27 June, Friday", content)
}

func TestAddToJournalFromShortcut(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, "jj record"))
	r.NoError(err)

	files, err := userFS.FilesAndDirs("journal")
	r.NoError(err)
	r.Len(files, 1)
}

func TestAddToJournalFromShortcutRu(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, "/ж запись"))
	r.NoError(err)

	files, err := userFS.FilesAndDirs("journal")
	r.NoError(err)
	r.Len(files, 1)
}

func TestAddToJournalFromShortcutRuCases(t *testing.T) {
	r := require.New(t)

	savedNow := journal.Now
	defer func() {
		journal.Now = savedNow
	}()
	journal.Now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpd(-1, "жЖ запись"))
	r.NoError(err)

	files, err := userFS.FilesAndDirs("journal")
	r.NoError(err)
	r.Len(files, 1)

	content, err := userFS.Read("journal", files[0].Name)
	r.NoError(err)
	r.Equal("## 1 January, Thursday\n`00:00` Запись\n", content)

	err = bot.Reply(tg.NewUpd(-1, "Запись2 ЖЖ"))
	r.NoError(err)

	content, err = userFS.Read("journal", files[0].Name)
	r.NoError(err)
	r.Equal("## 1 January, Thursday\n`00:00` Запись\n`00:00` Запись2\n", content)
}

func TestShowForADay(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("sc_day", []string{"1c8f819d075"})))
	r.NoError(err)

	r.Equal(tg.NewKeyboard([]tg.Row{
		[]tg.Btn{{Name: "🔄️ Repeat the task", Cmd: tg.Cmd{Name: "sc_day_r", Params: []string{"1c8f819d075"}, Type: "cmd"}}},
		[]tg.Btn{
			{Name: "Mon", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "345600", ""}, Type: "cmd"}},
			{Name: "Tue", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "432000", ""}, Type: "cmd"}},
			{Name: "Wed", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "518400", ""}, Type: "cmd"}},
			{Name: "Thu", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "604800", ""}, Type: "cmd"}},
		},
		[]tg.Btn{
			{Name: "Fri", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "86400", ""}, Type: "cmd"}},
			{Name: "Sat", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "172800", ""}, Type: "cmd"}},
			{Name: "Sun", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "259200", ""}, Type: "cmd"}},
		},
		[]tg.Btn{
			{Name: "1", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "2678400", ""}, Type: "cmd"}},
			{Name: "2", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "86400", ""}, Type: "cmd"}},
			{Name: "3", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "172800", ""}, Type: "cmd"}},
			{Name: "4", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "259200", ""}, Type: "cmd"}},
			{Name: "5", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "345600", ""}, Type: "cmd"}},
			{Name: "6", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "432000", ""}, Type: "cmd"}},
			{Name: "7", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "518400", ""}, Type: "cmd"}},
			{Name: "8", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "604800", ""}, Type: "cmd"}},
		},
		[]tg.Btn{
			{Name: "9", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "691200", ""}, Type: "cmd"}},
			{Name: "10", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "777600", ""}, Type: "cmd"}},
			{Name: "11", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "864000", ""}, Type: "cmd"}},
			{Name: "12", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "950400", ""}, Type: "cmd"}},
			{Name: "13", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1036800", ""}, Type: "cmd"}},
			{Name: "14", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1123200", ""}, Type: "cmd"}},
			{Name: "15", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1209600", ""}, Type: "cmd"}},
			{Name: "16", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1296000", ""}, Type: "cmd"}},
		},
		[]tg.Btn{
			{Name: "17", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1382400", ""}, Type: "cmd"}},
			{Name: "18", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1468800", ""}, Type: "cmd"}},
			{Name: "19", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1555200", ""}, Type: "cmd"}},
			{Name: "20", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1641600", ""}, Type: "cmd"}},
			{Name: "21", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1728000", ""}, Type: "cmd"}},
			{Name: "22", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1814400", ""}, Type: "cmd"}},
			{Name: "23", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1900800", ""}, Type: "cmd"}},
			{Name: "24", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "1987200", ""}, Type: "cmd"}},
		},
		[]tg.Btn{
			{Name: "25", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "2073600", ""}, Type: "cmd"}},
			{Name: "26", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "2160000", ""}, Type: "cmd"}},
			{Name: "27", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "2246400", ""}, Type: "cmd"}},
			{Name: "28", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "2332800", ""}, Type: "cmd"}},
			{Name: "29", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "2419200", ""}, Type: "cmd"}},
			{Name: "30", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "2505600", ""}, Type: "cmd"}},
			{Name: "31", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f819d075", "2592000", ""}, Type: "cmd"}},
		},
		tg.Btn{Name: "➡️ Move to today", Cmd: tg.Cmd{Name: "home", Params: []string(nil), Type: "cmd"}},
	}), tgram.LastSentKeyboard)
}

func TestShowForADayRecurring(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("sc_day_r", []string{"1c8f819d075"})))
	r.NoError(err)

	r.Equal(tg.NewKeyboard([]tg.Row{
		[]tg.Btn{
			{Name: "Weekdays", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "86400", "0 0 * * 1-5"}, Type: "cmd"}},
			{Name: "Every day", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "86400", "0 0 * * *"}, Type: "cmd"}},
		}, []tg.Btn{
			{Name: "Mon", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "345600", "0 0 * * 1"}, Type: "cmd"}},
			{Name: "Tue", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "432000", "0 0 * * 2"}, Type: "cmd"}},
			{Name: "Wed", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "518400", "0 0 * * 3"}, Type: "cmd"}},
			{Name: "Thu", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "604800", "0 0 * * 4"}, Type: "cmd"}},
		}, []tg.Btn{
			{Name: "Fri", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "86400", "0 0 * * 5"}, Type: "cmd"}},
			{Name: "Sat", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "172800", "0 0 * * 6"}, Type: "cmd"}},
			{Name: "Sun", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "259200", "0 0 * * 0"}, Type: "cmd"}},
		}, []tg.Btn{
			{Name: "1", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "2678400", "0 0 1 * *"}, Type: "cmd"}},
			{Name: "2", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "86400", "0 0 2 * *"}, Type: "cmd"}},
			{Name: "3", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "172800", "0 0 3 * *"}, Type: "cmd"}},
			{Name: "4", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "259200", "0 0 4 * *"}, Type: "cmd"}},
			{Name: "5", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "345600", "0 0 5 * *"}, Type: "cmd"}},
			{Name: "6", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "432000", "0 0 6 * *"}, Type: "cmd"}},
			{Name: "7", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "518400", "0 0 7 * *"}, Type: "cmd"}},
		}, []tg.Btn{
			{Name: "8", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "604800", "0 0 8 * *"}, Type: "cmd"}},
			{Name: "9", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "691200", "0 0 9 * *"}, Type: "cmd"}},
			{Name: "10", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "777600", "0 0 10 * *"}, Type: "cmd"}},
			{Name: "11", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "864000", "0 0 11 * *"}, Type: "cmd"}},
			{Name: "12", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "950400", "0 0 12 * *"}, Type: "cmd"}},
			{Name: "13", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1036800", "0 0 13 * *"}, Type: "cmd"}},
			{Name: "14", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1123200", "0 0 14 * *"}, Type: "cmd"}},
		}, []tg.Btn{
			{Name: "15", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1209600", "0 0 15 * *"}, Type: "cmd"}},
			{Name: "16", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1296000", "0 0 16 * *"}, Type: "cmd"}},
			{Name: "17", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1382400", "0 0 17 * *"}, Type: "cmd"}},
			{Name: "18", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1468800", "0 0 18 * *"}, Type: "cmd"}},
			{Name: "19", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1555200", "0 0 19 * *"}, Type: "cmd"}},
			{Name: "20", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1641600", "0 0 20 * *"}, Type: "cmd"}},
			{Name: "21", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1728000", "0 0 21 * *"}, Type: "cmd"}},
		}, []tg.Btn{
			{Name: "22", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1814400", "0 0 22 * *"}, Type: "cmd"}},
			{Name: "23", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1900800", "0 0 23 * *"}, Type: "cmd"}},
			{Name: "24", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "1987200", "0 0 24 * *"}, Type: "cmd"}},
			{Name: "25", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "2073600", "0 0 25 * *"}, Type: "cmd"}},
			{Name: "26", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "2160000", "0 0 26 * *"}, Type: "cmd"}},
			{Name: "27", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "2246400", "0 0 27 * *"}, Type: "cmd"}},
			{Name: "28", Cmd: tg.Cmd{Name: "sc", Params: []string{"1c8f", "2332800", "0 0 28 * *"}, Type: "cmd"}},
		}, tg.Btn{Name: "➡️ Move to today", Cmd: tg.Cmd{Name: "home", Params: []string(nil), Type: "cmd"}},
	}), tgram.LastSentKeyboard)
}

func TestSchedule(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)

	err = bot.Reply(tg.NewUpd(-1, "Task"))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("sc", []string{inboxMsgHash(t, userFS, 0), "345600", "0 0 * * 1-5"})))
	r.NoError(err)

	tasksForToday, err := userFS.FilesAndDirs("home")
	r.NoError(err)
	r.Empty(tasksForToday)

	laterMD, err := userFS.Read(fs.DirUserRoot, fs.LaterFilename)
	r.NoError(err)

	items, _ := txt.ChecklistItems(laterMD)
	r.Contains(items, "Task")

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Len(sc, 1)
	r.Equal("Task", sc[0].Filename)
	r.Equal(int64(345600), sc[0].ScheduledAt)
	r.Equal("0 0 * * 1-5", sc[0].Cron)
}

// The recurring picker truncates the chat msgHash to its 4-char prefix to fit
// Telegram's 64-byte callback_data limit when a cron expression is also packed
// in. moveFromChat must accept the prefix and still locate the inbox entry.
func TestScheduleRepeatedWithTruncatedHash(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)

	err = bot.Reply(tg.NewUpd(-1, "Task"))
	r.NoError(err)

	fullHash := inboxMsgHash(t, userFS, 0)
	truncated := fullHash[:4]

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("sc", []string{truncated, "345600", "0 0 * * 1-5"})))
	r.NoError(err)

	laterMD, err := userFS.Read(fs.DirUserRoot, fs.LaterFilename)
	r.NoError(err)
	items, _ := txt.ChecklistItems(laterMD)
	r.Contains(items, "Task")

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Len(sc, 1)
	r.Equal("Task", sc[0].Filename)
	r.Equal("0 0 * * 1-5", sc[0].Cron)
}

func TestScheduleNoRepeat(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)

	err = bot.Reply(tg.NewUpd(-1, "Task"))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("sc", []string{inboxMsgHash(t, userFS, 0), "345600", ""})))
	r.NoError(err)

	tasksForToday, err := userFS.FilesAndDirs("home")
	r.NoError(err)
	r.Empty(tasksForToday)

	laterMD, err := userFS.Read(fs.DirUserRoot, fs.LaterFilename)
	r.NoError(err)

	items, _ := txt.ChecklistItems(laterMD)
	r.Contains(items, "Task")

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Len(sc, 1)
	r.Equal("Task", sc[0].Filename)
	r.Equal(int64(345600), sc[0].ScheduledAt)
}

func TestInlineRequestTask(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("dir", "File.md", "File content")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, " dir / File ")
	upd.IsSentViaBotVal = true

	err = bot.Reply(upd)
	r.NoError(err)

	r.Equal("<b>File</b>\n\nFile content", tgram.LastSentText)
}

func TestInlineRequestFile(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, " File ")
	upd.IsSentViaBotVal = true

	err = bot.Reply(upd)
	r.NoError(err)

	r.Equal("<b>File</b>\n\nFile content", tgram.LastSentText)
}

func TestInlineRequestFileOutsideTheDirectory(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "../File")
	upd.IsSentViaBotVal = true

	err = bot.Reply(upd)
	r.Error(err)
	r.EqualError(err, "insecure input '../File': invalid request from inline query")
}

func TestInlineRequestFileOutsideTheDirectoryTwoSlashes(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "..//File")
	upd.IsSentViaBotVal = true

	err = bot.Reply(upd)
	r.Error(err)
	r.EqualError(err, "insecure input '..//File': invalid request from inline query")
}

func TestInlineRequestFileListOutsideDirs(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "../")
	upd.IsSentViaBotVal = true

	err = bot.Reply(upd)
	r.Error(err)
	r.EqualError(err, "insecure input '../': invalid request from inline query")
}

func TestInlineRequestFileListRootDirs(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	// cd /tmp//.. would lead to root
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "/..")
	upd.IsSentViaBotVal = true

	err = bot.Reply(upd)
	r.Error(err)
	r.EqualError(err, "insecure input '/..': invalid request from inline query")
}

func TestInlineRequestFileListRootDirsWithoutSlash(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	// cd /tmp/.. would lead to root
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "..")
	upd.IsSentViaBotVal = true

	err = bot.Reply(upd)
	r.Error(err)
	r.EqualError(err, "show file: can't find file: can't unhash '..' in '/': cannot unhash, maybe the file is missing")
}

func TestAnswerSearch(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	u := tg.NewUpd(-1, "")
	u.InlineQueryVal = "File"
	u.IsInlineQueryVal = true

	err = bot.answerSearch(u)
	r.NoError(err)

	var results []interface{}
	article := tgbotapi.NewInlineQueryResultArticleHTML("0", "File", "File.md")
	results = append(results, article)

	r.Equal(results, tgram.InlineQueryResults)
}

func TestAnswerSearchShowAllRoot(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)
	err = userFS.MakeDir("Dir")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	u := tg.NewUpd(-1, "")
	u.InlineQueryVal = "File"
	u.IsInlineQueryVal = true

	err = bot.answerSearch(u)
	r.NoError(err)

	var results []interface{}
	article := tgbotapi.NewInlineQueryResultArticleHTML("0", "File", "File.md")
	results = append(results, article)

	r.Equal(results, tgram.InlineQueryResults)
}

func TestAnswerSearchShowOutsideTheRoot(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)
	err = userFS.MakeDir("Dir")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	u := tg.NewUpd(-1, "")
	u.InlineQueryVal = "../"
	u.IsInlineQueryVal = true

	err = bot.answerSearch(u)
	r.Error(err)
	r.EqualError(err, "insecure input '../': invalid inline query")
}

func TestAnswerSearchShowOutsideTheRootNoSlash(t *testing.T) {
	r := require.New(t)

	memFS := afero.NewMemMapFs()
	err := afero.WriteFile(memFS, "/secret", []byte("secret"), 0o644)
	r.NoError(err)

	userFS, err := fs.NewFS("/-1", memFS)
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("", "File.md", "File content")
	r.NoError(err)
	err = userFS.MakeDir("Dir")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	u := tg.NewUpd(-1, "")
	u.InlineQueryVal = ".."
	u.IsInlineQueryVal = true

	err = bot.answerSearch(u)
	r.Error(err)
	r.EqualError(err, "inline reply: search notes: exists: unsafe path '/': unsafe path, possible security issue")
}

func TestShowFileEscapesHTML(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("", "File.md", "<b>bold*italic*")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	err = bot.showFile([]string{"/", "File.md"})
	r.NoError(err)
	r.Equal("<b>File</b>\n\n&lt;b&gt;bold<i>italic</i>", tgram.LastSentText)
}

func TestSaveToNewTask(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "New task"))
	r.NoError(err)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	content, err := userFS.Read("notes", "New task.md")
	r.NoError(err)
	r.Equal("", content)

	r.Nil(database.InputExpectation())
	msgID, ok := database.LastKeyboardMsgID()
	r.True(ok)
	r.Equal(3, msgID)
	r.Equal(msgID, tgram.LastSentMessageID)
}

func TestSaveToExistingFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	err = userFS.Write("/", "Chat.md", "#### 27 June, Friday\n`01:01` Existing\nmessage")
	r.NoError(err)
	err = userFS.Write("", "File.md", "#### 1 January 1970, Thursday\nExisting content")
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "New content"))
	r.NoError(err)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 1)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 1)})))
	r.NoError(err)

	selectFileKB := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("File", tg.NewCmd("mf", []string{"7595e", inboxMsgHash(t, userFS, 1)})),
		),
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(
			tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 1)})),
		),
	})
	r.Equal(selectFileKB, tgram.LastEditedKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mf", []string{"7595e", inboxMsgHash(t, userFS, 1)})))
	r.NoError(err)

	r.Nil(tgram.LastEditedKeyboard)

	content, err := userFS.Read("", "File.md")
	r.NoError(err)
	r.Equal("#### 1 January 1970, Thursday\nExisting content\nNew content", content)

	r.Nil(database.InputExpectation())
	keybdMsgID, ok := database.LastKeyboardMsgID()
	r.True(ok)
	r.Equal(3, keybdMsgID)
	r.Equal(3, tgram.LastSentMessageID)
}

func TestSaveToExistingFileModeTasks(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	err = userFS.Write("/", "Chat.md", "#### 27 June, Friday\n`01:01` New\ncontent")
	r.NoError(err)
	err = userFS.Write("", "File.md", "#### 1 January 1970, Thursday\nExisting\ncontent")
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "Text"))
	r.NoError(err)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 1)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 1)})))
	r.NoError(err)

	selectFileKB := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("File", tg.NewCmd("mf", []string{"7595e", inboxMsgHash(t, userFS, 1)})),
		),
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(
			tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 1)})),
		),
	})
	r.Equal(selectFileKB, tgram.LastEditedKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mf", []string{"7595e", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	r.Nil(tgram.LastEditedKeyboard)

	content, err := userFS.Read("", "File.md")
	r.NoError(err)
	r.Equal("#### 1 January 1970, Thursday\nExisting\ncontent\nNew\ncontent", content)

	r.Nil(database.InputExpectation())
	keybdMsgID, ok := database.LastKeyboardMsgID()
	r.True(ok)
	r.Equal(3, keybdMsgID)
	r.Equal(3, tgram.LastSentMessageID)
}

func TestSaveToNewFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("/", "Chat.md", "#### 1 January, Thursday\n`01:01` New\ncontent")
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "Text"))
	r.NoError(err)

	content, err := userFS.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 1 January, Thursday\n`01:01` New\ncontent\n- [ ] `00:00` Text\n", content)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 1)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 1)})),
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 1)})))
	r.NoError(err)

	selectFileKB := tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(
			tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 1)})),
		),
	})
	r.Equal(selectFileKB, tgram.LastEditedKeyboard)

	//err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mf", []string{"23200", inboxMsgHash(t, userFS, 0)})))
	//r.NoError(err)
	err = bot.Reply(tg.NewUpd(-1, "Myfile"))
	r.NoError(err)

	content, err = userFS.Read("/", "Myfile.md")
	r.NoError(err)
	r.Equal("Text", content)

	r.Nil(database.InputExpectation())
	msgID, ok := database.LastKeyboardMsgID()
	r.True(ok)
	r.Equal(1, msgID)
	r.Equal(2, tgram.LastSentMessageID)
}

func TestSaveToNewDirFull(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("/", "Chat.md", "#### 1 January, Thursday\n")
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "Text"))
	r.NoError(err)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	selectFileKB := tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(
			tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 0)})),
		),
	})
	r.Equal(selectFileKB, tgram.LastEditedKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	r.Equal("OK. Send me the name for your new dir", tgram.LastEditedText)
	r.Nil(tgram.LastEditedKeyboard)
	r.Equal(tg.NewCmd("mv_to_new_dir", []string{inboxMsgHash(t, userFS, 0), "%s"}), *database.InputExpectation())

	err = bot.Reply(tg.NewUpd(-1, "My dir"))
	r.NoError(err)

	r.Equal("🌴 Nothing here yet - send me something!", tgram.LastSentText)

	content, err := userFS.Read("my dir", "Text.md")
	r.NoError(err)
	r.Empty(content)

	r.Nil(database.InputExpectation())
	msgID, ok := database.LastKeyboardMsgID()
	r.True(ok)
	r.Equal(3, msgID)
	r.Equal(3, tgram.LastSentMessageID)
}

func TestSaveToNewDir(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "Text"))
	r.NoError(err)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	selectFileKB := tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(
			tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 0)})),
		),
	})
	r.Equal(selectFileKB, tgram.LastEditedKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	r.Equal("OK. Send me the name for your new dir", tgram.LastEditedText)
	r.Nil(tgram.LastEditedKeyboard)
	r.Equal(tg.NewCmd("mv_to_new_dir", []string{inboxMsgHash(t, userFS, 0), "%s"}), *database.InputExpectation())

	err = bot.Reply(tg.NewUpd(-1, "My dir"))
	r.NoError(err)

	r.Equal("🌴 Nothing here yet - send me something!", tgram.LastSentText)

	content, err := userFS.Read("my dir", "Text.md")
	r.NoError(err)
	r.Empty(content)

	r.Nil(database.InputExpectation())
	msgID, ok := database.LastKeyboardMsgID()
	r.True(ok)
	r.Equal(3, msgID)
	r.Equal(3, tgram.LastSentMessageID)
}

func TestSaveToNewMultilineFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("", "Text.md", "")
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "Multiline\ncontent"))
	r.NoError(err)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	selectFileKB := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("Text", tg.NewCmd("mf", []string{"23200", inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(
			tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 0)})),
		),
	})
	r.Equal(selectFileKB, tgram.LastEditedKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mf", []string{"23200", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	r.Empty(tgram.LastEditedKeyboard)

	content, err := userFS.Read("/", "Text.md")
	r.NoError(err)
	r.Equal("#### 1 January 1970, Thursday\nMultiline\ncontent", content)

	r.Nil(database.InputExpectation())
	msgID, ok := database.LastKeyboardMsgID()
	r.True(ok)
	r.Equal(3, msgID)
	r.Equal(msgID, tgram.LastSentMessageID)
}

func TestSaveToNewCustomFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "Text"))
	r.NoError(err)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	selectFileKB := tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(
			tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 0)})),
		),
	})
	r.Equal(selectFileKB, tgram.LastEditedKeyboard)

	err = bot.Reply(tg.NewUpd(-1, "new file"))
	r.NoError(err)

	r.Empty(tgram.LastEditedKeyboard.Btns)

	content, err := userFS.Read("", "New file.md")
	r.NoError(err)
	r.Equal("Text", content)

	r.Nil(database.InputExpectation())
	msgID, ok := database.LastKeyboardMsgID()
	r.True(ok)
	r.Equal(1, msgID)
	r.Equal(2, tgram.LastSentMessageID)
}

func TestSaveToRecentFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("", "Text.md", "Text")
	r.NoError(err)
	err = userFS.MakeDir("dir")
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	_ = cfg.AddMoveToCmd(CmdScheduleForTmrw)
	_ = cfg.AddMoveToCmd(CmdMoveToLater)
	_ = cfg.AddMoveToCmd(CmdShowScheduleForDay)
	_ = cfg.AddMoveToCmd(CmdShowMoveToDirOrFile)
	_ = cfg.AddMoveToCmd(CmdMoveToJournal)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "New text"))
	r.NoError(err)

	kb := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	selectFileKeyboard := tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("Text", tg.NewCmd("mf", []string{"23200", inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewBtn("Search", tg.NewCustomCmd("search", nil, "iq")),
		tg.NewRow(
			tg.NewBtn("🗂️ Dir", tg.NewCmd("mv", []string{"73600", inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("🗂 New Dir", tg.NewCmd("new_dir", []string{inboxMsgHash(t, userFS, 0)})),
		),
	})
	r.Equal(selectFileKeyboard, tgram.LastEditedKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mf", []string{"23200", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	r.Empty(tgram.LastEditedKeyboard)

	content, err := userFS.Read("", "Text.md")
	r.NoError(err)
	r.Equal("#### 1 January 1970, Thursday\nNew text\n\nText", content)

	recentCMD, ok := database.RecentCommand()
	r.Equal("mf", recentCMD)
	r.True(ok)

	// Adding text again to see if we have a recent file
	err = bot.Reply(tg.NewUpd(-1, "Text2"))
	r.NoError(err)

	kb = tg.NewKeyboard([]tg.Row{
		tg.NewRow(
			tg.NewBtn("🌚 To tmrw", tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("⏳ To later", tg.NewCmd("mv_later", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("📆 To a day", tg.NewCmd("sc_day", []string{inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewRow(
			tg.NewBtn("📄 To File", tg.NewCmd("to_file", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("💚 To Journal", tg.NewCmd("mv_to_journal", []string{inboxMsgHash(t, userFS, 0)})),
			tg.NewBtn("⭐️ Text", tg.NewCmd("mf", []string{"23200", inboxMsgHash(t, userFS, 0)})),
		),
		tg.NewRow(
			tg.NewBtn("👌", tg.NewCmd("home", []string{})),
		),
	})
	r.Equal(kb, tgram.LastSentKeyboard)

	r.Nil(database.InputExpectation())
	msgID, ok := database.LastKeyboardMsgID()
	r.True(ok)
	r.Equal(4, msgID)
	r.Equal(msgID, tgram.LastSentMessageID)
}

func TestSaveToTodayTask(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	err = userFS.Write("", "Chat.md", txt.AddChecklistItem("", "Existing task", false))
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	err = cfg.CreateDefaultIfNotExists()
	r.NoError(err)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	bot := NewBot(-1, tgram, userFS, database, cfg)
	err = bot.Reply(tg.NewUpd(-1, "New task"))
	r.NoError(err)

	// Click "move to checklist
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("home", nil)))
	r.NoError(err)
	// Block 0 is the top-level "Existing task"; the inbox entry "New task" is block 1.
	kb := tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Existing task", tg.NewCmd("c", []string{"04fcfb3c2ef"})),
		tg.NewBtn("New task", tg.NewCmd("c", []string{"58d765d4752"})),
	})

	r.Equal(kb, tgram.LastEditedKeyboard)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("c", []string{"58d765d4752"})))
	r.NoError(err)

	kb = tg.NewKeyboard([]tg.Row{
		tg.NewBtn("Existing task", tg.NewCmd("c", []string{"04fcfb3c2ef"})),
	})
	r.Equal(kb, tgram.LastEditedKeyboard)
}

func TestCollapseToMsg(t *testing.T) {
	var userID int64 = -1
	r := require.New(t)

	clean := func() {
		firstMsgHashes.Range(func(key, value interface{}) bool {
			firstMsgTimes.Delete(key)
			return true
		})
		firstMsgTimes.Range(func(key, value interface{}) bool {
			firstMsgTimes.Delete(key)
			return true
		})
	}
	clean()

	// Collapse same second messages
	setFirstMsgHash(userID, "h", 100)
	setFirstMsgTime(userID, 100)
	_, shouldCollapse := collapseToMsg(userID, 100)
	r.True(shouldCollapse)
	//r.Equal("file1.md", filename)
	clean()

	// Collapse next second messages
	setFirstMsgHash(userID, "h", 100)
	setFirstMsgTime(userID, 100)
	_, shouldCollapse = collapseToMsg(userID, 101)
	require.True(t, shouldCollapse, "Expected to collapse the message")
	//require.Equal(t, "file2.md", filename, "Expected filename to match the first message")
	clean()

	// Do not collapse distant messages
	setFirstMsgHash(userID, "h", 100)
	setFirstMsgTime(userID, 100)
	_, shouldCollapse = collapseToMsg(userID, 103)
	require.False(t, shouldCollapse, "Expected not to collapse the message")
	//require.Empty(t, filename, "Expected no filename for non-collapsing messages")
	clean()

	// Collapse consecutive batch messages
	setFirstMsgHash(userID, "h", 200)
	setFirstMsgTime(userID, 200)
	// Loop to simulate a series of consecutive messages within a one-second interval
	for i := 0; i < 5; i++ {
		_, shouldCollapse = collapseToMsg(userID, 200+i)
		require.True(t, shouldCollapse, "Expected to collapse the message in the batch")
		//require.Equal(t, "file4.md", filename, "Expected filename to match the initial message")
		setFirstMsgTime(userID, 200+i)
	}
	clean()
}

func TestCollapseForwardedMessages(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	tgram := tg.NewFakeTG()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
	upd := tg.NewUpd(-1, "First msg")
	upd.TimeVal = 0
	upd.HasTimeVal = true
	err = bot.Reply(upd)
	r.NoError(err)

	upd = tg.NewUpd(-1, "Second msg")
	upd.TimeVal = 0
	upd.HasTimeVal = true
	err = bot.Reply(upd)
	r.NoError(err)

	upd = tg.NewUpd(-1, "Third msg")
	upd.TimeVal = 1
	upd.HasTimeVal = true
	err = bot.Reply(upd)
	r.NoError(err)

	upd = tg.NewUpd(-1, "Fourth msg")
	upd.TimeVal = 3
	upd.HasTimeVal = true
	err = bot.Reply(upd)
	r.NoError(err)

	content, err := userFS.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 1 January, Thursday\n- [ ] `00:00` First msg\nSecond msg\nThird msg\n- [ ] `00:00` Fourth msg\n", content)

	// Clean
	firstMsgHashes.Range(func(key, value interface{}) bool {
		firstMsgTimes.Delete(key)
		return true
	})
	firstMsgTimes.Range(func(key, value interface{}) bool {
		firstMsgTimes.Delete(key)
		return true
	})
}

func TestTitleChecklist(t *testing.T) {
	r := require.New(t)

	title := checklistTitle("_checklist_")
	r.Equal("Checklist", title)
}

func TestTitleChecklistItem(t *testing.T) {
	r := require.New(t)

	title := checklistTitle("_checklist_item")
	r.Equal("Item", title)
}

func TestRestoreMsg_EmptyMessage(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, nil, userFS, nil, nil)

	filename := "Empty.md"
	err = userFS.Write("home", filename, "")
	r.NoError(err)

	title, err := bot.restoreMsg("home", filename)
	r.NoError(err)
	r.Equal("Empty", title)
}

func TestRestoreMsg_ContentWithoutTitle(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, nil, userFS, nil, nil)

	filename := "NewTask.md"
	content := "Some content that doesn't include the title"
	err = userFS.Write("home", filename, content)
	r.NoError(err)

	msg, err := bot.restoreMsg("home", filename)
	r.NoError(err)
	r.Equal("NewTask\nSome content that doesn't include the title", msg)
}

func TestRestoreMsg_ContentWithTitle(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, nil, userFS, nil, nil)

	filename := "Task.md"
	content := "Task\nDetails about the task"
	err = userFS.Write("home", filename, content)
	r.NoError(err)

	msg, err := bot.restoreMsg("home", filename)
	r.NoError(err)
	r.Equal(content, msg)
}

func TestRestoreMsg_ContentWithSanitizedTitle(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, nil, userFS, nil, nil)

	filename := "Task／Slash.md"
	content := "Task/Slash\nDetails about the task"
	err = userFS.Write("home", filename, content)
	r.NoError(err)

	msg, err := bot.restoreMsg("home", filename)
	r.NoError(err)
	r.Equal(content, msg)
}

func TestRestoreMsg_WithImage(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, nil, userFS, nil, nil)

	filename := "Caption.md"
	content := "![img](tg_url.jpg)\nCaption"
	err = userFS.Write("home", filename, content)
	r.NoError(err)

	msg, err := bot.restoreMsg("home", filename)
	r.NoError(err)
	r.Equal(content, msg)
}

func TestRestoreMsg_WithImageSanitizedFilename(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, nil, userFS, nil, nil)

	filename := "Caption／File"
	content := "![img](tg_url.jpg)\nCaption/File"
	err = userFS.Write("home", filename, content)
	r.NoError(err)

	msg, err := bot.restoreMsg("home", filename)
	r.NoError(err)
	r.Equal(content, msg)
}

func TestSaveFromImage_NewFile(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)
	userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = "New Image"

	err = bot.saveFromImage(upd)
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)
	r.Len(files, 1)
	r.Equal("New Image.md", files[0].Name)

	content, err := bot.fs.Read("notes", "New Image.md")
	r.NoError(err)
	r.Equal("![](media/tg_PHOTO_ID)\nNew Image", content)
}

func TestSaveFromImage_LongCaption(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)
	userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = strings.Repeat("a", 34)

	err = bot.saveFromImage(upd)
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	filename := "A" + strings.Repeat("a", 33) + ".md"
	content, err := bot.fs.Read("notes", filename)
	r.NoError(err)
	r.Equal(fmt.Sprintf("![](media/tg_PHOTO_ID)\nA%s", strings.Repeat("a", 33)), content)
}

func TestSaveFromImage_MultilineCaption(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = "abc\ndef"

	err = bot.saveFromImage(upd)
	r.NoError(err)

	content, err := userFS.Read("/", "Chat.md")
	r.NoError(err)
	r.Equal("#### 11 August, Sunday\n- [ ] `09:54` ![](media/tg_PHOTO_ID)\nAbc\ndef\n", content)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	filename := "Abc.md"
	content, err = bot.fs.Read("notes", filename)
	r.NoError(err)
	r.Equal("![](media/tg_PHOTO_ID)\nAbc\ndef", content)
}

func TestSaveFromImage_ReplyToExistingFile(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	// Setup in-memory filesystem and add an existing file
	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.Write("home", "Existing file.md", "Existing content")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	database := db.NewFakeDB()
	database.SetHashOrPathByMsgID(255, "home/Existing file.md")

	bot := NewBot(-1, tgram, userFS, database, fakeConfig())

	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"
	upd.PhotoCaption = "Image Caption"
	upd.ReplyToMessageID = 255

	err = bot.saveFromImage(upd)
	r.NoError(err)

	content, err := bot.fs.Read("home", "Existing file.md")
	r.NoError(err)
	r.Equal("Existing content\n\n![](media/tg_PHOTO_ID)\nImage Caption\n", content)
}

func TestSaveFromImage_EmptyCaption(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateDirsIfNotExist("notes")
	r.NoError(err)
	userFS.CreateDirsIfNotExist("notes")

	tgram := tg.NewFakeTG()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())

	upd := tg.NewUpd(-1, "")
	upd.PhotoID = "PHOTO_ID"

	err = bot.saveFromImage(upd)
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	files, err := bot.fs.FilesAndDirs("notes")
	r.NoError(err)
	r.Len(files, 1)
	r.Equal("Img 01.01.70 00꞉00.md", files[0].Name)

	content, err := bot.fs.Read("notes", "Img 01.01.70 00꞉00.md")
	r.NoError(err)
	r.Equal("![](media/tg_PHOTO_ID)", content)
}

func TestCreateOrAdd_NewFile(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, nil, userFS, nil, nil)

	dir := "home"
	filename := "NewFile.md"
	content := "This is new content"

	err = bot.createOrAdd(dir, filename, content)
	r.NoError(err)

	storedContent, err := bot.fs.Read(dir, filename)
	r.NoError(err)
	r.Equal(content, storedContent)
}

func TestCreateOrAdd_AppendToExistingFile(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, nil, userFS, nil, nil)

	dir := "home"
	filename := "ExistingFile.md"
	existingContent := "Existing content"
	newContent := "New content"

	err = userFS.Write(dir, filename, existingContent)
	r.NoError(err)

	err = bot.createOrAdd(dir, filename, newContent)
	r.NoError(err)

	expectedContent := "Existing content\nNew content"
	storedContent, err := bot.fs.Read(dir, filename)
	r.NoError(err)
	r.Equal(expectedContent, storedContent)
}

func TestCreateOrAdd_ReplaceEmptyContent(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	bot := NewBot(-1, nil, userFS, nil, nil)

	dir := "home"
	filename := "EmptyFile.md"
	newContent := "New content"

	err = userFS.Write(dir, filename, "\n")
	r.NoError(err)

	err = bot.createOrAdd(dir, filename, newContent)
	r.NoError(err)

	storedContent, err := bot.fs.Read(dir, filename)
	r.NoError(err)
	r.Equal(newContent, storedContent)
}

func TestExtractTitleAndContent_EmptyMessage(t *testing.T) {
	r := require.New(t)

	bot := NewBot(-1, nil, nil, nil, nil)

	title, content, err := bot.extractHeaderAndBody("", 33)
	r.Error(err)
	r.Contains(err.Error(), "extract title: empty msg")
	r.Equal("", title)
	r.Equal("", content)
}

func TestExtractTitleAndContent_SimpleMessage(t *testing.T) {
	r := require.New(t)

	bot := NewBot(-1, nil, nil, nil, nil)

	msg := "Simple DisplayName"
	title, content, err := bot.extractHeaderAndBody(msg, 33)
	r.NoError(err)
	r.Equal("Simple DisplayName", title)
	r.Equal("", content)
}

func TestExtractTitleAndContent_MultilineMessage(t *testing.T) {
	r := require.New(t)

	bot := NewBot(-1, nil, nil, nil, nil)

	msg := "DisplayName Line\nThis is the content"
	title, content, err := bot.extractHeaderAndBody(msg, 33)
	r.NoError(err)
	r.Equal("DisplayName Line", title)
	r.Equal("This is the content", content)
}

func TestExtractTitleAndContent_TitleExceedsMaxLength(t *testing.T) {
	r := require.New(t)

	bot := NewBot(-1, nil, nil, nil, nil)

	longTitle := strings.Repeat("a", 33+1)
	msg := longTitle + "\nContent below"
	expectedTitle := txt.Substr(txt.Ucfirst(longTitle), 0, 33) + "..."

	title, content, err := bot.extractHeaderAndBody(msg, 33)
	r.NoError(err)
	r.Equal(expectedTitle, title)
	r.Equal("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nContent below", content)
}

func TestExtractTitleAndContent_TitleSameAsContent(t *testing.T) {
	r := require.New(t)

	bot := NewBot(-1, nil, nil, nil, nil)

	msg := "Identical DisplayName"
	title, content, err := bot.extractHeaderAndBody(msg, 33)
	r.NoError(err)
	r.Equal("Identical DisplayName", title)
	r.Equal("", content)
}

func TestExtractTitleAndContent_ContentStartsWithTitle(t *testing.T) {
	r := require.New(t)

	bot := NewBot(-1, nil, nil, nil, nil)

	msg := "DisplayName Line\nDisplayName Line\nAdditional content"
	title, content, err := bot.extractHeaderAndBody(msg, 33)
	r.NoError(err)
	r.Equal("DisplayName Line", title)
	r.Equal("DisplayName Line\nAdditional content", content)
}

func TestExtractTitleAndContent_TitleNeedsSanitization(t *testing.T) {
	r := require.New(t)

	bot := NewBot(-1, nil, nil, nil, nil)

	msg := "Invalid/DisplayName?Name\nContent here"

	title, content, err := bot.extractHeaderAndBody(msg, 33)
	r.NoError(err)
	r.Equal("Invalid／DisplayName？Name", title)
	r.Equal("Invalid/DisplayName?Name\nContent here", content)
}

func TestMoveToExistingNote_Success(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.CreateSystemDirs()
	r.NoError(err)

	err = userFS.MakeDir("notes")
	r.NoError(err)
	err = userFS.Write("notes", "ExistingNote.md", "Existing content\n")
	r.NoError(err)

	tgram := tg.NewFakeTG()
	cfg := fakeConfig()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)

	err = bot.Reply(tg.NewUpd(-1, "Task content"))
	r.NoError(err)

	toDirHash := fs.Hash("notes")
	toFilenameHash := fs.Hash("ExistingNote.md")
	err = bot.moveToExistingNote([]string{toFilenameHash, toDirHash, inboxMsgHash(t, userFS, 0)})
	r.NoError(err)

	content, err := userFS.Read("notes", "ExistingNote.md")
	r.NoError(err)
	r.Equal("#### 1 January 1970, Thursday\nTask content\n\nExisting content\n", content)

	_, err = userFS.Read("home", "Task.md")
	r.Error(err)
}

func TestMoveToExistingNote_FileNotFound(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()
	cfg := fakeConfig()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)

	toDirHash := fs.Hash("notes")
	toFilenameHash := fs.Hash("ExistingNote.md")
	fromFilenameHash := fs.Hash("Task.md")

	err = bot.moveToExistingNote([]string{toFilenameHash, toDirHash, fromFilenameHash})
	r.Error(err)
}

func TestMoveToExistingNote_InvalidIndex(t *testing.T) {
	r := require.New(t)

	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeFull
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	tgram := tg.NewFakeTG()
	cfg := fakeConfig()
	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)

	toDirHash := "invalidHash"
	toFilenameHash := "invalidHash"
	msgIndex := "-1"

	err = bot.moveToExistingNote([]string{toFilenameHash, toDirHash, msgIndex})
	r.Error(err)
	r.Contains(err.Error(), "move to existing note")
}

// TODO move to Chat.md
func FuzzSaveFromTextMsg(f *testing.F) {
	mode := userconfig.DefaultConfig.Mode
	userconfig.DefaultConfig.Mode = userconfig.ModeTasks
	defer func() {
		userconfig.DefaultConfig.Mode = mode
	}()

	seedInputs := []string{
		"Normal task",
		"Special char /\\:*?|<>",
		"Emoji 😃🚀",
		strings.Repeat("a", 5000),
	}
	for _, input := range seedInputs {
		f.Add(input)
	}
	f.Add(".")
	f.Add("..")
	f.Add("/today/..md")
	f.Add("/valid/path")
	f.Add("../file")
	f.Add("../../file")
	f.Add("../../../file")

	f.Fuzz(func(t *testing.T, input string) {
		if len(strings.TrimSpace(input)) == 0 {
			return
		}

		// Telegram trims space
		input = strings.TrimSpace(input)

		// Ignore input with shortcuts like adding to journal or to recent file.
		// Because in that case file won't be created in "home" folder
		shortcuts := []string{"jj", "жж", "++"}
		for _, shortcut := range shortcuts {
			lowerInput := strings.ToLower(input)
			if strings.HasPrefix(lowerInput, shortcut) || strings.HasSuffix(lowerInput, shortcut) {
				return
			}
		}

		// fs.sanitizeName will strip away \0 characters as they're not allowed in filenames
		if strings.Contains(input, "\x00") {
			fmt.Println("Skipping string with null character")
			return
		}

		r := require.New(t)

		memfs := afero.NewMemMapFs()
		_ = memfs.Mkdir("/user", 0o755)
		userFS, err := fs.NewFS("/user", memfs)
		userFS.CreateDirsIfNotExist("notes")
		r.NoError(err)
		err = userFS.CreateSystemDirs()
		r.NoError(err)
		err = userFS.CreateDirsIfNotExist("notes")
		r.NoError(err)

		tgram := tg.NewFakeTG()

		savedPlugins := BotPlugins
		BotPlugins = []BotPlugin{}
		defer func() {
			BotPlugins = savedPlugins
		}()

		bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), fakeConfig())
		err = bot.Reply(tg.NewUpd(-1, input))
		if err != nil {
			// Check that no entries are created besides our user folder
			entries, err := afero.ReadDir(memfs, "/")
			r.NoError(err)
			r.Len(entries, 1)
			return
		}

		err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("mv", []string{"4358b5009c6", inboxMsgHash(t, userFS, 0)})))
		r.NoError(err)

		tasks, err := bot.fs.FilesAndDirs("notes")
		r.NoError(err)

		if input == "" {
			r.Len(tasks, 0)
			return
		}

		r.Len(tasks, 1, "No tasks created for input %q", input)
		title := txt.Ucfirst(strings.TrimSpace(strings.SplitN(strings.TrimSpace(input), "\n", 2)[0]))
		if utf8.RuneCountInString(title) > 100 {
			title = txt.Substr(title, 0, 100) + "..."
		}
		filename := fs.SanitizeFilename(title) + ".md"
		r.Equal(filename, tasks[0].Name)

		_, err = bot.fs.Read("notes", filename)
		r.NoError(err)
	})
}

func TestJournalOnlyMode_SaveTextMessage(t *testing.T) {
	r := require.New(t)

	savedNow := journal.Now
	defer func() {
		journal.Now = savedNow
	}()
	journal.Now = func() time.Time {
		return time.Date(2024, 8, 11, 9, 54, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	err = cfg.SetMode(userconfig.ModeJournal)
	r.NoError(err)

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.Reply(tg.NewUpd(-1, "Journal entry"))
	r.NoError(err)

	todayFiles, err := bot.fs.FilesAndDirs("home")
	r.NoError(err)
	r.Len(todayFiles, 0)

	journalFiles, err := bot.fs.FilesAndDirs("journal")
	r.NoError(err)
	r.Len(journalFiles, 1)

	content, err := bot.fs.Read("journal", journalFiles[0].Name)
	r.NoError(err)
	r.Contains(content, "Journal entry")
	r.Contains(content, "11 August, Sunday")
}

//func TestFileOnlyMode_SaveTextMessage(t *testing.T) {
//	r := require.New(t)
//
//	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
//	r.NoError(err)
//	err = userFS.CreateSystemDirs()
//	r.NoError(err)
//
//	tgram := tg.NewFakeTG()
//
//	cfg := fakeConfig()
//	err = cfg.SetMode(userconfig.ModeOneFile)
//	r.NoError(err)
//
//	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
//	err = bot.Reply(tg.NewUpd(-1, "File content"))
//	r.NoError(err)
//
//	todayFiles, err := bot.fs.FilesAndDirs("home")
//	r.NoError(err)
//	r.Len(todayFiles, 0)
//
//	rootFiles, err := bot.fs.FilesAndDirs("/")
//	r.NoError(err)
//	r.True(len(rootFiles) > 0)
//
//	content, err := bot.fs.Read("", "Chat.md")
//	r.NoError(err)
//	r.Equal("File content", content)
//}

func TestShowToday_NotesOnlyMode(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.MakeDir("test-dir")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	err = cfg.SetMode(userconfig.ModeNotes)
	r.NoError(err)

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.ShowHome(nil)
	r.NoError(err)

	// Should show directories instead of today tasks
	// The exact message depends on showDirs implementation, but we verify it was called
	r.NotEmpty(tgram.LastSentText)
	// In notes-only mode, ShowToday should redirect to showing directories
}

func TestShowToday_JournalOnlyMode(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	err = cfg.SetMode(userconfig.ModeJournal)
	r.NoError(err)

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.ShowHome(nil)
	r.NoError(err)

	// Should send "What's on your mind?" message
	r.Contains(tgram.LastSentText, "What's on your mind?")
	r.Nil(tgram.LastSentKeyboard) // No keyboard should be sent
}

func TestShowToday_NormalMode(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	// Default mode (not notes-only, journal-only, or one-file-only)
	err = cfg.SetMode(userconfig.ModeFull)
	r.NoError(err)

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.ShowHome(nil)
	r.NoError(err)

	// Should show empty today list
	r.Equal("🌴 Nothing here yet - send me something!", tgram.LastSentText)
}

func TestShowToday_NormalModeWithTasks(t *testing.T) {
	r := require.New(t)

	savedCtime := fs.Ctime
	defer func() {
		fs.Ctime = savedCtime
	}()
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 0
	}

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	err = userFS.Write("/", "Chat.md", "- [ ] test task")
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	err = cfg.SetMode(userconfig.ModeFull)
	r.NoError(err)

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.ShowHome(nil)
	r.NoError(err)

	r.Contains(tgram.LastSentText, "1")
	r.Contains(tgram.LastSentText, "item")

	r.Len(tgram.LastSentKeyboard.Btns, 1)
}

// TestShowToday_InboxMixedFormat confirms ShowToday renders inbox entries in
// both the timestamped `- [ ] HH:MM body` and timestamp-less `- [ ] body`
// forms, and hides completed `- [x]` entries while keeping the on-disk index
// stable so callback params (Complete, MoveTo, ...) still point at the right line.
func TestShowToday_InboxMixedFormat(t *testing.T) {
	r := require.New(t)

	savedCtime := fs.Ctime
	defer func() { fs.Ctime = savedCtime }()
	fs.Ctime = func(fi os.FileInfo) int64 { return 0 }

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	// Inbox has three entries at on-disk positions 0, 1, 2:
	//   0: no-timestamp form `- [ ] body`       -> shown
	//   1: timestamped form `- [ ] HH:MM body`  -> shown
	//   2: completed `- [x] HH:MM body`         -> hidden
	inbox := "#### 1 January, Thursday\n" +
		"- [ ] Plain msg\n" +
		"- [ ] `09:05` New msg\n" +
		"- [x] `09:10` Done msg\n"
	err = userFS.Write(fs.DirUserRoot, fs.ChatFilename, inbox)
	r.NoError(err)

	tgram := tg.NewFakeTG()

	cfg := fakeConfig()
	err = cfg.SetMode(userconfig.ModeFull)
	r.NoError(err)

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.ShowHome(nil)
	r.NoError(err)

	// Label: 2 tasks left (plain + timestamped unchecked); completed one excluded.
	r.Equal("<b>2</b> items"+wideSpacer, tgram.LastSentText)

	// Two rows, each with one button. The completed entry (disk position 2) is
	// not rendered but its slot is preserved — the next fresh entry added to
	// the inbox would be position 3, not 2.
	r.Len(tgram.LastSentKeyboard.Btns, 2)

	firstBtn, ok := tgram.LastSentKeyboard.Btns[0].(tg.Btn)
	r.True(ok)
	r.Equal(tg.Cmd{Name: CmdComplete, Params: []string{chatBlockHash("- [ ] Plain msg")}, Type: "cmd"}, firstBtn.Cmd)
	r.Contains(firstBtn.Name, "Plain msg")

	secondBtn, ok := tgram.LastSentKeyboard.Btns[1].(tg.Btn)
	r.True(ok)
	// The completed `- [x] `09:10` Done msg` entry is hidden, but the
	// remaining unchecked entry keeps its own stable hash (unchanged by
	// completion-toggle behaviour).
	r.Equal(tg.Cmd{Name: CmdComplete, Params: []string{chatBlockHash("- [ ] `09:05` New msg")}, Type: "cmd"}, secondBtn.Cmd)
	r.Contains(secondBtn.Name, "New msg")
}

func TestShowToday_TodayCommandModeJournal(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()
	cfg := fakeConfig()

	tgram = tg.NewFakeTG()

	err = cfg.SetMode(userconfig.ModeJournal)
	r.NoError(err)

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("home", nil)))
	r.NoError(err)

	r.Contains(tgram.LastSentText, "What's on your mind?")
}

func TestScheduleForTmrw(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() {
		now = savedNow
	}()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)

	tgram := tg.NewFakeTG()
	cfg := fakeConfig()

	bot := NewBot(-1, tgram, userFS, db.NewFakeDB(), cfg)
	err = bot.Reply(tg.NewUpd(-1, "Task for tomorrow"))
	r.NoError(err)

	err = bot.Reply(tg.NewUpdCmd(-1, tg.NewCmd("sc_tmrw", []string{inboxMsgHash(t, userFS, 0)})))
	r.NoError(err)

	laterMD, err := userFS.Read(fs.DirUserRoot, fs.LaterFilename)
	r.NoError(err)

	items, _ := txt.ChecklistItems(laterMD)
	r.Contains(items, "Task for tomorrow")

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Len(sc, 1)
	r.Equal("Task for tomorrow", sc[0].Filename)
	r.Equal(int64(86400), sc[0].ScheduledAt)
	r.Equal("", sc[0].Cron)
}
