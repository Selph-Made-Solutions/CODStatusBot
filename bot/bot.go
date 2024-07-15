package bot

import (
	"CODStatusBot/command"
	"CODStatusBot/command/addaccountnew"
	"CODStatusBot/command/removeaccountnew" // Add this new import
	"CODStatusBot/command/updateaccountnew"
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

	guilds, err := discord.UserGuilds(100, "", "", false)
	if err != nil {
		logger.Log.WithError(err).WithField("Bot startup", "Initiating Guilds").Error()
		return err
	}
	for _, guild := range guilds {
		logger.Log.WithField("guild", guild.Name).Info("Connected to guild")
		command.RegisterCommands(discord, guild.ID)
	}

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
				addaccountnew.HandleModalSubmit(s, i)
			case "remove_account_modal":
				logger.Log.Info("Handling remove account modal submission")
				removeaccountnew.HandleModalSubmit(s, i)
			case "update_account_modal":
				logger.Log.Info("Handling update account modal submission")
				updateaccountnew.HandleModalSubmit(s, i)
			default:
				logger.Log.WithField("customID", customID).Error("Unknown modal submission")
			}
		}
	})
	discord.AddHandler(OnGuildCreate)
	discord.AddHandler(OnGuildDelete)
	go services.CheckAccounts(discord)
	return nil
}

func OnGuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	guildID := event.Guild.ID
	logger.Log.WithField("guild", guildID).Info("Bot joined server:")
	command.RegisterCommands(s, guildID)
}

func OnGuildDelete(s *discordgo.Session, event *discordgo.GuildDelete) {
	guildID := event.Guild.ID
	logger.Log.WithField("guild", guildID).Info("Bot left guild")
	command.UnregisterCommands(s, guildID)
}
