package tent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type hooks struct {
	url string
}

var Hooks hooks

func init() {
	Hooks.url = os.Getenv("WEBHOOK_URL")
}

func (h *hooks) send(message string) {
	fmt.Printf("\033[1;34mWebhook message: %s\033[0m\n", message)
	if h.url == "" {
		return
	}
	// send webhook
	payload := map[string]string{"content": message}
	body, _ := json.Marshal(payload)
	response, err := http.Post(h.url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		fmt.Printf("\033[1;34mWebhook error: %s\033[0m\n", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 204 {
		fmt.Printf("\033[1;34mWebhook error: Status code %s\033[0m\n", response.Status)
	}
}

func (h *hooks) onLaunched() {
	h.send("Server is ready")
}

func (h *hooks) onSaved() {
	Tent.uploadSave()
}

func (h *hooks) onJoined(name string) {
	h.send(fmt.Sprintf("Joined: %s", name))
}

func (h *hooks) onLeft(name string) {
	h.send(fmt.Sprintf("Left: %s", name))
}

func (h *hooks) onDrained(timeLeft time.Duration) {
	const halfSecond = time.Second >> 1
	// round to the nearest second
	timeLeft += halfSecond - (timeLeft+halfSecond)%time.Second
	h.send(fmt.Sprintf("Server is empty, shutting down in %s", timeLeft))
}

func (h *hooks) onQuit() {
	h.send("Server is shutting down")
}
