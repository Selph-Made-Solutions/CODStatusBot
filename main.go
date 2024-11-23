package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/bradselph/CODStatusBot/bot"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bradselph/CODStatusBot/services"
	"github.com/bwmarrin/discordgo"

	"github.com/joho/godotenv"
)

var discord *discordgo.Session

func main() {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Recovered from panic: %v\n%s", r, debug.Stack())
		}
	}()

	if services.IsDonationsEnabled() {
		logger.Log.Info("Donations system enabled")
	} else {
		logger.Log.Info("Donations system disabled")
	}

	if err := run(); err != nil {
		logger.Log.WithError(err).Error("Bot encountered an error and is shutting down")
		logger.Log.Fatal("Exiting due to error")
	}
}

func run() error {
	logger.Log.Info("Starting COD Status Bot...")

	if err := godotenv.Load(); err != nil {
		return fmt.Errorf("failed to load .env file: %w", err)
	}

	if err := services.InitializeConfig(); err != nil {
		return fmt.Errorf("failed to initialize configuration: %w", err)
	}

	if !services.IsServiceEnabled("ezcaptcha") && !services.IsServiceEnabled("2captcha") {
		logger.Log.Warn("Starting bot with no captcha services enabled - functionality will be limited")
	} else {
		var enabledServices []string
		if services.IsServiceEnabled("ezcaptcha") {
			enabledServices = append(enabledServices, "EZCaptcha")
			if services.VerifyEZCaptchaConfig() {
				logger.Log.Info("EZCaptcha service enabled and configured correctly")
			} else {
				logger.Log.Error("EZCaptcha service enabled but configuration is invalid")
			}
		}
		if services.IsServiceEnabled("2captcha") {
			enabledServices = append(enabledServices, "2Captcha")
			logger.Log.Info("2Captcha service enabled and configured correctly")
		}
		logger.Log.Infof("Enabled captcha services: %s", strings.Join(enabledServices, ", "))
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

func loadEnvironmentVariables() error {
	logger.Log.Info("Loading environment variables...")
	envPaths := []string{
		".env",
		"../.env",
		"../../.env",
		os.Getenv("ENV_FILE"),
	}

	var loaded bool
	var lastErr error

	for _, path := range envPaths {
		if err := godotenv.Load(path); err == nil {
			loaded = true
			logger.Log.Infof("Loaded environment from: %s", path)
			break
		} else {
			lastErr = err
			logger.Log.Debugf("Tried loading .env from %s: %v", path, err)
		}
	}

	if !loaded {
		logger.Log.WithError(lastErr).Error("Failed to load .env file from any location")
		return fmt.Errorf("failed to load .env file: %w", lastErr)
	}

	requiredEnvVars := []string{
		"DONATIONS_ENABLED",
		"BITCOIN_ADDRESS",
		"CASHAPP_ID",
		"DISCORD_TOKEN",
		"DEVELOPER_ID",
		"DB_USER",
		"DB_PASSWORD",
		"DB_NAME",
		"DB_HOST",
		"DB_PORT",
		"DB_VAR",
		"EZCAPTCHA_ENABLED",
		"TWOCAPTCHA_ENABLED",
		"MAX_RETRIES",
		"RECAPTCHA_SITE_KEY",
		"RECAPTCHA_URL",
		"SITE_ACTION",
		"EZAPPID",
		"EZCAPTCHA_CLIENT_KEY",
		"SOFT_ID",
		"COOLDOWN_DURATION",
		"CHECK_INTERVAL",
		"NOTIFICATION_INTERVAL",
		"SLEEP_DURATION",
		"COOKIE_CHECK_INTERVAL_PERMABAN",
		"STATUS_CHANGE_COOLDOWN",
		"GLOBAL_NOTIFICATION_COOLDOWN",
		"COOKIE_EXPIRATION_WARNING",
		"TEMP_BAN_UPDATE_INTERVAL",
		"CHECK_ENDPOINT",
		"PROFILE_ENDPOINT",
		"CHECK_VIP_ENDPOINT",
		"REDEEM_CODE_ENDPOINT",
		"SESSION_KEY",
		"STATIC_DIR",
		"ADMIN_PORT",
		"ADMIN_USERNAME",
		"ADMIN_PASSWORD",
		"CHECK_NOW_RATE_LIMIT",
		"DEFAULT_RATE_LIMIT",
		"DEFAULT_USER_MAXACCOUNTS",
		"PREM_USER_MAXACCOUNTS",
		"CHECKCIRCLE",
		"BANCIRCLE",
		"INFOCIRCLE",
		"STOPWATCH",
		"QUESTIONCIRCLE",
	}

	missingVars := []string{}
	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			missingVars = append(missingVars, envVar)
		}
	}

	if len(missingVars) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missingVars, ", "))
	}

	logger.Log.Info("Environment variables loaded successfully")
	logger.Log.Debugf("PROFILE_ENDPOINT: %s", os.Getenv("PROFILE_ENDPOINT"))
	logger.Log.Debugf("CHECK_ENDPOINT: %s", os.Getenv("CHECK_ENDPOINT"))
	logger.Log.Debugf("CHECK_VIP_ENDPOINT: %s", os.Getenv("CHECK_VIP_ENDPOINT"))

	return nil
}

func initializeDatabase() error {
	if err := database.DB.AutoMigrate(&models.Account{}, &models.Ban{}, &models.UserSettings{}); err != nil {
		return fmt.Errorf("failed to migrate database tables: %w", err)
	}

	var accounts []models.Account
	database.DB.Find(&accounts)
	for _, account := range accounts {
		if account.LastStatus == models.StatusShadowban {
			account.IsShadowbanned = true
			database.DB.Save(&account)
		}
	}
	return nil
}

func startPeriodicTasks(ctx context.Context, s *discordgo.Session) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				services.CheckAccounts(s)
				sleepDuration := time.Duration(services.GetEnvInt("SLEEP_DURATION", 3)) * time.Minute
				time.Sleep(sleepDuration)
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
						time.Duration(user.NotificationInterval)*time.Hour {
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
