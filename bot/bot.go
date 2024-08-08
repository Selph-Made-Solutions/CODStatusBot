package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/command/accountage"
	"CODStatusBot/command/accountlogs"
	"CODStatusBot/command/addaccount"
	"CODStatusBot/command/checknow"
	"CODStatusBot/command/removeaccount"
	"CODStatusBot/command/setcaptchaservice"
	"CODStatusBot/command/setcheckinterval"
	"CODStatusBot/command/updateaccount"
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"errors"
	"github.com/bwmarrin/discordgo"
	"os"
	"strings"
)

var discord *discordgo.Session

func StartBot() error {
	envToken := os.Getenv("DISCORD_TOKEN")
	if envToken == "" {
		err := errors.New("DISCORD_TOKEN environment variable not set")
		logger.Log.WithError(err).WithField("env", "DISCORD_TOKEN").Error()
		return err
	}
	var err error
	discord, err = discordgo.New("Bot " + envToken)
	if err != nil {
		logger.Log.WithError(err).WithField("Bot startup", "Token").Error()
		return err
	}

	err = discord.Open()
	if err != nil {
		logger.Log.WithError(err).WithField("Bot startup", "Opening Session").Error()
		return err
	}

	err = discord.UpdateWatchStatus(0, "the Status of your Accounts so you dont have to.")
	if err != nil {
		logger.Log.WithError(err).WithField("Bot startup", "Setting Presence Status").Error()
		return err
	}

	command.RegisterCommands(discord)
	logger.Log.Info("Registering global commands")

	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if h, ok := command.Handlers[i.ApplicationCommandData().Name]; ok {
				logger.Log.WithField("command", i.ApplicationCommandData().Name).Info("Handling command")
				h(s, i)
			} else {
				logger.Log.WithField("command", i.ApplicationCommandData().Name).Error("Command handler not found")
			}
		case discordgo.InteractionModalSubmit:
			customID := i.ModalSubmitData().CustomID
			switch {
			case customID == "set_captcha_service_modal":
				setcaptchaservice.HandleModalSubmit(s, i)
				logger.Log.Info("Handling set captcha service modal submission")

			case customID == "add_account_modal":
				addaccount.HandleModalSubmit(s, i)
				logger.Log.Info("Handling add account modal submission")

			case strings.HasPrefix(customID, "update_account_modal_"):
				updateaccount.HandleModalSubmit(s, i)
				logger.Log.Info("Handling update account modal submission")

			case customID == "set_check_interval_modal":
				setcheckinterval.HandleModalSubmit(s, i)
				logger.Log.Info("Handling set check interval modal submission")

			default:
				logger.Log.WithField("customID", customID).Error("Unknown modal submission")
			}

		case discordgo.InteractionMessageComponent:
			customID := i.MessageComponentData().CustomID
			switch {
			case strings.HasPrefix(customID, "account_age_"):
				accountage.HandleAccountSelection(s, i)
				logger.Log.Info("Handling account age selection")

			case strings.HasPrefix(customID, "account_logs_"):
				accountlogs.HandleAccountSelection(s, i)
				logger.Log.Info("Handling account logs selection")

			case customID == "account_logs_select":
				accountlogs.HandleAccountSelection(s, i)
				logger.Log.Info("Handling account logs selection")

			case strings.HasPrefix(customID, "update_account_"):
				updateaccount.HandleAccountSelection(s, i)
				logger.Log.Info("Handling update account selection")

			case customID == "update_account_select":
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

			default:
				logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
			}
		}
	})

	go services.CheckAccounts(discord)
	return nil
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
