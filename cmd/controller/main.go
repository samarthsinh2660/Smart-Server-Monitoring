package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"smartserver/internal/config"
	"smartserver/internal/controller"
	"smartserver/internal/httpserver"
	"smartserver/internal/proxy"
	"smartserver/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(config.Env("SQLITE_PATH", "data/monitor.sqlite3"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	primary := controller.NewServer("primary", cfg.PrimaryAgent, cfg.PrimaryApp, db)
	servers := map[string]*controller.Server{"primary": primary}
	var standby *controller.Server
	if cfg.StandbyApp != "" {
		standby = controller.NewServer("standby", cfg.StandbyAgent, cfg.StandbyApp, db)
		servers["standby"] = standby
	}
	for _, server := range servers {
		server.Run(ctx, db)
	}
	prediction := &controller.PredictionPoller{URL: config.Env("MODEL_URL", "http://127.0.0.1:8090"), Client: &http.Client{Timeout: 5 * time.Second}}
	go prediction.Run(ctx)
	scaling := &controller.Scaling{Config: cfg, Router: proxy.Runtime{Socket: cfg.ProxySocket}, Store: db}
	go scaling.Run(ctx, primary, standby, prediction)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		s := primary.Status()
		_, _, lastSample := primary.Metrics.State()
		var lastAt *time.Time
		if !lastSample.IsZero() {
			lastAt = &lastSample
		}
		scale := scaling.Status()
		message := "Monitoring only. Configure standby and enable scaling for hardware control."
		if scale.Enabled {
			message = scale.Reason
		}
		httpserver.WriteJSON(w, 200, map[string]any{"state": scale.State, "hardware_ready": scale.Ready, "message": message, "metrics_available": s.MetricsAvailable, "metrics_error": s.MetricsError, "last_sample_at": lastAt, "traffic_available": s.TrafficAvailable, "app_health": s.App, "scaling": scale})
	})
	mux.HandleFunc("GET /api/servers", func(w http.ResponseWriter, r *http.Request) {
		items := []controller.ServerStatus{primary.Status()}
		if standby != nil {
			items = append(items, standby.Status())
		}
		httpserver.WriteJSON(w, 200, map[string]any{"servers": items})
	})
	selectServer := func(w http.ResponseWriter, r *http.Request) *controller.Server {
		id := r.URL.Query().Get("server")
		if id == "" {
			id = "primary"
		}
		s := servers[id]
		if s == nil {
			httpserver.WriteJSON(w, 400, map[string]string{"error": "unknown or unconfigured server"})
		}
		return s
	}
	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) {
		s := selectServer(w, r)
		if s == nil {
			return
		}
		limit, ok := readLimit(w, r)
		if !ok {
			return
		}
		samples, err := db.RecentFor(r.Context(), s.ID, limit)
		if err != nil {
			log.Printf("read metrics: %v", err)
			httpserver.WriteJSON(w, 500, map[string]string{"error": "read metrics failed"})
			return
		}
		httpserver.WriteJSON(w, 200, map[string]any{"available": s.Status().MetricsAvailable, "samples": samples})
	})
	mux.HandleFunc("GET /api/metrics/latest", func(w http.ResponseWriter, r *http.Request) {
		s := selectServer(w, r)
		if s == nil {
			return
		}
		sample, err := db.LatestFor(r.Context(), s.ID)
		if errors.Is(err, sql.ErrNoRows) {
			httpserver.WriteJSON(w, 404, map[string]string{"error": "no samples"})
			return
		}
		if err != nil {
			httpserver.WriteJSON(w, 500, map[string]string{"error": "read metrics failed"})
			return
		}
		httpserver.WriteJSON(w, 200, map[string]any{"available": s.Status().MetricsAvailable, "sample": sample})
	})
	mux.HandleFunc("GET /api/traffic", func(w http.ResponseWriter, r *http.Request) {
		s := selectServer(w, r)
		if s == nil {
			return
		}
		limit, ok := readLimit(w, r)
		if !ok {
			return
		}
		samples, err := db.RecentTrafficFor(r.Context(), s.ID, limit)
		if err != nil {
			log.Printf("read traffic: %v", err)
			httpserver.WriteJSON(w, 500, map[string]string{"error": "read traffic failed"})
			return
		}
		httpserver.WriteJSON(w, 200, map[string]any{"available": s.Status().TrafficAvailable, "samples": samples})
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		limit, ok := readLimit(w, r)
		if !ok {
			return
		}
		events, err := db.RecentEvents(r.Context(), limit)
		if err != nil {
			httpserver.WriteJSON(w, 500, map[string]string{"error": "read events failed"})
			return
		}
		httpserver.WriteJSON(w, 200, map[string]any{"events": events})
	})
	mux.HandleFunc("GET /api/prediction", func(w http.ResponseWriter, r *http.Request) { httpserver.WriteJSON(w, 200, prediction.State()) })
	mux.HandleFunc("GET /api/scaling", func(w http.ResponseWriter, r *http.Request) { httpserver.WriteJSON(w, 200, scaling.Status()) })
	mux.HandleFunc("POST /api/standby/{action}", func(w http.ResponseWriter, r *http.Request) {
		if !operatorAuthorized(cfg.Token, r.Header.Get("Authorization")) {
			httpserver.WriteJSON(w, 401, map[string]string{"error": "operator authentication required"})
			return
		}
		action := r.PathValue("action")
		if action != "activate" && action != "retry" && action != "reset" {
			httpserver.WriteJSON(w, 400, map[string]string{"error": "unknown standby action"})
			return
		}
		other := controller.ServerStatus{ID: "standby"}
		if standby != nil {
			other = standby.Status()
		}
		actionCtx, done := context.WithTimeout(r.Context(), 3*time.Second)
		defer done()
		if err := scaling.Action(actionCtx, action, primary.Status(), other); err != nil {
			httpserver.WriteJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		httpserver.WriteJSON(w, 202, scaling.Status())
	})
	log.Printf("monitoring %d server(s); automatic scaling enabled: %v", len(servers), cfg.Enabled)
	if err := httpserver.Run(config.Env("CONTROLLER_ADDR", "127.0.0.1:8080"), mux); err != nil {
		log.Print(err)
		cancel()
		os.Exit(1)
	}
}

func operatorAuthorized(token, authorization string) bool {
	return token != "" && strings.HasPrefix(authorization, "Bearer ") && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(authorization, "Bearer ")), []byte(token)) == 1
}

func readLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 1000 {
			httpserver.WriteJSON(w, 400, map[string]string{"error": "limit must be 1 to 1000"})
			return 0, false
		}
		return limit, true
	}
	return 60, true
}
