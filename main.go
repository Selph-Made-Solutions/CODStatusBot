package main

import (
	"CODStatusBot/bot"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Recovered from panic: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	logger.Log.Info("Bot starting...")
	err := loadEnvironmentVariables()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to load environment variables")
	}

	err = initializeDatabase()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize database")
	}

	err = services.InitializeServices()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize services")
	}

	discord, err := bot.StartBot()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to start bot")
	}

	logger.Log.Info("Bot is running")                                // Log that the bot is running.
	sc := make(chan os.Signal, 1)                                    // Set up a channel to receive system signals.
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt) // Notify the channel when a SIGINT, SIGTERM, or Interrupt signal is received.
	<-sc                                                             // Block until a signal is received.

	logger.Log.Info("Shutting down...")
	err = discord.Close()
	if err != nil {
		logger.Log.WithError(err).Error("Error closing Discord session")
	}
}

// loadEnvironmentVariables loads environment variables from a .env file.
func loadEnvironmentVariables() error {
	logger.Log.Info("Loading environment variables...")
	err := godotenv.Load()
	if err != nil {
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
	}

	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			return fmt.Errorf("%s is not set in the environment", envVar)
		}
	}

	return nil
}

func initializeDatabase() error {
	err := database.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	err = database.DB.AutoMigrate(&models.Account{}, &models.Ban{}, &models.UserSettings{})
	if err != nil {
		return fmt.Errorf("failed to migrate database tables: %w", err)
	}

	return nil
}

// Helper function to create a pointer to a bool
func BoolPtr(b bool) *bool {
	return &b
}
