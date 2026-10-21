package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	modelURL := os.Getenv("MODEL_URL")
	if modelURL == "" {
		modelURL = "http://127.0.0.1:8090"
	}
	modelClient := &http.Client{Timeout: 5 * time.Second}
	poller := &controller.Poller{AgentURL: agentURL, Client: &http.Client{Timeout: 3 * time.Second}, Store: db, Interval: 5 * time.Second}
	appProbe := &controller.HealthProbe{AppURL: appURL, Client: &http.Client{Timeout: 2 * time.Second}, Interval: 5 * time.Second}
	trafficPoller := &controller.TrafficPoller{AppURL: appURL, Client: &http.Client{Timeout: 2 * time.Second}, Store: db, Interval: 5 * time.Second}
	events := &controller.EventMonitor{Store: db, Metrics: poller, App: appProbe, Traffic: trafficPoller}
	go poller.Run(context.Background())
	go appProbe.Run(context.Background())
	go trafficPoller.Run(context.Background())
	go events.Run(context.Background())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		available, lastError, lastSample := poller.State()
		trafficAvailable, _, _ := trafficPoller.State()
		httpserver.WriteJSON(w, http.StatusOK, struct {
			State            string               `json:"state"`
			HardwareReady    bool                 `json:"hardware_ready"`
			Message          string               `json:"message"`
			MetricsAvailable bool                 `json:"metrics_available"`
			MetricsError     string               `json:"metrics_error,omitempty"`
			LastSampleAt     *time.Time           `json:"last_sample_at,omitempty"`
			AppHealth        controller.AppHealth `json:"app_health"`
			TrafficAvailable bool                 `json:"traffic_available"`
		}{
			State:            "UNCONFIGURED",
			HardwareReady:    false,
			Message:          "Hardware monitoring is not configured yet.",
			MetricsAvailable: available,
			MetricsError:     lastError,
			LastSampleAt:     timePointer(lastSample),
			AppHealth:        appProbe.State(),
			TrafficAvailable: trafficAvailable,
		})
	})
	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) {
		limit, ok := readLimit(w, r)
		if !ok {
			return
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
	mux.HandleFunc("GET /api/traffic", func(w http.ResponseWriter, r *http.Request) {
		limit, ok := readLimit(w, r)
		if !ok {
			return
		}
		samples, err := db.RecentTraffic(r.Context(), limit)
		if err != nil {
			log.Printf("read traffic: %v", err)
			httpserver.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "read traffic failed"})
			return
		}
		available, _, _ := trafficPoller.State()
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"available": available, "samples": samples})
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		limit, ok := readLimit(w, r)
		if !ok {
			return
		}
		items, err := db.RecentEvents(r.Context(), limit)
		if err != nil {
			log.Printf("read events: %v", err)
			httpserver.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "read events failed"})
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{"events": items})
	})
	mux.HandleFunc("GET /api/prediction", func(w http.ResponseWriter, r *http.Request) {
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, strings.TrimRight(modelURL, "/")+"/prediction", nil)
		if err != nil {
			httpserver.WriteJSON(w, http.StatusOK, map[string]any{"available": false, "reason": "invalid Python model URL"})
			return
		}
		response, err := modelClient.Do(request)
		if err != nil {
			httpserver.WriteJSON(w, http.StatusOK, map[string]any{"available": false, "reason": "Python model service unavailable"})
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			httpserver.WriteJSON(w, http.StatusOK, map[string]any{"available": false, "reason": "Python model service returned an error"})
			return
		}
		var result map[string]any
		if err := json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&result); err != nil || result == nil {
			httpserver.WriteJSON(w, http.StatusOK, map[string]any{"available": false, "reason": "invalid Python model response"})
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, result)
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

func readLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 1000 {
			httpserver.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be 1 to 1000"})
			return 0, false
		}
		return limit, true
	}
	return 60, true
}
