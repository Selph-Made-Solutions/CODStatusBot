package feedback

import (
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
)

func CommandFeedback(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	feedbackMessage := i.ApplicationCommandData().Options[0].StringValue()
	developerID := os.Getenv("DEVELOPER_ID")
	if developerID == "" {
		logger.Log.Error("DEVELOPER_ID not set in environment variables")
		sendResponse(s, i, "Configuration error. Please try again later.", true)
		return
	}

	// Send anonymous feedback to developer
	channel, err := s.UserChannelCreate(developerID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel with developer")
		sendResponse(s, i, "There was an error sending your feedback. Please try again later.", true)
		return
	}

	anonymousFeedback := fmt.Sprintf("Anonymous Feedback:\n\n%s", feedbackMessage)

	_, err = s.ChannelMessageSend(channel.ID, anonymousFeedback)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send feedback to developer")
		sendResponse(s, i, "There was an error sending your feedback. Please try again later.", true)
		return
	}

	// Respond to user
	sendResponse(s, i, "Your anonymous feedback has been sent to the developer. Thank you for your input!", true)
}

func sendResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string, ephemeral bool) {
	flags := discordgo.MessageFlags(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   flags,
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Failed to send interaction response")
	}
}
