package setcheckinterval

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
)

func CommandSetCheckInterval(s *discordgo.Session, i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options
	if len(options) != 1 {
		respondToInteraction(s, i, "Invalid command usage. Please provide the check interval in minutes.")
		return
	}

	interval := options[0].IntValue()
	if interval < 1 {
		respondToInteraction(s, i, "Invalid interval. Please provide a positive integer value in minutes.")
		return
	}

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

	userSettings, err := services.GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		respondToInteraction(s, i, "Error setting check interval. Please try again.")
		return
	}

	userSettings.CheckInterval = int(interval)
	if err := database.DB.Save(&userSettings).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving user settings")
		respondToInteraction(s, i, "Error setting check interval. Please try again.")
		return
	}

	respondToInteraction(s, i, fmt.Sprintf("Your account check interval has been set to %d minutes.", interval))
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
