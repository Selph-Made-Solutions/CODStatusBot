package addaccount

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"fmt"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"strings"
	"unicode"
)

func sanitizeInput(input string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, input)
}

func CommandAddAccount(client bot.Client, event *events.ApplicationCommandInteractionCreate, installType models.InstallationType) error {
	return event.CreateModal(discord.NewModalCreateBuilder().
		SetCustomID("add_account_modal").
		SetTitle("Add New Account").
		AddActionRow(discord.TextInputComponent{
			CustomID:    "account_title",
			Label:       "Account Title",
			Style:       discord.TextInputStyleShort,
			Placeholder: "Enter a title for this account",
			Required:    true,
			MinLength:   1,
			MaxLength:   100,
		}).
		AddActionRow(discord.TextInputComponent{
			CustomID:    "sso_cookie",
			Label:       "SSO Cookie",
			Style:       discord.TextInputStyleParagraph,
			Placeholder: "Enter the SSO cookie for this account",
			Required:    true,
			MinLength:   1,
			MaxLength:   4000,
		}).
		Build())
}

func HandleModalSubmit(client bot.Client, event *events.ModalSubmitInteractionCreate) error {
	data := event.Data

	title := sanitizeInput(strings.TrimSpace(data.Text("account_title")))
	ssoCookie := strings.TrimSpace(data.Text("sso_cookie"))

	logger.Log.Infof("Attempting to add account. Title: %s, SSO Cookie length: %d", title, len(ssoCookie))

	if !services.VerifySSOCookie(ssoCookie) {
		logger.Log.Error("Invalid SSO cookie provided")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Invalid SSO cookie. Please make sure you've copied the entire cookie value.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	expirationTimestamp, err := services.DecodeSSOCookie(ssoCookie)
	if err != nil {
		logger.Log.WithError(err).Error("Error decoding SSO cookie")
		return event.CreateMessage(discord.MessageCreate{
			Content: fmt.Sprintf("Error processing SSO cookie: %v", err),
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	userID := event.User().ID
	guildID := event.GuildID()

	var existingAccount models.Account
	result := database.DB.Where("user_id = ?", userID).First(&existingAccount)

	notificationType := "channel"
	if result.Error == nil {
		notificationType = existingAccount.NotificationType
	}

	account := models.Account{
		UserID:              userID.String(),
		Title:               title,
		SSOCookie:           ssoCookie,
		SSOCookieExpiration: expirationTimestamp,
		GuildID:             guildID.String(),
		ChannelID:           event.ChannelID().String(),
		NotificationType:    notificationType,
		InstallationType:    installType,
	}

	result = database.DB.Create(&account)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error creating account")
		return event.CreateMessage(discord.MessageCreate{
			Content: "Error creating account. Please try again.",
			Flags:   discord.MessageFlagEphemeral,
		})
	}

	logger.Log.Infof("Account added successfully. ID: %d, Title: %s, UserID: %s", account.ID, account.Title, account.UserID)

	formattedExpiration := services.FormatExpirationTime(expirationTimestamp)
	return event.CreateMessage(discord.MessageCreate{
		Content: fmt.Sprintf("Account added successfully! SSO cookie will expire in %s", formattedExpiration),
		Flags:   discord.MessageFlagEphemeral,
	})
}
