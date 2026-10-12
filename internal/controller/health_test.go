package controller

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHealthProbeChecksApplicationResponse(t *testing.T) {
	status := http.StatusOK
	body := `{"status":"ok"}`
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/health" {
			t.Errorf("path = %s, want /health", r.URL.Path)
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})}
	probe := &HealthProbe{AppURL: "http://app", Client: client, Interval: 5 * time.Second}
	if got := probe.State().Status; got != "unknown" {
		t.Fatalf("before first check = %s, want unknown", got)
	}
	probe.Check(context.Background())
	if got := probe.State(); got.Status != "healthy" || got.LastCheckedAt == nil {
		t.Fatalf("healthy response = %+v", got)
	}
	status = http.StatusServiceUnavailable
	probe.Check(context.Background())
	if got := probe.State(); got.Status != "unhealthy" || got.Error == "" {
		t.Fatalf("HTTP failure = %+v", got)
	}
	status = http.StatusOK
	body = `{"status":"starting"}`
	probe.Check(context.Background())
	if got := probe.State(); got.Status != "unhealthy" {
		t.Fatalf("non-ok app body = %+v", got)
	}
}
