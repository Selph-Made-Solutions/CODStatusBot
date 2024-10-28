package addaccount

import (
	"fmt"
	"os"
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

/*
	func sanitizeInput(input string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '-' || r == '_' {
				return r
			}
			return -1
		}, input)
	}
*/
var (
	rateLimiter = make(map[string]time.Time)
	rateLimit   = 5 * time.Minute
)

func getMaxAccounts(hasCustomKey bool) int {
	if hasCustomKey {
		premiumMax, err := strconv.Atoi(os.Getenv("PREMIUM_USER_MAXACCOUNTS"))
		if err != nil || premiumMax <= 0 {
			logger.Log.WithError(err).Info("Using default premium max accounts value")
			return 25
		}
		return premiumMax
	}

	defaultMax, err := strconv.Atoi(os.Getenv("DEFAULT_USER_MAXACCOUNTS"))
	if err != nil || defaultMax <= 0 {
		logger.Log.WithError(err).Info("Using default max accounts value")
		return 10
	}
	return defaultMax
}

func CommandAddAccount(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID := getUserID(i)
	if userID == "" {
		logger.Log.Error("Failed to get user ID")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	if !checkRateLimit(userID) {
		respondToInteraction(s, i, fmt.Sprintf("Please wait %v before adding another account.", rateLimit))
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

	if userSettings.EZCaptchaAPIKey != "" || userSettings.TwoCaptchaAPIKey != "" {
		_, balance, err := services.GetUserCaptchaKey(userID)
		if err != nil {
			logger.Log.WithError(err).Error("Error checking captcha balance")
			respondToInteraction(s, i, "Error validating your captcha API key. Please check your key using /setcaptchaservice.")
			return
		}

		if balance <= 0 {
			respondToInteraction(s, i, fmt.Sprintf("Your captcha balance (%.2f) is too low to add new accounts. Please recharge your balance.", balance))
			return
		}
	}

	var accountCount int64
	if err := database.DB.Model(&models.Account{}).Where("user_id = ?", userID).Count(&accountCount).Error; err != nil {
		logger.Log.WithError(err).Error("Error counting user accounts")
		respondToInteraction(s, i, "Error checking account limit. Please try again.")
		return
	}

	hasCustomKey := userSettings.EZCaptchaAPIKey != "" || userSettings.TwoCaptchaAPIKey != ""
	maxAccounts := getMaxAccounts(hasCustomKey)

	if accountCount >= int64(maxAccounts) {
		msg := fmt.Sprintf("You've reached the maximum limit of %d accounts.", maxAccounts)
		if !hasCustomKey {
			msg += " Upgrade to premium by adding your own API key using /setcaptchaservice to increase your account limit!"
		} else {
			msg += " Please remove some accounts before adding new ones."
		}
		respondToInteraction(s, i, msg)
		return
	}

	showAddAccountModal(s, i)
}

func showAddAccountModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
		logger.Log.WithError(err).Error("Error showing add account modal")
	}
}

func HandleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	userID := getUserID(i)
	if userID == "" {
		logger.Log.Error("Failed to get user ID")
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

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

	channelID := getChannelID(s, i)
	if channelID == "" {
		respondToInteraction(s, i, "An error occurred while processing your request.")
		return
	}

	userSettings, err := services.GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		respondToInteraction(s, i, "Error fetching user settings. Please try again.")
		return
	}

	var accountCount int64
	if err := database.DB.Model(&models.Account{}).Where("user_id = ?", userID).Count(&accountCount).Error; err != nil {
		logger.Log.WithError(err).Error("Error counting user accounts")
		respondToInteraction(s, i, "Error checking account limit. Please try again.")
		return
	}

	hasCustomKey := userSettings.EZCaptchaAPIKey != "" || userSettings.TwoCaptchaAPIKey != ""
	maxAccounts := getMaxAccounts(hasCustomKey)

	if accountCount >= int64(maxAccounts) {
		msg := fmt.Sprintf("You've reached the maximum limit of %d accounts.", maxAccounts)
		if !hasCustomKey {
			msg += " Upgrade to premium by adding your own API key using /setcaptchaservice to increase your account limit!"
		} else {
			msg += " Please remove some accounts before adding new ones."
		}
		respondToInteraction(s, i, msg)
		return
	}

	account := models.Account{
		UserID:              userID,
		Title:               title,
		SSOCookie:           ssoCookie,
		SSOCookieExpiration: expirationTimestamp,
		ChannelID:           channelID,
		NotificationType:    userSettings.NotificationType,
		LastSuccessfulCheck: time.Now(),
	}

	if err := database.DB.Create(&account).Error; err != nil {
		logger.Log.WithError(err).Error("Error creating account")
		respondToInteraction(s, i, "Error creating account. Please try again.")
		return
	}

	isVIP, vipErr := services.CheckVIPStatus(ssoCookie)
	vipStatus := "Regular Account"
	if vipErr == nil && isVIP {
		vipStatus = "VIP Account ⭐"
	}

	remainingSlots := maxAccounts - int(accountCount) - 1
	slotInfo := fmt.Sprintf("\nYou have %d account slot(s) remaining.", remainingSlots)

	embed := &discordgo.MessageEmbed{
		Title:       "Account Added Successfully",
		Description: fmt.Sprintf("Account '%s' has been added to monitoring.%s", account.Title, slotInfo),
		Color:       0x00ff00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Account Type",
				Value:  vipStatus,
				Inline: true,
			},
			{
				Name:   "Cookie Expiration",
				Value:  services.FormatExpirationTime(expirationTimestamp),
				Inline: true,
			},
			{
				Name:   "Notification Type",
				Value:  account.NotificationType,
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Use /listaccounts to view all your monitored accounts",
		},
	}

	if err := services.SendNotification(s, account, embed, "", "account_added"); err != nil {
		logger.Log.WithError(err).Error("Failed to send account added notification")
		respondToInteraction(s, i, fmt.Sprintf("Account added successfully, but there was an error sending the confirmation message: %v", err))
		return
	}

	respondToInteraction(s, i, "Account added successfully!")

	go func() {
		time.Sleep(2 * time.Second)
		if status, err := services.CheckAccount(ssoCookie, userID, ""); err == nil {
			account.LastStatus = status
			account.LastCheck = time.Now().Unix()
			database.DB.Save(&account)
		}
	}()
}

func getUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func getChannelID(s *discordgo.Session, i *discordgo.InteractionCreate) string {
	userID := getUserID(i)
	if userID == "" {
		return ""
	}

	if i.ChannelID != "" {
		return i.ChannelID
	}

	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating DM channel")
		return ""
	}
	return channel.ID
}

func checkRateLimit(userID string) bool {
	lastUse, exists := rateLimiter[userID]
	if !exists || time.Since(lastUse) >= rateLimit {
		rateLimiter[userID] = time.Now()
		return true
	}
	return false
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
