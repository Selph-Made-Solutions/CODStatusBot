package feedback

import (
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"os"
)

func CommandFeedback(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	options := event.SlashCommandInteractionData().Options
	feedbackMessage := options.String("message")

	developerID := os.Getenv("DEVELOPER_ID")
	if developerID == "" {
		logger.Log.Error("DEVELOPER_ID not set in environment variables")
		return sendResponse(event, "Configuration error. Please try again later.", true)
	}

	// Send anonymous feedback to developer
	channel, err := client.Rest().CreateDM(discord.Snowflake(developerID))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create DM channel with developer")
		return sendResponse(event, "There was an error sending your feedback. Please try again later.", true)
	}

	anonymousFeedback := fmt.Sprintf("Anonymous Feedback:\n\n%s", feedbackMessage)

	_, err = client.Rest().CreateMessage(channel.ID, discord.NewMessageCreateBuilder().
		SetContent(anonymousFeedback).
		Build())
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send feedback to developer")
		return sendResponse(event, "There was an error sending your feedback. Please try again later.", true)
	}

	// Respond to user
	return sendResponse(event, "Your anonymous feedback has been sent to the developer. Thank you for your input!", true)
}

func sendResponse(event *events.ApplicationCommandInteractionCreate, content string, ephemeral bool) error {
	var flags discord.MessageFlags
	if ephemeral {
		flags = discord.MessageFlagEphemeral
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: content,
		Flags:   flags,
	})
}
