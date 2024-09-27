package admin

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sync"
	"time"

	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"

	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
)

var (
	statsLimiter    *limiter.Limiter
	cachedStats     Stats
	cachedStatsLock sync.RWMutex
	cacheInterval   = 15 * time.Minute
)

type Stats struct {
	TotalAccounts          int
	ActiveAccounts         int
	BannedAccounts         int
	TotalUsers             int
	ChecksLastHour         int
	ChecksLast24Hours      int
	TotalBans              int
	RecentBans             int
	AverageChecksPerDay    float64
	MostCheckedAccount     string
	LeastCheckedAccount    string
	TotalNotifications     int
	RecentNotifications    int
	UsersWithCustomAPIKey  int
	AverageAccountsPerUser float64
	OldestAccount          time.Time
	NewestAccount          time.Time
	TotalShadowbans        int
	TotalTempbans          int
	BanDates               []string `json:"banDates"`
	BanCounts              []int    `json:"banCounts"`
}

func init() {
	statsLimiter = tollbooth.NewLimiter(1, &limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour})
}

func StartStatsCaching() {
	go func() {
		for {
			updateCachedStats()
			time.Sleep(cacheInterval)
		}
	}()
}

func updateCachedStats() {
	stats, err := getStats()
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
	json.NewEncoder(w).Encode(stats)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, stats)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to execute dashboard template")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	logger.Log.Info("Dashboard rendered successfully")
}

func getStats() (Stats, error) {
	var stats Stats

	stats.TotalAccounts, _ = getTotalAccounts()
	stats.ActiveAccounts, _ = getActiveAccounts()
	stats.TotalUsers, _ = getTotalUsers()
	stats.ChecksLast24Hours, _ = getChecksInTimeRange(24 * time.Hour)
	stats.TotalBans, _ = getTotalBans()
	stats.RecentBans, _ = getRecentBans(24 * time.Hour)
	stats.AverageChecksPerDay, _ = getAverageChecksPerDay()
	stats.TotalNotifications, _ = getTotalNotifications()
	stats.RecentNotifications, _ = getRecentNotifications(24 * time.Hour)
	stats.UsersWithCustomAPIKey, _ = getUsersWithCustomAPIKey()
	stats.AverageAccountsPerUser, _ = getAverageAccountsPerUser()
	stats.OldestAccount, stats.NewestAccount, _ = getAccountAgeRange()
	stats.TotalShadowbans, _ = getTotalBansByType(models.StatusShadowban)
	stats.TotalTempbans, _ = getTotalBansByType(models.StatusTempban)

	var banData []struct {
		Date  time.Time
		Count int
	}
	err := database.DB.Model(&models.Ban{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at > ?", time.Now().AddDate(0, 0, -30)).
		Group("DATE(created_at)").
		Order("date").
		Scan(&banData).Error
	if err != nil {
		return stats, err
	}

	for _, data := range banData {
		stats.BanDates = append(stats.BanDates, data.Date.Format("2006-01-02"))
		stats.BanCounts = append(stats.BanCounts, data.Count)
	}

	return stats, nil
}

func getTotalAccounts() (int, error) {
	var count int64
	err := database.DB.Model(&models.Account{}).Count(&count).Error
	return int(count), err
}

func getActiveAccounts() (int, error) {
	var count int64
	err := database.DB.Model(&models.Account{}).Where("is_permabanned = ? AND is_expired_cookie = ?", false, false).Count(&count).Error
	return int(count), err
}

func getTotalUsers() (int, error) {
	var count int64
	err := database.DB.Model(&models.UserSettings{}).Count(&count).Error
	return int(count), err
}

func getChecksInTimeRange(duration time.Duration) (int, error) {
	var count int64
	timeThreshold := time.Now().Add(-duration).Unix()
	err := database.DB.Model(&models.Account{}).Where("last_check > ?", timeThreshold).Count(&count).Error
	return int(count), err
}

func getTotalBans() (int, error) {
	var count int64
	err := database.DB.Model(&models.Ban{}).Count(&count).Error
	return int(count), err
}

func getRecentBans(duration time.Duration) (int, error) {
	var count int64
	err := database.DB.Model(&models.Ban{}).Where("created_at > ?", time.Now().Add(-duration)).Count(&count).Error
	return int(count), err
}

func getAverageChecksPerDay() (float64, error) {
	var result struct {
		AvgChecks float64
	}
	oneDayAgo := time.Now().Add(-24 * time.Hour).Unix()
	err := database.DB.Model(&models.Account{}).
		Select("COUNT(*) / 1.0 as avg_checks").
		Where("last_check > ?", oneDayAgo).
		Scan(&result).Error
	return result.AvgChecks, err
}

func getTotalNotifications() (int, error) {
	var count int64
	err := database.DB.Model(&models.Account{}).Where("last_notification > 0").Count(&count).Error
	return int(count), err
}

func getRecentNotifications(duration time.Duration) (int, error) {
	var count int64
	err := database.DB.Model(&models.Account{}).Where("last_notification > ?", time.Now().Add(-duration).Unix()).Count(&count).Error
	return int(count), err
}

func getUsersWithCustomAPIKey() (int, error) {
	var count int64
	err := database.DB.Model(&models.UserSettings{}).Where("captcha_api_key != ''").Count(&count).Error
	return int(count), err
}

func getAverageAccountsPerUser() (float64, error) {
	var result struct {
		AvgAccounts float64
	}
	err := database.DB.Model(&models.Account{}).Select("COUNT(DISTINCT id) / COUNT(DISTINCT user_id) as avg_accounts").Scan(&result).Error
	return result.AvgAccounts, err
}

func getAccountAgeRange() (time.Time, time.Time, error) {
	var oldestAccount, newestAccount models.Account
	err := database.DB.Order("created ASC").First(&oldestAccount).Error
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	err = database.DB.Order("created DESC").First(&newestAccount).Error
	return time.Unix(oldestAccount.Created, 0), time.Unix(newestAccount.Created, 0), err
}

func getTotalBansByType(banType models.Status) (int, error) {
	var count int64
	err := database.DB.Model(&models.Ban{}).Where("status = ?", banType).Count(&count).Error
	return int(count), err
}
