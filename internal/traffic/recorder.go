package traffic

import (
	"net/http"
	"sync"
	"time"
)

// UserCounts describes current workload visits independently of request rates.
type UserCounts struct {
	Active int    `json:"active_users"`
	Served uint64 `json:"total_users_served"`
	Limit  int    `json:"max_concurrent_users"`
}

// Snapshot contains cumulative client traffic counters since the app started.
type Snapshot struct {
	Users          *UserCounts `json:"users,omitempty"`
	Hostname       string      `json:"hostname"`
	StartedAt      time.Time   `json:"started_at"`
	SampledAt      time.Time   `json:"sampled_at"`
	RequestCount   uint64      `json:"request_count"`
	ErrorCount     uint64      `json:"error_count"`
	DurationSumMS  float64     `json:"duration_sum_ms"`
	LatencyBuckets [11]uint64  `json:"latency_buckets"`
}

type Recorder struct {
	mu             sync.Mutex
	hostname       string
	startedAt      time.Time
	requestCount   uint64
	errorCount     uint64
	durationSumNS  uint64
	latencyBuckets [11]uint64
}

func NewRecorder(hostname string) *Recorder {
	return &Recorder{hostname: hostname, startedAt: time.Now().UTC()}
}

func (r *Recorder) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Snapshot{
		Hostname: r.hostname, StartedAt: r.startedAt, SampledAt: time.Now().UTC(), RequestCount: r.requestCount,
		ErrorCount: r.errorCount, DurationSumMS: float64(r.durationSumNS) / 1e6,
		LatencyBuckets: r.latencyBuckets,
	}
}

// Wrap counts application requests. Health, stats and user-count probes are excluded.
func (r *Recorder) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/health" || req.URL.Path == "/stats" || req.URL.Path == "/users" {
			next.ServeHTTP(w, req)
			return
		}
		start := time.Now()
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, req)
		duration := time.Since(start)
		r.mu.Lock()
		r.requestCount++
		if writer.status >= http.StatusBadRequest {
			r.errorCount++
		}
		r.durationSumNS += uint64(duration.Nanoseconds())
		r.latencyBuckets[LatencyBucket(duration.Seconds()*1000)]++
		r.mu.Unlock()
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}
