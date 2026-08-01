package server

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/zakirullin/files.md/server/fs"
	"github.com/zakirullin/files.md/server/pkg/tg"
	"github.com/zakirullin/files.md/server/pkg/txt"
	"github.com/zakirullin/files.md/server/userconfig"
)

func TestUcfirst(t *testing.T) {
	r := require.New(t)
	r.Equal("Abc", txt.Ucfirst("abc"))
}

func TestUcfirstRu(t *testing.T) {
	r := require.New(t)
	r.Equal("Абв", txt.Ucfirst("абв"))
}

func TestTomorrow(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 10, 45, 10, 0, time.UTC)
	}

	tomorrow := Tomorrow()
	r.Equal(time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC).Unix(), tomorrow)
}

func TestFormatTaskDate(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 10, 45, 10, 0, time.UTC)
	}

	r.Equal("Today", formatTaskDate(0))
	r.Equal("Today", formatTaskDate(10))         // seconds are ignored
	r.Equal("Today", formatTaskDate(60*60*2))    // hours are ignored
	r.Equal("Today", formatTaskDate(60*60*24-1)) // hours are ignored
	r.Equal("Tomorrow", formatTaskDate(60*60*24))
	r.Equal("Tomorrow", formatTaskDate(60*60*24))
	r.Equal("Saturday 03", formatTaskDate(60*60*24*2))
	r.Equal("Wednesday 07", formatTaskDate(60*60*24*6))
	r.Equal("Next Thursday 08", formatTaskDate(60*60*24*7))
}

func TestNextExcludeTodayClosestMonday(t *testing.T) {
	savedNow := now
	now = func() time.Time {
		return time.Date(2024, time.September, 22, 15, 30, 0, 0, time.UTC) // Sunday
	}
	defer func() { now = savedNow }()

	cronExpr := "0 0 * * 1"
	nextUnix := NextExcludeToday(cronExpr)

	expectedNext := time.Date(2024, time.September, 23, 0, 0, 0, 0, time.UTC)
	require.Equal(t, expectedNext.Unix(), nextUnix, "next scheduled time should be Monday 00:00 UTC")

	invalidCronExpr := "invalid cron expression"
	require.Panics(t, func() { NextExcludeToday(invalidCronExpr) }, "should panic for invalid cron expression")
}

func TestNextExcludeToday(t *testing.T) {
	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(2024, time.September, 23, 10, 0, 0, 0, time.UTC)
	}

	// Monday
	require.Equal(t, time.Date(2024, time.September, 30, 0, 0, 0, 0, time.UTC).Unix(), NextExcludeToday("0 0 * * 1"))
	// Wednesday
	require.Equal(t, time.Date(2024, time.September, 25, 0, 0, 0, 0, time.UTC).Unix(), NextExcludeToday("0 0 * * 3"))
	// Friday
	require.Equal(t, time.Date(2024, time.September, 27, 0, 0, 0, 0, time.UTC).Unix(), NextExcludeToday("0 0 * * 5"))
	// Sunday
	require.Equal(t, time.Date(2024, time.September, 29, 0, 0, 0, 0, time.UTC).Unix(), NextExcludeToday("0 0 * * 0"))
	// 1st of month
	require.Equal(t, time.Date(2024, time.October, 1, 0, 0, 0, 0, time.UTC).Unix(), NextExcludeToday("0 0 1 * *"))
	// 15th of month
	require.Equal(t, time.Date(2024, time.October, 15, 0, 0, 0, 0, time.UTC).Unix(), NextExcludeToday("0 0 15 * *"))
	// 31st of month
	require.Equal(t, time.Date(2024, time.October, 31, 0, 0, 0, 0, time.UTC).Unix(), NextExcludeToday("0 0 31 * *"))
}

func TestMoveDueTasksFromArchive(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC)
	}

	fsBackend := afero.NewMemMapFs()
	userFS, err := fs.NewFS("/-1", fsBackend)
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	_ = userFS.Write(fs.DirArchive, fs.DoneFilename, "- [ ] due task")

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	_ = cfg.CreateDefaultIfNotExists()
	_ = cfg.AddToSchedule("due task", 0, "")

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Equal("due task", sc[0].Filename)
	r.Equal(int64(0), sc[0].ScheduledAt)
	r.Equal("", sc[0].Cmd)
	r.Equal("", sc[0].Cron)

	tgram := tg.NewFakeTG()
	err = MoveDueTasks("/", "config.json", fsBackend, tgram)
	r.NoError(err)

	inboxMD, err := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	r.NoError(err)
	r.Contains(inboxMD, "due task")

	// Task should be removed from Done.md now that it has moved to inbox.
	doneMD, _ := userFS.Read(fs.DirArchive, fs.DoneFilename)
	r.NotContains(doneMD, "due task")

	sc, err = cfg.Schedules()
	r.NoError(err)
	r.Empty(sc)
}

