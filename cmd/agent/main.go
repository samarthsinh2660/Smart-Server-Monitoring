package main

import (
	"log"
	"net/http"
	"os"

	"smartserver/internal/httpserver"
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
	if err := httpserver.Run(addr, mux); err != nil {
		log.Fatal(err)
	}
}
