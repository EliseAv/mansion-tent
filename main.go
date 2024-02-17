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
		tower.NewBot().Run()
	case "launch":
		tent.NewLauncher().Run()
	case "dispatch":
		tower.NewDispatcher().ConsoleLaunch()
	default:
		log.Println("Invalid mode:", mode)
	}
}
