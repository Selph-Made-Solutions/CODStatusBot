package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/command/accountage"
	"CODStatusBot/command/accountlogs"
	"CODStatusBot/command/addaccount"
	"CODStatusBot/command/checknow"
	"CODStatusBot/command/feedback"
	"CODStatusBot/command/removeaccount"
	"CODStatusBot/command/setcaptchaservice"
	"CODStatusBot/command/setcheckinterval"
	"CODStatusBot/command/togglecheck"
	"CODStatusBot/command/updateaccount"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

var discord *discordgo.Session

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

	err = discord.UpdateWatchStatus(0, "the Status of your Accounts so you dont have to.")
	if err != nil {
		logger.Log.WithError(err).WithField("Bot startup", "Setting Presence Status").Error()
		return nil, err
	}

	command.RegisterCommands(discord)
	logger.Log.Info("Registering global commands")

	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			command.HandleCommand(s, i)
		case discordgo.InteractionModalSubmit:
			handleModalSubmit(s, i)
		case discordgo.InteractionMessageComponent:
			handleMessageComponent(s, i)
		}
	})

	// Add new event handler for GUILD_DELETE
	discord.AddHandler(handleGuildDelete)

	go services.CheckAccounts(discord)
	go periodicUserCheck(discord) // Start periodic check for user applications

	return discord, nil
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.ModalSubmitData().CustomID
	switch {
	case customID == "set_captcha_service_modal":
		setcaptchaservice.HandleModalSubmit(s, i)
	case customID == "add_account_modal":
		addaccount.HandleModalSubmit(s, i)
	case strings.HasPrefix(customID, "update_account_modal_"):
		updateaccount.HandleModalSubmit(s, i)
	case customID == "set_check_interval_modal":
		setcheckinterval.HandleModalSubmit(s, i)
	default:
		logger.Log.WithField("customID", customID).Error("Unknown modal submission")
	}
}

func handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	switch {
	case strings.HasPrefix(customID, "feedback_"):
		feedback.HandleFeedbackChoice(s, i)
		logger.Log.Info("Handling feedback choice")
	case strings.HasPrefix(customID, "account_age_"):
		accountage.HandleAccountSelection(s, i)
		logger.Log.Info("Handling account age selection")
	case strings.HasPrefix(customID, "account_logs_"):
		accountlogs.HandleAccountSelection(s, i)
		logger.Log.Info("Handling account logs selection")
	case customID == "account_logs_all":
		accountlogs.HandleAccountSelection(s, i)
		logger.Log.Info("Handling account logs selection")
	case strings.HasPrefix(customID, "update_account_"):
		updateaccount.HandleAccountSelection(s, i)
		logger.Log.Info("Handling update account selection")
	case strings.HasPrefix(customID, "remove_account_"):
		removeaccount.HandleAccountSelection(s, i)
		logger.Log.Info("Handling remove account selection")
	case customID == "cancel_remove" || strings.HasPrefix(customID, "confirm_remove_"):
		removeaccount.HandleConfirmation(s, i)
		logger.Log.Info("Handling remove account confirmation")
	case strings.HasPrefix(customID, "check_now_"):
		checknow.HandleAccountSelection(s, i)
		logger.Log.Info("Handling check now selection")
	case strings.HasPrefix(customID, "toggle_check_"):
		togglecheck.HandleAccountSelection(s, i)
		logger.Log.Info("Handling toggle check selection")
	default:
		logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
	}
}

func handleGuildDelete(s *discordgo.Session, g *discordgo.GuildDelete) {
	logger.Log.Infof("Bot removed from guild: %s", g.ID)

	// Update all UserSettings for this guild
	err := database.DB.Model(&models.UserSettings{}).
		Where("install_type = ? AND user_id IN (SELECT user_id FROM accounts WHERE channel_id LIKE ?)",
			"guild", g.ID+"%").
		Updates(map[string]interface{}{"is_bot_installed": false}).Error

	if err != nil {
		logger.Log.WithError(err).Error("Failed to update UserSettings after guild removal")
	}

	// Disable checks for all accounts in this guild
	err = database.DB.Model(&models.Account{}).
		Where("channel_id LIKE ?", g.ID+"%").
		Update("is_check_disabled", true).Error

	if err != nil {
		logger.Log.WithError(err).Error("Failed to disable checks for accounts after guild removal")
	}
}

func periodicUserCheck(s *discordgo.Session) {
	ticker := time.NewTicker(24 * time.Hour) // Check once a day
	for range ticker.C {
		var userSettings []models.UserSettings
		err := database.DB.Where("install_type = ? AND is_bot_installed = ?", "user", true).Find(&userSettings).Error
		if err != nil {
			logger.Log.WithError(err).Error("Failed to fetch user applications for periodic check")
			continue
		}

		for _, settings := range userSettings {
			_, err := s.UserChannelCreate(settings.UserID)
			if err != nil {
				logger.Log.WithError(err).Infof("Failed to create DM channel with user %s, marking as uninstalled", settings.UserID)
				settings.IsBotInstalled = false
				database.DB.Save(&settings)

				// Disable checks for all accounts of this user
				err = database.DB.Model(&models.Account{}).
					Where("user_id = ?", settings.UserID).
					Update("is_check_disabled", true).Error

				if err != nil {
					logger.Log.WithError(err).Error("Failed to disable checks for accounts after user removal")
				}
			}
		}
	}
}

func HandleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if the user has seen the announcement
	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	} else {
		logger.Log.Error("Interaction doesn't have Member or User")
		return
	}

	var userSettings models.UserSettings
	result := database.DB.Where(models.UserSettings{UserID: userID}).FirstOrCreate(&userSettings)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error getting user settings")
	} else {
		if !userSettings.IsBotInstalled {
			// Re-enable the bot for this user
			userSettings.IsBotInstalled = true
			if err := database.DB.Save(&userSettings).Error; err != nil {
				logger.Log.WithError(err).Error("Error updating user settings after re-enabling bot")
			}

			// Re-enable checks for all accounts of this user
			err := database.DB.Model(&models.Account{}).
				Where("user_id = ?", userID).
				Update("is_check_disabled", false).Error

			if err != nil {
				logger.Log.WithError(err).Error("Failed to re-enable checks for accounts after user interaction")
			}
		}

		if !userSettings.HasSeenAnnouncement {
			// Send the announcement to the user
			if err := services.SendGlobalAnnouncement(s, userID); err != nil {
				logger.Log.WithError(err).Error("Error sending announcement to user")
			}
		}
	}

	// Continue with regular command handling
	if h, ok := command.Handlers[i.ApplicationCommandData().Name]; ok {
		h(s, i)
	} else {
		logger.Log.Warnf("Unhandled command: %s", i.ApplicationCommandData().Name)
	}
}
