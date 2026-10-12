//go:build linux

package metrics

import (
	"context"
	"testing"
	"time"
)

func TestCollectLiveHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sample, err := Collect(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if sample.Hostname == "" || sample.SampledAt.IsZero() || sample.UptimeSeconds == 0 {
		t.Fatalf("incomplete live sample: %+v", sample)
	}
	for name, value := range map[string]float64{"cpu": sample.CPUPercent, "memory": sample.MemoryPercent, "disk": sample.DiskPercent} {
		if value < 0 || value > 100 {
			t.Errorf("%s percentage outside 0..100: %f", name, value)
		}
	}
	t.Logf("live laptop: cpu=%.1f%% memory=%.1f%% disk=%.1f%% uptime=%ds", sample.CPUPercent, sample.MemoryPercent, sample.DiskPercent, sample.UptimeSeconds)
}
