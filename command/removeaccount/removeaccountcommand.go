package removeaccount

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

func CommandRemoveAccount(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	userID := event.User().ID

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error fetching your accounts. Please try again.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	if len(accounts) == 0 {
		return event.CreateMessage(discord.MessageCreate{
			Content: "You don't have any monitored accounts to remove.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	var components []discord.MessageComponent
	for _, account := range accounts {
		components = append(components, discord.ButtonComponent{
			Label:    account.Title,
			Style:    discord.ButtonStylePrimary,
			CustomID: fmt.Sprintf("remove_account_%d", account.ID),
		})
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: "Select an account to remove:",
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
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "remove_account_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error processing your selection. Please try again.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error: Account not found or you don't have permission to remove it.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	return event.UpdateMessage(discord.MessageUpdate{
		Content: discord.NewValueOptional(fmt.Sprintf("Are you sure you want to remove the account '%s'? This action is permanent and cannot be undone.", account.Title)),
		Components: []discord.MessageComponent{
			discord.ActionRowComponent{
				Components: []discord.MessageComponent{
					discord.ButtonComponent{
						Label:    "Delete",
						Style:    discord.ButtonStyleDanger,
						CustomID: fmt.Sprintf("confirm_remove_%d", account.ID),
					},
					discord.ButtonComponent{
						Label:    "Cancel",
						Style:    discord.ButtonStyleSecondary,
						CustomID: "cancel_remove",
					},
				},
			},
		},
	})
}

func HandleConfirmation(client bot.Client, event *events.ComponentInteractionCreate) error {
	customID := event.Data.CustomID()

	if customID == "cancel_remove" {
		return respondToInteraction(event, "Account removal cancelled.")
	}

	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "confirm_remove_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		return respondToInteraction(event, "Error processing your confirmation. Please try again.")
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		return respondToInteraction(event, "Error: Account not found or you don't have permission to remove it.")
	}

	tx := database.DB.Begin()

	if err := tx.Where("account_id = ?", account.ID).Delete(&models.Ban{}).Error; err != nil {
		tx.Rollback()
		logger.Log.WithError(err).Error("Error deleting associated bans")
		return respondToInteraction(event, "Error removing account. Please try again.")
	}

	if err := tx.Delete(&account).Error; err != nil {
		tx.Rollback()
		logger.Log.WithError(err).Error("Error deleting account")
		return respondToInteraction(event, "Error removing account. Please try again.")
	}

	if err := tx.Commit().Error; err != nil {
		logger.Log.WithError(err).Error("Error committing transaction")
		return respondToInteraction(event, "Error removing account. Please try again.")
	}

	return respondToInteraction(event, fmt.Sprintf("Account '%s' has been successfully removed from the database.", account.Title))
}

func respondToInteraction(event *events.ComponentInteractionCreate, message string) error {
	return event.UpdateMessage(discord.MessageUpdate{
		Content:    discord.NewValueOptional(message),
		Components: []discord.MessageComponent{},
	})
}
