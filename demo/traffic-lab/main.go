// Traffic Lab generates bounded HTTP load for the local faculty demo.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
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
	startRate  float64
	endRate    float64
	duration   time.Duration
	workloadMS int
}

var presets = map[string]preset{
	"steady": {name: "steady", startRate: 2, endRate: 2, duration: 2 * time.Minute, workloadMS: 100},
	"busy":   {name: "busy", startRate: 6, endRate: 6, duration: 2 * time.Minute, workloadMS: 200},
	"burst":  {name: "burst", startRate: 12, endRate: 12, duration: 45 * time.Second, workloadMS: 350},
	"ramp":   {name: "ramp", startRate: 2, endRate: 12, duration: 90 * time.Second, workloadMS: 200},
}

type run struct {
	preset    string
	state     string
	startedAt time.Time
	endsAt    time.Time
	cancel    context.CancelFunc
	sent      int64
	succeeded int64
	failed    int64
	cancelled int64
	dropped   int64
}

type status struct {
	State     string    `json:"state"`
	Preset    string    `json:"preset,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndsAt    time.Time `json:"ends_at,omitempty"`
	Sent      int64     `json:"sent"`
	Succeeded int64     `json:"succeeded"`
	Failed    int64     `json:"failed"`
	Cancelled int64     `json:"cancelled"`
	Dropped   int64     `json:"dropped"`
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
	return status{
		State: r.state, Preset: r.preset, StartedAt: r.startedAt, EndsAt: r.endsAt,
		Sent: r.sent, Succeeded: r.succeeded, Failed: r.failed, Cancelled: r.cancelled, Dropped: r.dropped,
	}
}

func (m *manager) start(p preset) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.state == "running" {
		return errors.New("a traffic run is already active")
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.duration)
	now := time.Now().UTC()
	r := &run{preset: p.name, state: "running", startedAt: now, endsAt: now.Add(p.duration), cancel: cancel}
	m.active = r
	go m.generate(ctx, r, p)
	return nil
}

func (m *manager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.active.state == "running" {
		m.active.state = "stopped"
		m.active.cancel()
	}
}

func (m *manager) generate(ctx context.Context, r *run, p preset) {
	defer r.cancel()
	defer func() {
		m.mu.Lock()
		if r.state == "running" {
			r.state = "completed"
		}
		m.mu.Unlock()
	}()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	seats := make(chan struct{}, 8)
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
			credit += rate * now.Sub(last).Seconds()
			last = now
			for credit >= 1 {
				credit--
				select {
				case seats <- struct{}{}:
					m.mu.Lock()
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
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			err = response.Body.Close()
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
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" {
		log.Fatal("TRAFFIC_LAB_TARGET must be an HTTP origin without a path or query")
	}
	m := &manager{client: &http.Client{Timeout: 3 * time.Second}, target: target}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, m.snapshot())
	})
	mux.HandleFunc("POST /start", func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			Preset string `json:"preset"`
		}
		if err := json.NewDecoder(io.LimitReader(request.Body, 1024)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		p, ok := presets[body.Preset]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown traffic preset"})
			return
		}
		if err := m.start(p); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
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
