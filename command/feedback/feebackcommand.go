package feedback

import (
	"CODStatusBot/logger"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
)

func CommandFeedback(s *discordgo.Session, i *discordgo.InteractionCreate) {
	feedbackMessage := i.ApplicationCommandData().Options[0].StringValue()
	developerID := os.Getenv("DEVELOPER_ID")
	if developerID == "" {
		logger.Log.Error("DEVELOPER_ID not set in environment variables")
		sendResponse(s, i, "Configuration error. Please try again later.", true)
		return
	}

	// Send feedback to developer
	channel, err := s.UserChannelCreate(developerID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel with developer")
		sendResponse(s, i, "There was an error sending your feedback. Please try again later.", true)
		return
	}

	var userInfo string
	if i.Member != nil {
		userInfo = fmt.Sprintf("User: %s (ID: %s) in Guild: %s", i.Member.User.Username, i.Member.User.ID, i.GuildID)
	} else if i.User != nil {
		userInfo = fmt.Sprintf("User: %s (ID: %s) via DM", i.User.Username, i.User.ID)
	} else {
		userInfo = "Unknown user"
	}

	_, err = s.ChannelMessageSend(channel.ID, fmt.Sprintf("New feedback:\n\nFrom: %s\n\nMessage:\n%s", userInfo, feedbackMessage))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send feedback to developer")
		sendResponse(s, i, "There was an error sending your feedback. Please try again later.", true)
		return
	}

	// Respond to user
	sendResponse(s, i, "Your feedback has been sent to the developer. Thank you for your input!", true)
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
