package store

import (
	"context"
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
