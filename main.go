package main

import (
	"log/slog"
	"mansionTent/share"
	"mansionTent/tent"
	"mansionTent/tower"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
)

func main() {
	timer := share.NewPerfTimer()

	err := godotenv.Load("mt.env")
	if err != nil {
		slog.Error("Error loading .env file", "err", err)
		panic(err)
	}

	// color my world 💖
	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      parseLevel(os.Getenv("LOG_LEVEL")),
		TimeFormat: "Mon _2 15:04:05",
	})))

	// check command-line arguments
	mode := os.Getenv("TENT_MODE")
	slog.Info("Starting", "mode", mode)
	switch mode {
	case "":
		tower.NewBot().Run()
	case "launch":
		tent.NewLauncher().Run()
	case "dispatch":
		tower.NewDispatcher().ConsoleLaunch()
	default:
		slog.Error("Failed to start", "mode", mode)
	}
	slog.Info("Exiting cleanly", "elapsed", timer.Elapsed())
}

func parseLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
