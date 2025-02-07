package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/bradselph/CODStatusBot/models"
	"github.com/bwmarrin/discordgo"
)

func CreateAnnouncementEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "Important Update: Changes to COD Status Bot's Captcha Service",
		Description: "I am excited to announce that I have upgraded the captcha service to provide better reliability and performance:",
		Color:       0xFFD700,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "What's Changing",
				Value: "• I have switched to Capsolver as our primary captcha service provider\n" +
					"• The bot now uses Capsolver's API for improved reliability\n" +
					"• Users can still provide their own API keys for unlimited checks",
			},
			{
				Name: "How to Get Your Own API Key",
				Value: "1. Sign up at [Capsolver](https://dashboard.capsolver.com/passport/register?inviteCode=6YjROhACQnvP)\n" +
					"2. Purchase credits (starting at just $0.001 per solve)\n" +
					"3. Use the `/setcaptchaservice` command to set your API key in the bot",
			},
			{
				Name: "Existing EZCaptcha Users",
				Value: "• If you have an existing EZCaptcha key, it will continue to work\n" +
					"• However, we highly recommend switching to Capsolver for:\n" +
					"  - Better reliability and faster solves\n" +
					"  - Lower cost per solve\n" +
					"  - Improved enterprise support",
			},
			{
				Name: "Benefits of Using Your Own API Key",
				Value: "• Uninterrupted access to the check now feature\n" +
					"• Ability to customize check intervals\n" +
					"• Support the bot's development through our referral program",
			},
			{
				Name: "Next Steps",
				Value: "1. Visit Capsolver to obtain your API key\n" +
					"2. Set up your key using the `/setcaptchaservice` command\n" +
					"3. Adjust your check interval preferences if desired",
			},
			{
				Name: "Our Commitment",
				Value: "I am committed to providing the most reliable and cost-effective service.\n" +
					"I have tried to maintain compatibility with EZCaptcha for existing users, but\n" +
					"I strongly recommend Capsolver for its superior performance and cost benefits.",
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Thank you for using CODStatusbot!",
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
		return 0xFF0000 // Red for permanent ban
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

func GetStatusDescription(status models.Status, accountTitle string, ban models.Ban) string {
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
