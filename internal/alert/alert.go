package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type Level string

const (
	Info  Level = "INFO"
	Warn  Level = "WARN"
	Error Level = "ERROR"
)

type Alerter struct {
	slackURL   string
	tgToken    string
	tgChatID   string
	httpClient *http.Client
}

func New() *Alerter {
	return &Alerter{
		slackURL: os.Getenv("SLACK_WEBHOOK_URL"),
		tgToken:  os.Getenv("TELEGRAM_BOT_TOKEN"),
		tgChatID: os.Getenv("TELEGRAM_CHAT_ID"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (a *Alerter) Send(ctx context.Context, level Level, msg string) {
	slog.Warn("alert", "level", string(level), "msg", msg)
	if a.slackURL != "" {
		go a.sendSlack(msg)
	}
	if a.tgToken != "" && a.tgChatID != "" {
		go a.sendTelegram(fmt.Sprintf("[%s] %s", level, msg))
	}
}

func (a *Alerter) sendSlack(msg string) {
	body, _ := json.Marshal(map[string]string{"text": msg})
	req, err := http.NewRequest(http.MethodPost, a.slackURL, bytes.NewReader(body))
	if err != nil {
		slog.Error("alert: slack request build", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		slog.Error("alert: slack send", "error", err)
		return
	}
	resp.Body.Close()
}

func (a *Alerter) sendTelegram(msg string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", a.tgToken)
	body, _ := json.Marshal(map[string]string{"chat_id": a.tgChatID, "text": msg})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		slog.Error("alert: telegram request build", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		slog.Error("alert: telegram send", "error", err)
		return
	}
	resp.Body.Close()
}
