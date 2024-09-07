package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"errors"
	"github.com/bwmarrin/discordgo"
	"os"
	"runtime/debug"
	"strings"
)

var discord *discordgo.Session

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

	discord.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		logger.Log.Infof("Bot is now running. Press CTRL-C to exit.")
		err = s.UpdateGameStatus(0, "Type /help for commands")
		if err != nil {
			logger.Log.WithError(err).Error("Error updating game status")
		}
	})

	err = discord.Open()
	if err != nil {
		logger.Log.WithError(err).WithField("Bot startup", "Opening Session").Error()
		return nil, err
	}

	command.RegisterCommands(discord)
	logger.Log.Info("Registering global commands")

	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		defer func() {
			if r := recover(); r != nil {
				logger.Log.Errorf("Recovered from panic in interaction handler: %v\nStack trace: %s", r, debug.Stack())
				respondToInteraction(s, i, "An unexpected error occurred. Please try again later.")
			}
		}()

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			command.HandleCommand(s, i)
		case discordgo.InteractionModalSubmit:
			handleModalSubmit(s, i)
		case discordgo.InteractionMessageComponent:
			handleMessageComponent(s, i)
		}
	})

	go services.CheckAccounts(discord)
	return discord, nil
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.ModalSubmitData().CustomID == "" {
		logger.Log.Error("Received modal submit interaction with empty CustomID")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	customID := i.ModalSubmitData().CustomID
	var handler func(*discordgo.Session, *discordgo.InteractionCreate)
	var ok bool

	switch {
	case customID == "set_captcha_service_modal":
		handler, ok = command.Handlers["setcaptchaservice_modal"]
	case customID == "add_account_modal":
		handler, ok = command.Handlers["addaccount_modal"]
	case strings.HasPrefix(customID, "update_account_modal_"):
		handler, ok = command.Handlers["updateaccount_modal"]
	case customID == "set_check_interval_modal":
		handler, ok = command.Handlers["setcheckinterval_modal"]
	default:
		logger.Log.WithField("customID", customID).Error("Unknown modal submission")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	if !ok || handler == nil {
		logger.Log.WithField("customID", customID).Error("Handler not found for modal submission")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	handler(s, i)
}

func handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.MessageComponentData().CustomID == "" {
		logger.Log.Error("Received message component interaction with empty CustomID")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	customID := i.MessageComponentData().CustomID
	var handler func(*discordgo.Session, *discordgo.InteractionCreate)
	var ok bool

	switch {
	case strings.HasPrefix(customID, "account_age_"):
		handler, ok = command.Handlers["accountage_select"]
		logger.Log.Info("Handling account age selection")
	case strings.HasPrefix(customID, "account_logs_"):
		handler, ok = command.Handlers["accountlogs_select"]
		logger.Log.Info("Handling account logs selection")
	case customID == "account_logs_select":
		handler, ok = command.Handlers["accountlogs_select"]
		logger.Log.Info("Handling account logs selection")
	case strings.HasPrefix(customID, "update_account_"):
		handler, ok = command.Handlers["updateaccount_select"]
		logger.Log.Info("Handling update account selection")
	case customID == "update_account_select":
		handler, ok = command.Handlers["updateaccount_select"]
		logger.Log.Info("Handling update account selection")
	case strings.HasPrefix(customID, "remove_account_"):
		handler, ok = command.Handlers["removeaccount_select"]
		logger.Log.Info("Handling remove account selection")
	case customID == "cancel_remove" || strings.HasPrefix(customID, "confirm_remove_"):
		handler, ok = command.Handlers["removeaccount_confirm"]
		logger.Log.Info("Handling remove account confirmation")
	case strings.HasPrefix(customID, "check_now_"):
		handler, ok = command.Handlers["checknow_select"]
		logger.Log.Info("Handling check now selection")
	case strings.HasPrefix(customID, "toggle_check_"):
		handler, ok = command.Handlers["togglecheck_select"]
		logger.Log.Info("Handling toggle check selection")
	default:
		logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	if !ok || handler == nil {
		logger.Log.WithField("customID", customID).Error("Handler not found for message component interaction")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	handler(s, i)
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}

func init() {
	err := database.Databaselogin()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize database connection")
	}

	err = database.DB.AutoMigrate(&models.UserSettings{})
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to create or update UserSettings table")
	}
}
