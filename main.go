package main

import (
	"CODStatusBot/bot"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/services"
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger.Log.Info("Bot starting...") // Log that the bot is starting up.
	err := loadEnvironmentVariables()  // Load environment variables from .env file.
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup", "Environment Variables").Error()
		os.Exit(1)
	}

	err = services.LoadEnvironmentVariables() // Initialize EZ-Captcha service
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup", "EZ-Captcha Initialization").Error()
		os.Exit(1)
	}

	err = services.LoadTwoCaptchaEnvironmentVariables()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to load 2captcha environment variables")
		os.Exit(1)
	}

	err = database.Databaselogin()
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup", "Database login").Error()
		os.Exit(1)
	}

	err = bot.StartBot() // Start the Discord bot.

	if err != nil {

		logger.Log.WithError(err).WithField("Bot Startup", "Discord login").Error()
		os.Exit(1)
	}

	logger.Log.Info("Bot is running")                                // Log that the bot is running.
	sc := make(chan os.Signal, 1)                                    // Set up a channel to receive system signals.
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt) // Notify the channel when a SIGINT, SIGTERM, or Interrupt signal is received.
	<-sc                                                             // Block until a signal is received.
}

// loadEnvironmentVariables loads environment variables from a .env file.
func loadEnvironmentVariables() error {
	logger.Log.Info("Loading environment variables...") // Log that environment variables are being loaded.
	err := godotenv.Load()                              // Load environment variables from .env file.
	if err != nil {
		logger.Log.WithError(err).Error("Error loading .env file")
		return fmt.Errorf("error loading .env file: %w", err)
	}

	requiredEnvVars := []string{
		"DISCORD_TOKEN",
		"EZCAPTCHA_CLIENT_KEY",
		"RECAPTCHA_SITE_KEY",
		"RECAPTCHA_URL",
		// "TWOCAPAPPID",
		"TWOCAPTCHA_API_KEY",
	}

	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			logger.Log.Errorf("%s is not set in the environment", envVar)
			return fmt.Errorf("%s is not set in the environment", envVar)
		}
	}

	return nil
}
