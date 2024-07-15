package setpreference

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

func RegisterCommand(s *discordgo.Session, guildID string) {
	command := &discordgo.ApplicationCommand{
		Name:        "setpreference",
		Description: "Set your preference for where you want to receive status notifications",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "type",
				Description: "Where do you want to receive Status Notifications?",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{
						Name:  "Channel",
						Value: "channel",
					},
					{
						Name:  "Direct Message",
						Value: "dm",
					},
				},
			},
		},
	}

	_, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, command)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating setpreference command")
	}
}

func UnregisterCommand(s *discordgo.Session, guildID string) {
	commands, err := s.ApplicationCommands(s.State.User.ID, guildID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting application commands")
		return
	}

	for _, command := range commands {
		if command.Name == "setpreference" {
			err := s.ApplicationCommandDelete(s.State.User.ID, guildID, command.ID)
			if err != nil {
				logger.Log.WithError(err).Error("Error deleting setpreference command")
			}
			return
		}
	}
}

func CommandSetPreference(s *discordgo.Session, i *discordgo.InteractionCreate) {
	preferenceType := i.ApplicationCommandData().Options[0].StringValue()
	userID := i.Member.User.ID
	guildID := i.GuildID

	var accounts []models.Account
	result := database.DB.Where("user_id = ? AND guild_id = ?", userID, guildID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error setting preference. Please try again.")
		return
	}

	for _, account := range accounts {
		account.NotificationType = preferenceType
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Errorf("Error updating preference for account %s", account.Title)
		}
	}

	message := "Your notification preference has been updated. "
	if preferenceType == "channel" {
		message += "You will now receive notifications in the channel."
	} else {
		message += "You will now receive notifications via direct message."
	}

	respondToInteraction(s, i, message)
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}
