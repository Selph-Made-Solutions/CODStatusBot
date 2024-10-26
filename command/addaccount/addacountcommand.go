package addaccount

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
	"github.com/bradselph/CODStatusBot/services"
	"github.com/bradselph/CODStatusBot/utils"

	"github.com/bwmarrin/discordgo"
)

func sanitizeInput(input string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, input)
}

func CommandAddAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !services.IsServiceEnabled("ezcaptcha") && !services.IsServiceEnabled("2captcha") {
		respondToInteraction(s, i, "Account monitoring is currently unavailable as no captcha services are enabled. Please try again later.")
		return
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "add_account_modal",
			Title:    "Add New Account",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "account_title",
							Label:       "Account Title",
							Style:       discordgo.TextInputShort,
							Placeholder: "Enter a title for this account",
							Required:    true,
							MinLength:   1,
							MaxLength:   100,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "sso_cookie",
							Label:       "SSO Cookie",
							Style:       discordgo.TextInputParagraph,
							Placeholder: "Enter the SSO cookie for this account",
							Required:    true,
							MinLength:   1,
							MaxLength:   100,
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

	title := utils.SanitizeInput(strings.TrimSpace(data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value))
	ssoCookie := strings.TrimSpace(data.Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value)
	logger.Log.Infof("Attempting to add account. Title: %s, SSO Cookie length: %d", title, len(ssoCookie))

	if !services.VerifySSOCookie(ssoCookie) {
		logger.Log.Error("Invalid SSO cookie provided")
		respondToInteraction(s, i, "Invalid SSO cookie. Please make sure you've copied the entire cookie value.")
		return
	}

	expirationTimestamp, err := services.DecodeSSOCookie(ssoCookie)
	if err != nil {
		logger.Log.WithError(err).Error("Error decoding SSO cookie")
		respondToInteraction(s, i, fmt.Sprintf("Error processing SSO cookie: %v", err))
		return
	}

	var userID string
	var channelID string
	if i.Member != nil {
		userID = i.Member.User.ID
		channelID = i.ChannelID
	} else if i.User != nil {
		userID = i.User.ID
		channel, err := s.UserChannelCreate(userID)
		if err != nil {
			logger.Log.WithError(err).Error("Error creating DM channel")
			respondToInteraction(s, i, "An error occurred while processing your request.")
			return
		}
		channelID = channel.ID
	} else {
		logger.Log.Error("Interaction doesn't have Member or User")
		respondToInteraction(s, i, "An error occurred while processing your request.")
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
			msg += "Please set up EZCaptcha using /setcaptchaservice before adding accounts."
		} else if services.IsServiceEnabled("2captcha") {
			msg += "Please set up 2Captcha using /setcaptchaservice before adding accounts."
		} else {
			msg += "No captcha services are currently available. Please try again later."
		}
		respondToInteraction(s, i, msg)
		return
	}

	var existingAccount models.Account
	result := database.DB.Where("user_id = ?", userID).First(&existingAccount)

	notificationType := "channel"
	if result.Error == nil {
		notificationType = existingAccount.NotificationType
	} else if i.User != nil {
		notificationType = "dm"
	}

	account := models.Account{
		UserID:              userID,
		Title:               title,
		SSOCookie:           ssoCookie,
		SSOCookieExpiration: expirationTimestamp,
		ChannelID:           channelID,
		NotificationType:    notificationType,
	}

	result = database.DB.Create(&account)
	if result.Error != nil {
		logger.Log.WithError(result.Error).Error("Error creating account")
		respondToInteraction(s, i, "Error creating account. Please try again.")
		return
	}

	logger.Log.Infof("Account added successfully. ID: %d, Title: %s, UserID: %s", account.ID, account.Title, account.UserID)

	formattedExpiration := services.FormatExpirationTime(expirationTimestamp)
	embed := &discordgo.MessageEmbed{
		Title:       "Account Added Successfully",
		Description: fmt.Sprintf("Account '%s' has been added to monitoring. SSO cookie will expire in %s", account.Title, formattedExpiration),
		Color:       0x00FF00, // Green color for success
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	err = services.SendNotification(s, account, embed, "", "account_added")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to send account added notification")
		respondToInteraction(s, i, fmt.Sprintf("Account added successfully, but there was an error sending the confirmation message: %v", err))
		return
	}

	respondToInteraction(s, i, "Account added successfully!")
}

func respondToInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
}
