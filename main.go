package main

import (
	"CODStatusBot/admin"
	"CODStatusBot/command"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"os"
	"os/signal"
	"syscall"
)

var discord *discordgo.Session

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

	err = database.Databaselogin()
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup", "Database login").Error()
		os.Exit(1)
	}

	err = initializeDatabase()
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup", "Database initialization").Error()
		os.Exit(1)
	}

	discord, err = startBot() // Start the Discord bot.
	if err != nil {
		logger.Log.WithError(err).WithField("Bot Startup", "Discord login").Error()
		os.Exit(1)
	}

	// Start the admin panel
	go admin.StartAdminPanel()

	logger.Log.Info("Bot is running")                                // Log that the bot is running.
	sc := make(chan os.Signal, 1)                                    // Set up a channel to receive system signals.
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt) // Notify the channel when a SIGINT, SIGTERM, or Interrupt signal is received.
	<-sc                                                             // Block until a signal is received.

	// Gracefully close the Discord session
	err = discord.Close()
	if err != nil {
		logger.Log.WithError(err).Error("Error closing Discord session")
	}
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
			logger.Log.Errorf("%s is not set in the environment", envVar)
			return fmt.Errorf("%s is not set in the environment", envVar)
		}
	}

	return nil
}

func initializeDatabase() error {
	err := database.DB.AutoMigrate(&models.Account{}, &models.Ban{}, &models.UserSettings{})
	if err != nil {
		return fmt.Errorf("failed to migrate database tables: %w", err)
	}

	return nil
}

func startBot() (*discordgo.Session, error) {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return nil, errors.New("DISCORD_TOKEN not set in environment variables")
	}

	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}

	discord.AddHandler(command.HandleCommand)

	err = command.RegisterCommands(discord)
	if err != nil {
		return nil, fmt.Errorf("error registering commands: %w", err)
	}

	err = discord.Open()
	if err != nil {
		return nil, fmt.Errorf("error opening connection: %w", err)
	}

	go services.CheckAccounts(discord)

	return discord, nil
}

func BoolPtr(b bool) *bool {
	return &b
}
