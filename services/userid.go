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
