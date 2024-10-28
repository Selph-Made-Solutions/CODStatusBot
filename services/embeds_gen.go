package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

func createTempBanLiftedEmbed(account models.Account) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Temporary Ban Lifted", account.Title),
		Description: fmt.Sprintf("The temporary ban for account %s has been lifted. The account is now in good standing.", account.Title),
		Color:       GetColorForStatus(models.StatusGood, false, account.IsCheckDisabled),
		Timestamp:   time.Now().Format(time.RFC3339),
	}
}

func createTempBanEscalatedEmbed(account models.Account) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Temporary Ban Escalated", account.Title),
		Description: fmt.Sprintf("The temporary ban for account %s has been escalated to a permanent ban.", account.Title),
		Color:       GetColorForStatus(models.StatusPermaban, false, account.IsCheckDisabled),
		Timestamp:   time.Now().Format(time.RFC3339),
	}
}

func createTempBanStillActiveEmbed(account models.Account, status models.Status) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s - Temporary Ban Update", account.Title),
		Description: fmt.Sprintf("The temporary ban for account %s is still in effect. Current status: %s", account.Title, status),
		Color:       GetColorForStatus(status, false, account.IsCheckDisabled),
		Timestamp:   time.Now().Format(time.RFC3339),
	}
}

func createAnnouncementEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "Important Update: Changes to COD Status Bot",
		Description: "Due to high demand, we've reached our limit of free EZCaptcha tokens. To ensure continued functionality, we're introducing some changes:",
		Color:       0xFFD700, // Gold color
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "What's Changing",
				Value: "• The check ban feature now requires users to provide their own EZCaptcha API key.\n" +
					"• Without an API key, the bot's check ban functionality will be limited.",
			},
			{
				Name: "How to Get Your Own API Key",
				Value: "1. Sign up at [EZ-Captcha](https://dashboard.ez-captcha.com/#/register?inviteCode=uyNrRgWlEKy) using our referral link.\n" +
					"2. Request a free trial of 10,000 tokens.\n" +
					"3. Use the `/setcaptchaservice` command to set your API key in the bot.",
			},
			{
				Name: "Benefits of Using Your Own API Key",
				Value: "• Uninterrupted access to the check ban feature\n" +
					"• Ability to customize check intervals\n" +
					"• Support the bot's development through our referral program",
			},
			{
				Name: "Next Steps",
				Value: "1. Obtain your API key as soon as possible.\n" +
					"2. Set up your key using the `/setcaptchaservice` command.\n" +
					"3. Adjust your check interval preferences if desired.",
			},
			{
				Name:  "Our Commitment",
				Value: "We're actively exploring ways to maintain a free tier for all users. Your support through the referral program directly contributes to this goal.",
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Thank you for your understanding and continued support!",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func GetColorForStatus(status models.Status, isExpiredCookie bool, isCheckDisabled bool) int {
	if isCheckDisabled {
		return 0xA9A9A9 // Dark Gray for disabled checks
	}
	if isExpiredCookie {
		return 0xFF6347 // Tomato for expired cookie
	}
	switch status {
	case models.StatusPermaban:
		return 0x8B0000 // Dark Red for permanent ban
	case models.StatusShadowban:
		return 0xFFA500 // Orange for shadowban
	case models.StatusTempban:
		return 0xFF8C00 // Dark Orange for temporary ban
	case models.StatusGood:
		return 0x32CD32 // Lime Green for good status
	default:
		return 0x708090 // Slate Gray for unknown status
	}
}

func EmbedTitleFromStatus(status models.Status) string {
	switch status {
	case models.StatusTempban:
		return "TEMPORARY BAN DETECTED"
	case models.StatusPermaban:
		return "PERMANENT BAN DETECTED"
	case models.StatusShadowban:
		return "ACCOUNT UNDER REVIEW (SHADOWBAN)"
	default:
		return "ACCOUNT NOT BANNED"
	}
}

func getStatusDescription(status models.Status, accountTitle string, ban models.Ban) string {
	affectedGames := strings.Split(ban.AffectedGames, ",")
	gamesList := strings.Join(affectedGames, ", ")

	switch status {
	case models.StatusPermaban:
		return fmt.Sprintf("The account %s has been permanently banned.\nAffected games: %s", accountTitle, gamesList)
	case models.StatusShadowban:
		return fmt.Sprintf("The account %s has been placed under review (shadowban).\nAffected games: %s", accountTitle, gamesList)
	case models.StatusTempban:
		return fmt.Sprintf("The account %s is temporarily banned for %s.\nAffected games: %s", accountTitle, ban.TempBanDuration, gamesList)
	default:
		return fmt.Sprintf("The account %s is currently not banned.", accountTitle)
	}
}
