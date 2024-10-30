package updateaccount

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bradselph/CODStatusBot/services"
	"github.com/bradselph/CODStatusBot/utils"

	"github.com/bwmarrin/discordgo"
)

func CommandUpdateAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	if !services.IsServiceEnabled("ezcaptcha") && !services.IsServiceEnabled("2captcha") {
		respondToInteraction(s, i, "Account updates are currently unavailable as no captcha services are enabled. Please try again later.")
		return
	}

	userSettings, err := services.GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		respondToInteraction(s, i, "Error fetching user settings. Please try again.")
		return
	}

	if !services.IsServiceEnabled(userSettings.PreferredCaptchaProvider) {
		msg := fmt.Sprintf("Your preferred captcha service (%s) is currently disabled. ", userSettings.PreferredCaptchaProvider)
		if services.IsServiceEnabled("ezcaptcha") {
			msg += "Please switch to EZCaptcha using /setcaptchaservice."
		} else if services.IsServiceEnabled("2captcha") {
			msg += "Please switch to 2Captcha using /setcaptchaservice."
		} else {
			msg += "No captcha services are currently available. Please try again later."
		}
		respondToInteraction(s, i, msg)
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

	var (
		components []discordgo.MessageComponent
		currentRow []discordgo.MessageComponent
	)

	for _, account := range accounts {
		label := account.Title
		if isVIP, err := services.CheckVIPStatus(account.SSOCookie); err == nil && isVIP {
			label += " ⭐"
		}

		button := discordgo.Button{
			Label:    label,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("update_account_%d", account.ID),
		}

		if account.IsCheckDisabled {
			button.Style = discordgo.SecondaryButton
			label += " (Disabled)"
		}

		currentRow = append(currentRow, button)

		if len(currentRow) == 5 {
			components = append(components, discordgo.ActionsRow{Components: currentRow})
			currentRow = []discordgo.MessageComponent{}
		}
	}

	if len(currentRow) > 0 {
		components = append(components, discordgo.ActionsRow{Components: currentRow})
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    "Select an account to update:",
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
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "update_account_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		respondToInteraction(s, i, "Error processing your selection. Please try again.")
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		respondToInteraction(s, i, "Error: Account not found or you don't have permission to update it.")
		return
	}

	userID := ""
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	if account.UserID != userID {
		respondToInteraction(s, i, "Error: You don't have permission to update this account.")
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

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	accountIDStr := strings.TrimPrefix(data.CustomID, "update_account_modal_")
	accountID, err := strconv.Atoi(accountIDStr)
	if err != nil {
		logger.Log.WithError(err).Error("Error converting account ID from modal custom ID")
		respondToInteractionWithEmbed(s, i, "Error processing your update. Please try again.", nil)
		return
	}

	var newSSOCookie string
	for _, comp := range data.Components {
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, rowComp := range row.Components {
				if v, ok := rowComp.(*discordgo.TextInput); ok && v.CustomID == "new_sso_cookie" {
					newSSOCookie = utils.SanitizeInput(strings.TrimSpace(v.Value))
				}
			}
		}
	}

	if newSSOCookie == "" {
		respondToInteractionWithEmbed(s, i, "Error: New SSO cookie must be provided.", nil)
		return
	}

	if !services.VerifySSOCookie(newSSOCookie) {
		respondToInteractionWithEmbed(s, i, "Error: The provided SSO cookie is invalid. Please check and try again.", nil)
		return
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		respondToInteractionWithEmbed(s, i, "Error: Account not found or you don't have permission to update it.", nil)
		return
	}

	userID := ""
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	if account.UserID != userID {
		respondToInteractionWithEmbed(s, i, "Error: You don't have permission to update this account.", nil)
		return
	}

	userSettings, err := services.GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		respondToInteractionWithEmbed(s, i, "Error fetching user settings. Please try again.", nil)
		return
	}

	if !services.IsServiceEnabled(userSettings.PreferredCaptchaProvider) {
		msg := fmt.Sprintf("Your preferred captcha service (%s) is currently disabled. ", userSettings.PreferredCaptchaProvider)
		if services.IsServiceEnabled("ezcaptcha") {
			msg += "Please switch to EZCaptcha using /setcaptchaservice."
		} else if services.IsServiceEnabled("2captcha") {
			msg += "Please switch to 2Captcha using /setcaptchaservice."
		}
		respondToInteractionWithEmbed(s, i, msg, nil)
		return
	}

	oldVIP, _ := services.CheckVIPStatus(account.SSOCookie)
	newVIP, _ := services.CheckVIPStatus(newSSOCookie)

	var vipStatusChange string
	if oldVIP != newVIP {
		if newVIP {
			vipStatusChange = "Your account is now a VIP account! ⭐"
		} else {
			vipStatusChange = "Your account is no longer a VIP account"
		}
	}

	expirationTimestamp, err := services.DecodeSSOCookie(newSSOCookie)
	if err != nil {
		logger.Log.WithError(err).Error("Error decoding SSO cookie")
		respondToInteractionWithEmbed(s, i, fmt.Sprintf("Error processing SSO cookie: %v", err), nil)
		return
	}

	account.LastNotification = time.Now().Unix()
	account.LastCookieNotification = 0
	account.SSOCookie = newSSOCookie
	account.SSOCookieExpiration = expirationTimestamp
	account.IsExpiredCookie = false
	wasDisabled := account.IsCheckDisabled
	account.IsCheckDisabled = false
	account.DisabledReason = ""
	account.ConsecutiveErrors = 0
	account.LastSuccessfulCheck = time.Now()

	services.DBMutex.Lock()
	if err := database.DB.Save(&account).Error; err != nil {
		services.DBMutex.Unlock()
		logger.Log.WithError(err).Error("Failed to update account after modification")
		respondToInteractionWithEmbed(s, i, "Error updating account. Please try again.", nil)
		return
	}
	services.DBMutex.Unlock()

	embed := &discordgo.MessageEmbed{
		Title:       "Account Update Successful",
		Description: fmt.Sprintf("Account '%s' has been updated successfully.", account.Title),
		Color:       0x00ff00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Cookie Expiration",
				Value:  services.FormatExpirationTime(expirationTimestamp),
				Inline: true,
			},
			{
				Name:   "VIP Status",
				Value:  getVIPStatusText(newVIP),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if wasDisabled {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Account Status",
			Value:  "Account checks have been re-enabled",
			Inline: false,
		})
	}

	if vipStatusChange != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Status Change",
			Value:  vipStatusChange,
			Inline: false,
		})
	}

	respondToInteractionWithEmbed(s, i, "", embed)
}

func getVIPStatusText(isVIP bool) string {
	if isVIP {
		return "VIP Account ⭐"
	}
	return "Regular Account"
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

func respondToInteractionWithEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, message string, embed *discordgo.MessageEmbed) {
	responseData := &discordgo.InteractionResponseData{
		Flags: discordgo.MessageFlagsEphemeral,
	}

	if message != "" {
		responseData.Content = message
	}
	if embed != nil {
		responseData.Embeds = []*discordgo.MessageEmbed{embed}
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: responseData,
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction with embed")
	}
}
