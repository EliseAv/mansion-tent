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
	fmt.Printf("args: %v\n", os.Args)
	if len(os.Args) > 1 {
		if os.Args[1] == "tent" {
			tent.Tent.Run()
		} else if os.Args[1] == "launch" {
			tower.Launcher.ConsoleLaunch()
		} else if os.Args[1] == "bot" {
			tower.Bot.Run()
		} else {
			fmt.Println("Invalid argument.")
		}
	}
}