func TestMoveDueTasksSkipsDuplicateAlreadyInInbox(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC)
	}

	fsBackend := afero.NewMemMapFs()
	userFS, err := fs.NewFS("/-1", fsBackend)
	r.NoError(err)
	r.NoError(userFS.CreateSystemDirs())

	// The recurring task is already sitting in the inbox, unhandled.
	_ = userFS.Write(fs.DirUserRoot, fs.ChatFilename, "- [ ] `09:00` daily task.md")

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	_ = cfg.CreateDefaultIfNotExists()
	_ = cfg.AddToSchedule("daily task.md", 0, "0 0 * * *") // due now, daily

	tgram := tg.NewFakeTG()
	r.NoError(MoveDueTasks("/", "config.json", fsBackend, tgram))

	// Not appended a second time.
	inboxMD, err := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	r.NoError(err)
	r.Equal(1, strings.Count(inboxMD, "daily task.md"))

	// Still rescheduled for the next day despite the skipped append.
	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Len(sc, 1)
	r.Greater(sc[0].ScheduledAt, int64(0))
}

func TestMoveDueTasksFromLater(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC)
	}

	fsBackend := afero.NewMemMapFs()
	userFS, err := fs.NewFS("/-1", fsBackend)
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	_ = userFS.Write(fs.DirUserRoot, fs.LaterFilename, "- [ ] due task")

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	_ = cfg.CreateDefaultIfNotExists()
	_ = cfg.AddToSchedule("due task", 0, "")

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Equal("due task", sc[0].Filename)

	tgram := tg.NewFakeTG()
	err = MoveDueTasks("/", "config.json", fsBackend, tgram)
	r.NoError(err)

	inboxMD, err := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
	r.NoError(err)
	r.Contains(inboxMD, "due task")

	laterMD, _ := userFS.Read(fs.DirUserRoot, fs.LaterFilename)
	r.NotContains(laterMD, "due task")

	sc, err = cfg.Schedules()
	r.NoError(err)
	r.Empty(sc)
}

func TestMoveDueTasksDoesntMove(t *testing.T) {
	r := require.New(t)

	savedNow := now
	defer func() { now = savedNow }()
	now = func() time.Time {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	fsBackend := afero.NewMemMapFs()
	userFS, err := fs.NewFS("/-1", fsBackend)
	r.NoError(err)
	err = userFS.CreateSystemDirs()
	r.NoError(err)
	err = userFS.Write("archive", "due task.md", "")
	r.NoError(err)

	cfg := userconfig.NewConfig(userFS, -1, "config.json")
	r.NoError(cfg.CreateDefaultIfNotExists())
	r.NoError(cfg.AddToSchedule("due task.md", 7*24*int64(time.Hour.Seconds())+1, ""))

	sc, err := cfg.Schedules()
	r.NoError(err)
	r.Equal("due task.md", sc[0].Filename)
	r.Equal(7*24*int64(time.Hour.Seconds())+1, sc[0].ScheduledAt)

	tgram := tg.NewFakeTG()
	err = MoveDueTasks("/", "config.json", fsBackend, tgram)
	r.NoError(err)

	exists, err := userFS.Exists("archive", "due task.md")
	r.NoError(err)
	r.True(exists)

	sc, err = cfg.Schedules()
	r.NoError(err)
	r.Len(sc, 1)
}

func TestRemoveCompletedInboxEntries(t *testing.T) {
	r := require.New(t)

	md := "#### 1 January, Thursday\n" +
		"- [ ] `09:00` open task\n" +
		"- [x] `09:05` done task\n" +
		"- [x] `09:10` multi-line done\n" +
		"continuation line\n" +
		"- [ ] `09:15` another open task\n"

	newMD, removedMD := removeCompletedInboxEntries(md)

	r.Contains(newMD, "- [ ] `09:00` open task")
	r.Contains(newMD, "- [ ] `09:15` another open task")
	r.NotContains(newMD, "done task")
	r.NotContains(newMD, "multi-line done")

	r.Contains(removedMD, "- [x] done task")
	r.Contains(removedMD, "- [x] multi-line done continuation line")
}
