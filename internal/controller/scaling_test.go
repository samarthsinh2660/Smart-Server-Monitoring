package controller

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"smartserver/internal/config"
	"smartserver/internal/metrics"
	"smartserver/internal/proxy"
)

type memoryControl struct {
	body []byte
	fail bool
}

func (m *memoryControl) SaveControl(_ context.Context, v any, _ string) error {
	if m.fail {
		return errors.New("disk full")
	}
	b, e := json.Marshal(v)
	m.body = b
	return e
}
func (m *memoryControl) LoadControl(_ context.Context, v any) error {
	if len(m.body) == 0 {
		return sql.ErrNoRows
	}
	return json.Unmarshal(m.body, v)
}

type fakeRouting struct {
	targets map[string]proxy.Target
	failSet bool
}

func (r *fakeRouting) Targets(context.Context) (map[string]proxy.Target, error) {
	out := map[string]proxy.Target{}
	for k, v := range r.targets {
		out[k] = v
	}
	return out, nil
}
func (r *fakeRouting) Set(_ context.Context, id, state string) error {
	if r.failSet {
		return errors.New("proxy rejected command")
	}
	t := r.targets[id]
	t.Enabled = state != "maint"
	t.Healthy = state == "ready"
	t.Status = map[string]string{"ready": "UP", "maint": "MAINT", "drain": "DRAIN"}[state]
	r.targets[id] = t
	return nil
}

func observed(id string, at time.Time, cpu float64, healthy bool) ServerStatus {
	status := "unhealthy"
	if healthy {
		status = "healthy"
	}
	return ServerStatus{ID: id, MetricsAvailable: true, Metrics: &metrics.Sample{CPUPercent: cpu, SampledAt: at}, App: AppHealth{Status: status, LastCheckedAt: &at}}
}

func newScalingTest() (*Scaling, *fakeRouting, *memoryControl, *int) {
	r := &fakeRouting{targets: map[string]proxy.Target{"primary": {Enabled: true, Healthy: true, Status: "UP"}, "standby": {Status: "MAINT"}}}
	db := &memoryControl{}
	wakes := new(int)
	s := &Scaling{Config: config.Runtime{Enabled: true, WakeEnabled: true, WarningCPU: 60, HighCPU: 70, Hold: 15 * time.Second, StartTimeout: 180 * time.Second}, Router: r, Store: db, Wake: func(context.Context, string, string) error { *wakes++; return nil }}
	return s, r, db, wakes
}

func stepTest(t *testing.T, s *Scaling, at time.Time, cpu float64, standbyHealthy bool) {
	t.Helper()
	if err := s.Step(context.Background(), at, observed("primary", at, cpu, true), observed("standby", at, 0, standbyHealthy), Prediction{}); err != nil {
		t.Fatal(err)
	}
}

func TestCPUHoldRequiresFullIntervalAndFreshSamples(t *testing.T) {
	base := time.Now()
	var hold CPUHold
	for _, n := range []int{0, 5, 10} {
		at := base.Add(time.Duration(n) * time.Second)
		hold.Observe(at, observed("primary", at, 70, true), 70)
	}
	if hold.Progress != 10*time.Second {
		t.Fatalf("three readings span %v, want 10s", hold.Progress)
	}
	at := base.Add(15 * time.Second)
	hold.Observe(at, observed("primary", at, 70, true), 70)
	if hold.Progress != 15*time.Second {
		t.Fatalf("full hold = %v", hold.Progress)
	}
	hold.Observe(at.Add(time.Second), observed("primary", at, 70, true), 70)
	if hold.Progress != 15*time.Second {
		t.Fatal("cached sample advanced hold")
	}
	missing := observed("primary", at, 70, true)
	missing.MetricsAvailable = false
	hold.Observe(at, missing, 70)
	if !hold.Since.IsZero() {
		t.Fatal("missing CPU did not reset hold")
	}
	hold.Observe(at, observed("primary", at.Add(-16*time.Second), 95, true), 70)
	if !hold.Since.IsZero() {
		t.Fatal("stale CPU started hold")
	}
	hold.Observe(at, observed("primary", at, 69, true), 70)
	if !hold.Since.IsZero() {
		t.Fatal("below threshold started hold")
	}
}

