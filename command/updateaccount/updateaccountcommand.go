package updateaccount

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strconv"
	"strings"
)

func CommandUpdateAccount(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
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
		respondToInteraction(s, i, "You don't have any monitored accounts to update.")
		return
	}

	// Create buttons for each account
	var components []discordgo.MessageComponent
	for _, account := range accounts {
		components = append(components, discordgo.Button{
			Label:    account.Title,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("update_account_%d", account.ID),
		})
	}

	// Send message with account buttons
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Select an account to update:",
			Flags:   discordgo.MessageFlagsEphemeral,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: components,
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding with account selection")
	}
}

func HandleAccountSelection(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	customID := i.MessageComponentData().CustomID
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "update_account_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		respondToInteraction(s, i, "Error processing your selection. Please try again.")
		return
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: fmt.Sprintf("update_account_modal_%d", accountID),
			Title:    "Update Account",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "new_sso_cookie",
							Label:       "Enter the new SSO cookie",
							Style:       discordgo.TextInputParagraph,
							Placeholder: "Enter the new SSO cookie",
							Required:    true,
							MinLength:   1,
							MaxLength:   4000,
						},
					},
				},
			},
		},
	})

	if err != nil {
		logger.Log.WithError(err).Error("Error responding with modal")
	}
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, installType models.InstallationType) {
	data := i.ModalSubmitData()

	accountIDStr := strings.TrimPrefix(data.CustomID, "update_account_modal_")
	accountID, err := strconv.Atoi(accountIDStr)
	if err != nil {
		logger.Log.WithError(err).Error("Error converting account ID from modal custom ID")
		respondToInteraction(s, i, "Error processing your update. Please try again.")
		return
	}

	var newSSOCookie string

	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if v, ok := rowComp.(*discordgo.TextInput); ok && v.CustomID == "new_sso_cookie" {
					newSSOCookie = strings.TrimSpace(v.Value)
				}
			}
		}
	}

	if newSSOCookie == "" {
		respondToInteraction(s, i, "Error: New SSO cookie must be provided.")
		return
	}

	// Verify the new SSO cookie
	if !services.VerifySSOCookie(newSSOCookie) {
		respondToInteraction(s, i, "Error: The provided SSO cookie is invalid. Please check and try again.")
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found or you don't have permission to update it.")
		return
	}

	// Verify that the user owns this account
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

	if account.UserID != userID {
		logger.Log.Error("User attempted to update an account they don't own")
		respondToInteraction(s, i, "Error: You don't have permission to update this account.")
		return
	}

	// Get SSO Cookie expiration
	expirationTimestamp, err := services.DecodeSSOCookie(newSSOCookie)
	if err != nil {
		logger.Log.WithError(err).Error("Error decoding SSO cookie")
		respondToInteraction(s, i, fmt.Sprintf("Error processing SSO cookie: %v", err))
		return
	}

	// Update the account
	account.SSOCookie = newSSOCookie
	account.SSOCookieExpiration = expirationTimestamp
	account.IsExpiredCookie = false        // Reset the expired cookie flag
	account.InstallationType = installType // Update installation type

	services.DBMutex.Lock()
	if err := database.DB.Save(&account).Error; err != nil {
		services.DBMutex.Unlock()
		logger.Log.WithError(err).Error("Error updating account")
		respondToInteraction(s, i, "Error updating account. Please try again.")
		return
	}
	services.DBMutex.Unlock()

	formattedExpiration := services.FormatExpirationTime(expirationTimestamp)
	message := fmt.Sprintf("Account '%s' has been successfully updated. New SSO cookie will expire in %s", account.Title, formattedExpiration)
	respondToInteraction(s, i, message)
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	var err error
	if i.Type == discordgo.InteractionMessageComponent {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    message,
				Components: []discordgo.MessageComponent{},
				Flags:      discordgo.MessageFlagsEphemeral,
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
	}
}
