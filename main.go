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
	"github.com/bradselph/CODStatusBot/webserver"

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

	if err := run(); err != nil {
		logger.Log.WithError(err).Error("Bot encountered an error and is shutting down")
		logger.Log.Fatal("Exiting due to error")
	}
}

func run() error {
	logger.Log.Info("Starting COD Status Bot...")

	if err := loadEnvironmentVariables(); err != nil {
		return fmt.Errorf("failed to load environment variables: %w", err)
	}
	logger.Log.Info("Environment variables loaded successfully")

	if !services.IsServiceEnabled("ezcaptcha") && !services.IsServiceEnabled("2captcha") {
		logger.Log.Warn("Starting bot with no captcha services enabled - functionality will be limited")
	} else {
		var enabledServices []string
		if services.IsServiceEnabled("ezcaptcha") {
			enabledServices = append(enabledServices, "EZCaptcha")
			logger.Log.Info("EZCaptcha service enabled")
		}
		if services.IsServiceEnabled("2captcha") {
			enabledServices = append(enabledServices, "2Captcha")
			logger.Log.Info("2Captcha service enabled")
		}
		logger.Log.Infof("Enabled captcha services: %s", strings.Join(enabledServices, ", "))
	}

	if err := services.LoadEnvironmentVariables(); err != nil {
		return fmt.Errorf("failed to initialize EZ-Captcha service: %w", err)
	}
	logger.Log.Info("EZ-Captcha service initialized successfully")

	if err := database.Databaselogin(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	logger.Log.Info("Database connection established successfully")

	if err := initializeDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	logger.Log.Info("Database initialized successfully")

	server := webserver.StartAdminDashboard()

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Log.WithError(err).Error("Error shutting down webpage server")
	}

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
	if err := godotenv.Load(); err != nil {
		logger.Log.WithError(err).Error("Error loading .env file")
		return fmt.Errorf("error loading .env file: %w", err)
	}

	requiredEnvVars := []string{
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
	}

	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			return fmt.Errorf("%s is not set in the environment", envVar)
		}
	}

	return nil
}

func initializeDatabase() error {
	if err := database.DB.AutoMigrate(&models.Account{}, &models.CaptchaBalance{}, &models.Ban{}, &models.UserSettings{}); err != nil {
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

	go webserver.StartStatsCaching()
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
