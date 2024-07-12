package feedback

import (
	"CODStatusBot/logger"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
)

func RegisterCommand(s *discordgo.Session) {
	command := &discordgo.ApplicationCommand{
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
	}

	_, err := s.ApplicationCommandCreate(s.State.User.ID, "", command)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating feedback command")
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
			err := s.ApplicationCommandDelete(s.State.User.ID, "", command.ID)
			if err != nil {
				logger.Log.WithError(err).Error("Error deleting feedback command")
			}
			break
		}
	}
}

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

	_, err = s.ChannelMessageSend(channel.ID, fmt.Sprintf("New anonymous feedback:\n\n%s", feedbackMessage))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send feedback to developer")
		sendResponse(s, i, "There was an error sending your feedback. Please try again later.", true)
		return
	}

	// Respond to user
	sendResponse(s, i, "Your feedback has been sent anonymously to the developer. Thank you for your input!", true)
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
