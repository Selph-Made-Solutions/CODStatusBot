package feedback

import (
	"CODStatusBot/logger"
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
)

func RegisterCommand(s *discordgo.Session) {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "feedback",
			Description: "Send anonymous feedback or suggestions to the bot developer",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "message",
					Description: "Your feedback or suggestion",
					Required:    true,
				},
			},
		},
	}

	existingCommands, err := s.ApplicationCommands(s.State.User.ID, "")
	if err != nil {
		logger.Log.WithError(err).Error("Error getting global application commands")
		return
	}

	var existingCommand *discordgo.ApplicationCommand
	for _, command := range existingCommands {
		if command.Name == "feedback" {
			existingCommand = command
			break
		}
	}

	newCommand := commands[0]

	if existingCommand != nil {
		logger.Log.Info("Updating feedback command")
		_, err = s.ApplicationCommandEdit(s.State.User.ID, "", existingCommand.ID, newCommand)
		if err != nil {
			logger.Log.WithError(err).Error("Error updating feedback command")
			return
		}
	} else {
		logger.Log.Info("Creating feedback command")
		_, err = s.ApplicationCommandCreate(s.State.User.ID, "", newCommand)
		if err != nil {
			logger.Log.WithError(err).Error("Error creating feedback command")
			return
		}
	}
}

func UnregisterCommand(s *discordgo.Session) {
	commands, err := s.ApplicationCommands(s.State.User.ID, "")
	if err != nil {
		logger.Log.WithError(err).Error("Error getting global application commands")
		return
	}

	for _, command := range commands {
		if command.Name == "feedback" {
			logger.Log.Infof("Deleting command %s ", command.Name)
			err := s.ApplicationCommandDelete(s.State.User.ID, "", command.ID)
			if err != nil {

				logger.Log.WithError(err).Errorf("Error deleting command %s ", command.Name)

				return
			}
		}
	}
}

func CommandFeedback(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Send an ephemeral message to the user about anonymity
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Please note that your feedback will be sent anonymously to the developer, unless you include any identifying information in your message.",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send anonymity message")
		return
	}

	feedbackMessage := i.ApplicationCommandData().Options[0].StringValue()

	// Check if the developer ID is set in the environment variables
	developerID := os.Getenv("DEVELOPER_ID")
	if developerID == "" {
		err := errors.New("developerID not set or missing in environment variables for user feedback command")
		logger.Log.WithError(err).WithField("User Feedback Command", "environment variable DEVELOPER_ID").Error()
		sendErrorResponse(s, i, "Missing configuration for feedback command. Please try again later.")
		return
	}

	// Defer the response to the user
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to defer interaction response")
		return
	}

	// Create a DM channel with the developer
	channel, err := s.UserChannelCreate(developerID)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel with developer")
		sendFollowUpMessage(s, i, "There was an error sending your feedback. Please try again later.")
		return
	}

	// Send the feedback to the developer's DM
	_, err = s.ChannelMessageSend(channel.ID, fmt.Sprintf("New anonymous feedback:\n\n%s", feedbackMessage))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send feedback to developer")
		sendFollowUpMessage(s, i, "There was an error sending your feedback. Please try again later.")
		return
	}

	// Send a follow-up message to the user
	sendFollowUpMessage(s, i, "Your feedback has been sent anonymously to the developer. Thank you for your input!")
}

func sendErrorResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func sendFollowUpMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send follow-up message")
	}
}
