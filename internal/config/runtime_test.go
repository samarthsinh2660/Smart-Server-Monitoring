package config

import "testing"

func TestRuntimeRejectsInvalidControlConfiguration(t *testing.T) {
	for _, name := range []string{"APP_URL", "AGENT_URL", "STANDBY_APP_URL", "STANDBY_AGENT_URL", "SCALING_ENABLED", "AI_SCALING_ENABLED", "CPU_WARNING_PERCENT", "CPU_ACTIVATE_PERCENT", "CPU_HOLD_SECONDS", "STANDBY_TIMEOUT_SECONDS", "STANDBY_MAC", "WOL_BROADCAST", "CONTROL_TOKEN"} {
		t.Setenv(name, "")
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Enabled || c.WarningCPU != 60 || c.HighCPU != 70 || c.Hold.Seconds() != 15 {
		t.Fatalf("wrong defaults: %+v", c)
	}
	t.Setenv("SCALING_ENABLED", "true")
	if _, err := Load(); err == nil {
		t.Fatal("enabled unconfigured hardware control")
	}
	t.Setenv("STANDBY_APP_URL", "http://192.0.2.12:8000")
	t.Setenv("STANDBY_AGENT_URL", "http://192.0.2.12:8081")
	t.Setenv("STANDBY_MAC", "02:11:22:33:44:55")
	t.Setenv("WOL_BROADCAST", "192.0.2.255")
	t.Setenv("CONTROL_TOKEN", "a-real-local-token")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"NaN", "Inf", "101", "50"} {
		t.Setenv("CPU_ACTIVATE_PERCENT", bad)
		if _, err := Load(); err == nil {
			t.Fatalf("accepted bad CPU threshold %q", bad)
		}
	}
	t.Setenv("CPU_ACTIVATE_PERCENT", "70")
	t.Setenv("STANDBY_MAC", "00:00:00:00:00:00")
	if _, err := Load(); err == nil {
		t.Fatal("accepted placeholder wake MAC")
	}
}
