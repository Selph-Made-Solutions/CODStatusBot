package main

import (
	"CODStatusBot/admin"
	"CODStatusBot/command"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"os"
	"os/signal"
	"syscall"
)

var discord *discordgo.Session

func main() {
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

	if err := services.LoadEnvironmentVariables(); err != nil {
		return fmt.Errorf("failed to initialize EZ-Captcha service: %w", err)
	}

	if err := database.Databaselogin(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := initializeDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	var err error
	discord, err = startBot()
	if err != nil {
		return fmt.Errorf("failed to start Discord bot: %w", err)
	}

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

func startBot() (*discordgo.Session, error) {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN not set in environment variables")
	}

	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}

	discord.AddHandler(command.HandleCommand)

	if err := command.RegisterCommands(discord); err != nil {
		return nil, fmt.Errorf("error registering commands: %w", err)
	}

	if err := discord.Open(); err != nil {
		return nil, fmt.Errorf("error opening connection: %w", err)
	}

	go services.CheckAccounts(discord)

	return discord, nil
}

func BoolPtr(b bool) *bool {
	return &b
}
