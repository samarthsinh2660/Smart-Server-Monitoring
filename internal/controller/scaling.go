package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"smartserver/internal/config"
	"smartserver/internal/proxy"
)

type Routing interface {
	Targets(context.Context) (map[string]proxy.Target, error)
	Set(context.Context, string, string) error
}

type ControlStore interface {
	SaveControl(context.Context, any, string) error
	LoadControl(context.Context, any) error
}

type ScalingStatus struct {
	State        string                  `json:"state"`
	Enabled      bool                    `json:"enabled"`
	AIEnabled    bool                    `json:"ai_enabled"`
	Ready        bool                    `json:"ready"`
	WarningCPU   float64                 `json:"warning_cpu_percent"`
	HighCPU      float64                 `json:"activation_cpu_percent"`
	HoldSeconds  float64                 `json:"hold_seconds"`
	HoldProgress float64                 `json:"hold_progress_seconds"`
	CPULevel     string                  `json:"cpu_level"`
	Reason       string                  `json:"reason"`
	Error        string                  `json:"error,omitempty"`
	ChangedAt    time.Time               `json:"changed_at"`
	Deadline     time.Time               `json:"deadline,omitempty"`
	Targets      map[string]proxy.Target `json:"targets"`
}

// CPUHold requires distinct, contiguous, fresh samples over the entire hold.
// Re-reading the same cached sample neither advances nor resets the timer.
type CPUHold struct {
	Since, Last time.Time
	Progress    time.Duration
}

func (h *CPUHold) Observe(now time.Time, sample ServerStatus, threshold float64) {
	if !sample.MetricsAvailable || sample.Metrics == nil || !validPercent(sample.Metrics.CPUPercent) || sample.Metrics.CPUPercent < threshold {
		*h = CPUHold{}
		return
	}
	at := sample.Metrics.SampledAt
	if at.IsZero() || now.Sub(at) > 15*time.Second || at.After(now.Add(5*time.Second)) {
		*h = CPUHold{}
		return
	}
	if at.Equal(h.Last) {
		return
	}
	if h.Since.IsZero() || at.Before(h.Last) || at.Sub(h.Last) > 10*time.Second {
		h.Since = at
	}
	h.Last = at
	h.Progress = at.Sub(h.Since)
}

type Scaling struct {
	Config     config.Runtime
	Router     Routing
	Store      ControlStore
	Wake       func(context.Context, string, string) error
	mu         sync.Mutex
	status     ScalingStatus
	hold       CPUHold
	failures   int
	lastHealth time.Time
	reconciled bool
	loaded     bool
}

func (s *Scaling) Status() ScalingStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialize()
	r := s.status
	r.Targets = make(map[string]proxy.Target, len(s.status.Targets))
	for id, t := range s.status.Targets {
		r.Targets[id] = t
	}
	return r
}

func (s *Scaling) initialize() {
	if s.status.State != "" {
		return
	}
	state := "UNCONFIGURED"
	if s.Config.Enabled {
		state = "RECONCILING"
	}
	s.status = ScalingStatus{State: state, Enabled: s.Config.Enabled, AIEnabled: s.Config.AIEnabled, WarningCPU: s.Config.WarningCPU, HighCPU: s.Config.HighCPU, HoldSeconds: s.Config.Hold.Seconds(), CPULevel: "unavailable", Reason: "hardware control disabled", ChangedAt: time.Now().UTC(), Targets: map[string]proxy.Target{}}
}

func (s *Scaling) transition(ctx context.Context, state, reason string, deadline time.Time) error {
	next := s.status
	next.State = state
	next.Reason = reason
	next.ChangedAt = time.Now().UTC()
	next.Deadline = deadline
	if err := s.Store.SaveControl(ctx, next, state+": "+reason); err != nil {
		s.status.Ready = false
		s.status.Error = "persist scaling state: " + err.Error()
		return err
	}
	s.status = next
	return nil
}

