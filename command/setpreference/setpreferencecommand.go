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

	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	} else {
		logger.Log.Error("Interaction doesn't have Member or User")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	// Validate preference type
	if preferenceType != "channel" && preferenceType != "dm" {
		respondToInteraction(s, i, "Invalid preference type. Please choose either 'channel' or 'dm'.")
		return
	}

	// Update all existing accounts for this user
	result := database.DB.Model(&models.Account{}).
		Where("user_id = ?", userID).
		Update("notification_type", preferenceType)

	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error updating user accounts")
		respondToInteraction(s, i, "Error setting preference. Please try again.")
		return
	}

	// Log the number of accounts updated
	logger.Log.Infof("Updated %d accounts for user %s", result.RowsAffected, userID)

	message := "Your notification preference has been updated for all your accounts. "
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
