package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"smartserver/internal/controller"
	"smartserver/internal/httpserver"
	"smartserver/internal/store"
)

func main() {
	addr := os.Getenv("CONTROLLER_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	agentURL := os.Getenv("AGENT_URL")
	if agentURL == "" {
		agentURL = "http://127.0.0.1:8081"
	}
	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "http://127.0.0.1:8000"
	}
	dbPath := os.Getenv("SQLITE_PATH")
	if dbPath == "" {
		dbPath = "data/monitor.sqlite3"
	}
	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	poller := &controller.Poller{AgentURL: agentURL, Client: &http.Client{Timeout: 3 * time.Second}, Store: db, Interval: 5 * time.Second}
	appProbe := &controller.HealthProbe{AppURL: appURL, Client: &http.Client{Timeout: 2 * time.Second}, Interval: 5 * time.Second}
	go poller.Run(context.Background())
	go appProbe.Run(context.Background())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		available, lastError, lastSample := poller.State()
		httpserver.WriteJSON(w, http.StatusOK, struct {
			State            string               `json:"state"`
			HardwareReady    bool                 `json:"hardware_ready"`
			Message          string               `json:"message"`
			MetricsAvailable bool                 `json:"metrics_available"`
			MetricsError     string               `json:"metrics_error,omitempty"`
			LastSampleAt     *time.Time           `json:"last_sample_at,omitempty"`
			AppHealth        controller.AppHealth `json:"app_health"`
		}{
			State:            "UNCONFIGURED",
			HardwareReady:    false,
			Message:          "Hardware monitoring is not configured yet.",
			MetricsAvailable: available,
			MetricsError:     lastError,
			LastSampleAt:     timePointer(lastSample),
			AppHealth:        appProbe.State(),
		})
	})
	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) {
		limit := 60
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 1000 {
				httpserver.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be 1 to 1000"})
				return
			}
			limit = parsed
		}
		samples, err := db.Recent(r.Context(), limit)
		if err != nil {
			log.Printf("read metrics: %v", err)
			httpserver.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "read metrics failed"})
			return
		}
		available, _, _ := poller.State()
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"available": available, "samples": samples})
	})
	mux.HandleFunc("GET /api/metrics/latest", func(w http.ResponseWriter, r *http.Request) {
		sample, err := db.Latest(r.Context())
		if errors.Is(err, sql.ErrNoRows) {
			httpserver.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "no samples"})
			return
		}
		if err != nil {
			log.Printf("read latest metric: %v", err)
			httpserver.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "read metrics failed"})
			return
		}
		available, _, _ := poller.State()
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"available": available, "sample": sample})
	})
	if err := httpserver.Run(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func timePointer(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
