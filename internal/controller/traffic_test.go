package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"smartserver/internal/store"
	"smartserver/internal/traffic"
)

func TestTrafficPollerCalculatesIntervalAndRetainsHistory(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "monitor.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	snapshot := traffic.Snapshot{Hostname: "laptop", StartedAt: time.Now().UTC().Add(-time.Minute), SampledAt: time.Now().UTC().Add(-5 * time.Second)}
	status := http.StatusOK
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/stats" {
			t.Errorf("path = %s, want /stats", r.URL.Path)
		}
		body, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header), Request: r}, nil
	})}
	poller := &TrafficPoller{AppURL: "http://app", Client: client, Store: db, Interval: 5 * time.Second}
	poller.Poll(context.Background())
	snapshot.SampledAt = snapshot.SampledAt.Add(5 * time.Second)
	snapshot.RequestCount = 10
	snapshot.ErrorCount = 2
	snapshot.DurationSumMS = 200
	snapshot.LatencyBuckets[2] = 10
	poller.Poll(context.Background())
	available, lastErr, _ := poller.State()
	if !available || lastErr != "" {
		t.Fatalf("poller state: available=%v error=%q", available, lastErr)
	}
	samples, err := db.RecentTraffic(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 || samples[1].RequestRate != nil || samples[0].RequestRate == nil || *samples[0].RequestRate != 2 || *samples[0].ErrorRate != 20 || *samples[0].AvgLatencyMS != 20 || samples[0].P95LatencyMS == nil || *samples[0].P95LatencyMS != 50 {
		t.Fatalf("traffic samples = %+v", samples)
	}
	snapshot.StartedAt = time.Now().UTC().Add(-time.Second)
	snapshot.SampledAt = time.Now().UTC()
	snapshot.RequestCount = 1
	snapshot.ErrorCount = 0
	snapshot.DurationSumMS = 10
	poller.Poll(context.Background())
	samples, err = db.RecentTraffic(context.Background(), 10)
	if err != nil || len(samples) != 3 || samples[0].RequestRate != nil {
		t.Fatalf("app restart should reset rate: samples=%+v error=%v", samples, err)
	}
	status = http.StatusServiceUnavailable
	poller.Poll(context.Background())
	available, lastErr, _ = poller.State()
	if available || lastErr == "" {
		t.Fatalf("failed poll: available=%v error=%q", available, lastErr)
	}
	samples, err = db.RecentTraffic(context.Background(), 10)
	if err != nil || len(samples) != 3 {
		t.Fatalf("history after failure: count=%d error=%v", len(samples), err)
	}
}