func TestActivationWakesOnceAndWaitsForAppAndProxyHealth(t *testing.T) {
	s, r, _, wakes := newScalingTest()
	base := time.Now()
	stepTest(t, s, base.Add(-time.Second), 20, false)
	for _, n := range []int{0, 5, 10} {
		stepTest(t, s, base.Add(time.Duration(n)*time.Second), 80, false)
	}
	if s.Status().State != "PRIMARY_ONLY" || *wakes != 0 {
		t.Fatal("activated before full CPU interval")
	}
	stepTest(t, s, base.Add(15*time.Second), 80, false)
	if s.Status().State != "STARTING_STANDBY" || *wakes != 1 {
		t.Fatalf("activation status=%+v wakes=%d", s.Status(), *wakes)
	}
	stepTest(t, s, base.Add(16*time.Second), 85, false)
	if r.targets["standby"].Enabled {
		t.Fatal("unhealthy standby received traffic")
	}
	stepTest(t, s, base.Add(17*time.Second), 85, true)
	if s.Status().State != "STARTING_STANDBY" {
		t.Fatal("must observe actual proxy readiness after enable")
	}
	stepTest(t, s, base.Add(18*time.Second), 85, true)
	stepTest(t, s, base.Add(23*time.Second), 85, true)
	if s.Status().State != "BOTH_ACTIVE" || *wakes != 1 {
		t.Fatalf("active=%+v wakes=%d", s.Status(), *wakes)
	}
}

func TestAlwaysOnActivationNeverSendsWakeAndWaitsForHealth(t *testing.T) {
	s, r, _, wakes := newScalingTest()
	s.Config.WakeEnabled = false
	now := time.Now()
	stepTest(t, s, now, 20, false)
	for _, n := range []int{1, 6, 11, 16} {
		stepTest(t, s, now.Add(time.Duration(n)*time.Second), 80, false)
	}
	if *wakes != 0 || s.Status().State != "STARTING_STANDBY" || r.targets["standby"].Enabled {
		t.Fatalf("unhealthy always-on target received wake/traffic: %+v wakes=%d", s.Status(), *wakes)
	}
	stepTest(t, s, now.Add(17*time.Second), 80, true)
	stepTest(t, s, now.Add(18*time.Second), 80, true)
	if s.Status().State != "BOTH_ACTIVE" || *wakes != 0 {
		t.Fatalf("always-on activation failed: %+v", s.Status())
	}
}

func TestRestartDoesNotRepeatWakeAndTimesOut(t *testing.T) {
	s, r, db, wakes := newScalingTest()
	now := time.Now()
	if err := db.SaveControl(context.Background(), ScalingStatus{State: "STARTING_STANDBY", Deadline: now.Add(time.Minute)}, ""); err != nil {
		t.Fatal(err)
	}
	stepTest(t, s, now, 90, false)
	if *wakes != 0 || s.Status().State != "STARTING_STANDBY" {
		t.Fatal("restart repeated activation")
	}
	stepTest(t, s, now.Add(61*time.Second), 90, false)
	if s.Status().State != "ACTIVATION_FAILED" || r.targets["standby"].Enabled || *wakes != 0 {
		t.Fatalf("timeout status=%+v", s.Status())
	}
}

func TestPersistenceFailurePreventsWake(t *testing.T) {
	s, _, db, wakes := newScalingTest()
	now := time.Now()
	stepTest(t, s, now, 20, false)
	db.fail = true
	err := s.Action(context.Background(), "activate", observed("primary", time.Now(), 20, true), observed("standby", time.Now(), 0, false))
	if err == nil || *wakes != 0 {
		t.Fatalf("failed persistence err=%v wakes=%d", err, *wakes)
	}
}

func TestProxyFailureCannotReportBothActive(t *testing.T) {
	s, r, _, _ := newScalingTest()
	now := time.Now()
	stepTest(t, s, now, 20, false)
	if err := s.Action(context.Background(), "activate", observed("primary", now, 20, true), observed("standby", now, 0, false)); err != nil {
		t.Fatal(err)
	}
	r.failSet = true
	err := s.Step(context.Background(), now.Add(time.Second), observed("primary", now.Add(time.Second), 20, true), observed("standby", now.Add(time.Second), 0, true), Prediction{})
	if err == nil || s.Status().State != "ACTIVATION_FAILED" {
		t.Fatalf("proxy error=%v state=%s", err, s.Status().State)
	}
}

func TestResetDrainsRequestsBeforeDisablingStandby(t *testing.T) {
	s, r, _, _ := newScalingTest()
	now := time.Now()
	r.targets["standby"] = proxy.Target{Enabled: true, Healthy: true, Status: "UP", Sessions: 2}
	stepTest(t, s, now, 20, true)
	if err := s.Action(context.Background(), "reset", observed("primary", now, 20, true), observed("standby", now, 0, true)); err != nil {
		t.Fatal(err)
	}
	stepTest(t, s, now.Add(time.Second), 20, true)
	if !r.targets["standby"].Enabled || s.Status().State != "DRAINING_STANDBY" {
		t.Fatal("reset dropped active requests")
	}
	target := r.targets["standby"]
	target.Sessions = 0
	r.targets["standby"] = target
	stepTest(t, s, now.Add(2*time.Second), 20, true)
	if r.targets["standby"].Enabled || s.Status().State != "PRIMARY_ONLY" {
		t.Fatal("completed drain did not disable target")
	}
}

