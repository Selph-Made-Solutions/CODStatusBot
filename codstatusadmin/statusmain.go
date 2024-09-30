package codstatusadmin

import (
	"codstatusadmin/logger"
	"encoding/json"
	"fmt"
	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
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
	if err := godotenv.Load(".env.dashboard"); err != nil {
		log.Fatal("Error loading .env.dashboard file")
	}

	if err := initDatabase(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	go StartStatsCaching()

	http.HandleFunc("/admin", DashboardHandler)
	http.HandleFunc("/admin/stats", StatsHandler)

	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
}

func main() {
	log.Println("Starting COD Status Bot Admin Dashboard")

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Admin dashboard listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

var DB *gorm.DB

func initDatabase() error {
	dbUser := os.Getenv("DASHBOARD_DB_USER")
	dbPassword := os.Getenv("DASHBOARD_DB_PASSWORD")
	dbHost := os.Getenv("DASHBOARD_DB_HOST")
	dbPort := os.Getenv("DASHBOARD_DB_PORT")
	dbName := os.Getenv("DASHBOARD_DB_NAME")
	dbVar := os.Getenv("DASHBOARD_DB_VAR")

	if dbUser == "" || dbPassword == "" || dbHost == "" || dbPort == "" || dbName == "" || dbVar == "" {
		return fmt.Errorf("one or more environment variables for dashboard database not set or missing")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s", dbUser, dbPassword, dbHost, dbPort, dbName, dbVar)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to dashboard database: %w", err)
	}

	DB = db
	return nil
}

func StartStatsCaching() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		UpdateCachedStats()
		<-ticker.C
	}
}

func GetCachedStats() Stats {
	cachedStatsLock.RLock()
	defer cachedStatsLock.RUnlock()
	return cachedStats
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
	stats := GetCachedStats()
	logger.Log.WithField("stats", stats).Info("Retrieved cached stats")

	tmpl, err := template.ParseFiles("templates/dashboard.html")
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, stats)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
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
