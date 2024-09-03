package togglecheck

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"strconv"
	"strings"
)

func CommandToggleCheck(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	userID := event.User().ID

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		return respondToInteraction(event, "Error fetching your accounts. Please try again.")
	}

	if len(accounts) == 0 {
		return respondToInteraction(event, "You don't have any monitored accounts.")
	}

	var components []discord.MessageComponent
	for _, account := range accounts {
		label := fmt.Sprintf("%s (%s - %s)", account.Title, getCheckStatus(account.IsCheckDisabled), account.LastStatus.Overall)
		components = append(components, discord.ButtonComponent{
			Label:    label,
			Style:    discord.ButtonStylePrimary,
			CustomID: fmt.Sprintf("toggle_check_%d", account.ID),
		})
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: "Select an account to toggle auto check On/Off:",
		Flags:   discord.MessageFlagEphemeral,
		Components: []discord.MessageComponent{
			discord.ActionRowComponent{
				Components: components,
			},
		},
	})
}

func HandleAccountSelection(client bot.Client, event *events.ComponentInteractionCreate) error {
	customID := event.Data.CustomID()
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "toggle_check_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		return respondToInteraction(event, "Error processing your selection. Please try again.")
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		return respondToInteraction(event, "Error: Account not found")
	}

	account.IsCheckDisabled = !account.IsCheckDisabled

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving account changes")
		return respondToInteraction(event, "Error toggling account checks. Please try again.")
	}

	status := getCheckStatus(account.IsCheckDisabled)
	message := fmt.Sprintf("Checks for account '%s' are now %s.", account.Title, status)
	return respondToInteraction(event, message)
}

func getCheckStatus(isDisabled bool) string {
	if isDisabled {
		return "disabled"
	}
	return "enabled"
}

func respondToInteraction(event interface{}, message string) error {
	switch e := event.(type) {
	case *events.ComponentInteractionCreate:
		return e.UpdateMessage(discord.MessageUpdate{
			Content:    discord.NewValueOptional(message),
			Components: []discord.MessageComponent{},
		})
	case *events.ApplicationCommandInteractionCreate:
		return e.CreateMessage(discord.MessageCreate{
			Content: message,
			Flags:   discord.MessageFlagEphemeral,
		})
	default:
		return nil
	}
}
