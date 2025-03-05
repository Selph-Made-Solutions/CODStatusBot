package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/configuration"
	"github.com/bradselph/CODStatusBot/database"
	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bradselph/CODStatusBot/models"
)

var allowedAPIKeys []string

func StartAdminAPI() {
	cfg := configuration.Get()

	if cfg.Admin.APIKey != "" {
		allowedAPIKeys = append(allowedAPIKeys, cfg.Admin.APIKey)
	}

	http.HandleFunc("/api/stats/daily", authMiddleware(getDailyStats))
	http.HandleFunc("/api/stats/users", authMiddleware(getUserStats))
	http.HandleFunc("/api/stats/accounts", authMiddleware(getAccountStats))
	http.HandleFunc("/api/stats/commands", authMiddleware(getCommandStats))
	http.HandleFunc("/api/stats/status", authMiddleware(getStatusStats))
	http.HandleFunc("/api/stats/trends", authMiddleware(getTrendStats))
	http.HandleFunc("/api/health", getHealthStatus)

	go func() {
		addr := ":" + strconv.Itoa(cfg.Admin.Port)
		if addr == ":" || addr == ":0" {
			addr = ":8080"
		}

		logger.Log.Infof("Admin API server started on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			logger.Log.WithError(err).Error("Failed to start admin API server")
		}
	}()
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(allowedAPIKeys) == 0 {
			next(w, r)
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			http.Error(w, "API key required", http.StatusUnauthorized)
			return
		}

		authorized := false
		for _, key := range allowedAPIKeys {
			if apiKey == key {
				authorized = true
				break
			}
		}

		if !authorized {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		cfg := configuration.Get()
		if !checkAPIRateLimit(r.RemoteAddr, cfg.Admin.StatsRateLimit) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}

var apiRateLimiter = struct {
	sync.RWMutex
	requests map[string][]time.Time
}{
	requests: make(map[string][]time.Time),
}

func checkAPIRateLimit(ipAddress string, rateLimit float64) bool {
	apiRateLimiter.Lock()
	defer apiRateLimiter.Unlock()

	now := time.Now()
	minuteWindow := now.Add(-time.Minute)

	if _, exists := apiRateLimiter.requests[ipAddress]; !exists {
		apiRateLimiter.requests[ipAddress] = []time.Time{now}
		return true
	}

	var recentRequests []time.Time
	for _, t := range apiRateLimiter.requests[ipAddress] {
		if t.After(minuteWindow) {
			recentRequests = append(recentRequests, t)
		}
	}

	if float64(len(recentRequests)) >= rateLimit {
		return false
	}

	apiRateLimiter.requests[ipAddress] = append(recentRequests, now)
	return true
}

func parseTimeRange(r *http.Request) (startTime, endTime time.Time) {
	endTime = time.Now()
	startTime = endTime.AddDate(0, 0, -7)

	if start := r.URL.Query().Get("start_date"); start != "" {
		if parsedStart, err := time.Parse("2006-01-02", start); err == nil {
			startTime = parsedStart
		}
	}

	if end := r.URL.Query().Get("end_date"); end != "" {
		if parsedEnd, err := time.Parse("2006-01-02", end); err == nil {
			endTime = parsedEnd.Add(24 * time.Hour)
		}
	}

	return startTime, endTime
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
}

func writeJSONResponse(w http.ResponseWriter, data interface{}) {
	enableCORS(w)
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Log.WithError(err).Error("Failed to encode JSON response")
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
}

func getHealthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		enableCORS(w)
		return
	}

	enableCORS(w)
	writeJSONResponse(w, map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func getDailyStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		enableCORS(w)
		return
	}

	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	stats, err := GetDailyStats(date)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get daily stats")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, stats)
}

func getUserStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		enableCORS(w)
		return
	}

	startTime, endTime := parseTimeRange(r)

	var users []struct {
		UserID       string `json:"user_id"`
		CommandCount int64  `json:"command_count"`
		AccountCount int64  `json:"account_count"`
		IsCustomKey  bool   `json:"is_custom_key"`
		LastActive   string `json:"last_active"`
		InstallType  string `json:"install_type"`
	}

	cmdStatsQuery := database.DB.Model(&models.Analytics{}).
		Select("user_id, COUNT(*) as command_count").
		Where("type = ? AND timestamp BETWEEN ? AND ?", "command", startTime, endTime).
		Group("user_id")

	var userData []struct {
		UserID       string `json:"user_id"`
		CommandCount int64  `json:"command_count"`
	}

	if err := cmdStatsQuery.Find(&userData).Error; err != nil {
		logger.Log.WithError(err).Error("Failed to fetch user command stats")
		http.Error(w, "Error fetching user statistics", http.StatusInternalServerError)
		return
	}

	for _, user := range userData {
		var userSettings models.UserSettings
		var accountCount int64

		database.DB.Model(&models.Account{}).Where("user_id = ?", user.UserID).Count(&accountCount)

		database.DB.Where("user_id = ?", user.UserID).First(&userSettings)

		var lastActive time.Time
		if userSettings.LastGuildInteraction.After(userSettings.LastDirectInteraction) {
			lastActive = userSettings.LastGuildInteraction
		} else {
			lastActive = userSettings.LastDirectInteraction
		}

		hasCustomKey := userSettings.CapSolverAPIKey != "" ||
			userSettings.EZCaptchaAPIKey != "" ||
			userSettings.TwoCaptchaAPIKey != ""

		users = append(users, struct {
			UserID       string `json:"user_id"`
			CommandCount int64  `json:"command_count"`
			AccountCount int64  `json:"account_count"`
			IsCustomKey  bool   `json:"is_custom_key"`
			LastActive   string `json:"last_active"`
			InstallType  string `json:"install_type"`
		}{
			UserID:       user.UserID,
			CommandCount: user.CommandCount,
			AccountCount: accountCount,
			IsCustomKey:  hasCustomKey,
			LastActive:   lastActive.Format(time.RFC3339),
			InstallType:  userSettings.InstallationType,
		})
	}

	writeJSONResponse(w, map[string]interface{}{
		"time_range": map[string]string{
			"start": startTime.Format("2006-01-02"),
			"end":   endTime.Format("2006-01-02"),
		},
		"total_users": len(users),
		"users":       users,
	})
}

func getAccountStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		enableCORS(w)
		return
	}

	var stats []struct {
		Status     string  `json:"status"`
		Count      int64   `json:"count"`
		Percentage float64 `json:"percentage"`
	}

	var totalAccounts int64
	database.DB.Model(&models.Account{}).Count(&totalAccounts)

	database.DB.Model(&models.Account{}).
		Select("last_status as status, COUNT(*) as count").
		Group("last_status").
		Find(&stats)

	for i := range stats {
		if totalAccounts > 0 {
			stats[i].Percentage = float64(stats[i].Count) / float64(totalAccounts) * 100
		}
	}

	var disabledCount int64
	database.DB.Model(&models.Account{}).Where("is_check_disabled = ?", true).Count(&disabledCount)

	var expiredCount int64
	database.DB.Model(&models.Account{}).Where("is_expired_cookie = ?", true).Count(&expiredCount)

	var vipCount int64
	database.DB.Model(&models.Account{}).Where("is_vip = ?", true).Count(&vipCount)

	writeJSONResponse(w, map[string]interface{}{
		"total":            totalAccounts,
		"disabled":         disabledCount,
		"expired_cookies":  expiredCount,
		"vip_accounts":     vipCount,
		"status_breakdown": stats,
	})
}

func getCommandStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		enableCORS(w)
		return
	}

	startTime, endTime := parseTimeRange(r)
	startDateStr := startTime.Format("2006-01-02")
	endDateStr := endTime.Format("2006-01-02")

	var stats []struct {
		CommandName string  `json:"command_name"`
		Count       int64   `json:"count"`
		SuccessRate float64 `json:"success_rate"`
	}

	database.DB.Model(&models.Analytics{}).
		Select("command_name, COUNT(*) as count").
		Where("type = ? AND day BETWEEN ? AND ?", "command", startDateStr, endDateStr).
		Group("command_name").
		Order("count DESC").
		Find(&stats)

	for i := range stats {
		var successCount int64
		database.DB.Model(&models.Analytics{}).
			Where("type = ? AND command_name = ? AND success = ? AND day BETWEEN ? AND ?",
				"command", stats[i].CommandName, true, startDateStr, endDateStr).
			Count(&successCount)

		if stats[i].Count > 0 {
			stats[i].SuccessRate = float64(successCount) / float64(stats[i].Count) * 100
		}
	}

	var totalCommands int64
	var totalSuccessful int64

	database.DB.Model(&models.Analytics{}).
		Where("type = ? AND day BETWEEN ? AND ?", "command", startDateStr, endDateStr).
		Count(&totalCommands)

	database.DB.Model(&models.Analytics{}).
		Where("type = ? AND success = ? AND day BETWEEN ? AND ?",
			"command", true, startDateStr, endDateStr).
		Count(&totalSuccessful)

	writeJSONResponse(w, map[string]interface{}{
		"time_range": map[string]string{
			"start": startDateStr,
			"end":   endDateStr,
		},
		"total_commands": totalCommands,
		"success_rate":   calculatePercentage(totalSuccessful, totalCommands),
		"commands":       stats,
	})
}

func getStatusStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		enableCORS(w)
		return
	}

	startTime, endTime := parseTimeRange(r)
	startDateStr := startTime.Format("2006-01-02")
	endDateStr := endTime.Format("2006-01-02")

	var stats []struct {
		Status         string `json:"status"`
		PreviousStatus string `json:"previous_status"`
		Count          int64  `json:"count"`
	}

	database.DB.Model(&models.Analytics{}).
		Select("status, previous_status, COUNT(*) as count").
		Where("type = ? AND day BETWEEN ? AND ?", "status_change", startDateStr, endDateStr).
		Group("status, previous_status").
		Order("count DESC").
		Find(&stats)

	var statusCounts = make(map[string]int64)
	for _, stat := range stats {
		statusCounts[stat.Status] += stat.Count
	}

	writeJSONResponse(w, map[string]interface{}{
		"time_range": map[string]string{
			"start": startDateStr,
			"end":   endDateStr,
		},
		"status_changes": stats,
		"status_totals":  statusCounts,
	})
}

func getTrendStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		enableCORS(w)
		return
	}

	startTime, endTime := parseTimeRange(r)
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "day"
	}

	if interval != "day" && interval != "week" && interval != "month" {
		interval = "day"
	}

	var results []struct {
		Day           string `json:"day"`
		CommandCount  int64  `json:"command_count"`
		AccountChecks int64  `json:"account_checks"`
		StatusChanges int64  `json:"status_changes"`
		UniqueUsers   int64  `json:"unique_users"`
	}

	currentDate := startTime
	for currentDate.Before(endTime) {
		dateStr := currentDate.Format("2006-01-02")

		var commandCount int64
		var accountChecks int64
		var statusChanges int64
		var uniqueUsers int64

		database.DB.Model(&models.Analytics{}).
			Where("type = ? AND day = ?", "command", dateStr).
			Count(&commandCount)

		database.DB.Model(&models.Analytics{}).
			Where("type = ? AND day = ?", "account_check", dateStr).
			Count(&accountChecks)

		database.DB.Model(&models.Analytics{}).
			Where("type = ? AND day = ?", "status_change", dateStr).
			Count(&statusChanges)

		database.DB.Model(&models.Analytics{}).
			Where("day = ?", dateStr).
			Distinct("user_id").
			Count(&uniqueUsers)

		results = append(results, struct {
			Day           string `json:"day"`
			CommandCount  int64  `json:"command_count"`
			AccountChecks int64  `json:"account_checks"`
			StatusChanges int64  `json:"status_changes"`
			UniqueUsers   int64  `json:"unique_users"`
		}{
			Day:           dateStr,
			CommandCount:  commandCount,
			AccountChecks: accountChecks,
			StatusChanges: statusChanges,
			UniqueUsers:   uniqueUsers,
		})

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	if interval == "week" || interval == "month" {
		results = aggregateData(results, interval)
	}

	writeJSONResponse(w, map[string]interface{}{
		"time_range": map[string]string{
			"start": startTime.Format("2006-01-02"),
			"end":   endTime.Format("2006-01-02"),
		},
		"interval": interval,
		"trends":   results,
	})
}

func aggregateData(dailyData []struct {
	Day           string `json:"day"`
	CommandCount  int64  `json:"command_count"`
	AccountChecks int64  `json:"account_checks"`
	StatusChanges int64  `json:"status_changes"`
	UniqueUsers   int64  `json:"unique_users"`
}, interval string) []struct {
	Day           string `json:"day"`
	CommandCount  int64  `json:"command_count"`
	AccountChecks int64  `json:"account_checks"`
	StatusChanges int64  `json:"status_changes"`
	UniqueUsers   int64  `json:"unique_users"`
} {
	var aggregated []struct {
		Day           string `json:"day"`
		CommandCount  int64  `json:"command_count"`
		AccountChecks int64  `json:"account_checks"`
		StatusChanges int64  `json:"status_changes"`
		UniqueUsers   int64  `json:"unique_users"`
	}

	if len(dailyData) == 0 {
		return aggregated
	}

	aggregate := make(map[string]struct {
		CommandCount  int64
		AccountChecks int64
		StatusChanges int64
		UniqueUsers   int64
		Count         int
	})

	for _, data := range dailyData {
		date, err := time.Parse("2006-01-02", data.Day)
		if err != nil {
			continue
		}

		var key string
		if interval == "week" {
			year, week := date.ISOWeek()
			key = fmt.Sprintf("%d-W%02d", year, week)
		} else {
			key = date.Format("2006-01")
		}

		entry := aggregate[key]
		entry.CommandCount += data.CommandCount
		entry.AccountChecks += data.AccountChecks
		entry.StatusChanges += data.StatusChanges
		entry.UniqueUsers = max(entry.UniqueUsers, data.UniqueUsers)
		entry.Count++
		aggregate[key] = entry
	}

	for key, entry := range aggregate {
		aggregated = append(aggregated, struct {
			Day           string `json:"day"`
			CommandCount  int64  `json:"command_count"`
			AccountChecks int64  `json:"account_checks"`
			StatusChanges int64  `json:"status_changes"`
			UniqueUsers   int64  `json:"unique_users"`
		}{
			Day:           key,
			CommandCount:  entry.CommandCount,
			AccountChecks: entry.AccountChecks,
			StatusChanges: entry.StatusChanges,
			UniqueUsers:   entry.UniqueUsers,
		})
	}

	return aggregated
}

func calculatePercentage(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
