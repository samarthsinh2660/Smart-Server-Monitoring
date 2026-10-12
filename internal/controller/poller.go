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

	"smartserver/internal/metrics"
	"smartserver/internal/store"
)

// Poller keeps reachability separate from historical samples.
type Poller struct {
	AgentURL string
	Client   *http.Client
	Store    *store.Metrics
	Interval time.Duration
	mu       sync.RWMutex
	lastErr  string
	lastOK   time.Time
}

func (p *Poller) Run(ctx context.Context) {
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

func (p *Poller) Poll(ctx context.Context) {
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	u, err := url.Parse(p.AgentURL)
	if err != nil {
		p.fail(fmt.Errorf("agent URL: %w", err))
		return
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/metrics"
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
		p.fail(fmt.Errorf("agent returned %s", resp.Status))
		return
	}
	var sample metrics.Sample
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&sample); err != nil {
		p.fail(fmt.Errorf("decode agent metrics: %w", err))
		return
	}
	if sample.Hostname == "" || sample.SampledAt.IsZero() || sample.SampledAt.After(time.Now().Add(5*time.Second)) || sample.SampledAt.Before(time.Now().Add(-15*time.Second)) || !validPercent(sample.CPUPercent) || !validPercent(sample.MemoryPercent) || !validPercent(sample.DiskPercent) {
		p.fail(fmt.Errorf("agent returned invalid or stale metrics"))
		return
	}
	if err := p.Store.Save(requestCtx, sample); err != nil {
		p.fail(err)
		return
	}
	p.mu.Lock()
	p.lastOK = sample.SampledAt
	p.lastErr = ""
	p.mu.Unlock()
}

func validPercent(n float64) bool { return n >= 0 && n <= 100 }

func (p *Poller) fail(err error) {
	p.mu.Lock()
	p.lastErr = err.Error()
	p.mu.Unlock()
}

func (p *Poller) State() (bool, string, time.Time) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	available := p.lastErr == "" && !p.lastOK.IsZero() && time.Since(p.lastOK) <= 3*p.Interval
	return available, p.lastErr, p.lastOK
}
