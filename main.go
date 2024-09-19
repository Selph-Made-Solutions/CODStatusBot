package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"CODStatusBot/admin"
	"CODStatusBot/bot"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"

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

	logger.Log.Info("Bot starting...")
	if err := run(); err != nil {
		logger.Log.WithError(err).Error("Bot encountered an error and is shutting down")
		os.Exit(1)
	}
}

func run() error {
	if err := loadEnvironmentVariables(); err != nil {
		return fmt.Errorf("failed to load environment variables: %w", err)
	}
	logger.Log.Info("Environment variables loaded successfully")

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

	var err error
	discord, err = bot.StartBot()
	if err != nil {
		return fmt.Errorf("failed to start Discord bot: %w", err)
	}
	logger.Log.Info("Discord bot started successfully")

	// Start the admin panel
	go admin.StartAdminPanel()

	logger.Log.Info("Bot is running")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Gracefully close the Discord session
	if err := discord.Close(); err != nil {
		logger.Log.WithError(err).Error("Error closing Discord session")
	}

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
		"EZCAPTCHA_CLIENT_KEY",
		"RECAPTCHA_SITE_KEY",
		"RECAPTCHA_URL",
		"DB_USER",
		"DB_PASSWORD",
		"DB_HOST",
		"DB_PORT",
		"DB_NAME",
		"DB_VAR",
		"DEVELOPER_ID",
	}

	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			return fmt.Errorf("%s is not set in the environment", envVar)
		}
	}

	return nil
}

func initializeDatabase() error {
	if err := database.DB.AutoMigrate(&models.Account{}, &models.Ban{}, &models.UserSettings{}); err != nil {
		return fmt.Errorf("failed to migrate database tables: %w", err)
	}
	return nil
}

func startCleanupTask() {
	ticker := time.NewTicker(24 * time.Hour) // Run cleanup daily
	go func() {
		for range ticker.C {
			cleanupDisabledAccounts()
		}
	}()
}
