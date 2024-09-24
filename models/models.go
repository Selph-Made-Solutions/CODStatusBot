package models

import (
	"gorm.io/gorm"
	"time"
)

type Account struct {
	gorm.Model
	UserID                 string `gorm:"index"` // The ID of the user.
	ChannelID              string // The ID of the channel associated with the account.
	Title                  string // The title of the account.
	LastStatus             Status `gorm:"default:unknown"` // The last known status of the account.
	LastCheck              int64  `gorm:"default:0"`       // The timestamp of the last check performed on the account.
	LastNotification       int64  // The timestamp of the last daily notification sent out on the account.
	LastCookieNotification int64  // The timestamp of the last notification sent out on the account for an expired ssocookie.
	SSOCookie              string // The SSO cookie associated with the account.
	Created                int64  // The timestamp of when the account was created on Activision.
	IsExpiredCookie        bool   `gorm:"default:false"`   // A flag indicating if the SSO cookie has expired.
	NotificationType       string `gorm:"default:channel"` // User preference for location of notifications either channel or dm
	IsPermabanned          bool   `gorm:"default:false"`   // A flag indicating if the account is permanently banned
	LastCookieCheck        int64  `gorm:"default:0"`       // The timestamp of the last cookie check for permanently banned accounts.
	LastStatusChange       int64  `gorm:"default:0"`       // The timestamp of the last status change
	IsCheckDisabled        bool   `gorm:"default:false"`   // A flag indicating if checks are disabled for this account
	DisabledReason         string
	SSOCookieExpiration    int64
	ConsecutiveErrors      int `gorm:"default:0"`
	LastSuccessfulCheck    time.Time
	LastErrorTime          time.Time
}

type UserSettings struct {
	gorm.Model
	UserID                  string    `gorm:"uniqueIndex"`
	CaptchaAPIKey           string    // User's own API key, if provided
	CheckInterval           int       // In minutes
	NotificationInterval    float64   // In hours
	CooldownDuration        float64   // In hours
	NotificationType        string    `gorm:"default:channel"` // User preference for location of notifications either channel or dm
	StatusChangeCooldown    float64   // In hours
	HasSeenAnnouncement     bool      `gorm:"default:false"` // Flag to track if the user has seen the global announcement.
	LastBalanceNotification time.Time // Timestamp of the last balance notification

}

type Ban struct {
	gorm.Model
	Account         Account // The account that has a status history.
	AccountID       uint    // The ID of the account.
	Status          Status  // The status of the ban.
	TempBanDuration string  // Duration of the temporary ban (if applicable)
	AffectedGames   string  // Comma-separated list of affected games
}

type Status string

const (
	StatusGood          Status = "Good"           // The account status returned as good standing.
	StatusPermaban      Status = "Permaban"       // The account status returned as permanent ban.
	StatusShadowban     Status = "Shadowban"      // The account status returned as shadowban.
	StatusUnknown       Status = "Unknown"        // The account status not known.
	StatusInvalidCookie Status = "Invalid_Cookie" // The account has an invalid SSO cookie.
	StatusTempban       Status = "Temporary"      // The account status returned as temporarily banned.
)
