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
)

// HealthProbe checks the demo application independently of the metrics agent.
type HealthProbe struct {
	AppURL   string
	Client   *http.Client
	Interval time.Duration
	mu       sync.RWMutex
	checked  time.Time
	healthy  bool
	lastErr  string
}

type AppHealth struct {
	Status        string     `json:"status"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	Error         string     `json:"error,omitempty"`
}

func (p *HealthProbe) Run(ctx context.Context) {
	p.Check(ctx)
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.Check(ctx)
		}
	}
}

func (p *HealthProbe) Check(ctx context.Context) {
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	u, err := url.Parse(p.AppURL)
	if err != nil {
		p.record(fmt.Errorf("app URL: %w", err))
		return
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/health"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		p.record(err)
		return
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		p.record(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		p.record(fmt.Errorf("app returned %s", resp.Status))
		return
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&result); err != nil {
		p.record(fmt.Errorf("decode app health: %w", err))
		return
	}
	if result.Status != "ok" {
		p.record(fmt.Errorf("app reported %q", result.Status))
		return
	}
	p.mu.Lock()
	p.checked = time.Now().UTC()
	p.healthy = true
	p.lastErr = ""
	p.mu.Unlock()
}

func (p *HealthProbe) record(err error) {
	p.mu.Lock()
	p.checked = time.Now().UTC()
	p.healthy = false
	p.lastErr = err.Error()
	p.mu.Unlock()
}

func (p *HealthProbe) State() AppHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.checked.IsZero() {
		return AppHealth{Status: "unknown"}
	}
	checked := p.checked
	if time.Since(checked) > 3*p.Interval {
		return AppHealth{Status: "unknown", LastCheckedAt: &checked, Error: "app health check is stale"}
	}
	if p.healthy {
		return AppHealth{Status: "healthy", LastCheckedAt: &checked}
	}
	return AppHealth{Status: "unhealthy", LastCheckedAt: &checked, Error: p.lastErr}
}
