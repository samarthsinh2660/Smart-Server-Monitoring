package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"smartserver/internal/metrics"
)

func TestMetricsPersistAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.sqlite3")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sample := metrics.Sample{Hostname: "laptop", SampledAt: time.Now().UTC(), CPUPercent: 42, MemoryPercent: 63, DiskPercent: 28, NetworkRXBytes: 123, NetworkTXBytes: 456, UptimeSeconds: 789}
	if err := db.Save(context.Background(), sample); err != nil {
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
	got, err := db.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname != sample.Hostname || got.CPUPercent != sample.CPUPercent || !got.SampledAt.Equal(sample.SampledAt) {
		t.Fatalf("persisted sample: got %+v, want %+v", got, sample)
	}
	recent, err := db.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 {
		t.Fatalf("recent count: got %d, want 1", len(recent))
	}
}
