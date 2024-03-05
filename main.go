package main

import (
	"fmt"
	"log/slog"
	"mansionTent/share"
	"mansionTent/tent"
	"mansionTent/tower"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
)

func main() {
	timer := share.NewPerfTimer()
	loadDotEnv()
	activateLogger()

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

	slog.Info("Exiting cleanly", "elapsed", timer)
}

func loadDotEnv() {
	// in the tent, mt.env and the binary will be in an unwritable directory, and cwd is for writing all the files
	binaryPath, err := os.Executable()
	if err != nil {
		slog.Error("Error getting binary path", "err", err)
		panic(err)
	}
	envFilePath := filepath.Join(filepath.Dir(binaryPath), "mt.env")
	err = godotenv.Load(envFilePath)
	if err != nil {
		slog.Error("Error loading .env file", "err", err)
		panic(err)
	}
}

func activateLogger() {
	level := slog.LevelInfo
	switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}

	// color my world 💖
	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      level,
		TimeFormat: "Mon _2 15:04:05",
	})))
}

func usage() {
	fmt.Printf("Usage: %s <mode>\n", os.Args[0])
	fmt.Println("Modes:")
	fmt.Println("  bot      - Run the bot")
	fmt.Println("  launch   - Launch the server")
	fmt.Println("  dispatch - Dispatch the server")
}
