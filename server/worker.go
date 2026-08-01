package server

import (
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/spf13/afero"
	"golang.org/x/exp/slog"

	"github.com/zakirullin/files.md/server/db"
	"github.com/zakirullin/files.md/server/fs"
	"github.com/zakirullin/files.md/server/journal"
	"github.com/zakirullin/files.md/server/pkg/txt"
	"github.com/zakirullin/files.md/server/userconfig"
)

// alreadyRemoved tracks which users had their completed checklist items
// cleaned up today. Key is userID#yyyy-mm-dd.
var alreadyRemoved = make(map[string]bool)

func BeginningOfTheDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func Tomorrow() int64 {
	tomorrow := now().AddDate(0, 0, 1)
	return BeginningOfTheDay(tomorrow).Unix()
}

// NextExcludeToday returns the next unix time for a cron expression, skipping today.
func NextExcludeToday(crn string) int64 {
	sched, err := cron.ParseStandard(crn)
	if err != nil {
		// Cron expressions come from our code, not user input.
		panic(fmt.Errorf("invalid cron expression %s: %w", crn, err))
	}

	endOfDay := now().Truncate(24 * time.Hour).Add(24*time.Hour - time.Nanosecond)
	// TODO release take into account user timezone
	return sched.Next(endOfDay).Unix()
}

func ScheduleReport(scheduledTasks []userconfig.Schedule) string {
	schedule := make(map[string][]string)
	order := []string{}

	addToSchedule := func(day string, task string) {
		if _, exists := schedule[day]; !exists {
			order = append(order, day)
		}
		schedule[day] = append(schedule[day], task)
	}
	for _, task := range scheduledTasks {
		addToSchedule(formatTaskDate(task.ScheduledAt), fs.DisplayName(task.Filename))
	}

	var report string
	for _, day := range order {
		report += fmt.Sprintf("<b>%s</b>\n", day)
		for _, task := range schedule[day] {
			report += fmt.Sprintf("- %s\n", task)
		}
		report += "\n"
	}

	return strings.TrimSpace(report)
}

func formatTaskDate(scheduledAt int64) string {
	today := now().Truncate(24 * time.Hour)
	taskDate := time.Unix(scheduledAt, 0).In(now().Location()).Truncate(24 * time.Hour)

	diffDays := int(taskDate.Sub(today).Hours() / 24)

	switch {
	case diffDays == 0:
		return "Today"
	case diffDays == 1:
		return "Tomorrow"
	case diffDays > 1 && diffDays <= 6:
		return taskDate.Format("Monday 02")
	case diffDays >= 7 && diffDays <= 13:
		return "Next " + taskDate.Format("Monday 02")
	default:
		return taskDate.Format("02 January, Monday")
	}
}

// MoveDueTasks moves due scheduled tasks into the user's inbox.
func MoveDueTasks(
	storagePath,
	configFilename string,
	fsBackend afero.Fs,
	telegram Chat,
) error {
	infolog := slog.New(slog.NewTextHandler(os.Stdout, nil))

	rootFS, err := fs.NewFS(storagePath, fsBackend)
	if err != nil {
		return fmt.Errorf("schedule worker: can't create FS: %s", err)
	}

	userDirs, err := rootFS.FilesAndDirs(fs.DirUserRoot)
	if err != nil {
		return fmt.Errorf("schedule worker: %w", err)
	}

	for _, userDir := range userDirs {
		userID, err := strconv.ParseInt(userDir.Name, 10, 64)
		if err != nil {
			slog.Error("schedule worker: can't parse user ID", "dir", userDir.Name, "err", err)
			continue
		}
		userPath := path.Join(storagePath, txt.I64(userID))
		userFS, err := fs.NewFS(userPath, fsBackend)
		if err != nil {
			slog.Error("schedule worker: can't create user FS", "err", err)
			continue
		}

		userconf := userconfig.NewConfig(userFS, userID, configFilename)

		schedules, err := userconf.Schedules()
		if err != nil {
			slog.Error("schedule worker: can't get schedules", "err", err)
			continue
		}
		for _, schedule := range schedules {
			secondsLeft := schedule.ScheduledAt - now().Unix()
			if secondsLeft > 0 {
				continue
			}

			// Skip the append if an identical incomplete entry is already in
			// the inbox (a recurring task the user hasn't dealt with yet), so a
			// daily schedule doesn't pile up duplicates. Mirrors the dedup in
			// txt.AddChecklistItem. The reschedule below still runs.
			chatMD, _ := userFS.Read(fs.DirUserRoot, fs.ChatFilename)
			if !txt.ContainsIncompleteChecklistItem(chatMD, schedule.Filename) {
				bot := NewBot(userID, telegram, userFS, db.NewDB(userID), userconf)
				if _, err := bot.appendToChat(schedule.Filename, userconf.Timezone()); err != nil {
					slog.Error("schedule worker: can't append to inbox", "err", err)
					continue
				}
				// Remove the task from Done.md / Later.md so it doesn't linger as a duplicate.
				if doneMD, rerr := userFS.Read(fs.DirArchive, fs.DoneFilename); rerr == nil {
					if reduced, _ := txt.RemoveChecklistItem(doneMD, schedule.Filename); reduced != doneMD {
						_ = userFS.Write(fs.DirArchive, fs.DoneFilename, reduced)
					}
				}
				if laterMD, rerr := userFS.Read(fs.DirUserRoot, fs.LaterFilename); rerr == nil {
					if reduced, _ := txt.RemoveChecklistItem(laterMD, schedule.Filename); reduced != laterMD {
						_ = userFS.Write(fs.DirUserRoot, fs.LaterFilename, reduced)
					}
				}

				infolog.Info("scheduled task moved to inbox", schedule.Filename, "filename")

				_ = bot.ShowHome(nil)
			}

			if len(schedule.Cron) != 0 {
				nextScheduledAt := NextExcludeToday(schedule.Cron)
				err = userconf.AddToSchedule(schedule.Filename, nextScheduledAt, schedule.Cron)
				if err != nil {
					slog.Error("schedule worker: can't add to schedule", "err", err)
					continue
				}
				infolog.Info("task was rescheduled", "filename", schedule.Filename, "schedule", schedule.Cron, "scheduledAt", nextScheduledAt)
				continue
			}

			err = userconf.DelFromSchedule(schedule.Filename)
			if err != nil {
				slog.Error("schedule worker: can't delete from schedule", "err", err)
				continue
			}
		}
	}

	return nil
}

