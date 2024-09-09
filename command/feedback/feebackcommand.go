package feedback

import (
	"CODStatusBot/logger"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"strings"
)

func CommandFeedback(s *discordgo.Session, i *discordgo.InteractionCreate) {
	feedbackMessage := i.ApplicationCommandData().Options[0].StringValue()
	developerID := os.Getenv("DEVELOPER_ID")
	if developerID == "" {
		logger.Log.Error("DEVELOPER_ID not set in environment variables")
		sendResponse(s, i, "Configuration error. Please try again later.", true)
		return
	}

	var userID string
	var username string
	if i.Member != nil {
		userID = i.Member.User.ID
		username = i.Member.User.Username
	} else if i.User != nil {
		userID = i.User.ID
		username = i.User.Username
	} else {
		logger.Log.Error("Interaction doesn't have Member or User")
		sendResponse(s, i, "An error occurred while processing your request.", true)
		return
	}

	// Send feedback to developer with anonymity option
	channel, err := s.UserChannelCreate(developerID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel with developer")
		sendResponse(s, i, "There was an error sending your feedback. Please try again later.", true)
		return
	}

	// Create message with buttons for anonymity choice
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Do you want to send this feedback anonymously?",
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Send Anonymously",
							Style:    discordgo.PrimaryButton,
							CustomID: fmt.Sprintf("feedback_anonymous_%s", userID),
						},
						discordgo.Button{
							Label:    "Send with ID",
							Style:    discordgo.SecondaryButton,
							CustomID: fmt.Sprintf("feedback_with_id_%s", userID),
						},
					},
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Failed to send anonymity choice message")
		sendResponse(s, i, "There was an error processing your feedback. Please try again later.", true)
		return
	}

	// Store the feedback message temporarily (you might want to use a more persistent storage in a production environment)
	tempFeedbackStore[userID] = feedbackMessage
}

func HandleFeedbackChoice(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, "_")
	if len(parts) != 3 {
		logger.Log.Error("Invalid custom ID format for feedback choice")
		sendResponse(s, i, "An error occurred while processing your request.", true)
		return
	}

	isAnonymous := parts[1] == "anonymous"
	userID := parts[2]

	feedbackMessage, ok := tempFeedbackStore[userID]
	if !ok {
		logger.Log.Error("Feedback message not found for user")
		sendResponse(s, i, "Your feedback session has expired. Please submit your feedback again.", true)
		return
	}
	delete(tempFeedbackStore, userID)

	var feedbackToSend string
	if isAnonymous {
		feedbackToSend = fmt.Sprintf("Anonymous Feedback:\n\n%s", feedbackMessage)
	} else {
		feedbackToSend = fmt.Sprintf("Feedback from User ID %s:\n\n%s", userID, feedbackMessage)
	}

	developerID := os.Getenv("DEVELOPER_ID")
	channel, err := s.UserChannelCreate(developerID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel with developer")
		sendResponse(s, i, "There was an error sending your feedback. Please try again later.", true)
		return
	}

	_, err = s.ChannelMessageSend(channel.ID, feedbackToSend)
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
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   flags,
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Failed to send interaction response")
	}
}

// Temporary storage for feedback messages
var tempFeedbackStore = make(map[string]string)
