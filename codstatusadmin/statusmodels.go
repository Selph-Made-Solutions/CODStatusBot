package codstatusadmin

import (
	"time"

	"gorm.io/gorm"
)

type Account struct {
	gorm.Model
	UserID                 string
	ChannelID              string
	Title                  string
	LastStatus             string
	LastCheck              int64
	LastNotification       int64
	LastCookieNotification int64
	SSOCookie              string
	Created                int64
	IsExpiredCookie        bool
	NotificationType       string
	IsPermabanned          bool
	LastCookieCheck        int64
	LastStatusChange       int64
	IsCheckDisabled        bool
	DisabledReason         string
	SSOCookieExpiration    int64
	ConsecutiveErrors      int
	LastSuccessfulCheck    time.Time
	LastErrorTime          time.Time
	Last24HourNotification time.Time
}

type UserSettings struct {
	gorm.Model
	UserID                   string
	CaptchaAPIKey            string
	CheckInterval            int
	NotificationInterval     float64
	CooldownDuration         float64
	StatusChangeCooldown     float64
	HasSeenAnnouncement      bool
	NotificationType         string
	LastNotification         time.Time
	LastBalanceNotification  time.Time
	LastErrorNotification    time.Time
	LastDisabledNotification time.Time
}

type Ban struct {
	gorm.Model
	AccountID       uint
	Status          string
	TempBanDuration string
	AffectedGames   string
}
