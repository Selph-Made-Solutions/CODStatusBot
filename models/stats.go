package models

import (
	"time"

	"gorm.io/gorm"
)

type BotStatistics struct {
	gorm.Model
	Date             time.Time `gorm:"index"`
	CommandsUsed     int
	ActiveUsers      int
	AccountsChecked  int
	StatusChanges    int
	CaptchaUsed      int
	CaptchaErrors    int
	AverageCheckTime float64
}

type CommandStatistics struct {
	gorm.Model
	Date          time.Time `gorm:"index"`
	CommandName   string    `gorm:"index"`
	UsageCount    int
	SuccessCount  int
	ErrorCount    int
	AverageTimeMs float64
}
