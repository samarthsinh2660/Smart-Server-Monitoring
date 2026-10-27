package main

import (
	"net/http"
	"testing"
	"time"
)

func TestCustomLoadLimits(t *testing.T) {
	valid := startRequest{Preset: "custom", Unit: "rps", Rate: 2000, Concurrency: 256, WorkloadMS: 2000, DurationSeconds: 3600}
	if _, err := valid.profile(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*startRequest){func(v *startRequest) { v.Rate = 2001 }, func(v *startRequest) { v.Rate = 0 }, func(v *startRequest) { v.Concurrency = 257 }, func(v *startRequest) { v.WorkloadMS = 2001 }, func(v *startRequest) { v.DurationSeconds = 3601 }, func(v *startRequest) { v.Unit = "requests" }} {
		v := valid
		change(&v)
		if _, err := v.profile(); err == nil {
			t.Fatalf("accepted unbounded custom load: %+v", v)
		}
	}
}

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStopCancelsInflightWorkWithinConcurrencyLimit(t *testing.T) {
	entered := make(chan struct{}, 2)
	client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		entered <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	m := &manager{client: client, target: "http://test-app"}
	m.start(preset{name: "custom", unit: "rps", startRate: 2000, endRate: 2000, duration: time.Minute, workloadMS: 500, concurrent: 2})
	defer m.stop()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("generator did not issue bounded requests")
		}
	}
	m.stop()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := m.snapshot()
		if status.Cancelled == 2 {
			if status.State != "stopped" || status.Sent != 2 || status.Failed != 0 {
				t.Fatalf("incorrect cancelled run: %+v", status)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("in-flight requests did not cancel: %+v", m.snapshot())
}
