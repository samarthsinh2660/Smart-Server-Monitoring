// Traffic Lab generates bounded HTTP load for the local faculty demo.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type preset struct {
	name       string
	unit       string
	startRate  float64
	endRate    float64
	duration   time.Duration
	workloadMS int
	concurrent int
}

var presets = map[string]preset{
	"rpm_200":  {name: "rpm_200", unit: "rpm", startRate: 200, endRate: 200, duration: 2 * time.Minute, workloadMS: 150, concurrent: 32},
	"rpm_500":  {name: "rpm_500", unit: "rpm", startRate: 500, endRate: 500, duration: 2 * time.Minute, workloadMS: 180, concurrent: 32},
	"rpm_1000": {name: "rpm_1000", unit: "rpm", startRate: 1000, endRate: 1000, duration: 2 * time.Minute, workloadMS: 220, concurrent: 32},
	"rpm_2000": {name: "rpm_2000", unit: "rpm", startRate: 2000, endRate: 2000, duration: 2 * time.Minute, workloadMS: 250, concurrent: 32},
	"rpm_ramp": {name: "rpm_ramp", unit: "rpm", startRate: 200, endRate: 2000, duration: 2 * time.Minute, workloadMS: 200, concurrent: 32},
	"rps_200":  {name: "rps_200", unit: "rps", startRate: 200, endRate: 200, duration: 90 * time.Second, workloadMS: 2, concurrent: 256},
	"rps_500":  {name: "rps_500", unit: "rps", startRate: 500, endRate: 500, duration: 90 * time.Second, workloadMS: 2, concurrent: 256},
	"rps_1000": {name: "rps_1000", unit: "rps", startRate: 1000, endRate: 1000, duration: 90 * time.Second, workloadMS: 2, concurrent: 256},
	"rps_2000": {name: "rps_2000", unit: "rps", startRate: 2000, endRate: 2000, duration: 90 * time.Second, workloadMS: 2, concurrent: 256},
	"rps_ramp": {name: "rps_ramp", unit: "rps", startRate: 200, endRate: 2000, duration: 90 * time.Second, workloadMS: 2, concurrent: 256},
}

type run struct {
	preset     string
	profile    preset
	state      string
	startedAt  time.Time
	endsAt     time.Time
	finishedAt time.Time
	cancel     context.CancelFunc
	sent       int64
	succeeded  int64
	failed     int64
	cancelled  int64
	dropped    int64
}

