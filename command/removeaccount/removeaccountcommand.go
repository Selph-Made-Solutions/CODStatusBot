package removeaccount

import (
	"fmt"
	"strconv"
	"strings"

	"CODStatusBot/database"
	"CODStatusBot/errorhandler"
	"CODStatusBot/logger"
	"CODStatusBot/models"

	"github.com/bwmarrin/discordgo"
)

func CommandRemoveAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, err := getUserID(i)
	if err != nil {
		handleError(s, i, err)
		return
	}

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		handleError(s, i, errorhandler.NewDatabaseError(result.Error, "fetching user accounts"))
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts to remove.")
		return
	}

	// Create buttons for each account
	var components []discordgo.MessageComponent
	var currentRow []discordgo.MessageComponent

	for _, account := range accounts {
		currentRow = append(currentRow, discordgo.Button{
			Label:    account.Title,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("remove_account_%d", account.ID),
		})

		if len(currentRow) == 5 {
			components = append(components, discordgo.ActionsRow{Components: currentRow})
			currentRow = []discordgo.MessageComponent{}
		}
	}

	// Add the last row if it is not empty
	if len(currentRow) > 0 {
		components = append(components, discordgo.ActionsRow{Components: currentRow})
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    "Select an account to remove:",
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
	if err != nil {
		handleError(s, i, errorhandler.NewDiscordError(err, "sending account selection message"))
	}
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "remove_account_"))
	if err != nil {
		handleError(s, i, errorhandler.NewValidationError(err, "account ID"))
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		handleError(s, i, errorhandler.NewDatabaseError(result.Error, "fetching account"))
		return
	}

	// Show confirmation buttons
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Are you sure you want to remove the account '%s'? This action is permanent and cannot be undone.", account.Title),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Delete",
							Style:    discordgo.DangerButton,
							CustomID: fmt.Sprintf("confirm_remove_%d", account.ID),
						},
						discordgo.Button{
							Label:    "Cancel",
							Style:    discordgo.SecondaryButton,
							CustomID: "cancel_remove",
						},
					},
				},
			},
		},
	})
	if err != nil {
		handleError(s, i, errorhandler.NewDiscordError(err, "sending confirmation message"))
	}
}

func HandleConfirmation(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if customID == "cancel_remove" {
		respondToInteraction(s, i, "Account removal cancelled.")
		return
	}

	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "confirm_remove_"))
	if err != nil {
		handleError(s, i, errorhandler.NewValidationError(err, "account ID"))
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		handleError(s, i, errorhandler.NewDatabaseError(result.Error, "fetching account"))
		return
	}

	// Start a transaction
	tx := database.DB.Begin()

	// Delete associated bans
	if err := tx.Where("account_id = ?", account.ID).Delete(&models.Ban{}).Error; err != nil {
		tx.Rollback()
		handleError(s, i, errorhandler.NewDatabaseError(err, "deleting associated bans"))
		return
	}

	// Delete the account
	if err := tx.Delete(&account).Error; err != nil {
		tx.Rollback()
		handleError(s, i, errorhandler.NewDatabaseError(err, "deleting account"))
		return
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		handleError(s, i, errorhandler.NewDatabaseError(err, "committing transaction"))
		return
	}

	respondToInteraction(s, i, fmt.Sprintf("Account '%s' has been successfully removed from the database.", account.Title))
}

func handleError(s *discordgo.Session, i *discordgo.InteractionCreate, err error) {
	userMsg, _ := errorhandler.HandleError(err)
	respondToInteraction(s, i, userMsg)
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    message,
			Components: []discordgo.MessageComponent{},
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}

func getUserID(i *discordgo.InteractionCreate) (string, error) {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID, nil
	}
	if i.User != nil {
		return i.User.ID, nil
	}
	return "", errorhandler.NewValidationError(fmt.Errorf("unable to determine user ID"), "user identification")
}
