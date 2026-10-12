package main

import (
	"log"
	"net/http"
	"os"

	"smartserver/internal/httpserver"
)

func main() {
	addr := os.Getenv("CONTROLLER_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, struct {
			State         string `json:"state"`
			HardwareReady bool   `json:"hardware_ready"`
			Message       string `json:"message"`
		}{
			State:         "UNCONFIGURED",
			HardwareReady: false,
			Message:       "Hardware monitoring is not configured yet.",
		})
	})
	if err := httpserver.Run(addr, mux); err != nil {
		log.Fatal(err)
	}
}
