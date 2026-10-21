package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"smartserver/internal/store"
	"smartserver/internal/traffic"
)

// TrafficPoller reads the demo app's cumulative counters and stores interval rates.
type TrafficPoller struct {
	AppURL   string
	Client   *http.Client
	Store    *store.Metrics
	Interval time.Duration
	mu       sync.RWMutex
	previous traffic.Snapshot
	lastOK   time.Time
	lastErr  string
}

func (p *TrafficPoller) Run(ctx context.Context) {
	p.Poll(ctx)
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.Poll(ctx)
		}
	}
}

func (p *TrafficPoller) Poll(ctx context.Context) {
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	u, err := url.Parse(p.AppURL)
	if err != nil {
		p.fail(fmt.Errorf("app URL: %w", err))
		return
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/stats"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		p.fail(err)
		return
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		p.fail(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		p.fail(fmt.Errorf("app stats returned %s", resp.Status))
		return
	}
	var snapshot traffic.Snapshot
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&snapshot); err != nil {
		p.fail(fmt.Errorf("decode app stats: %w", err))
		return
	}
	if snapshot.Hostname == "" || snapshot.StartedAt.IsZero() || snapshot.SampledAt.IsZero() || snapshot.StartedAt.After(snapshot.SampledAt) || snapshot.SampledAt.After(time.Now().Add(5*time.Second)) || snapshot.SampledAt.Before(time.Now().Add(-15*time.Second)) || snapshot.ErrorCount > snapshot.RequestCount || snapshot.DurationSumMS < 0 {
		p.fail(fmt.Errorf("app returned invalid or stale stats"))
		return
	}
	p.mu.RLock()
	previous := p.previous
	p.mu.RUnlock()
	sample := traffic.Sample{Snapshot: snapshot}
	seconds := snapshot.SampledAt.Sub(previous.SampledAt).Seconds()
	if snapshot.Hostname == previous.Hostname && snapshot.StartedAt.Equal(previous.StartedAt) && seconds > 0 && snapshot.RequestCount >= previous.RequestCount && snapshot.ErrorCount >= previous.ErrorCount && snapshot.DurationSumMS >= previous.DurationSumMS {
		requests := snapshot.RequestCount - previous.RequestCount
		errors := snapshot.ErrorCount - previous.ErrorCount
		rate := float64(requests) / seconds
		sample.RequestRate = &rate
		if requests > 0 {
			errorRate := 100 * float64(errors) / float64(requests)
			avgLatency := (snapshot.DurationSumMS - previous.DurationSumMS) / float64(requests)
			sample.ErrorRate = &errorRate
			sample.AvgLatencyMS = &avgLatency
			if p95, ok := traffic.P95Latency(snapshot.LatencyBuckets, previous.LatencyBuckets); ok {
				sample.P95LatencyMS = &p95
			}
		}
	}
	if err := p.Store.SaveTraffic(requestCtx, sample); err != nil {
		p.fail(err)
		return
	}
	p.mu.Lock()
	p.previous = snapshot
	p.lastOK = snapshot.SampledAt
	p.lastErr = ""
	p.mu.Unlock()
}

func (p *TrafficPoller) fail(err error) {
	p.mu.Lock()
	p.lastErr = err.Error()
	p.mu.Unlock()
}

func (p *TrafficPoller) State() (bool, string, time.Time) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastErr == "" && !p.lastOK.IsZero() && time.Since(p.lastOK) <= 3*p.Interval, p.lastErr, p.lastOK
}
