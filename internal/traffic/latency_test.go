package traffic

import "testing"

func TestP95Latency(t *testing.T) {
	var previous, current [11]uint64
	previous[0] = 4
	current[0] = 5
	current[2] = 19
	got, ok := P95Latency(current, previous)
	if !ok || got != 50 {
		t.Fatalf("p95: got %v, available %v", got, ok)
	}
	if _, ok := P95Latency(previous, current); ok {
		t.Fatal("decreasing counters should be unavailable")
	}
	if bucket := LatencyBucket(51); bucket != 3 {
		t.Fatalf("bucket for 51 ms = %d, want 3", bucket)
	}
}
