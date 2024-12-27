package services

import (
	"sync"
	"time"

	"github.com/bradselph/CODStatusBot/logger"
	"github.com/bwmarrin/discordgo"
)

type NotificationQueue struct {
	items    []NotificationItem
	mutex    sync.Mutex
	shutdown chan struct{}
}

type NotificationItem struct {
	UserID     string
	Content    string
	AddedAt    time.Time
	RetryCount int
}

var notificationQueue = &NotificationQueue{
	items:    make([]NotificationItem, 0),
	shutdown: make(chan struct{}),
}

func StartNotificationProcessor(discord *discordgo.Session) {
	go func() {
		for {
			select {
			case <-notificationQueue.shutdown:
				return
			default:
				notificationQueue.processNextNotification(discord)
				time.Sleep(time.Second)
			}
		}
	}()
}

func (q *NotificationQueue) processNextNotification(discord *discordgo.Session) {
	q.mutex.Lock()
	if len(q.items) == 0 {
		q.mutex.Unlock()
		return
	}

	item := q.items[0]
	q.items = q.items[1:]
	q.mutex.Unlock()

	if IsUserRateLimited(item.UserID) {
		if item.RetryCount < 3 {
			item.RetryCount++
			q.AddNotification(item)
		} else {
			logger.Log.Warnf("Dropping notification for user %s after max retries", item.UserID)
		}
		return
	}

	channel, err := discord.UserChannelCreate(item.UserID)
	if err != nil {
		logger.Log.Errorf("Error creating DM channel for user %s: %v", item.UserID, err)
		return
	}

	_, err = discord.ChannelMessageSend(channel.ID, item.Content)
	if err != nil {
		logger.Log.Errorf("Error sending notification to user %s: %v", item.UserID, err)
		return
	}

	adaptiveRateLimits.Lock()
	backoff, exists := adaptiveRateLimits.UserBackoffs[item.UserID]
	if !exists {
		backoff = &UserBackoff{
			BackoffMultiplier: 1.0,
		}
		adaptiveRateLimits.UserBackoffs[item.UserID] = backoff
	}
	backoff.LastSent = time.Now()
	backoff.NotificationHistory = append(backoff.NotificationHistory, time.Now())
	adaptiveRateLimits.Unlock()
}

func (q *NotificationQueue) AddNotification(item NotificationItem) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.items = append(q.items, item)
}

func QueueNotification(userID string, content string) {
	item := NotificationItem{
		UserID:     userID,
		Content:    content,
		AddedAt:    time.Now(),
		RetryCount: 0,
	}
	notificationQueue.AddNotification(item)
}

type UserBackoff struct {
	LastSent            time.Time
	NotificationHistory []time.Time
	BackoffMultiplier   float64
	ConsecutiveCount    int
}

type AdaptiveRateLimits struct {
	sync.RWMutex
	UserBackoffs  map[string]*UserBackoff
	BaseLimit     int
	HistoryWindow time.Duration
}

var adaptiveRateLimits = &AdaptiveRateLimits{
	UserBackoffs:  make(map[string]*UserBackoff),
	BaseLimit:     5,
	HistoryWindow: time.Hour * 24,
}

func (a *AdaptiveRateLimits) GetBackoffDuration(userID string) time.Duration {
	backoff, exists := a.UserBackoffs[userID]
	if !exists {
		return 0
	}

	baseBackoff := time.Minute * 5
	if backoff.BackoffMultiplier <= 1.0 {
		return baseBackoff
	}

	duration := time.Duration(float64(baseBackoff) * backoff.BackoffMultiplier)
	maxBackoff := time.Hour * 1
	if duration > maxBackoff {
		return maxBackoff
	}
	return duration
}

func CleanupOldRateLimitData() {
	adaptiveRateLimits.Lock()
	defer adaptiveRateLimits.Unlock()

	now := time.Now()
	for userID, backoff := range adaptiveRateLimits.UserBackoffs {
		var recentHistory []time.Time
		for _, t := range backoff.NotificationHistory {
			if now.Sub(t) <= adaptiveRateLimits.HistoryWindow {
				recentHistory = append(recentHistory, t)
			}
		}

		if len(recentHistory) == 0 {
			backoff.BackoffMultiplier = 1.0
			backoff.ConsecutiveCount = 0
		}

		backoff.NotificationHistory = recentHistory

		if len(recentHistory) == 0 && now.Sub(backoff.LastSent) > adaptiveRateLimits.HistoryWindow {
			delete(adaptiveRateLimits.UserBackoffs, userID)
		}
	}

	logger.Log.Info("Completed cleanup of rate limit data")
}

func IsUserRateLimited(userID string) bool {
	adaptiveRateLimits.RLock()
	defer adaptiveRateLimits.RUnlock()

	backoff, exists := adaptiveRateLimits.UserBackoffs[userID]
	if !exists {
		return false
	}

	if time.Since(backoff.LastSent) < adaptiveRateLimits.GetBackoffDuration(userID) {
		return true
	}

	if len(backoff.NotificationHistory) >= adaptiveRateLimits.BaseLimit {
		oldest := backoff.NotificationHistory[0]
		if time.Since(oldest) < time.Hour {
			return true
		}
	}

	return false
}

func GetUserRateLimitStatus(userID string) (bool, time.Duration, int) {
	adaptiveRateLimits.RLock()
	defer adaptiveRateLimits.RUnlock()

	backoff, exists := adaptiveRateLimits.UserBackoffs[userID]
	if !exists {
		return false, 0, adaptiveRateLimits.BaseLimit
	}

	isLimited := IsUserRateLimited(userID)
	remainingBackoff := time.Duration(0)
	if isLimited {
		remainingBackoff = adaptiveRateLimits.GetBackoffDuration(userID) - time.Since(backoff.LastSent)
		if remainingBackoff < 0 {
			remainingBackoff = 0
		}
	}

	recentCount := 0
	now := time.Now()
	for _, t := range backoff.NotificationHistory {
		if now.Sub(t) < time.Hour {
			recentCount++
		}
	}
	remainingAllowed := adaptiveRateLimits.BaseLimit - recentCount

	return isLimited, remainingBackoff, remainingAllowed
}
