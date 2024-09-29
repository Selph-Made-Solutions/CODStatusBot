package codstatusadmin

import (
	"codstatusadmin/logger"
	"encoding/json"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"gorm.io/gorm"
)

var (
	statsLimiter    *limiter.Limiter
	cachedStats     Stats
	cachedStatsLock sync.RWMutex
	cacheInterval   = 15 * time.Minute
)

type Stats struct {
	TotalAccounts          int64     `json:"totalAccounts"`
	ActiveAccounts         int64     `json:"activeAccounts"`
	BannedAccounts         int64     `json:"bannedAccounts"`
	TotalUsers             int64     `json:"totalUsers"`
	ChecksLastHour         int64     `json:"checksLastHour"`
	ChecksLast24Hours      int64     `json:"checksLast24Hours"`
	TotalBans              int64     `json:"totalBans"`
	RecentBans             int64     `json:"recentBans"`
	AverageChecksPerDay    float64   `json:"averageChecksPerDay"`
	TotalNotifications     int64     `json:"totalNotifications"`
	RecentNotifications    int64     `json:"recentNotifications"`
	UsersWithCustomAPIKey  int64     `json:"usersWithCustomAPIKey"`
	AverageAccountsPerUser float64   `json:"averageAccountsPerUser"`
	OldestAccount          time.Time `json:"oldestAccount"`
	NewestAccount          time.Time `json:"newestAccount"`
	TotalShadowbans        int64     `json:"totalShadowbans"`
	TotalTempbans          int64     `json:"totalTempbans"`
	BanDates               []string  `json:"banDates"`
	BanCounts              []int64   `json:"banCounts"`
}

func init() {
	statsLimiter = tollbooth.NewLimiter(1, &limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour})
	go startStatsCaching()
}

func startStatsCaching() {
	ticker := time.NewTicker(cacheInterval)
	defer ticker.Stop()

	for {
		UpdateCachedStats()
		<-ticker.C
	}
}

func UpdateCachedStats() {
	stats, err := GetStats()
	if err != nil {
		logger.Log.WithError(err).Error("Error updating cached stats")
		return
	}

	cachedStatsLock.Lock()
	cachedStats = stats
	cachedStatsLock.Unlock()
}

func GetCachedStats() Stats {
	cachedStatsLock.RLock()
	defer cachedStatsLock.RUnlock()
	return cachedStats
}

func StatsHandler(w http.ResponseWriter, r *http.Request) {
	httpError := tollbooth.LimitByRequest(statsLimiter, w, r)
	if httpError != nil {
		http.Error(w, httpError.Message, httpError.StatusCode)
		return
	}

	stats := GetCachedStats()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		logger.Log.WithError(err).Error("Failed to encode stats")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	logger.Log.Info("Dashboard handler called")
	httpError := tollbooth.LimitByRequest(statsLimiter, w, r)
	if httpError != nil {
		logger.Log.WithError(httpError).Error("Rate limit exceeded")
		http.Error(w, httpError.Message, httpError.StatusCode)
		return
	}

	stats := GetCachedStats()
	logger.Log.WithField("stats", stats).Info("Retrieved cached stats")

	tmpl, err := template.ParseFiles("templates/dashboard.html")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to parse dashboard template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, stats); err != nil {
		logger.Log.WithError(err).Error("Failed to execute dashboard template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
	logger.Log.Info("Dashboard rendered successfully")
}

func GetStats() (Stats, error) {
	var stats Stats
	var err error

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Account{}).Count(&stats.TotalAccounts).Error; err != nil {
			return err
		}

		if err := tx.Model(&Account{}).Where("is_permabanned = ? AND is_expired_cookie = ?", false, false).Count(&stats.ActiveAccounts).Error; err != nil {
			return err
		}

		stats.BannedAccounts = stats.TotalAccounts - stats.ActiveAccounts

		if err := tx.Model(&UserSettings{}).Count(&stats.TotalUsers).Error; err != nil {
			return err
		}

		oneHourAgo := time.Now().Add(-1 * time.Hour).Unix()
		if err := tx.Model(&Account{}).Where("last_check > ?", oneHourAgo).Count(&stats.ChecksLastHour).Error; err != nil {
			return err
		}

		oneDayAgo := time.Now().Add(-24 * time.Hour).Unix()
		if err := tx.Model(&Account{}).Where("last_check > ?", oneDayAgo).Count(&stats.ChecksLast24Hours).Error; err != nil {
			return err
		}

		if err := tx.Model(&Ban{}).Count(&stats.TotalBans).Error; err != nil {
			return err
		}

		if err := tx.Model(&Ban{}).Where("created_at > ?", time.Now().Add(-24*time.Hour)).Count(&stats.RecentBans).Error; err != nil {
			return err
		}

		var result struct {
			AvgChecks float64
		}
		if err := tx.Model(&Account{}).Select("COUNT(*) / 1.0 as avg_checks").Where("last_check > ?", oneDayAgo).Scan(&result).Error; err != nil {
			return err
		}
		stats.AverageChecksPerDay = result.AvgChecks

		if err := tx.Model(&Account{}).Where("last_notification > 0").Count(&stats.TotalNotifications).Error; err != nil {
			return err
		}

		if err := tx.Model(&Account{}).Where("last_notification > ?", oneDayAgo).Count(&stats.RecentNotifications).Error; err != nil {
			return err
		}

		if err := tx.Model(&UserSettings{}).Where("captcha_api_key != ''").Count(&stats.UsersWithCustomAPIKey).Error; err != nil {
			return err
		}

		var avgAccountsResult struct {
			AvgAccounts float64
		}
		if err := tx.Model(&Account{}).Select("COUNT(DISTINCT id) / COUNT(DISTINCT user_id) as avg_accounts").Scan(&avgAccountsResult).Error; err != nil {
			return err
		}
		stats.AverageAccountsPerUser = avgAccountsResult.AvgAccounts

		var oldestAccount, newestAccount Account
		if err := tx.Order("created ASC").First(&oldestAccount).Error; err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if err := tx.Order("created DESC").First(&newestAccount).Error; err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		stats.OldestAccount = time.Unix(oldestAccount.Created, 0)
		stats.NewestAccount = time.Unix(newestAccount.Created, 0)

		if err := tx.Model(&Ban{}).Where("status = ?", "Shadowban").Count(&stats.TotalShadowbans).Error; err != nil {
			return err
		}

		if err := tx.Model(&Ban{}).Where("status = ?", "Temporary").Count(&stats.TotalTempbans).Error; err != nil {
			return err
		}

		var banData []struct {
			Date  time.Time
			Count int64
		}
		if err := tx.Model(&Ban{}).
			Select("DATE(created_at) as date, COUNT(*) as count").
			Where("created_at > ?", time.Now().AddDate(0, 0, -30)).
			Group("DATE(created_at)").
			Order("date").
			Scan(&banData).Error; err != nil {
			return err
		}

		for _, data := range banData {
			stats.BanDates = append(stats.BanDates, data.Date.Format("2006-01-02"))
			stats.BanCounts = append(stats.BanCounts, data.Count)
		}

		return nil
	})

	if err != nil {
		logger.Log.WithError(err).Error("Failed to get stats")
		return stats, err
	}

	return stats, nil
}
