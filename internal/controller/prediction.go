package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Prediction struct {
	Available      bool      `json:"available"`
	Reason         string    `json:"reason,omitempty"`
	Source         string    `json:"source,omitempty"`
	ModelVersion   string    `json:"model_version,omitempty"`
	RiskScore      float64   `json:"risk_score"`
	Horizon        int       `json:"horizon_seconds,omitempty"`
	Forecast       float64   `json:"forecast_rpm"`
	Upper          float64   `json:"upper_rpm"`
	SurgeAlert     bool      `json:"surge_alert"`
	ShadowAction   string    `json:"shadow_action,omitempty"`
	DecisionReason string    `json:"decision_reason,omitempty"`
	Capacity       *float64  `json:"shadow_capacity_rpm"`
	Operational    bool      `json:"operational_scale_ready"`
	ScoredAt       time.Time `json:"scored_at,omitempty"`
}

type PredictionPoller struct {
	URL    string
	Client *http.Client
	mu     sync.RWMutex
	result Prediction
}

func (p *PredictionPoller) Run(ctx context.Context) {
	p.Poll(ctx)
	ticker := time.NewTicker(15 * time.Second)
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

func (p *PredictionPoller) Poll(ctx context.Context) {
	result := Prediction{Reason: "Python prediction service unavailable"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.URL, "/")+"/prediction", nil)
	if err == nil {
		resp, e := p.Client.Do(req)
		if e == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var candidate Prediction
				if e := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&candidate); e == nil {
					result = candidate
				}
			}
		}
	}
	p.mu.Lock()
	p.result = result
	p.mu.Unlock()
}

func (p *PredictionPoller) State() Prediction {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r := p.result
	if r.Available && (r.ScoredAt.IsZero() || time.Since(r.ScoredAt) > 30*time.Second || r.ScoredAt.After(time.Now().Add(5*time.Second))) {
		r.Available = false
		r.Reason = "prediction is stale"
	}
	if !r.Available && r.Reason == "" {
		r.Reason = "waiting for Python prediction service"
	}
	return r
}
