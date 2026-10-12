package main

import (
	"log"
	"net/http"
	"os"

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
	if err := httpserver.Run(addr, mux); err != nil {
		log.Fatal(err)
	}
}
