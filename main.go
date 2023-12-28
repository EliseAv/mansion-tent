package main

import (
	"fmt"
	"log"
	"mansionTent/tent"
	"mansionTent/tower"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// check command-line arguments
	mode := os.Getenv("TENT_MODE")
	switch mode {
	case "":
		tower.Bot.Run()
	case "launch":
		tent.Launcher.Run()
	case "dispatch":
		tower.Dispatcher.ConsoleLaunch()
	default:
		fmt.Println("Invalid mode:", mode)
	}
}
