package main

import (
	"log"
	"net/http"
	"os"

	"smartserver/internal/httpserver"
	"smartserver/internal/metrics"
)

func main() {
	addr := os.Getenv("AGENT_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("read hostname: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	})
	mux.HandleFunc("GET /identity", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, struct {
			Hostname string `json:"hostname"`
		}{Hostname: hostname})
	})
	diskPath := os.Getenv("METRICS_DISK_PATH")
	if diskPath == "" {
		diskPath = "/"
	}
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		sample, err := metrics.Collect(r.Context(), diskPath)
		if err != nil {
			log.Printf("collect metrics: %v", err)
			httpserver.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "metrics unavailable"})
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, sample)
	})
	if err := httpserver.Run(addr, mux); err != nil {
		log.Fatal(err)
	}
}
