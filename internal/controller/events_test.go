package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"smartserver/internal/store"
)

func TestEventMonitorRecordsOnlyTransitions(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "monitor.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	metrics := &Poller{Interval: 5 * time.Second, lastOK: now}
	app := &HealthProbe{Interval: 5 * time.Second, checked: now, healthy: true}
	traffic := &TrafficPoller{Interval: 5 * time.Second, lastOK: now}
	monitor := &EventMonitor{Store: db, Metrics: metrics, App: app, Traffic: traffic}
	if err := monitor.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := monitor.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, err := db.RecentEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("initial event count = %d, want 3", len(events))
	}
	app.record(fmt.Errorf("app returned 503"))
	if err := monitor.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, err = db.RecentEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[0].Kind != "app" || events[0].Status != "unhealthy" {
		t.Fatalf("transition events = %+v", events)
	}
}
