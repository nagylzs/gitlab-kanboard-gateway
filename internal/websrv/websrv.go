package websrv

import (
	"encoding/json"
	"fmt"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/webhooks"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

var secretToken string = ""

func RootHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if secretToken != "" {
		token := r.Header.Get("X-Gitlab-Token")
		if token != secretToken {
			http.Error(w, "X-Gitlab-Token does not match.",
				http.StatusUnauthorized)
			return
		}
	}
	var event webhooks.PushEvent
	err := json.NewDecoder(r.Body).Decode(&event)
	if err != nil {
		http.Error(w, "Failed to decode push event request, only push events are supported.",
			http.StatusBadRequest)
		return
	}
	if event.EventName != "push" {
		http.Error(w, fmt.Sprintf("Invalid event '%v' - only 'push' is supported", event.EventName),
			http.StatusBadRequest)
		return
	}

	commits := make([]string, 0)
	for _, commit := range event.Commits {
		commits = append(commits, commit.Message)
	}
	event.CanRetry = true // we can retry (once)
	slog.Info("Received push event", "username", event.UserName, "commits", strings.Join(commits, ", "))
	var ok bool
	select {
	case webhooks.PushQueue <- event:
		ok = true
	case <-time.After(1 * time.Second):
		ok = false
	}

	if !ok {
		http.Error(w, fmt.Sprintf("Could not receive push event, processing queue is full"),
			http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write([]byte("{\"status\":\"OK\"}"))
	if err != nil {
		slog.Error(err.Error())
	}
}

func ServeHttp(cfg config.WebhookConfig) {
	slog.Info("Starting http service", "listen", cfg.ListenAddress)
	secretToken = cfg.SecretToken
	http.HandleFunc("/", RootHandler)
	err := http.ListenAndServe(cfg.ListenAddress, nil)
	if err != nil {
		slog.Error("Error starting http service", "error", err)
	}
}
