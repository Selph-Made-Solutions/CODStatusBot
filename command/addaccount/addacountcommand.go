package addaccount

import (
	"fmt"
	"gorm.io/gorm"
	"strings"
	"unicode"

	"CODStatusBot/database"
	"CODStatusBot/errorhandler"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"CODStatusBot/services"
	"CODStatusBot/utils"

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
							MinLength:   35,
							MaxLength:   100,
						},
					},
				},
			},
		},
	})
	if err != nil {
		handleInteractionError(s, i, errorhandler.NewAPIError(err, "Discord"))
	}
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()

	title := utils.SanitizeInput(strings.TrimSpace(data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value))
	ssoCookie := strings.TrimSpace(data.Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value)

	if !services.VerifySSOCookie(ssoCookie) {
		handleInteractionError(s, i, errorhandler.NewValidationError(fmt.Errorf("invalid SSO cookie"), "SSO cookie"))
		return
	}

	expirationTimestamp, err := services.DecodeSSOCookie(ssoCookie)
	if err != nil {
		handleInteractionError(s, i, errorhandler.NewValidationError(err, "SSO cookie"))
		return
	}

	userID, channelID, isUserApplication, err := getUserAndChannelID(s, i)
	if err != nil {
		handleInteractionError(s, i, err)
		return
	}

	notificationType, err := getNotificationType(userID, isUserApplication)
	if err != nil {
		handleInteractionError(s, i, err)
		return
	}

	account := models.Account{
		UserID:              userID,
		Title:               title,
		SSOCookie:           ssoCookie,
		SSOCookieExpiration: expirationTimestamp,
		ChannelID:           channelID,
		NotificationType:    notificationType,
	}

	result := database.DB.Create(&account)
	if result.Error != nil {
		handleInteractionError(s, i, errorhandler.NewDatabaseError(result.Error, "creating account"))
		return
	}

	formattedExpiration := services.FormatExpirationTime(expirationTimestamp)
	respondToInteraction(s, i, fmt.Sprintf("Account added successfully! SSO cookie will expire in %s", formattedExpiration))
}

func getUserAndChannelID(s *discordgo.Session, i *discordgo.InteractionCreate) (string, string, bool, error) {
	var userID, channelID string
	var isUserApplication bool

	if i.Member != nil {
		userID = i.Member.User.ID
		channelID = i.ChannelID
		isUserApplication = false
	} else if i.User != nil {
		userID = i.User.ID
		isUserApplication = true
		// For user applications, we'll use DM as the default channel.
		channel, err := s.UserChannelCreate(userID)
		if err != nil {
			return "", "", false, errorhandler.NewAPIError(err, "Discord")
		}
		channelID = channel.ID
	} else {
		return "", "", false, errorhandler.NewValidationError(fmt.Errorf("interaction doesn't have Member or User"), "user identification")
	}

	return userID, channelID, isUserApplication, nil
}

func getNotificationType(userID string, isUserApplication bool) (string, error) {
	var existingAccount models.Account
	result := database.DB.Where("user_id = ?", userID).First(&existingAccount)

	if result.Error == nil {
		return existingAccount.NotificationType, nil
	} else if result.Error == gorm.ErrRecordNotFound {
		if isUserApplication {
			return "dm", nil
		} else {
			return "channel", nil
		}
	} else {
		return "", errorhandler.NewDatabaseError(
			result.Error,
			"fetching user preference",
		)
	}
}

func handleInteractionError(s *discordgo.Session, i *discordgo.InteractionCreate, err error) {
	userMsg, _ := errorhandler.HandleError(err)
	respondToInteraction(s, i, userMsg)
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
