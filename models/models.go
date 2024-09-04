package models

import (
	"gorm.io/gorm"
)

type Account struct {
	gorm.Model
	GuildID                string           `gorm:"index"` // The ID of the guild the account belongs to.
	UserID                 string           `gorm:"index"` // The ID of the user.
	ChannelID              string           // The ID of the channel associated with the account.
	Title                  string           // The title of the account.
	LastStatus             Status           `gorm:"default:unknown"` // The last known status of the account.
	LastCheck              int64            `gorm:"default:0"`       // The timestamp of the last check performed on the account.
	LastNotification       int64            // The timestamp of the last daily notification sent out on the account.
	LastCookieNotification int64            // The timestamp of the last notification sent out on the account for an expired ssocookie.
	SSOCookie              string           // The SSO cookie associated with the account.
	SSOCookieExpiration    int64            // The timestamp of the expiration of the SSO cookie.
	Created                string           // The timestamp of when the account was created on Activision.
	IsExpiredCookie        bool             `gorm:"default:false"`   // A flag indicating if the SSO cookie has expired.
	NotificationType       string           `gorm:"default:channel"` // User preference for location of notifications either channel or dm
	IsPermabanned          bool             `gorm:"default:false"`   // A flag indicating if the account is permanently banned
	LastCookieCheck        int64            `gorm:"default:0"`       // The timestamp of the last cookie check for permanently banned accounts
	LastStatusChange       int64            `gorm:"default:0"`       // The timestamp of the last status change
	IsCheckDisabled        bool             `gorm:"default:false"`   // A flag indicating if checks are disabled for this account
	InstallationType       InstallationType `gorm:"default:0"`       // The type of installation for the account.
}

type Ban struct {
	gorm.Model
	Account   Account // The account that has been banned.
	AccountID uint    // The ID of the banned account.
	Status    Status  // The status of the ban.
}

type UserSettings struct {
	gorm.Model
	UserID               string           `gorm:"uniqueIndex"`
	CaptchaAPIKey        string           // User's own API key, if provided
	CheckInterval        int              // in minutes
	NotificationInterval float64          // in hours
	CooldownDuration     float64          // in hours
	StatusChangeCooldown float64          // in hours
	NotificationType     string           `gorm:"default:channel"` // User preference for location of notifications either channel or dm
	HasSeenAnnouncement  bool             `gorm:"default:false"`   // Flag to track if the user has seen the global announcement
	InstallationType     InstallationType `gorm:"default:0"`       // The type of installation for the user settings.
}

type InstallationType int

const (
	InstallTypeUnknown InstallationType = iota // Unknown installation type
	InstallTypeUser                            // User installation
	InstallTypeGuild                           // Guild installation
)

type Status string

const (
	StatusGood               Status = "good"           // The account is in good standing.
	StatusPermaban           Status = "permaban"       // The account has been permanently banned.
	StatusShadowban          Status = "shadowban"      // The account has been shadowbanned.
	StatusTempban            Status = "tempban"        // The account is temporarily banned
	StatusUnknown            Status = "unknown"        // The status of the account is unknown.
	StatusInvalidCookie      Status = "invalid_cookie" // The account has an invalid SSO cookie.
	StatusUnderInvestigation Status = "under_investigation"
	StatusBanFinal           Status = "ban_final"
)

type AccountStatus struct {
	Overall Status                `json:"overall"` // The overall status of the account.
	Games   map[string]GameStatus `json:"games"`   // A map of game titles to their status.
}

type GameStatus struct {
	Title           string `json:"title"`           // The title of the game.
	Status          Status `json:"status"`          // The status of the account.
	Enforcement     string `json:"enforcement"`     // The enforcement status of the account.
	CanAppeal       bool   `json:"canAppeal"`       // whether the account can appeal the ban
	CaseNumber      string `json:"caseNumber"`      // Case number of the account
	CaseStatus      string `json:"caseStatus"`      // Case status of the account
	DurationSeconds int    `json:"durationSeconds"` // Duration of temporary ban in seconds
}
type BotStatistics struct {
	gorm.Model
	TotalAccounts        int
	ActiveAccounts       int
	TotalUsers           int
	TotalGuilds          int
	TotalChecksPerformed int64
	TotalStatusChanges   int64
	AverageCheckTime     float64
}

func (BotStatistics) TableName() string {
	return "bot_statistics"
}

type CommandUsage struct {
	gorm.Model
	CommandName string
	UsageCount  int64
}

func (CommandUsage) TableName() string {
	return "command_usage"
}

type ErrorLog struct {
	gorm.Model
	ErrorMessage string
	StackTrace   string
	UserID       string
	CommandName  string
}

func (ErrorLog) TableName() string {
	return "error_logs"
}
