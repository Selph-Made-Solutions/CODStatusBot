package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/command/accountage"
	"CODStatusBot/command/accountlogs"
	"CODStatusBot/command/addaccount"
	"CODStatusBot/command/removeaccount"
	"CODStatusBot/command/updateaccount"
	"CODStatusBot/logger"
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
			case customID == "add_account_modal":
				logger.Log.Info("Handling add account modal submission")
				addaccount.HandleModalSubmit(s, i)
			case strings.HasPrefix(customID, "update_account_modal_"):
				logger.Log.Info("Handling update account modal submission")
				updateaccount.HandleModalSubmit(s, i)
			default:
				logger.Log.WithField("customID", customID).Error("Unknown modal submission")
			}

		case discordgo.InteractionMessageComponent:
			customID := i.MessageComponentData().CustomID
			switch {
			case customID == "account_logs_select":
				logger.Log.Info("Handling account logs selection")
				accountlogs.HandleAccountSelection(s, i)
			case customID == "update_account_select":
				logger.Log.Info("Handling update account selection")
				updateaccount.HandleAccountSelection(s, i)
			case customID == "account_age_select":
				logger.Log.Info("Handling account age selection")
				accountage.HandleAccountSelection(s, i)
			case strings.HasPrefix(customID, "remove_account_"):
				logger.Log.Info("Handling remove account selection")
				removeaccount.HandleAccountSelection(s, i)
			case customID == "cancel_remove" || strings.HasPrefix(customID, "confirm_remove_"):
				logger.Log.Info("Handling remove account confirmation")
				removeaccount.HandleConfirmation(s, i)
			default:
				logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
			}
		}
	})

	go services.CheckAccounts(discord)
	return nil
}
