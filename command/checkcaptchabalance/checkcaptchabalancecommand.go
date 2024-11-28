package checkcaptchabalance

import (
	"fmt"
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/services"
	"github.com/bwmarrin/discordgo"
)

func CommandCheckCaptchaBalance(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	userSettings, err := services.GetUserSettings(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error fetching user settings")
		respondToInteraction(s, i, "Error fetching your settings. Please try again.")
		return
	}

	if !services.IsServiceEnabled("ezcaptcha") && !services.IsServiceEnabled("2captcha") {
		respondToInteraction(s, i, "No captcha services are currently available. Please try again later.")
		return
	}

	if !services.IsServiceEnabled(userSettings.PreferredCaptchaProvider) {
		msg := fmt.Sprintf("Your preferred captcha service (%s) is currently disabled. ", userSettings.PreferredCaptchaProvider)
		if services.IsServiceEnabled("ezcaptcha") {
			msg += "Please switch to EZCaptcha using /setcaptchaservice."
		} else if services.IsServiceEnabled("2captcha") {
			msg += "Please switch to 2Captcha using /setcaptchaservice."
		}
		respondToInteraction(s, i, msg)
		return
	}

	apiKey, balance, err := services.GetUserCaptchaKey(userID)
	if err != nil {
		logger.Log.WithError(err).Error("Error getting user captcha key")
		respondToInteraction(s, i, fmt.Sprintf("Error checking balance: %v", err))
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "Captcha Service Balance",
		Color: 0x00ff00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Current Service",
				Value:  userSettings.PreferredCaptchaProvider,
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if apiKey == "" {
		embed.Description = "You are currently using the bot's default API key. Consider setting up your own key using /setcaptchaservice for unlimited checks."
		embed.Color = 0xFFA500
	} else {
		embed.Fields = append(embed.Fields,
			&discordgo.MessageEmbedField{
				Name:   "Balance",
				Value:  fmt.Sprintf("%.2f points", balance),
				Inline: true,
			})

		cfg := configuration.Get()
		var threshold float64
		if userSettings.PreferredCaptchaProvider == "ezcaptcha" {
			threshold = cfg.CaptchaService.EZCaptcha.BalanceMin
		} else {
			threshold = cfg.CaptchaService.TwoCaptcha.BalanceMin
		}

		if balance < threshold {
			embed.Description = fmt.Sprintf("⚠️ Your balance is below the recommended threshold of %.2f points. Please recharge soon to avoid service interruption.", threshold)
			embed.Color = 0xFFA500
		}
	}

	lastCheckTime := userSettings.LastBalanceCheck
	if !lastCheckTime.IsZero() {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Last Balance Check",
			Value:  lastCheckTime.Format("2006-01-02 15:04:05"),
			Inline: false,
		})
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		logger.Log.WithError(err).Error("Error responding to interaction")
	}
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
