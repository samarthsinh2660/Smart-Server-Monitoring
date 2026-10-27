package controller

import (
	"context"
	"net/http"
	"time"

	"smartserver/internal/metrics"
	"smartserver/internal/store"
)

type Server struct {
	ID      string
	Metrics *Poller
	App     *HealthProbe
	Traffic *TrafficPoller
}

type ServerStatus struct {
	ID               string          `json:"id"`
	MetricsAvailable bool            `json:"metrics_available"`
	TrafficAvailable bool            `json:"traffic_available"`
	MetricsError     string          `json:"metrics_error,omitempty"`
	Metrics          *metrics.Sample `json:"metrics,omitempty"`
	App              AppHealth       `json:"app_health"`
}

func NewServer(id, agent, app string, db *store.Metrics) *Server {
	return &Server{ID: id, Metrics: &Poller{ServerID: id, AgentURL: agent, Client: &http.Client{Timeout: 3 * time.Second}, Store: db, Interval: 5 * time.Second}, App: &HealthProbe{AppURL: app, Client: &http.Client{Timeout: 2 * time.Second}, Interval: 5 * time.Second}, Traffic: &TrafficPoller{ServerID: id, AppURL: app, Client: &http.Client{Timeout: 2 * time.Second}, Store: db, Interval: 5 * time.Second}}
}

func (s *Server) Run(ctx context.Context, db *store.Metrics) {
	go s.Metrics.Run(ctx)
	go s.App.Run(ctx)
	go s.Traffic.Run(ctx)
	go (&EventMonitor{ServerID: s.ID, Store: db, Metrics: s.Metrics, App: s.App, Traffic: s.Traffic}).Run(ctx)
}

func (s *Server) Status() ServerStatus {
	ok, err, _ := s.Metrics.State()
	traffic, _, _ := s.Traffic.State()
	r := ServerStatus{ID: s.ID, MetricsAvailable: ok, TrafficAvailable: traffic, MetricsError: err, App: s.App.State()}
	if ok {
		sample := s.Metrics.Latest()
		r.Metrics = &sample
	}
	return r
}
