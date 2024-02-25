package main

import (
	"log/slog"
	"mansionTent/tent"
	"mansionTent/tower"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
)

func main() {
	started := time.Now()

	// color my world 💖
	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      slog.LevelDebug,
		TimeFormat: "Mon _2 15:04:05",
	})))

	err := godotenv.Load("mt.env")
	if err != nil {
		slog.Error("Error loading .env file", "err", err)
		panic(err)
	}

	// check command-line arguments
	mode := os.Getenv("TENT_MODE")
	slog.Debug("Starting", "mode", mode)
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
	slog.Debug("Exiting cleanly", "elapsed", time.Since(started).Round(time.Second))
}
