package togglecheck

import (
	"fmt"
	"strconv"
	"strings"

	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/utils"

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
		utils.RespondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		utils.RespondToInteraction(s, i, "Error fetching your accounts. Please try again.")
		return
	}

	if len(accounts) == 0 {
		utils.RespondToInteraction(s, i, "You don't have any monitored accounts.")
		return
	}

	// Create buttons for each account
	var components []discordgo.MessageComponent
	var currentRow []discordgo.MessageComponent

	for _, account := range accounts {
		label := fmt.Sprintf("%s (%s)", account.Title, getCheckStatus(account.IsCheckDisabled))
		style := discordgo.SuccessButton
		if account.IsCheckDisabled {
			style = discordgo.DangerButton
		}
		currentRow = append(currentRow, discordgo.Button{
			Label:    label,
			Style:    style,
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
		utils.RespondToInteraction(s, i, "Error processing your selection. Please try again.")
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		utils.RespondToInteraction(s, i, "Error: Account not found")
		return
	}

	if account.IsCheckDisabled {
		confirmMessage := fmt.Sprintf("Account '%s' is currently disabled. Reason: %s\n\nAre you sure you want to re-enable checks for this account?", account.Title, account.DisabledReason)
		showConfirmationButtons(s, i, account.ID, confirmMessage, "reenable")
	} else {
		confirmMessage := fmt.Sprintf("Are you sure you want to disable checks for account '%s'?", account.Title)
		showConfirmationButtons(s, i, account.ID, confirmMessage, "disable")
	}
}

func HandleConfirmation(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	if customID == "cancel_toggle" {
		utils.RespondToInteraction(s, i, "Action cancelled.")
		return
	}

	parts := strings.Split(customID, "_")
	if len(parts) != 3 {
		logger.Log.Error("Invalid custom ID format")
		utils.RespondToInteraction(s, i, "Error processing your confirmation. Please try again.")
		return
	}

	action := parts[1]
	accountID, err := strconv.Atoi(parts[2])
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		utils.RespondToInteraction(s, i, "Error processing your confirmation. Please try again.")
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		utils.RespondToInteraction(s, i, "Error: Account not found")
		return
	}

	switch action {
	case "reenable":
		account.IsCheckDisabled = false
		account.DisabledReason = ""
		account.ConsecutiveErrors = 0
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Error("Error saving account changes")
			utils.RespondToInteraction(s, i, "Error re-enabling account checks. Please try again.")
			return
		}
		utils.RespondToInteraction(s, i, fmt.Sprintf("Checks for account '%s' have been re-enabled.", account.Title))
	case "disable":
		account.IsCheckDisabled = true
		account.DisabledReason = "Manually disabled by user"
		if err := database.DB.Save(&account).Error; err != nil {
			logger.Log.WithError(err).Error("Error saving account changes")
			utils.RespondToInteraction(s, i, "Error disabling account checks. Please try again.")
			return
		}
		utils.RespondToInteraction(s, i, fmt.Sprintf("Checks for account '%s' have been disabled.", account.Title))
	default:
		utils.RespondToInteraction(s, i, "Invalid action. Please try again.")
	}
}

func showConfirmationButtons(s *discordgo.Session, i *discordgo.InteractionCreate, accountID uint, message, action string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    fmt.Sprintf("Confirm %s", strings.Title(action)),
							Style:    discordgo.SuccessButton,
							CustomID: fmt.Sprintf("confirm_%s_%d", action, accountID),
						},
						discordgo.Button{
							Label:    "Cancel",
							Style:    discordgo.DangerButton,
							CustomID: "cancel_toggle",
						},
					},
				},
			},
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error showing confirmation buttons")
		utils.RespondToInteraction(s, i, "An error occurred. Please try again.")
	}
}

func getCheckStatus(isDisabled bool) string {
	if isDisabled {
		return "disabled"
	}
	return "enabled"
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	var err error
	if i.Type == discordgo.InteractionMessageComponent {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    message,
				Components: []discordgo.MessageComponent{},
			},
		})
	} else {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: message,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
		// If we fail to respond to the interaction, try to send a follow-up message
		_, followUpErr := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "An error occurred while processing your request. Please try again.",
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		if followUpErr != nil {
			logger.Log.WithError(followUpErr).Error("Error sending follow-up message")
		}
	}
}
