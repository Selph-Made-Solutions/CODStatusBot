package codstatusadmin

import (
	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
)

var (
	statsLimiter          *limiter.Limiter
	cachedStats           Stats
	cachedStatsLock       sync.RWMutex
	cacheInterval         = 15 * time.Minute
	NotificationCooldowns = make(map[string]time.Time)
	NotificationMutex     sync.Mutex

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

// Total Accounts
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













	// Active Accounts
	if err := DB.Model(&Account{}).Where("is_permabanned = ? AND is_expired_cookie = ?", false, false).Count(&stats.ActiveAccounts).Error; err != nil {
		return stats, fmt.Errorf("failed to get active accounts: %w", err)
	}

	// Banned Accounts
	stats.BannedAccounts = stats.TotalAccounts - stats.ActiveAccounts

	// Total Users
	if err := DB.Model(&UserSettings{}).Count(&stats.TotalUsers).Error; err != nil {
		return stats, fmt.Errorf("failed to get total users: %w", err)
	}

	// Checks Last Hour
	oneHourAgo := time.Now().Add(-1 * time.Hour).Unix()
	if err := DB.Model(&Account{}).Where("last_check > ?", oneHourAgo).Count(&stats.ChecksLastHour).Error; err != nil {
		return stats, fmt.Errorf("failed to get checks last hour: %w", err)
	}

	// Checks Last 24 Hours
	oneDayAgo := time.Now().Add(-24 * time.Hour).Unix()
	if err := DB.Model(&Account{}).Where("last_check > ?", oneDayAgo).Count(&stats.ChecksLast24Hours).Error; err != nil {
		return stats, fmt.Errorf("failed to get checks last 24 hours: %w", err)
	}

	// Total Bans
	if err := DB.Model(&Ban{}).Count(&stats.TotalBans).Error; err != nil {
		return stats, fmt.Errorf("failed to get total bans: %w", err)
	}

	// Recent Bans
	if err := DB.Model(&Ban{}).Where("created_at > ?", time.Now().Add(-24*time.Hour)).Count(&stats.RecentBans).Error; err != nil {
		return stats, fmt.Errorf("failed to get recent bans: %w", err)
	}

	// Average Checks Per Day
	var result struct {
		AvgChecks float64
	}
	if err := DB.Model(&Account{}).Select("COUNT(*) / 1.0 as avg_checks").Where("last_check > ?", oneDayAgo).Scan(&result).Error; err != nil {
		return stats, fmt.Errorf("failed to get average checks per day: %w", err)
	}
	stats.AverageChecksPerDay = result.AvgChecks

	// Most Checked Account
	var mostCheckedAccount Account
	if err := DB.Order("last_check DESC").First(&mostCheckedAccount).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return stats, fmt.Errorf("failed to get most checked account: %w", err)
		}
	} else {
		stats.MostCheckedAccount = mostCheckedAccount.Title
	}

	// Least Checked Account
	var leastCheckedAccount Account
	if err := DB.Order("last_check ASC").First(&leastCheckedAccount).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return stats, fmt.Errorf("failed to get least checked account: %w", err)
		}
	} else {
		stats.LeastCheckedAccount = leastCheckedAccount.Title
	}

	// Total Notifications
	if err := DB.Model(&Account{}).Where("last_notification > 0").Count(&stats.TotalNotifications).Error; err != nil {
		return stats, fmt.Errorf("failed to get total notifications: %w", err)
	}

	// Recent Notifications
	if err := DB.Model(&Account{}).Where("last_notification > ?", oneDayAgo).Count(&stats.RecentNotifications).Error; err != nil {
		return stats, fmt.Errorf("failed to get recent notifications: %w", err)
	}

	// Users with Custom API Key
	if err := DB.Model(&UserSettings{}).Where("captcha_api_key != ''").Count(&stats.UsersWithCustomAPIKey).Error; err != nil {
		return stats, fmt.Errorf("failed to get users with custom API key: %w", err)
	}

	// Average Accounts per User
	var avgAccountsResult struct {
		AvgAccounts float64
	}
	if err := DB.Model(&Account{}).Select("COUNT(DISTINCT id) / COUNT(DISTINCT user_id) as avg_accounts").Scan(&avgAccountsResult).Error; err != nil {
		return stats, fmt.Errorf("failed to get average accounts per user: %w", err)
	}
	stats.AverageAccountsPerUser = avgAccountsResult.AvgAccounts

	// Oldest and Newest Account
	var oldestAccount, newestAccount Account
	if err := DB.Order("created ASC").First(&oldestAccount).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return stats, fmt.Errorf("failed to get oldest account: %w", err)
		}
	} else {
		stats.OldestAccount = time.Unix(oldestAccount.Created, 0)
	}
	if err := DB.Order("created DESC").First(&newestAccount).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return stats, fmt.Errorf("failed to get newest account: %w", err)
		}
	} else {
		stats.NewestAccount = time.Unix(newestAccount.Created, 0)
	}

	// Total Shadowbans
	if err := DB.Model(&Ban{}).Where("status = ?", "Shadowban").Count(&stats.TotalShadowbans).Error; err != nil {
		return stats, fmt.Errorf("failed to get total shadowbans: %w", err)
	}

	// Total Tempbans
	if err := DB.Model(&Ban{}).Where("status = ?", "Temporary").Count(&stats.TotalTempbans).Error; err != nil {
		return stats, fmt.Errorf("failed to get total tempbans: %w", err)
	}

	// Ban Dates and Counts
	var banData []struct {
		Date  time.Time
		Count int64
	}
	if err := DB.Model(&Ban{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("created_at > ?", time.Now().AddDate(0, 0, -30)).
		Group("DATE(created_at)").
		Order("date").
		Scan(&banData).Error; err != nil {
		return stats, fmt.Errorf("failed to get ban dates and counts: %w", err)
	}

	for _, data := range banData {
		stats.BanDates = append(stats.BanDates, data.Date.Format("2006-01-02"))
		stats.BanCounts = append(stats.BanCounts, data.Count)
	}

	return stats, nil
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	httpError := tollbooth.LimitByRequest(statsLimiter, w, r)
	if httpError != nil {
		http.Error(w, httpError.Message, httpError.StatusCode)
		return
	}

	cachedStatsLock.RLock()
	stats := cachedStats
	cachedStatsLock.RUnlock()

	tmpl, err := template.ParseFiles("templates/dashboard.html")
	if err != nil {
		log.Printf("Failed to parse dashboard template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, stats)
	if err != nil {
		log.Printf("Failed to execute dashboard template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	httpError := tollbooth.LimitByRequest(statsLimiter, w, r)
	if httpError != nil {
		http.Error(w, httpError.Message, httpError.StatusCode)
		return
	}

	cachedStatsLock.RLock()
	stats := cachedStats
	cachedStatsLock.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
