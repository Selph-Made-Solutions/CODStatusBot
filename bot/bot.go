package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"context"
	"errors"
	"github.com/bwmarrin/discordgo"
	"os"
	"strings"
	"time"
)

var discord *discordgo.Session
var commandQueue chan *discordgo.InteractionCreate
var workerPool chan struct{}

const (
	maxQueueSize  = 1000
	maxWorkers    = 50
	workerTimeout = 30 * time.Second
)

func StartBot() (*discordgo.Session, error) {
	envToken := os.Getenv("DISCORD_TOKEN")
	if envToken == "" {
		err := errors.New("DISCORD_TOKEN environment variable not set")
		logger.Log.WithError(err).WithField("env", "DISCORD_TOKEN").Error()
		return nil, err
	}

	var err error
	discord, err = discordgo.New("Bot " + envToken)
	if err != nil {
		logger.Log.WithError(err).WithField("Bot startup", "Token").Error()
		return nil, err
	}

	err = discord.Open()
	if err != nil {
		logger.Log.WithError(err).WithField("Bot startup", "Opening Session").Error()
		return nil, err
	}

	err = discord.UpdateWatchStatus(0, "the Status of your Accounts so you don't have to.")
	if err != nil {
		logger.Log.WithError(err).WithField("Bot startup", "Setting Presence Status").Error()
		return nil, err
	}

	command.RegisterCommands(discord)
	logger.Log.Info("Registering global commands")

	// Initialize command queue and worker pool
	commandQueue = make(chan *discordgo.InteractionCreate, maxQueueSize)
	workerPool = make(chan struct{}, maxWorkers)

	// Start workers
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		select {
		case commandQueue <- i:
			logger.Log.Debugf("Command queued: %s", i.ApplicationCommandData().Name)
		default:
			logger.Log.Warn("Command queue is full, dropping command")
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "The bot is currently experiencing high load. Please try again later.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
		}
	})

	go services.CheckAccounts(discord)
	return discord, nil
}

func worker() {
	for i := range commandQueue {
		workerPool <- struct{}{}
		processCommand(i)
		<-workerPool
	}
}

func processCommand(i *discordgo.InteractionCreate) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Panic recovered in processCommand: %v", r)
		}
	}()

	var installType models.InstallationType
	if i.GuildID != "" {
		installType = models.InstallTypeGuild
	} else {
		installType = models.InstallTypeUser
	}

	ctx, cancel := context.WithTimeout(context.Background(), workerTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			command.HandleCommand(discord, i, installType)
		case discordgo.InteractionModalSubmit:
			handleModalSubmit(discord, i, installType)
		case discordgo.InteractionMessageComponent:
			handleMessageComponent(discord, i, installType)
		}
	}()

	select {
	case <-ctx.Done():
		logger.Log.Warnf("Command processing timed out: %s", i.ApplicationCommandData().Name)
		discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "The command processing timed out. Please try again later.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	case <-done:
		// Command processed successfully
	}
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	customID := i.ModalSubmitData().CustomID
	switch {
	case customID == "set_captcha_service_modal":
		command.Handlers["set_captcha_service_modal"](s, i, installType)
	case customID == "add_account_modal":
		command.Handlers["add_account_modal"](s, i, installType)
	case strings.HasPrefix(customID, "update_account_modal_"):
		command.Handlers["update_account_modal"](s, i, installType)
	case customID == "set_check_interval_modal":
		command.Handlers["set_check_interval_modal"](s, i, installType)
	default:
		logger.Log.WithField("customID", customID).Error("Unknown modal submission")
	}
}

func handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	customID := i.MessageComponentData().CustomID
	switch {
	case strings.HasPrefix(customID, "account_age_"):
		command.Handlers["account_age"](s, i, installType)
		logger.Log.Info("Handling account age selection")
	case strings.HasPrefix(customID, "account_logs_"):
		command.Handlers["account_logs"](s, i, installType)
		logger.Log.Info("Handling account logs selection")
	case customID == "account_logs_select":
		command.Handlers["account_logs"](s, i, installType)
		logger.Log.Info("Handling account logs selection")
	case strings.HasPrefix(customID, "update_account_"):
		command.Handlers["update_account"](s, i, installType)
		logger.Log.Info("Handling update account selection")
	case customID == "update_account_select":
		command.Handlers["update_account"](s, i, installType)
		logger.Log.Info("Handling update account selection")
	case strings.HasPrefix(customID, "remove_account_"):
		command.Handlers["remove_account"](s, i, installType)
		logger.Log.Info("Handling remove account selection")
	case customID == "cancel_remove" || strings.HasPrefix(customID, "confirm_remove_"):
		command.Handlers["remove_account"](s, i, installType)
		logger.Log.Info("Handling remove account confirmation")
	case strings.HasPrefix(customID, "check_now_"):
		command.Handlers["check_now"](s, i, installType)
		logger.Log.Info("Handling check now selection")
	case strings.HasPrefix(customID, "toggle_check_"):
		command.Handlers["toggle_check"](s, i, installType)
		logger.Log.Info("Handling toggle check selection")
	default:
		logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
	}
}

func init() {
	// Initialize the database connection
	err := database.Databaselogin()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize database connection")
	}

	// Create or update the UserSettings table
	err = database.DB.AutoMigrate(&models.UserSettings{})
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to create or update UserSettings table")
	}
}
