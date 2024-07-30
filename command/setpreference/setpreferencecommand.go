package setpreference

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

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
