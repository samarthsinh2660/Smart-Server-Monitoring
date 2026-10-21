package prediction

import "testing"

func TestGateRequiresThreeRecordedScoresAndRearms(t *testing.T) {
	var gate Gate
	if gate.Observe(true, "synthetic", .99) || gate.Observe(true, "synthetic", .99) || gate.Observe(true, "synthetic", .99) {
		t.Fatal("synthetic score triggered gate")
	}
	if gate.Observe(true, "recorded", .8) || gate.Observe(true, "recorded", .9) {
		t.Fatal("gate triggered early")
	}
	if !gate.Observe(true, "recorded", .85) || gate.Observe(true, "recorded", .99) {
		t.Fatal("gate should trigger once per high streak")
	}
	if gate.Observe(false, "recorded", .99) {
		t.Fatal("unavailable score triggered gate")
	}
	gate.Observe(true, "recorded", .9)
	gate.Observe(true, "recorded", .9)
	if !gate.Observe(true, "recorded", .9) {
		t.Fatal("gate did not rearm after unavailable score")
	}
}
