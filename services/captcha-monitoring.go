package services

import (
	"CODStatusBot/models"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	captchaSolveTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "captcha_solve_attempts_total",
			Help: "Total number of captcha solve attempts",
		},
		[]string{"user_id", "status"},
	)

	captchaSolveErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "captcha_solve_errors_total",
			Help: "Total number of captcha solve errors by type",
		},
		[]string{"user_id", "error_type"},
	)

	captchaSolveDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "captcha_solve_duration_seconds",
			Help:    "Time spent solving captchas",
			Buckets: prometheus.LinearBuckets(0, 1, 10),
		},
		[]string{"user_id"},
	)

	checkRequestErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "check_request_errors_total",
			Help: "Total number of check request errors by type",
		},
		[]string{"user_id", "error_type"},
	)

	userRateTracker = &UserRateTracker{
		rates: make(map[string]*UserRate),
	}
)

type UserRate struct {
	LastCheck      time.Time
	CheckCount     int
	FailureCount   int
	LastFailure    time.Time
	CaptchaCost    float64
	LastCaptchaKey string
}

type UserRateTracker struct {
	mu    sync.RWMutex
	rates map[string]*UserRate
}

func (t *UserRateTracker) TrackCheck(userID string, success bool, captchaCost float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	rate, exists := t.rates[userID]
	if !exists {
		rate = &UserRate{}
		t.rates[userID] = rate
	}

	now := time.Now()
	rate.LastCheck = now
	rate.CheckCount++
	rate.CaptchaCost += captchaCost

	if !success {
		rate.FailureCount++
		rate.LastFailure = now
	}
}

type ErrorType int

const (
	ErrorCaptchaSolve ErrorType = iota
	ErrorHTTPRequest
	ErrorRateLimit
	ErrorInvalidResponse
)

func (e ErrorType) String() string {
	switch e {
	case ErrorCaptchaSolve:
		return "captcha_solve"
	case ErrorHTTPRequest:
		return "http_request"
	case ErrorRateLimit:
		return "rate_limit"
	case ErrorInvalidResponse:
		return "invalid_response"
	default:
		return "unknown"
	}
}

func shouldRetryRequest(err error) bool {
	if err == nil {
		return false
	}

	retryableErrors := []string{
		"connection reset",
		"connection refused",
		"no such host",
		"timeout",
		"temporary failure",
		"read: connection reset",
		"write: broken pipe",
	}

	errStr := err.Error()
	for _, retryErr := range retryableErrors {
		if strings.Contains(strings.ToLower(errStr), retryErr) {
			return true
		}
	}

	return false
}

func makeRequestWithTracking(ctx context.Context, userID, ssoCookie, gRecaptchaResponse string) (models.Status, error) {
	timer := prometheus.NewTimer(captchaSolveDuration.WithLabelValues(userID))
	defer timer.ObserveDuration()

	status, err := makeAccountCheckRequest(ssoCookie, gRecaptchaResponse)

	if err != nil {
		errorType := classifyError(err)
		checkRequestErrors.WithLabelValues(userID, errorType.String()).Inc()

		userRateTracker.TrackCheck(userID, false, 0)
		return status, err
	}

	userRateTracker.TrackCheck(userID, true, getCaptchaCost(userID))
	return status, nil
}

func classifyError(err error) ErrorType {
	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "captcha"):
		return ErrorCaptchaSolve
	case strings.Contains(errStr, "rate limit"):
		return ErrorRateLimit
	case strings.Contains(errStr, "invalid response"):
		return ErrorInvalidResponse
	default:
		return ErrorHTTPRequest
	}
}

func GetUserStats(userID string) (*UserRate, error) {
	userRateTracker.mu.RLock()
	defer userRateTracker.mu.RUnlock()

	rate, exists := userRateTracker.rates[userID]
	if !exists {
		return nil, fmt.Errorf("no stats found for user %s", userID)
	}

	return rate, nil
}
