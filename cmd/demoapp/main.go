package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"smartserver/internal/httpserver"
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
	if err := httpserver.Run(addr, mux); err != nil {
		log.Fatal(err)
	}
}
