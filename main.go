package main

import (
	"log"
	"mansionTent/tent"
	"mansionTent/tower"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load("mt.env")
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// check command-line arguments
	mode := os.Getenv("TENT_MODE")
	switch mode {
	case "":
		tower.AwsInit()
		tower.Bot.Run()
	case "launch":
		tent.AwsInit()
		tent.Launcher.Run()
	case "dispatch":
		tower.AwsInit()
		tower.Dispatcher.ConsoleLaunch()
	default:
		log.Println("Invalid mode:", mode)
	}
}
