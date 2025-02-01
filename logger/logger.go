package logger

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
)

var (
	Log           *logrus.Logger
	logFile       *os.File
	lastRotation  time.Time
	rotationMutex sync.Mutex
)

func init() {
	dsn := os.Getenv("SENTRY_DSN")
	debug := os.Getenv("SENTRY_DEBUG") == "true"
	traceRate := 1.0
	if rate, err := strconv.ParseFloat(os.Getenv("SENTRY_TRACES_SAMPLE_RATE"), 64); err == nil {
		traceRate = rate
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		TracesSampleRate: traceRate,
		Debug:            debug,
	})
	if err != nil {
		panic("sentry.Init: " + err.Error())
	}

	Log = logrus.New()
	Log.SetFormatter(&logrus.TextFormatter{
		ForceColors: true,
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "level",
			logrus.FieldKeyMsg:   "message",
		},
	})

	logDir := "logs/"
	err = os.MkdirAll(logDir, 0755)
	if err != nil {
		Log.WithError(err).Fatal("Failed to create log directory")
	}

	rotateLog()

	go checkRotation()
	Log.AddHook(NewSentryHook())

}

type SentryHook struct {
	levels []logrus.Level
}

func NewSentryHook() *SentryHook {
	return &SentryHook{
		levels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
		},
	}
}

func (hook *SentryHook) Levels() []logrus.Level {
	return hook.levels
}

func (hook *SentryHook) Fire(entry *logrus.Entry) error {
	if entry.HasCaller() {
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetExtra("file", entry.Caller.File)
			scope.SetExtra("line", entry.Caller.Line)
			scope.SetExtra("function", entry.Caller.Function)
		})
	}

	event := sentry.Event{
		Message: entry.Message,
		Level:   sentryLevel(entry.Level),
	}

	if err, ok := entry.Data[logrus.ErrorKey]; ok {
		if err, ok := err.(error); ok {
			event.Exception = []sentry.Exception{{
				Value:      err.Error(),
				Type:       "error",
				Stacktrace: sentry.NewStacktrace(),
			}}
		}
	}

	sentry.CaptureEvent(&event)
	return nil
}

func sentryLevel(level logrus.Level) sentry.Level {
	switch level {
	case logrus.PanicLevel:
		return sentry.LevelFatal
	case logrus.FatalLevel:
		return sentry.LevelFatal
	case logrus.ErrorLevel:
		return sentry.LevelError
	case logrus.WarnLevel:
		return sentry.LevelWarning
	case logrus.InfoLevel:
		return sentry.LevelInfo
	case logrus.DebugLevel:
		return sentry.LevelDebug
	default:
		return sentry.LevelInfo
	}
}

func LogAndCapture(message string, args ...interface{}) {
	Log.Info(message)
	sentry.CaptureMessage(message)
	sentry.Flush(2 * time.Second)
}

func rotateLog() {
	rotationMutex.Lock()
	defer rotationMutex.Unlock()

	if logFile != nil {
		err := logFile.Close()
		if err != nil {
			Log.WithError(err).Error("Failed to close log file")
		}
	}

	logDir := "logs/"
	logFileName := filepath.Join(logDir, time.Now().Format("2006-01-02")+".txt")
	newLogFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		Log.WithError(err).Fatal("Failed to open new log file")
	}

	logFile = newLogFile
	mw := io.MultiWriter(os.Stdout, logFile)
	Log.SetOutput(mw)
	lastRotation = time.Now()
}

func checkRotation() {
	for {
		time.Sleep(1 * time.Hour)

		now := time.Now()
		if now.YearDay() != lastRotation.YearDay() {
			rotateLog()
			Log.Info("Log file rotated")
		}
	}
}
