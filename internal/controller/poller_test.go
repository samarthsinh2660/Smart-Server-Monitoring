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

	"smartserver/internal/metrics"
	"smartserver/internal/store"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPollKeepsHistoryWhenAgentFails(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "metrics.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sample := metrics.Sample{Hostname: "laptop", SampledAt: time.Now().UTC(), CPUPercent: 35, MemoryPercent: 40, DiskPercent: 50}
	data, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	status := http.StatusOK
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/metrics" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
	})}
	poller := &Poller{AgentURL: "http://agent", Client: client, Store: db, Interval: 5 * time.Second}
	poller.Poll(context.Background())
	available, lastErr, _ := poller.State()
	if !available || lastErr != "" {
		t.Fatalf("successful poll: available=%v error=%q", available, lastErr)
	}
	status = http.StatusServiceUnavailable
	poller.Poll(context.Background())
	available, lastErr, _ = poller.State()
	if available || lastErr == "" {
		t.Fatalf("failed poll: available=%v error=%q", available, lastErr)
	}
	samples, err := db.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].Hostname != "laptop" {
		t.Fatalf("history changed after failure: %+v", samples)
	}
}

func TestRepeatedAgentTimestampBecomesUnavailableWithoutDuplicatingHistory(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "repeated.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sample := metrics.Sample{Hostname: "windows", SampledAt: time.Now().UTC(), CPUPercent: 85, MemoryPercent: 40, DiskPercent: 50}
	body, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	p := &Poller{ServerID: "primary", AgentURL: "http://agent", Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header), Request: r}, nil
	})}, Store: db, Interval: 5 * time.Second}
	p.Poll(context.Background())
	p.Poll(context.Background())
	available, message, _ := p.State()
	if available || message == "" {
		t.Fatal("repeated CPU sample remained available")
	}
	rows, err := db.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("repeated sample was persisted %d times", len(rows))
	}
}