func TestShadowPredictionCannotActivateHardware(t *testing.T) {
	s, _, _, wakes := newScalingTest()
	s.Config.AIEnabled = true
	now := time.Now()
	capacity := 100.0
	p := Prediction{Available: true, ScoredAt: now, Capacity: &capacity, ShadowAction: "would_wake"}
	primary, standby := observed("primary", now, 20, true), observed("standby", now, 0, false)
	if err := s.Step(context.Background(), now, primary, standby, p); err != nil {
		t.Fatal(err)
	}
	if err := s.Step(context.Background(), now, primary, standby, p); err != nil {
		t.Fatal(err)
	}
	if *wakes != 0 {
		t.Fatal("unvalidated forecast woke hardware")
	}
	p.Operational = true
	if err := s.Step(context.Background(), now, primary, standby, p); err != nil {
		t.Fatal(err)
	}
	if *wakes != 1 || s.Status().Reason != "ai_forecast" {
		t.Fatalf("validated forecast status=%+v", s.Status())
	}
}

func TestMagicPacketBytesAndInvalidMAC(t *testing.T) {
	packet, err := MagicPacket("02:11:22:33:44:55")
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != 102 || !bytes.Equal(packet[:6], bytes.Repeat([]byte{255}, 6)) {
		t.Fatal("invalid magic header")
	}
	for i := 0; i < 16; i++ {
		if !bytes.Equal(packet[6+i*6:12+i*6], []byte{2, 17, 34, 51, 68, 85}) {
			t.Fatal("invalid MAC repetition")
		}
	}
	for _, mac := range []string{"bad", "00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "01:00:00:00:00:01", "00:11:22:33:44:55:66:77"} {
		if _, err := MagicPacket(mac); err == nil {
			t.Fatalf("accepted invalid MAC %s", mac)
		}
	}
}

func TestHealthFailoverCountsDistinctProbesWithoutCPU(t *testing.T) {
	s, r, _, wakes := newScalingTest()
	base := time.Now()
	stepTest(t, s, base, 20, false)
	for i := 0; i < 3; i++ {
		at := base.Add(time.Duration(i*5+1) * time.Second)
		primary := observed("primary", at, 0, false)
		primary.MetricsAvailable = false
		primary.Metrics = nil
		for n := 0; n < 3; n++ {
			if err := s.Step(context.Background(), at, primary, observed("standby", at, 0, false), Prediction{}); err != nil {
				t.Fatal(err)
			}
		}
		if i < 2 && *wakes != 0 {
			t.Fatal("repeated cached checks triggered failover")
		}
	}
	if *wakes != 1 || r.targets["primary"].Enabled || s.Status().Reason != "primary_unhealthy" {
		t.Fatalf("failover state=%+v wakes=%d", s.Status(), *wakes)
	}
}

func TestCPUHoldResetsAcrossGapsAndBackwardsTime(t *testing.T) {
	base := time.Now()
	var h CPUHold
	h.Observe(base, observed("primary", base, 95, true), 70)
	at := base.Add(11 * time.Second)
	h.Observe(at, observed("primary", at, 95, true), 70)
	if h.Progress != 0 {
		t.Fatal("long sample gap preserved hold")
	}
	at = base.Add(10 * time.Second)
	h.Observe(at, observed("primary", at, 95, true), 70)
	if h.Progress != 0 || !h.Since.Equal(at) {
		t.Fatal("backwards timestamp preserved hold")
	}
}

func TestFailedSleepingStandbyCanBeResetWithoutWakingIt(t *testing.T) {
	s, _, _, wakes := newScalingTest()
	now := time.Now()
	stepTest(t, s, now, 20, false)
	s.mu.Lock()
	s.status.State = "ACTIVATION_FAILED"
	s.mu.Unlock()
	if err := s.Action(context.Background(), "reset", observed("primary", now, 20, true), observed("standby", now, 0, false)); err != nil {
		t.Fatal(err)
	}
	if s.Status().State != "PRIMARY_ONLY" || *wakes != 0 {
		t.Fatalf("reset failed state=%+v wakes=%d", s.Status(), *wakes)
	}
}

func TestResetCannotRemoveOnlyServingProxyTarget(t *testing.T) {
	s, r, _, _ := newScalingTest()
	now := time.Now()
	r.targets["standby"] = proxy.Target{Enabled: true, Healthy: true, Status: "UP"}
	stepTest(t, s, now, 20, true)
	r.targets["primary"] = proxy.Target{Status: "MAINT"}
	if err := s.Action(context.Background(), "reset", observed("primary", now, 20, true), observed("standby", now, 0, true)); err == nil {
		t.Fatal("reset removed the only serving proxy target")
	}
	if !r.targets["standby"].Enabled {
		t.Fatal("rejected reset disabled standby")
	}
}
