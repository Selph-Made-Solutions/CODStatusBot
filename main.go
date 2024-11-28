package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/bradselph/CODStatusBot/bot"
	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bradselph/CODStatusBot/services"
	"github.com/bwmarrin/discordgo"
)

var discord *discordgo.Session

func main() {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Recovered from panic: %v\n%s", r, debug.Stack())
		}
	}()

	if err := run(); err != nil {
		logger.Log.WithError(err).Error("Bot encountered an error and is shutting down")
		logger.Log.Fatal("Exiting due to error")
	}
}

func run() error {
	logger.Log.Info("Starting COD Status Bot...")

	if err := configuration.Load(); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	cfg := configuration.Get()

	if !cfg.CaptchaService.EZCaptcha.Enabled && !cfg.CaptchaService.TwoCaptcha.Enabled {
		logger.Log.Warn("Starting bot with no captcha services enabled - functionality will be limited")
	} else {
		var enabledServices []string
		if cfg.CaptchaService.EZCaptcha.Enabled {
			enabledServices = append(enabledServices, "EZCaptcha")
			if services.VerifyEZCaptchaConfig() {
				logger.Log.Info("EZCaptcha service enabled and configured correctly")
			} else {
				logger.Log.Error("EZCaptcha service enabled but configuration is invalid")
			}
		}
		if cfg.CaptchaService.TwoCaptcha.Enabled {
			enabledServices = append(enabledServices, "2Captcha")
			logger.Log.Info("2Captcha service enabled and configured correctly")
		}
		logger.Log.Infof("Enabled captcha services: %v", enabledServices)
	}

	if err := database.Databaselogin(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	logger.Log.Info("Database connection established successfully")

	var err error
	discord, err = bot.StartBot()
	if err != nil {
		return fmt.Errorf("failed to start Discord bot: %w", err)
	}
	logger.Log.Info("Discord bot started successfully")

	periodicTasksCtx, cancelPeriodicTasks := context.WithCancel(context.Background())
	go startPeriodicTasks(periodicTasksCtx, discord)

	logger.Log.Info("COD Status Bot startup complete")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Log.Info("Shutting down COD Status Bot...")

	cancelPeriodicTasks()

	if err := discord.Close(); err != nil {
		logger.Log.WithError(err).Error("Error closing Discord session")
	}

	if err := database.CloseConnection(); err != nil {
		logger.Log.WithError(err).Error("Error closing database connection")
	}

	logger.Log.Info("Shutdown complete")
	return nil
}

func startPeriodicTasks(ctx context.Context, s *discordgo.Session) {
	cfg := configuration.Get()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				services.CheckAccounts(s)
				time.Sleep(time.Duration(cfg.Intervals.Sleep) * time.Minute)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				var users []models.UserSettings
				if err := database.DB.Find(&users).Error; err != nil {
					logger.Log.WithError(err).Error("Failed to fetch users for consolidated updates")
					time.Sleep(time.Hour)
					continue
				}

				for _, user := range users {
					var accounts []models.Account
					if err := database.DB.Where("user_id = ? AND is_check_disabled = ? AND is_expired_cookie = ?",
						user.UserID, false, false).Find(&accounts).Error; err != nil {
						logger.Log.WithError(err).Error("Failed to fetch accounts for user")
						continue
					}

					if time.Since(user.LastDailyUpdateNotification) >=
						time.Duration(cfg.Intervals.Notification)*time.Hour {
						services.SendConsolidatedDailyUpdate(s, user.UserID, user, accounts)
					}
				}

				time.Sleep(time.Hour)
			}
		}
	}()

	go services.ScheduleBalanceChecks(s)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := services.SendAnnouncementToAllUsers(s); err != nil {
					logger.Log.WithError(err).Error("Failed to send global announcement")
				}
				time.Sleep(24 * time.Hour)
			}
		}
	}()

	logger.Log.Info("Periodic tasks started successfully")
}
