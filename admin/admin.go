package admin

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"sync"
	"time"

	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"

	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"github.com/gorilla/mux"
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
}

func init() {
	statsLimiter = tollbooth.NewLimiter(1, &limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour})
}

func StartAdminPanel() {
	StartStatsCaching()
	r := mux.NewRouter()

	r.HandleFunc("/admin", dashboardHandler).Methods("GET")
	r.HandleFunc("/admin/stats", statsHandler).Methods("GET")

	// Serve static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8080"
	}

	logger.Log.Infof("Admin panel starting on port %s", port)
	err := http.ListenAndServe(":"+port, r)
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to start admin panel")
	}
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

func statsHandler(w http.ResponseWriter, r *http.Request) {
	httpError := tollbooth.LimitByRequest(statsLimiter, w, r)
	if httpError != nil {
		http.Error(w, httpError.Message, httpError.StatusCode)
		return
	}

	stats := GetCachedStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	httpError := tollbooth.LimitByRequest(statsLimiter, w, r)
	if httpError != nil {
		http.Error(w, httpError.Message, httpError.StatusCode)
		return
	}

	stats := GetCachedStats()
	tmpl, err := template.ParseFiles("templates/dashboard.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, stats)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getStats() (Stats, error) {
	var stats Stats
	var err error

	stats.TotalAccounts, err = getTotalAccounts()
	if err != nil {
		return stats, err
	}

	stats.ActiveAccounts, err = getActiveAccounts()
	if err != nil {
		return stats, err
	}

	stats.TotalUsers, err = getTotalUsers()
	if err != nil {
		return stats, err
	}

	stats.ChecksLast24Hours, err = getChecksInTimeRange(24 * time.Hour)
	if err != nil {
		return stats, err
	}

	stats.TotalBans, err = getTotalBans()
	if err != nil {
		return stats, err
	}

	stats.RecentBans, err = getRecentBans(24 * time.Hour)
	if err != nil {
		return stats, err
	}

	stats.AverageChecksPerDay, err = getAverageChecksPerDay()
	if err != nil {
		return stats, err
	}

	stats.TotalNotifications, err = getTotalNotifications()
	if err != nil {
		return stats, err
	}

	stats.RecentNotifications, err = getRecentNotifications(24 * time.Hour)
	if err != nil {
		return stats, err
	}

	stats.UsersWithCustomAPIKey, err = getUsersWithCustomAPIKey()
	if err != nil {
		return stats, err
	}

	stats.AverageAccountsPerUser, err = getAverageAccountsPerUser()
	if err != nil {
		return stats, err
	}

	stats.OldestAccount, stats.NewestAccount, err = getAccountAgeRange()
	if err != nil {
		return stats, err
	}

	stats.TotalShadowbans, err = getTotalBansByType(models.StatusShadowban)
	if err != nil {
		return stats, err
	}

	stats.TotalTempbans, err = getTotalBansByType(models.StatusTempban)
	if err != nil {
		return stats, err
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
	err := database.DB.Model(&models.Account{}).Select("AVG(checks_count) as avg_checks").Scan(&result).Error
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