func (s *Scaling) fail(ctx context.Context, err error) {
	s.status.Error = err.Error()
	if e := s.transition(ctx, "ACTIVATION_FAILED", err.Error(), time.Time{}); e != nil {
		s.status.State = "ACTIVATION_FAILED"
	}
}

func freshHealth(now time.Time, server ServerStatus) bool {
	return server.App.LastCheckedAt != nil && now.Sub(*server.App.LastCheckedAt) <= 15*time.Second && !server.App.LastCheckedAt.After(now.Add(time.Second)) && server.App.Status != "unknown"
}

func (s *Scaling) Run(ctx context.Context, primary, standby *Server, prediction *PredictionPoller) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastError := ""
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			stepCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			other := ServerStatus{ID: "standby"}
			if standby != nil {
				other = standby.Status()
			}
			if err := s.Step(stepCtx, now, primary.Status(), other, prediction.State()); err != nil {
				if err.Error() != lastError {
					log.Printf("scaling: %v", err)
				}
				lastError = err.Error()
			} else {
				lastError = ""
			}
			cancel()
		}
	}
}

func (s *Scaling) Step(ctx context.Context, now time.Time, primary, standby ServerStatus, prediction Prediction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialize()
	s.hold.Observe(now, primary, s.Config.HighCPU)
	s.status.HoldProgress = s.hold.Progress.Seconds()
	s.status.CPULevel = "unavailable"
	if primary.MetricsAvailable && primary.Metrics != nil && now.Sub(primary.Metrics.SampledAt) <= 15*time.Second {
		s.status.CPULevel = "normal"
		if primary.Metrics.CPUPercent >= s.Config.WarningCPU {
			s.status.CPULevel = "warning"
		}
		if primary.Metrics.CPUPercent >= s.Config.HighCPU {
			s.status.CPULevel = "high"
		}
	}
	if !s.Config.Enabled {
		return nil
	}
	if !s.loaded {
		var previous ScalingStatus
		if err := s.Store.LoadControl(ctx, &previous); err != nil && !errors.Is(err, sql.ErrNoRows) {
			s.status.Error = err.Error()
			return err
		}
		if previous.State != "" {
			s.status.State = previous.State
			s.status.Reason = previous.Reason
			s.status.Deadline = previous.Deadline
			s.status.ChangedAt = previous.ChangedAt
		}
		s.loaded = true
	}
	targets, err := s.Router.Targets(ctx)
	if err != nil {
		s.status.Ready = false
		s.status.Error = err.Error()
		s.status.Targets = map[string]proxy.Target{}
		s.hold = CPUHold{}
		s.status.HoldProgress = 0
		return err
	}
	s.status.Targets = targets
	s.status.Ready = false
	if s.status.State == "STARTING_STANDBY" && !s.status.Deadline.IsZero() && !now.Before(s.status.Deadline) {
		if err := s.Router.Set(ctx, "standby", "maint"); err != nil {
			s.status.Error = err.Error()
			return err
		}
		s.fail(ctx, fmt.Errorf("standby activation timed out"))
		return nil
	}
	if !freshHealth(now, primary) || !freshHealth(now, standby) {
		s.hold = CPUHold{}
		s.status.HoldProgress = 0
		return nil
	}
	s.status.Ready = true
	if !s.reconciled {
		// Recover from actual proxy state. Never send a second wake on restart.
		if targets["standby"].Enabled && standby.App.Status != "healthy" {
			if err := s.Router.Set(ctx, "standby", "maint"); err != nil {
				return err
			}
			if s.status.State != "STARTING_STANDBY" {
				s.fail(ctx, fmt.Errorf("standby unhealthy during reconciliation"))
				s.reconciled = true
				return nil
			}
		}
		if s.status.State == "STARTING_STANDBY" || s.status.State == "DRAINING_STANDBY" {
			if s.status.Deadline.IsZero() {
				s.fail(ctx, fmt.Errorf("activation deadline missing after restart"))
			}
		} else if s.status.State == "ACTIVATION_FAILED" {
			// Preserve a failed attempt until an operator retries or resets it.
		} else if targets["standby"].Enabled && standby.App.Status == "healthy" {
			state := "BOTH_ACTIVE"
			if !targets["standby"].Healthy {
				if err := s.transition(ctx, "STARTING_STANDBY", "reconciling standby proxy health", now.Add(s.Config.StartTimeout)); err != nil {
					return err
				}
			} else {
				if primary.App.Status != "healthy" || !targets["primary"].Healthy {
					state = "FAILOVER"
				}
				if err := s.transition(ctx, state, "reconciled enabled standby", time.Time{}); err != nil {
					return err
				}
			}
		} else {
			if err := s.transition(ctx, "PRIMARY_ONLY", "reconciled primary-only routing", time.Time{}); err != nil {
				return err
			}
		}
		s.reconciled = true
		s.hold = CPUHold{}
	}
	s.status.Error = ""
	if primary.App.LastCheckedAt != nil && !primary.App.LastCheckedAt.Equal(s.lastHealth) {
		s.lastHealth = *primary.App.LastCheckedAt
		if primary.App.Status == "unhealthy" {
			s.failures++
		} else {
			s.failures = 0
		}
	}
	if s.failures >= 3 && targets["primary"].Enabled {
		if err := s.Router.Set(ctx, "primary", "maint"); err != nil {
			s.status.Error = err.Error()
			return err
		}
	}
	if primary.App.Status == "healthy" && !targets["primary"].Enabled {
		if err := s.Router.Set(ctx, "primary", "ready"); err != nil {
			s.status.Error = err.Error()
			return err
		}
	}
	switch s.status.State {
	case "PRIMARY_ONLY":
		reason := ""
		if s.failures >= 3 {
			reason = "primary_unhealthy"
		} else if s.hold.Progress >= s.Config.Hold {
			reason = "cpu_sustained"
		} else if s.Config.AIEnabled && predictiveActivation(now, prediction) {
			reason = "ai_forecast"
		}
		if reason != "" {
			return s.activate(ctx, now, standby, reason)
		}
	case "STARTING_STANDBY":
		if standby.App.Status == "healthy" {
			if !targets["standby"].Enabled {
				if err := s.Router.Set(ctx, "standby", "ready"); err != nil {
					s.fail(ctx, err)
					return err
				}
				return nil
			}
			if targets["standby"].Healthy {
				state := "BOTH_ACTIVE"
				if primary.App.Status != "healthy" || !targets["primary"].Healthy {
					state = "FAILOVER"
				}
				return s.transition(ctx, state, "standby healthy and proxy target ready", time.Time{})
			}
		}
	case "BOTH_ACTIVE", "FAILOVER":
		if standby.App.Status != "healthy" {
			if err := s.Router.Set(ctx, "standby", "maint"); err != nil {
				s.status.Error = err.Error()
				return err
			}
			s.fail(ctx, fmt.Errorf("active standby became unhealthy"))
			return nil
		}
		if !targets["standby"].Enabled {
			s.fail(ctx, fmt.Errorf("standby proxy target unexpectedly disabled"))
			return nil
		}
		if !targets["standby"].Healthy {
			return s.transition(ctx, "STARTING_STANDBY", "waiting for standby proxy health to recover", now.Add(s.Config.StartTimeout))
		}
		if s.failures >= 3 && s.status.State != "FAILOVER" {
			return s.transition(ctx, "FAILOVER", "primary unhealthy; standby retained", time.Time{})
		}
		if primary.App.Status == "healthy" && targets["primary"].Healthy && s.status.State == "FAILOVER" {
			return s.transition(ctx, "BOTH_ACTIVE", "primary recovered; standby retained", time.Time{})
		}
	case "DRAINING_STANDBY":
		if primary.App.Status != "healthy" || !now.Before(s.status.Deadline) {
			if standby.App.Status != "healthy" {
				if err := s.Router.Set(ctx, "standby", "maint"); err != nil {
					return err
				}
				s.fail(ctx, fmt.Errorf("cannot restore unhealthy standby after cancelled drain"))
				return nil
			}
			if err := s.Router.Set(ctx, "standby", "ready"); err != nil {
				s.fail(ctx, err)
				return err
			}
			state := "BOTH_ACTIVE"
			if primary.App.Status != "healthy" {
				state = "FAILOVER"
			}
			return s.transition(ctx, state, "drain cancelled: primary unavailable or drain timeout", time.Time{})
		}
		if targets["standby"].Sessions == 0 {
			if err := s.Router.Set(ctx, "standby", "maint"); err != nil {
				s.fail(ctx, err)
				return err
			}
			s.hold = CPUHold{}
			return s.transition(ctx, "PRIMARY_ONLY", "manual reset: standby drained; Windows left awake", time.Time{})
		}
	}
	return nil
}

