package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"smartserver/internal/metrics"
	"smartserver/internal/traffic"
)

func TestServerHistoriesAndControlStatePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.sqlite3")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, id := range []string{"primary", "standby"} {
		if err := db.Save(ctx, metrics.Sample{ServerID: id, Hostname: "same-hostname", SampledAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveTraffic(ctx, traffic.Sample{ServerID: id, Snapshot: traffic.Snapshot{Hostname: "same-app-name", StartedAt: time.Now(), SampledAt: time.Now()}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveControl(ctx, map[string]string{"state": "STARTING_STANDBY"}, "cpu_sustained"); err != nil {
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
	for _, id := range []string{"primary", "standby"} {
		samples, err := db.RecentFor(ctx, id, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(samples) != 1 || samples[0].ServerID != id {
			t.Fatalf("mixed metric identities: %+v", samples)
		}
		requests, err := db.RecentTrafficFor(ctx, id, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) != 1 || requests[0].ServerID != id {
			t.Fatalf("mixed traffic identities: %+v", requests)
		}
	}
	var status map[string]string
	if err := db.LoadControl(ctx, &status); err != nil {
		t.Fatal(err)
	}
	if status["state"] != "STARTING_STANDBY" {
		t.Fatalf("lost activation state: %v", status)
	}
	events, err := db.RecentEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Message != "cpu_sustained" {
		t.Fatalf("lost transition event: %v", events)
	}
}