type status struct {
	State           string    `json:"state"`
	Preset          string    `json:"preset,omitempty"`
	Unit            string    `json:"unit,omitempty"`
	TargetRate      float64   `json:"target_rate"`
	Concurrency     int       `json:"concurrency"`
	WorkloadMS      int       `json:"workload_ms"`
	DurationSeconds int       `json:"duration_seconds"`
	AchievedRPS     float64   `json:"achieved_rps"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	EndsAt          time.Time `json:"ends_at,omitempty"`
	Sent            int64     `json:"sent"`
	Succeeded       int64     `json:"succeeded"`
	Failed          int64     `json:"failed"`
	Cancelled       int64     `json:"cancelled"`
	Dropped         int64     `json:"dropped"`
}

type manager struct {
	mu     sync.Mutex
	client *http.Client
	target string
	active *run
}

func (m *manager) snapshot() status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return status{State: "idle"}
	}
	r := m.active
	elapsed := time.Since(r.startedAt)
	if !r.finishedAt.IsZero() {
		elapsed = r.finishedAt.Sub(r.startedAt)
	}
	fraction := min(1, max(0, float64(elapsed)/float64(r.profile.duration)))
	rate := r.profile.startRate + (r.profile.endRate-r.profile.startRate)*fraction
	return status{
		State: r.state, Preset: r.preset, Unit: r.profile.unit, TargetRate: rate,
		Concurrency: r.profile.concurrent, WorkloadMS: r.profile.workloadMS, DurationSeconds: int(r.profile.duration.Seconds()), AchievedRPS: float64(r.succeeded) / max(0.001, min(elapsed.Seconds(), r.profile.duration.Seconds())),
		StartedAt: r.startedAt, EndsAt: r.endsAt,
		Sent: r.sent, Succeeded: r.succeeded, Failed: r.failed, Cancelled: r.cancelled, Dropped: r.dropped,
	}
}

func (m *manager) start(p preset) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.state == "running" {
		m.active.state = "stopped"
		m.active.finishedAt = time.Now()
		m.active.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.duration)
	now := time.Now().UTC()
	r := &run{preset: p.name, profile: p, state: "running", startedAt: now, endsAt: now.Add(p.duration), cancel: cancel}
	m.active = r
	go m.generate(ctx, r, p)
}

func (m *manager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.state == "running" {
		m.active.state = "stopped"
		m.active.finishedAt = time.Now()
		m.active.cancel()
	}
}

func (m *manager) generate(ctx context.Context, r *run, p preset) {
	defer r.cancel()
	defer func() {
		m.mu.Lock()
		if r.state == "running" {
			r.state = "completed"
			r.finishedAt = time.Now()
		}
		m.mu.Unlock()
	}()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	seats := make(chan struct{}, p.concurrent)
	var workers sync.WaitGroup
	defer workers.Wait()
	credit := 0.0
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			elapsed := now.Sub(r.startedAt)
			fraction := min(1, float64(elapsed)/float64(p.duration))
			rate := p.startRate + (p.endRate-p.startRate)*fraction
			if p.unit == "rpm" {
				rate /= 60
			}
			credit += rate * now.Sub(last).Seconds()
			last = now
			// Bound catch-up bursts after a busy scheduler; report skipped work.
			if credit > float64(2*p.concurrent) {
				skipped := math.Floor(credit - float64(2*p.concurrent))
				credit -= skipped
				m.mu.Lock()
				r.dropped += int64(skipped)
				m.mu.Unlock()
			}
			for credit >= 1 {
				if ctx.Err() != nil {
					return
				}
				credit--
				select {
				case seats <- struct{}{}:
					m.mu.Lock()
					if ctx.Err() != nil || r.state != "running" {
						m.mu.Unlock()
						<-seats
						return
					}
					r.sent++
					m.mu.Unlock()
					workers.Add(1)
					go func() {
						defer workers.Done()
						defer func() { <-seats }()
						m.send(ctx, r, p.workloadMS)
					}()
				default:
					m.mu.Lock()
					r.dropped++
					m.mu.Unlock()
				}
			}
		}
	}
}

func (m *manager) send(ctx context.Context, r *run, workloadMS int) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, m.target+"/workload?duration_ms="+strconv.Itoa(workloadMS), nil)
	if err == nil {
		var response *http.Response
		response, err = m.client.Do(request)
		if err == nil {
			_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			err = errors.Join(readErr, response.Body.Close())
			if response.StatusCode >= 400 && err == nil {
				err = errors.New("demo app returned an HTTP error")
			}
		}
	}
	m.mu.Lock()
	if errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil) {
		r.cancelled++
	} else if err == nil {
		r.succeeded++
	} else {
		r.failed++
	}
	m.mu.Unlock()
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

type startRequest struct {
	Preset          string  `json:"preset"`
	Unit            string  `json:"unit"`
	Rate            float64 `json:"rate"`
	Concurrency     int     `json:"concurrency"`
	WorkloadMS      int     `json:"workload_ms"`
	DurationSeconds int     `json:"duration_seconds"`
}

func (b startRequest) profile() (preset, error) {
	if b.Preset != "custom" {
		p, ok := presets[b.Preset]
		if !ok {
			return preset{}, errors.New("unknown traffic preset")
		}
		return p, nil
	}
	if b.Unit != "rpm" && b.Unit != "rps" {
		return preset{}, errors.New("unit must be rpm or rps")
	}
	if math.IsNaN(b.Rate) || math.IsInf(b.Rate, 0) || b.Rate < 1 || b.Rate > 2000 {
		return preset{}, errors.New("rate must be between 1 and 2000")
	}
	if b.Concurrency < 1 || b.Concurrency > 256 {
		return preset{}, errors.New("concurrency must be between 1 and 256")
	}
	if b.WorkloadMS < 1 || b.WorkloadMS > 2000 {
		return preset{}, errors.New("workload_ms must be between 1 and 2000")
	}
	if b.DurationSeconds < 1 || b.DurationSeconds > 3600 {
		return preset{}, errors.New("duration_seconds must be between 1 and 3600")
	}
	return preset{name: "custom", unit: b.Unit, startRate: b.Rate, endRate: b.Rate, duration: time.Duration(b.DurationSeconds) * time.Second, workloadMS: b.WorkloadMS, concurrent: b.Concurrency}, nil
}

func main() {
	addr := os.Getenv("TRAFFIC_LAB_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8091"
	}
	target := strings.TrimRight(os.Getenv("TRAFFIC_LAB_TARGET"), "/")
	if target == "" {
		target = "http://127.0.0.1:8000"
	}
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.User != nil || parsed.Fragment != "" {
		log.Fatal("TRAFFIC_LAB_TARGET must be an HTTP origin without a path or query")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 256
	transport.MaxIdleConnsPerHost = 256
	transport.MaxConnsPerHost = 256
	m := &manager{client: &http.Client{Timeout: 5 * time.Second, Transport: transport}, target: target}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, m.snapshot())
	})
	mux.HandleFunc("POST /start", func(w http.ResponseWriter, request *http.Request) {
		var body startRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 2048))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		p, err := body.profile()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		m.start(p)
		writeJSON(w, http.StatusAccepted, m.snapshot())
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, _ *http.Request) {
		m.stop()
		writeJSON(w, http.StatusOK, m.snapshot())
	})
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	log.Printf("traffic lab listening on %s; target %s", addr, target)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
