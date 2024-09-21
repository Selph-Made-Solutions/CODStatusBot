package togglecheck

import (
	"fmt"
	"strconv"
	"strings"

	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"

	"github.com/bwmarrin/discordgo"
)

func CommandToggleCheck(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		respondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	if len(accounts) == 0 {
		respondToInteraction(s, i, "You don't have any monitored accounts.")
		return
	}

	// Create buttons for each account
	var components []discordgo.MessageComponent
	var currentRow []discordgo.MessageComponent

	for _, account := range accounts {
		label := fmt.Sprintf("%s (%s)", account.Title, getCheckStatus(account.IsCheckDisabled))
		currentRow = append(currentRow, discordgo.Button{
			Label:    label,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("toggle_check_%d", account.ID),
		})

		if len(currentRow) == 5 {
			components = append(components, discordgo.ActionsRow{Components: currentRow})
			currentRow = []discordgo.MessageComponent{}
		}
	}

	// Add the last row if it is not empty.
	if len(currentRow) > 0 {
		components = append(components, discordgo.ActionsRow{Components: currentRow})
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    "Select an account to toggle auto check On/Off:",
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding with account selection")
	}
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "toggle_check_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		respondToInteraction(s, i, "Error processing your selection. Please try again.")
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found")
		return
	}

	if account.IsDisabled {
		respondToInteraction(s, i, fmt.Sprintf("Account '%s' is currently disabled due to errors. Please contact support to re-enable it.", account.Title))
		return
	}

	// Toggle the IsCheckDisabled field
	account.IsCheckDisabled = !account.IsCheckDisabled

	if err := database.DB.Save(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Error saving account changes")
		respondToInteraction(s, i, "Error toggling account checks. Please try again.")
		return
	}

	status := getCheckStatus(account.IsCheckDisabled)
	message := fmt.Sprintf("Checks for account '%s' are now %s.", account.Title, status)
	respondToInteraction(s, i, message)
}

func getCheckStatus(account models.Account) string {
	if account.IsDisabled {
		return "disabled"
	}
	return "enabled"
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
	return "", fmt.Errorf("unable to determine user ID")
}
