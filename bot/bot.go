package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"errors"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var (
	discord *discordgo.Session
)

func StartBot() (*discordgo.Session, error) {
	envToken := os.Getenv("DISCORD_TOKEN")
	if envToken == "" {
		return nil, errors.New("DISCORD_TOKEN environment variable not set")
	}

	var err error
	discord, err = discordgo.New("Bot " + envToken)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create Discord session")
		return nil, err
	}

	discord.AddHandler(handleInteraction)
	discord.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	err = discord.Open()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to open Discord connection")
		return nil, err
	}

	err = discord.UpdateWatchStatus(0, "the Status of your Accounts so you don't have to.")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to set presence status")
		return nil, err
	}

	err = command.RegisterCommands(discord)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to register commands")
		return nil, err
	}

	go services.CheckAccounts(discord)

	return discord, nil
}

func handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	logger.Log.Infof("Received interaction: Type=%v, CustomID=%s", i.Type, i.MessageComponentData().CustomID)

	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Panic recovered in handleInteraction: %v", r)
			sendErrorResponse(s, i, "An unexpected error occurred. Please try again later.")
		}
	}()

	var installType models.InstallationType
	if i.GuildID != "" {
		installType = models.InstallTypeGuild
	} else {
		installType = models.InstallTypeUser
	}

	userID := getUserID(i)
	if userID == "" {
		logger.Log.Error("Failed to get user ID from interaction")
		sendErrorResponse(s, i, "An error occurred while processing your request.")
		return
	}

	err := checkAndSendAnnouncement(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error checking and sending announcement")
	}

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		logger.Log.Infof("Handling application command: %s", i.ApplicationCommandData().Name)
		command.HandleCommand(s, i, installType)
	case discordgo.InteractionModalSubmit:
		logger.Log.Infof("Handling modal submit: %s", i.ModalSubmitData().CustomID)
		handleModalSubmit(s, i, installType)
	case discordgo.InteractionMessageComponent:
		logger.Log.Infof("Handling message component: %s", i.MessageComponentData().CustomID)
		handleMessageComponent(s, i, installType)
	default:
		logger.Log.Warnf("Unknown interaction type: %v", i.Type)
		sendErrorResponse(s, i, "Unknown interaction type.")
	}
}

func checkAndSendAnnouncement(userID string) error {
	var userSettings models.UserSettings
	db := database.GetDB()
	err := db.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings).Error
	if err != nil {
		return err
	}

	if !userSettings.HasSeenAnnouncement {
		err = services.SendGlobalAnnouncement(discord, userID)
		if err != nil {
			return err
		}

		userSettings.HasSeenAnnouncement = true
		return db.Save(&userSettings).Error
	}

	return nil
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	customID := i.ModalSubmitData().CustomID
	logger.Log.Infof("Handling modal submit: %s", customID)

	var handler func(*discordgo.Session, *discordgo.InteractionCreate, models.InstallationType)

	switch {
	case customID == "set_captcha_service_modal":
		handler = command.Handlers["set_captcha_service_modal"]
	case customID == "add_account_modal":
		handler = command.Handlers["add_account_modal"]
	case strings.HasPrefix(customID, "update_account_modal_"):
		handler = command.Handlers["update_account_modal"]
	case customID == "set_check_interval_modal":
		handler = command.Handlers["set_check_interval_modal"]
	default:
		logger.Log.WithField("customID", customID).Error("Unknown modal submission")
		sendErrorResponse(s, i, "Unknown modal submission.")
		return
	}

	if handler == nil {
		logger.Log.Errorf("No handler found for modal: %s", customID)
		sendErrorResponse(s, i, "An error occurred while processing your request.")
		return
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Panic in modal handler for %s: %v", customID, r)
			sendErrorResponse(s, i, "An unexpected error occurred. Please try again later.")
		}
	}()

	handler(s, i, installType)
}

func handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	customID := i.MessageComponentData().CustomID
	logger.Log.Infof("Handling message component: %s", customID)

	var handler func(*discordgo.Session, *discordgo.InteractionCreate, models.InstallationType)

	switch {
	case strings.HasPrefix(customID, "account_age_"):
		logger.Log.Info("Handling account age selection")
		handler = command.Handlers["account_age"]
	case strings.HasPrefix(customID, "account_logs_"):
		logger.Log.Info("Handling account logs selection")
		handler = command.Handlers["account_logs"]
	case customID == "account_logs_select":
		logger.Log.Info("Handling selected account log")
		handler = command.Handlers["account_logs"]
	case strings.HasPrefix(customID, "update_account_"):
		logger.Log.Info("Handling update account selection")
		handler = command.Handlers["update_account"]
	case customID == "update_account_select":
		logger.Log.Info("Handling update account selection")
		handler = command.Handlers["update_account"]
	case strings.HasPrefix(customID, "remove_account_"):
		logger.Log.Info("Handling remove account selection")
		handler = command.Handlers["remove_account"]
	case customID == "cancel_remove" || strings.HasPrefix(customID, "confirm_remove_"):
		logger.Log.Info("Handling remove account confirmation")
		handler = command.Handlers["remove_account"]
	case strings.HasPrefix(customID, "check_now_"):
		logger.Log.Info("Handling check now selection")
		handler = command.Handlers["check_now"]
	case strings.HasPrefix(customID, "toggle_check_"):
		logger.Log.Info("Handling toggle check selection")
		handler = command.Handlers["toggle_check"]
	default:
		logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
		sendErrorResponse(s, i, "Unknown message component interaction.")
		return
	}

	if handler == nil {
		logger.Log.Errorf("No handler found for component: %s", customID)
		sendErrorResponse(s, i, "An error occurred while processing your request.")
		return
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Log.Errorf("Panic in component handler for %s: %v", customID, r)
			sendErrorResponse(s, i, "An unexpected error occurred. Please try again later.")
		}
	}()

	handler(s, i, installType)
}

func sendErrorResponse(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send error response")
	}
}

func getUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		return i.Member.User.ID
	} else if i.User != nil {
		return i.User.ID
	}
	return ""
}

func init() {
	// Initialize the database connection
	err := database.Databaselogin()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to initialize database connection")
	}

	// Create or update the UserSettings table
	err = database.GetDB().AutoMigrate(&models.UserSettings{})
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to create or update UserSettings table")
	}
}
