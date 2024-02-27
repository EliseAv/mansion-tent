package main

import (
	"fmt"
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
	main := make(map[string]func())
	main["bot"] = tower.RunBot
	main["launch"] = tent.RunLauncher
	main["dispatch"] = tower.RunDispatcher
	var command string
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	f, ok := main[command]
	if !ok {
		usage()
		os.Exit(1)
	}
	f()
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

func usage() {
	fmt.Printf("Usage: %s <mode>\n", os.Args[0])
	fmt.Println("Modes:")
	fmt.Println("  bot      - Run the bot")
	fmt.Println("  launch   - Launch the server")
	fmt.Println("  dispatch - Dispatch the server")
}
