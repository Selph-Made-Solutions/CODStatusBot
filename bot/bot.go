package bot

import (
	"errors"
	"strings"

	"github.com/bradselph/CODStatusBot/command"
	"github.com/bradselph/CODStatusBot/command/accountage"
	"github.com/bradselph/CODStatusBot/command/accountlogs"
	"github.com/bradselph/CODStatusBot/command/addaccount"
	"github.com/bradselph/CODStatusBot/command/checknow"
	"github.com/bradselph/CODStatusBot/command/feedback"
	"github.com/bradselph/CODStatusBot/command/globalannouncement"
	"github.com/bradselph/CODStatusBot/command/listaccounts"
	"github.com/bradselph/CODStatusBot/command/removeaccount"
	"github.com/bradselph/CODStatusBot/command/setcaptchaservice"
	"github.com/bradselph/CODStatusBot/command/setcheckinterval"
	"github.com/bradselph/CODStatusBot/command/setnotifications"
	"github.com/bradselph/CODStatusBot/command/togglecheck"
	"github.com/bradselph/CODStatusBot/command/updateaccount"
	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

const BotStatusMessage = "the Status of your Accounts so you dont have to."

var discord *discordgo.Session

func StartBot() (*discordgo.Session, error) {
	cfg := configuration.Get()
	if cfg.Discord.Token == "" {
		return nil, errors.New("discord token not configured")
	}

	var err error
	discord, err = discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, err
	}

	err = discord.Open()
	if err != nil {
		return nil, err
	}

	err = discord.UpdateWatchStatus(0, BotStatusMessage)
	if err != nil {
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
		case discordgo.InteractionWebhook:
			handleWebhook(s, i)
		}
	})

	return discord, nil
}

func handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.ModalSubmitData().CustomID
	switch {
	case strings.HasPrefix(customID, "set_notifications_modal_"):
		setnotifications.HandleModalSubmit(s, i)
	case customID == "set_captcha_service_modal" ||
		strings.HasPrefix(customID, "set_captcha_service_modal_capsolver") ||
		strings.HasPrefix(customID, "set_captcha_service_modal_ezcaptcha") ||
		strings.HasPrefix(customID, "set_captcha_service_modal_2captcha"):
		setcaptchaservice.HandleModalSubmit(s, i)
	case customID == "add_account_modal":
		addaccount.HandleModalSubmit(s, i)
	case strings.HasPrefix(customID, "update_account_modal_"):
		updateaccount.HandleModalSubmit(s, i)
	case customID == "set_check_interval_modal":
		setcheckinterval.HandleModalSubmit(s, i)
	case customID == "global_announcement_modal":
		globalannouncement.HandleModalSubmit(s, i)
	default:
		logger.Log.WithField("customID", customID).Error("Unknown modal submission")
	}
}

func handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	switch {
	case customID == "listaccounts":
		listaccounts.CommandListAccounts(s, i)
	case strings.HasPrefix(customID, "set_captcha_"):
		setcaptchaservice.HandleCaptchaServiceSelection(s, i)
	case strings.HasPrefix(customID, "feedback_"):
		feedback.HandleFeedbackChoice(s, i)
	case strings.HasPrefix(customID, "account_age_"):
		accountage.HandleAccountSelection(s, i)
	case strings.HasPrefix(customID, "account_logs_"):
		accountlogs.HandleAccountSelection(s, i)
	case customID == "account_logs_all":
		accountlogs.HandleAccountSelection(s, i)
	case strings.HasPrefix(customID, "update_account_"):
		updateaccount.HandleAccountSelection(s, i)
	case strings.HasPrefix(customID, "remove_account_"):
		removeaccount.HandleAccountSelection(s, i)
	case customID == "cancel_remove" || strings.HasPrefix(customID, "confirm_remove_"):
		removeaccount.HandleConfirmation(s, i)
	case strings.HasPrefix(customID, "check_now_"):
		checknow.HandleAccountSelection(s, i)
	case strings.HasPrefix(customID, "toggle_check_"):
		togglecheck.HandleAccountSelection(s, i)
	case strings.HasPrefix(customID, "confirm_reenable_") || customID == "cancel_reenable":
		togglecheck.HandleConfirmation(s, i)
	case customID == "show_interval_modal":
		setcheckinterval.HandleButton(s, i)
	default:
		logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
	}

}

func handleWebhook(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionApplicationCommand {
		command.HandleCommand(s, i)
	} else {
		switch i.Data.(type) {
		case *discordgo.ApplicationCommandInteractionData:
			if i.ApplicationCommandData().Name == "APPLICATION_AUTHORIZED" {
				installationType := i.ApplicationCommandData().Options[0].IntValue()
				userID := i.Member.User.ID
				guildID := ""
				if installationType == 0 {
					guildID = i.GuildID
				}

				settings := models.UserSettings{
					UserID:           userID,
					InstallationType: map[int64]string{0: "guild", 1: "user"}[installationType],
					GuildID:          guildID,
				}

				if err := database.DB.Create(&settings).Error; err != nil {
					logger.Log.WithError(err).Error("Failed to create user settings for new installation")
				}
			}
		}
	}
}
