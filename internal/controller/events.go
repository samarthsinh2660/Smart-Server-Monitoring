package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"smartserver/internal/store"
)

// EventMonitor records changes without turning monitoring failures into app failures.
type EventMonitor struct {
	ServerID string
	Store    *store.Metrics
	Metrics  *Poller
	App      *HealthProbe
	Traffic  *TrafficPoller
	seen     map[string]string
}

func (m *EventMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Observe(ctx); err != nil {
				log.Printf("record controller event: %v", err)
			}
		}
	}
}

func (m *EventMonitor) Observe(ctx context.Context) error {
	if m.seen == nil {
		m.seen = make(map[string]string)
	}
	metricsAvailable, metricsErr, metricsAt := m.Metrics.State()
	trafficAvailable, trafficErr, trafficAt := m.Traffic.State()
	app := m.App.State()
	metricsStatus := availability(metricsAvailable, metricsErr, !metricsAt.IsZero())
	trafficStatus := availability(trafficAvailable, trafficErr, !trafficAt.IsZero())
	observations := []struct{ kind, status, message string }{
		{"metrics", metricsStatus, "Laptop metrics " + metricsStatus},
		{"traffic", trafficStatus, "Application traffic stats " + trafficStatus},
		{"app", app.Status, "Demo application " + app.Status},
	}
	for _, observation := range observations {
		if observation.status == "unknown" && m.seen[observation.kind] == "" {
			continue
		}
		if m.seen[observation.kind] == observation.status {
			continue
		}
		event := store.Event{OccurredAt: time.Now().UTC(), Kind: observation.kind, Status: observation.status, Message: observation.message}
		if m.ServerID != "" {
			event.Message = m.ServerID + ": " + event.Message
		}
		if err := m.Store.SaveEvent(ctx, event); err != nil {
			return fmt.Errorf("%s event: %w", observation.kind, err)
		}
		m.seen[observation.kind] = observation.status
	}
	return nil
}

func availability(available bool, lastErr string, observed bool) string {
	if available {
		return "available"
	}
	if lastErr != "" || observed {
		return "unavailable"
	}
	return "unknown"
}
