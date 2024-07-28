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
			switch customID {
			case "add_account_modal":
				logger.Log.Info("Handling add account modal submission")
				addaccount.HandleModalSubmit(s, i)
			case "remove_account_modal":
				logger.Log.Info("Handling remove account modal submission")
				removeaccount.HandleModalSubmit(s, i)
			case "update_account_modal":
				logger.Log.Info("Handling update account modal submission")
				updateaccount.HandleModalSubmit(s, i)
			default:
				logger.Log.WithField("customID", customID).Error("Unknown modal submission")
			}
		case discordgo.InteractionMessageComponent:
			customID := i.MessageComponentData().CustomID
			switch customID {
			case "account_logs_select":
				logger.Log.Info("Handling account logs selection")
				accountlogs.HandleAccountSelection(s, i)
			case "update_account_select":
				logger.Log.Info("Handling update account selection")
				updateaccount.HandleAccountSelection(s, i)
			case "account_age_select":
				logger.Log.Info("Handling account age selection")
				accountage.HandleAccountSelection(s, i)
			case "remove_account_select":
				logger.Log.Info("Handling remove account selection")
				removeaccount.HandleAccountSelection(s, i)
			default:
				logger.Log.WithField("customID", customID).Error("Unknown message component interaction")
			}
		}
	})

	go services.CheckAccounts(discord)
	return nil
}