func predictiveActivation(now time.Time, p Prediction) bool {
	return p.Available && p.Operational && p.ShadowAction == "would_wake" && !p.ScoredAt.IsZero() && now.Sub(p.ScoredAt) <= 30*time.Second && !p.ScoredAt.After(now.Add(5*time.Second)) && p.Capacity != nil && *p.Capacity > 0 && !math.IsNaN(*p.Capacity) && !math.IsInf(*p.Capacity, 0)
}

func (s *Scaling) activate(ctx context.Context, now time.Time, standby ServerStatus, reason string) error {
	if err := s.transition(ctx, "STARTING_STANDBY", reason, now.Add(s.Config.StartTimeout)); err != nil {
		return err
	}
	if standby.App.Status != "healthy" {
		wake := s.Wake
		if wake == nil {
			wake = SendWake
		}
		if err := wake(ctx, s.Config.MAC, s.Config.Broadcast); err != nil {
			s.fail(ctx, err)
			return err
		}
	}
	return nil
}

func (s *Scaling) Action(ctx context.Context, action string, primary, standby ServerStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initialize()
	if !s.Config.Enabled || !s.reconciled || !s.status.Ready {
		return fmt.Errorf("control is disabled or proxy/health reconciliation is incomplete")
	}
	now := time.Now()
	if !freshHealth(now, primary) || !freshHealth(now, standby) {
		return fmt.Errorf("fresh app health checks are required")
	}
	switch action {
	case "activate", "retry":
		if s.status.State != "PRIMARY_ONLY" && s.status.State != "ACTIVATION_FAILED" {
			return fmt.Errorf("standby already starting or active")
		}
		return s.activate(ctx, now, standby, "manual_"+action)
	case "reset":
		if primary.App.Status != "healthy" || !primary.MetricsAvailable || primary.Metrics == nil || primary.Metrics.CPUPercent >= s.Config.WarningCPU || now.Sub(primary.Metrics.SampledAt) > 15*time.Second {
			return fmt.Errorf("reset requires a healthy primary with fresh CPU below the warning threshold")
		}
		if s.status.State == "STARTING_STANDBY" || s.status.State == "DRAINING_STANDBY" {
			return fmt.Errorf("cannot reset during activation or drain")
		}
		targets, err := s.Router.Targets(ctx)
		if err != nil {
			return err
		}
		if !targets["primary"].Healthy {
			return fmt.Errorf("reset requires a serving primary HAProxy target")
		}
		if !targets["standby"].Enabled {
			s.hold = CPUHold{}
			return s.transition(ctx, "PRIMARY_ONLY", "manual reset: standby already excluded; Windows left unchanged", time.Time{})
		}
		if err := s.transition(ctx, "DRAINING_STANDBY", "manual reset: waiting for in-flight requests", now.Add(30*time.Second)); err != nil {
			return err
		}
		if err := s.Router.Set(ctx, "standby", "drain"); err != nil {
			s.status.Error = err.Error()
			s.fail(ctx, err)
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown action")
	}
}
