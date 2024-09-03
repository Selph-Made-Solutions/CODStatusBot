package updateaccount

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"strconv"
	"strings"
)

func CommandUpdateAccount(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	userID := event.User().ID

	var accounts []models.Account
	result := database.DB.Where("user_id = ?", userID).Find(&accounts)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching user accounts")
		return respondToInteraction(event, "Error fetching your accounts. Please try again.")
	}

	if len(accounts) == 0 {
		return respondToInteraction(event, "You don't have any monitored accounts to update.")
	}

	var components []discord.MessageComponent
	for _, account := range accounts {
		components = append(components, discord.ButtonComponent{
			Label:    account.Title,
			Style:    discord.ButtonStylePrimary,
			CustomID: fmt.Sprintf("update_account_%d", account.ID),
		})
	}

	return event.CreateMessage(discord.MessageCreate{
		Content: "Select an account to update:",
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
	accountID, err := strconv.Atoi(strings.TrimPrefix(customID, "update_account_"))
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing account ID")
		return respondToInteraction(event, "Error processing your selection. Please try again.")
	}

	return event.CreateModal(discord.NewModalCreateBuilder().
		SetCustomID(fmt.Sprintf("update_account_modal_%d", accountID)).
		SetTitle("Update Account").
		AddActionRow(discord.TextInputComponent{
			CustomID:    "new_sso_cookie",
			Label:       "Enter the new SSO cookie",
			Style:       discord.TextInputStyleParagraph,
			Placeholder: "Enter the new SSO cookie",
			Required:    true,
			MinLength:   1,
			MaxLength:   4000,
		}).
		Build())
}

func HandleModalSubmit(client bot.Client, event *events.ModalSubmitInteractionCreate) error {
	data := event.Data
	customID := data.CustomID()

	accountIDStr := strings.TrimPrefix(customID, "update_account_modal_")
	accountID, err := strconv.Atoi(accountIDStr)
	if err != nil {
		logger.Log.WithError(err).Error("Error converting account ID from modal custom ID")
		return respondToInteraction(event, "Error processing your update. Please try again.")
	}

	newSSOCookie := strings.TrimSpace(data.Text("new_sso_cookie"))

	if newSSOCookie == "" {
		return respondToInteraction(event, "Error: New SSO cookie must be provided.")
	}

	if !services.VerifySSOCookie(newSSOCookie) {
		return respondToInteraction(event, "Error: The provided SSO cookie is invalid. Please check and try again.")
	}

	var account models.Account
	result := database.DB.First(&account, accountID)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error fetching account")
		return respondToInteraction(event, "Error: Account not found or you don't have permission to update it.")
	}

	if account.UserID != event.User().ID.String() {
		logger.Log.Error("User attempted to update an account they don't own")
		return respondToInteraction(event, "Error: You don't have permission to update this account.")
	}

	expirationTimestamp, err := services.DecodeSSOCookie(newSSOCookie)
	if err != nil {
		logger.Log.WithError(err).Error("Error decoding SSO cookie")
		return respondToInteraction(event, fmt.Sprintf("Error processing SSO cookie: %v", err))
	}

	account.SSOCookie = newSSOCookie
	account.SSOCookieExpiration = expirationTimestamp
	account.IsExpiredCookie = false
	account.InstallationType = models.InstallTypeUser
	account.LastStatus = models.AccountStatus{Overall: models.StatusUnknown, Games: make(map[string]models.GameStatus)}

	services.DBMutex.Lock()
	if err := database.DB.Save(&account).Error; err != nil {
		services.DBMutex.Unlock()
		logger.Log.WithError(err).Error("Error updating account")
		return respondToInteraction(event, "Error updating account. Please try again.")
	}
	services.DBMutex.Unlock()

	formattedExpiration := services.FormatExpirationTime(expirationTimestamp)
	message := fmt.Sprintf("Account '%s' has been successfully updated. New SSO cookie will expire in %s", account.Title, formattedExpiration)
	return respondToInteraction(event, message)
}

func respondToInteraction(event interface{}, message string) error {
	switch e := event.(type) {
	case *events.ComponentInteractionCreate:
		return e.UpdateMessage(discord.MessageUpdate{
			Content:    discord.NewValueOptional(message),
			Components: []discord.MessageComponent{},
			Flags:      discord.MessageFlagEphemeral,
		})
	case *events.ApplicationCommandInteractionCreate:
		return e.CreateMessage(discord.MessageCreate{
			Content: message,
			Flags:   discord.MessageFlagEphemeral,
		})
	case *events.ModalSubmitInteractionCreate:
		return e.CreateMessage(discord.MessageCreate{
			Content: message,
			Flags:   discord.MessageFlagEphemeral,
		})
	default:
		return nil
	}
}
