package tent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type hooks struct {
	launcher *launcher
	url      string
}

func NewHooks(launcher *launcher) *hooks {
	return &hooks{
		launcher: launcher,
		url:      os.Getenv("WEBHOOK_URL"),
	}
}

func (h *hooks) send(message string) {
	slog.Info("Webhook", "msg", message)
	if h.url == "" {
		return
	}
	// send webhook
	payload := map[string]string{"content": message}
	body, _ := json.Marshal(payload)
	response, err := http.Post(h.url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		slog.Error("Webhook error", "err", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 204 {
		slog.Error("Webhook error", "status", response.Status)
	}
}

func (h *hooks) onLaunched() {
	h.send("Server is ready")
}

func (h *hooks) onSaved() {
	h.launcher.uploadSave()
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
