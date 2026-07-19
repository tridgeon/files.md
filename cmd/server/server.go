package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/spf13/afero"

	"github.com/zakirullin/files.md/server"
	"github.com/zakirullin/files.md/server/config"
	"github.com/zakirullin/files.md/server/db"
	"github.com/zakirullin/files.md/server/fs"
	"github.com/zakirullin/files.md/server/pkg/tg"
	"github.com/zakirullin/files.md/server/sync"
	"github.com/zakirullin/files.md/server/userconfig"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found, relying on process environment")
	}
	if err := config.LoadBotConfig(); err != nil {
		panic(fmt.Sprintf("Error loading cfg: %s\n", err))
	}
	// Save all renames and deletes to an append-only log.
	fs.LogRename = sync.LogRename
	fs.LogDelete = sync.LogDelete

	// Start the server and bot.
	token := sync.GenOneTimeToken(123456789) // TODO: replace with actual user ID
	onetimeURL := fmt.Sprintf("%s?token=%s", config.ServerCfg.AppURL, token)
	fmt.Println("One-time login URL:", onetimeURL)

	// Launch the HTTP server, it does two things:
	// - Serves the PWA app
	// - Provides the API for synchronization (if API_URL is not empty)
	go sync.Serve(
		config.ServerCfg.APIHost(),
		config.ServerCfg.AppHost(),
		config.ServerCfg.ServerCertDir,
		config.ServerCfg.ServerLogFile,
	)

	// Telegram bot is optional - server can run as web-only.
	api, err := tgbotapi.NewBotAPI(config.ServerCfg.BotAPIToken)
	if err != nil {
		fmt.Println("No Telegram bot token found, running web and api server only")
		select {} // block forever
	}
	telegram := tg.NewTG(api)

	// If today or inbox was changed in web app, we need to send the updated items to the bot.
	sync.OnChatUpdate = func(userID int64) { updateBotHome(telegram, userID) }

	// Due tasks scheduler
	ticker := time.NewTicker(5 * time.Second)
	quit := make(chan struct{})
	defer func(quit chan struct{}) {
		close(quit)
	}(quit)
	go func(tg *tg.TG) {
		fsBackend := afero.NewOsFs()
		for {
			select {
			case <-ticker.C:
				err := server.MoveDueTasks(config.ServerCfg.StorageDir, config.ServerCfg.ConfigFilename, fsBackend, telegram)
				if err != nil {
					fmt.Printf("Worker's error: %s\n", err)
				}

				err = server.RemoveCompletedChecklistItems(config.ServerCfg.StorageDir, config.ServerCfg.ConfigFilename, fsBackend)
				if err != nil {
					fmt.Printf("Worker's error: %s\n", err)
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}(telegram)

	infolog := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Main bot loop.
	// Loop through updates from Telegram and process them sequentially in separate per-user goroutine.
	userChannels := make(map[int64]chan tgbotapi.Update)
	tgConfig := tgbotapi.NewUpdate(0)
	tgConfig.Timeout = 60 // TODO release, check if it's enough
	updates := api.GetUpdatesChan(tgConfig)
	for update := range updates {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("Bot panic", "update", update, "err", r, "stacktrace", string(debug.Stack()))
				}
			}()

			updJSON, _ := json.Marshal(update)
			infolog.Info("Bot update", "update", string(updJSON))

			var userID int64
			upd := tg.NewTGUpd(update)
			channelID, channelIDExists := upd.ChannelID()
			if channelIDExists {
				userID, err = telegram.ChannelCreatorID(channelID)
				if err != nil {
					slog.Error("Bot error: can't get channel creator ID", "upd", string(updJSON), "err", err)
				}
			} else {
				userID = upd.UserID()
			}

			userCh, channelIDExists := userChannels[userID]
			if !channelIDExists {
				userCh = make(chan tgbotapi.Update, 100)
				userChannels[userID] = userCh
				go supervisor(userID, userCh, telegram)
			}

			userCh <- update
		}()
	}
}

// Runs per-user worker that listens for updates.
// Restarts infinitely upon panics.
func supervisor(userID int64, updates <-chan tgbotapi.Update, telegram *tg.TG) {
	for {
		func() {
			defer func() {
				if err := recover(); err != nil {
					slog.Error("Bot panic", "userID", userID, "err", err, "stacktrace", string(debug.Stack()))
				}
			}()
			processUserUpdates(userID, updates, telegram)
		}()
		time.Sleep(time.Second)
		slog.Info("Restarting worker", "userID", userID)
	}
}

func processUserUpdates(userID int64, updates <-chan tgbotapi.Update, telegram *tg.TG) {
	for update := range updates {
		upd := tg.NewTGUpd(update)

		bot, err := newBot(telegram, userID)
		if err != nil {
			slog.Error("Bot error: can't create bot", "err", err)
			return
		}

		if err := bot.Reply(upd); err != nil {
			if errors.Is(err, tg.ErrFileTooBig) {
				if _, sendErr := telegram.Send(userID, "This file is too big. Telegram allows bots to download files up to 20MB.", nil, ""); sendErr != nil {
					slog.Error("Bot error: can't send file-too-big notice", "err", sendErr)
				}
			}
			slog.Error("Bot error", "err", err)
		}
	}
}

func newBot(telegram *tg.TG, userID int64) (*server.Bot, error) {
	userFS, err := fs.NewUserFS(userID)
	if err != nil {
		return nil, fmt.Errorf("can't create fs: %w", err)
	}
	err = userFS.CreateSystemDirs()
	if err != nil {
		return nil, fmt.Errorf("can't create user dirs: %w", err)
	}

	confFilename := config.ServerCfg.ConfigFilename
	userconf := userconfig.NewConfig(userFS, userID, confFilename)
	err = userconf.CreateDefaultIfNotExists()
	if err != nil {
		return nil, fmt.Errorf("can't create default user config: %w", err)
	}

	bot := server.NewBot(userID, telegram, userFS, db.NewDB(userID), userconf)

	return bot, nil
}

func updateBotHome(telegram *tg.TG, userID int64) {
	bot, err := newBot(telegram, userID)
	if err != nil {
		slog.Error("Bot error: can't create bot", "err", err)
		return
	}

	err = bot.ShowHome(nil)
	if err != nil {
		slog.Error("Server error: can't update bot home", "userID", userID, "err", err)
	}
}
