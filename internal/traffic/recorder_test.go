package traffic

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecorderExcludesProbesAndCountsErrors(t *testing.T) {
	r := NewRecorder("laptop")
	handler := r.Wrap(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/invalid" {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	for _, path := range []string{"/health", "/stats", "/users", "/", "/invalid"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	s := r.Snapshot()
	if s.Hostname != "laptop" || s.RequestCount != 2 || s.ErrorCount != 1 || s.SampledAt.IsZero() || s.DurationSumMS < 0 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	var bucketCount uint64
	for _, count := range s.LatencyBuckets {
		bucketCount += count
	}
	if bucketCount != s.RequestCount {
		t.Fatalf("latency buckets count %d, requests %d", bucketCount, s.RequestCount)
	}
}
