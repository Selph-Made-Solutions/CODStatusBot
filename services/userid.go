package services

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func GetUserID(i *discordgo.InteractionCreate) (string, error) {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID, nil
	}
	if i.User != nil {
		return i.User.ID, nil
	}
	return "", fmt.Errorf("unable to determine user ID")
}

func GetChannelID(s *discordgo.Session, i *discordgo.InteractionCreate, userID string) (string, error) {
	if i.ChannelID != "" {
		return i.ChannelID, nil
	}

	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		return "", fmt.Errorf("failed to create DM channel: %w", err)
	}
	return channel.ID, nil
}
