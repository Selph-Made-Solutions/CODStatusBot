package listaccounts

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

func CommandListAccounts(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	userID := event.User().ID

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error fetching your accounts. Please try again.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	if len(accounts) == 0 {
		return event.CreateMessage(discord.MessageCreate{
			Content: "You don't have any monitored accounts.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	embedBuilder := discord.NewEmbedBuilder().
		SetTitle("Your Monitored Accounts").
		SetDescription("Here's a list of all your monitored accounts:").
		SetColor(0x00ff00)

	for _, account := range accounts {
		embedBuilder.AddField(account.Title, fmt.Sprintf("Status: %s\nGuild: %s\nNotification Type: %s",
			account.LastStatus, account.GuildID, account.NotificationType), false)
	}

	return event.CreateMessage(discord.MessageCreate{
		Embeds: []discord.Embed{embedBuilder.Build()},
		Flags:  discord.MessageFlagEphemeral,
	})
}
