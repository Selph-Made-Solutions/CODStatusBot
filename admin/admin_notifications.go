package admin

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"time"
)

var adminSession *discordgo.Session

func InitAdminNotifications(token string) error {
	var err error
	adminSession, err = discordgo.New("Bot " + token)
	if err != nil {
		return err
	}
	return adminSession.Open()
}

func NotifyAdmin(message string) {
	adminID := os.Getenv("DEVELOPER_ID")
	if adminID == "" {
		return
	}

	channel, err := adminSession.UserChannelCreate(adminID)
	if err != nil {
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Admin Notification",
		Description: message,
		Color:       0xFF0000, // Red color for admin notifications
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	_, _ = adminSession.ChannelMessageSendEmbed(channel.ID, embed)
}

func NotifyAdminError(errorType, accountTitle string, accountID uint, userID string, errorMessage string) {
	adminID := os.Getenv("DEVELOPER_ID")
	if adminID == "" {
		return
	}

	channel, err := adminSession.UserChannelCreate(adminID)
	if err != nil {
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s Error", errorType),
		Description: fmt.Sprintf("An error occurred for account '%s' (ID: %d)", accountTitle, accountID),
		Color:       0xFF0000, // Red color for errors
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "User ID",
				Value:  userID,
				Inline: true,
			},
			{
				Name:   "Error Message",
				Value:  errorMessage,
				Inline: false,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, _ = adminSession.ChannelMessageSendEmbed(channel.ID, embed)
}
