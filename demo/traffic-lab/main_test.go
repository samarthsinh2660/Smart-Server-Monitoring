package main

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
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

func TestUserModeRepeatsVisitsAndHonorsUserCount(t *testing.T) {
	var active, peak, completed atomic.Int64
	m := &manager{target: "http://app", client: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(10 * time.Millisecond):
		}
		completed.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}}
	p, err := (startRequest{Preset: "custom", UserMode: true, Continuous: true, Concurrency: 3, WorkloadMS: 10}).profile()
	if err != nil {
		t.Fatal(err)
	}
	m.start(p)
	defer m.stop()
	deadline := time.Now().Add(2 * time.Second)
	for completed.Load() < 9 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if completed.Load() < 9 || peak.Load() > 3 {
		t.Fatalf("visits=%d peak users=%d", completed.Load(), peak.Load())
	}
	m.stop()
	deadline = time.Now().Add(2 * time.Second)
	for active.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if active.Load() != 0 {
		t.Fatal("users remained active after Stop")
	}
}

func TestCombinedLoadKeepsUsersAndBotsRunning(t *testing.T) {
	var calls atomic.Int64
	m := &manager{target: "http://app", client: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(20 * time.Millisecond):
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}}
	p, err := (startRequest{Preset: "custom", Unit: "rps", Rate: 20, Concurrency: 2, ActiveUsers: 2, Continuous: true, WorkloadMS: 20}).profile()
	if err != nil {
		t.Fatal(err)
	}
	m.start(p)
	defer m.stop()
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 12 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() < 12 {
		t.Fatal("combined load did not send repeated visits")
	}
	s := m.snapshot()
	if s.ActiveUsersTarget != 2 || s.UserMode || s.TargetRate != 20 || !s.Continuous {
		t.Fatalf("invalid combined status: %+v", s)
	}
	m.stop()
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s = m.snapshot()
		if s.Sent == s.Succeeded+s.Failed+s.Cancelled {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("combined workers did not stop")
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

func TestContinuousRunDoesNotExpireAndStops(t *testing.T) {
	entered := make(chan struct{}, 1)
	m := &manager{target: "http://test-app", client: &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		entered <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}}
	p, err := (startRequest{Preset: "custom", Unit: "rps", Rate: 100, Concurrency: 1, WorkloadMS: 1, Continuous: true}).profile()
	if err != nil {
		t.Fatal(err)
	}
	m.start(p)
	defer m.stop()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("continuous run sent no requests")
	}
	time.Sleep(30 * time.Millisecond)
	s := m.snapshot()
	if s.State != "running" || !s.Continuous || !s.EndsAt.IsZero() {
		t.Fatalf("continuous run expired: %+v", s)
	}
	m.stop()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s = m.snapshot()
		if s.Cancelled == 1 && s.State == "stopped" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("continuous run did not cancel: %+v", s)
}
