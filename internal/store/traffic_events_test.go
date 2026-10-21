package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"smartserver/internal/traffic"
)

func TestTrafficAndEventsSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.sqlite3")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rate := 2.5
	sample := traffic.Sample{Snapshot: traffic.Snapshot{Hostname: "laptop", StartedAt: time.Now().UTC().Add(-time.Minute), SampledAt: time.Now().UTC(), RequestCount: 5}, RequestRate: &rate}
	if err := db.SaveTraffic(context.Background(), sample); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEvent(context.Background(), Event{OccurredAt: time.Now().UTC(), Kind: "app", Status: "healthy", Message: "Demo application healthy"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	samples, err := db.RecentTraffic(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].RequestRate == nil || *samples[0].RequestRate != rate {
		t.Fatalf("stored traffic = %+v", samples)
	}
	events, err := db.RecentEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != "app" || events[0].Status != "healthy" {
		t.Fatalf("stored events = %+v", events)
	}
}

func TestExistingTrafficTableGetsP95Column(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite3")
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`CREATE TABLE traffic_samples (id INTEGER PRIMARY KEY, hostname TEXT, started_at TEXT, sampled_at TEXT, request_count INTEGER, error_count INTEGER, duration_sum_ms REAL, request_rate_per_second REAL, error_rate_percent REAL, avg_latency_ms REAL)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p95 := 50.0
	sample := traffic.Sample{Snapshot: traffic.Snapshot{Hostname: "laptop", StartedAt: time.Now().UTC().Add(-time.Minute), SampledAt: time.Now().UTC()}, P95LatencyMS: &p95}
	if err := db.SaveTraffic(context.Background(), sample); err != nil {
		t.Fatal(err)
	}
	got, err := db.RecentTraffic(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].P95LatencyMS == nil || *got[0].P95LatencyMS != p95 {
		t.Fatalf("migrated p95: %+v", got)
	}
}
