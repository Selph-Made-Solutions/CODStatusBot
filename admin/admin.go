package admin

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"time"

	"CODStatusBot/database"
	"CODStatusBot/logger"
	"CODStatusBot/models"

	"github.com/gorilla/mux"
)

func StartAdminPanel() {
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

func statsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := getStats()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to get stats")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := getStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

type Stats struct {
	TotalAccounts     int
	ActiveAccounts    int
	BannedAccounts    int
	TotalUsers        int
	ChecksLastHour    int
	ChecksLast24Hours int
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

	stats.BannedAccounts, err = getBannedAccounts()
	if err != nil {
		return stats, err
	}

	stats.TotalUsers, err = getTotalUsers()
	if err != nil {
		return stats, err
	}

	stats.ChecksLastHour, err = getChecksInTimeRange(time.Hour)
	if err != nil {
		return stats, err
	}

	stats.ChecksLast24Hours, err = getChecksInTimeRange(24 * time.Hour)
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

func getBannedAccounts() (int, error) {
	var count int64
	err := database.DB.Model(&models.Account{}).Where("is_permabanned = ?", true).Count(&count).Error
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
