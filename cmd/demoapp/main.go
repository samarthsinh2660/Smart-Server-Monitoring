package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"smartserver/internal/httpserver"
	"smartserver/internal/traffic"
)

func main() {
	addr := os.Getenv("DEMOAPP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8000"
	}
	name := os.Getenv("SERVER_NAME")
	if name == "" {
		var err error
		name, err = os.Hostname()
		if err != nil {
			log.Fatalf("read hostname: %v", err)
		}
	}
	mux := http.NewServeMux()
	recorder := traffic.NewRecorder(name)
	maxConcurrency, err := strconv.Atoi(env("DEMOAPP_MAX_CONCURRENCY", "32"))
	if err != nil || maxConcurrency < 1 || maxConcurrency > 256 {
		log.Fatal("DEMOAPP_MAX_CONCURRENCY must be 1..256")
	}
	// Bound CPU workload requests independently of health and statistics probes.
	seats := make(chan struct{}, maxConcurrency)
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, recorder.Snapshot())
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, struct {
			Server  string `json:"server"`
			Message string `json:"message"`
		}{
			Server:  name,
			Message: "Smart Server Monitoring demo app",
		})
	})
	mux.HandleFunc("GET /workload", func(w http.ResponseWriter, r *http.Request) {
		const maxDurationMS = 2000
		durationMS := 250
		if raw := r.URL.Query().Get("duration_ms"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > maxDurationMS {
				httpserver.WriteJSON(w, http.StatusBadRequest, struct {
					Error string `json:"error"`
				}{Error: "duration_ms must be between 1 and 2000"})
				return
			}
			durationMS = parsed
		}
		select {
		case seats <- struct{}{}:
			defer func() { <-seats }()
		default:
			httpserver.WriteJSON(w, http.StatusTooManyRequests, map[string]string{"error": "workload concurrency limit reached"})
			return
		}

		deadline := time.Now().Add(time.Duration(durationMS) * time.Millisecond)
		var checksum uint64
		for time.Now().Before(deadline) {
			for i := uint64(0); i < 10000; i++ {
				checksum = checksum*1664525 + i + 1013904223
			}
			select {
			case <-r.Context().Done():
				return
			default:
			}
		}
		httpserver.WriteJSON(w, http.StatusOK, struct {
			Server     string `json:"server"`
			DurationMS int    `json:"duration_ms"`
			Checksum   uint64 `json:"checksum"`
		}{Server: name, DurationMS: durationMS, Checksum: checksum})
	})
	if err := httpserver.Run(addr, recorder.Wrap(mux)); err != nil {
		log.Fatal(err)
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