// RemoveCompletedChecklistItems runs nightly (23:50 user-local) to sweep
// `- [x]` items out of Chat.md, Later.md and Inbox.md, archive them to
// Done.md, and append each to the user's journal.
func RemoveCompletedChecklistItems(
	storagePath,
	configFilename string,
	fsBackend afero.Fs,
) error {
	rootFS, err := fs.NewFS(storagePath, fsBackend)
	if err != nil {
		return fmt.Errorf("schedule worker: can't create FS: %s", err)
	}

	userDirs, err := rootFS.FilesAndDirs(fs.DirUserRoot)
	if err != nil {
		return fmt.Errorf("schedule worker: %w", err)
	}

	for _, userDir := range userDirs {
		userID, err := strconv.ParseInt(userDir.Name, 10, 64)
		if err != nil {
			slog.Error("schedule worker: can't parse user ID", "dir", userDir.Name, "err", err)
			continue
		}
		userPath := path.Join(storagePath, txt.I64(userID))
		userFS, err := fs.NewFS(userPath, fsBackend)
		if err != nil {
			slog.Error("schedule worker: can't create user FS", "err", err)
			continue
		}

		if alreadyRemoved[txt.I64(userID)+"#"+now().Format("2006-01-02")] {
			continue
		}

		userconf := userconfig.NewConfig(userFS, userID, configFilename)
		tz := userconf.Timezone()
		if now().In(tz).Hour() != 23 || now().In(tz).Minute() < 50 {
			continue
		}

		type target struct {
			filename string
			reducer  func(string) (string, string)
		}
		targets := []target{
			{fs.ChatFilename, txt.RemoveCompletedChecklistItems},
			{fs.LaterFilename, txt.RemoveCompletedChecklistItems},
			{fs.ChatFilename, removeCompletedInboxEntries},
		}

		for _, t := range targets {
			md, err := userFS.Read(fs.DirUserRoot, t.filename)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				slog.Error("schedule worker: can't read file", "file", t.filename, "err", err)
				continue
			}

			reducedMD, removedMD := t.reducer(md)
			if removedMD == "" {
				continue
			}

			err = userFS.Write(fs.DirUserRoot, t.filename, reducedMD)
			if err != nil {
				slog.Error("schedule worker: can't write file", "file", t.filename, "err", err)
				continue
			}

			doneMD, err := userFS.Read(fs.DirArchive, fs.DoneFilename)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Error("schedule worker: can't read done file", "err", err)
				continue
			}
			header := fmt.Sprintf("#### %d %s %d, %s", now().Day(), now().Format("January"), now().Year(), now().Weekday())
			doneMD = txt.AddHeaderAndText(doneMD, header, removedMD)

			err = userFS.Write(fs.DirArchive, fs.DoneFilename, doneMD)
			if err != nil {
				slog.Error("schedule worker: can't write done file", "err", err)
			}

			tasks, _ := txt.ChecklistItems(removedMD)
			for _, task := range tasks {
				// Strip any leading `HH:MM` so AddRecord's own prepended
				// timestamp isn't duplicated after the checkmark.
				_ = journal.AddRecord(userFS, fmt.Sprintf("✅ %s", txt.StripChatTimestamp(task)), userconf.Timezone())
			}
		}

		alreadyRemoved[txt.I64(userID)+"#"+now().Format("2006-01-02")] = true
	}

	return nil
}

// removeCompletedInboxEntries strips whole Inbox.md blocks whose first line
// begins with `- [x] `/`- [X] `. Returns the surviving content and a markdown
// string of the removed items formatted as `- [x] <inbox-content>` so the
// journal/archive flow can reuse txt.ChecklistItems on it.
func removeCompletedInboxEntries(md string) (string, string) {
	blocks := readChatMsgs(md)

	doneMarker := regexp.MustCompile(`^- \[[xX]\] `)
	tsRegex := regexp.MustCompile(`^(?:- \[[ xX]\] )?` + "`" + `\d{2}:\d{2}` + "`" + ` `)

	var kept []string
	var removed strings.Builder

	for _, block := range blocks {
		firstLine := block
		if nl := strings.Index(block, "\n"); nl != -1 {
			firstLine = block[:nl]
		}
		if !doneMarker.MatchString(firstLine) {
			kept = append(kept, block)
			continue
		}
		// Strip the optional checkbox + timestamp prefix so the archived item
		// reads like a checklist task, not an inbox entry.
		body := tsRegex.ReplaceAllString(block, "")
		// Flatten continuation lines into spaces — AddChecklistItem does the
		// same for multi-line items.
		body = strings.ReplaceAll(body, "\n", " ")
		removed.WriteString("- [x] ")
		removed.WriteString(body)
		removed.WriteString("\n")
	}

	newMD := strings.TrimSpace(strings.Join(kept, "\n"))
	return newMD, removed.String()
}
