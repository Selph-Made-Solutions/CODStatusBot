package admin

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
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

	_, _ = adminSession.ChannelMessageSend(channel.ID, fmt.Sprintf("Admin Notification: %s", message))
}

// Use this function for important error notifications
// Example: admin.NotifyAdmin(fmt.Sprintf("Critical error: %v", err))
