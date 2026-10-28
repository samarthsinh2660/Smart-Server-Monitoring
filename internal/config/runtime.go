package config

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Runtime struct {
	PrimaryApp, PrimaryAgent, StandbyApp, StandbyAgent string
	Enabled, AIEnabled                                 bool
	WakeEnabled                                        bool
	WarningCPU, HighCPU                                float64
	Hold, StartTimeout                                 time.Duration
	MAC, Broadcast, ProxySocket, Token                 string
}

func Env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func Load() (Runtime, error) {
	c := Runtime{PrimaryApp: Env("APP_URL", "http://127.0.0.1:8000"), PrimaryAgent: Env("AGENT_URL", "http://127.0.0.1:8081"), StandbyApp: os.Getenv("STANDBY_APP_URL"), StandbyAgent: os.Getenv("STANDBY_AGENT_URL"), MAC: os.Getenv("STANDBY_MAC"), Broadcast: os.Getenv("WOL_BROADCAST"), ProxySocket: Env("HAPROXY_SOCKET", "/run/haproxy/admin.sock"), Token: os.Getenv("CONTROL_TOKEN")}
	var err error
	if c.WakeEnabled, err = strconv.ParseBool(Env("STANDBY_WAKE_ENABLED", "true")); err != nil {
		return c, fmt.Errorf("STANDBY_WAKE_ENABLED: %w", err)
	}
	if c.Enabled, err = strconv.ParseBool(Env("SCALING_ENABLED", "false")); err != nil {
		return c, fmt.Errorf("SCALING_ENABLED: %w", err)
	}
	if c.AIEnabled, err = strconv.ParseBool(Env("AI_SCALING_ENABLED", "false")); err != nil {
		return c, fmt.Errorf("AI_SCALING_ENABLED: %w", err)
	}
	if c.WarningCPU, err = strconv.ParseFloat(Env("CPU_WARNING_PERCENT", "60"), 64); err != nil {
		return c, fmt.Errorf("CPU_WARNING_PERCENT: %w", err)
	}
	if c.HighCPU, err = strconv.ParseFloat(Env("CPU_ACTIVATE_PERCENT", "70"), 64); err != nil {
		return c, fmt.Errorf("CPU_ACTIVATE_PERCENT: %w", err)
	}
	if !(c.WarningCPU > 0 && c.WarningCPU < c.HighCPU && c.HighCPU <= 100) {
		return c, fmt.Errorf("require 0 < CPU_WARNING_PERCENT < CPU_ACTIVATE_PERCENT <= 100")
	}
	hold, err := strconv.Atoi(Env("CPU_HOLD_SECONDS", "15"))
	if err != nil || hold < 5 || hold > 300 {
		return c, fmt.Errorf("CPU_HOLD_SECONDS must be 5..300")
	}
	c.Hold = time.Duration(hold) * time.Second
	start, err := strconv.Atoi(Env("STANDBY_TIMEOUT_SECONDS", "180"))
	if err != nil || start < 10 || start > 600 {
		return c, fmt.Errorf("STANDBY_TIMEOUT_SECONDS must be 10..600")
	}
	c.StartTimeout = time.Duration(start) * time.Second
	if c.StandbyAgent != "" && c.StandbyApp == "" {
		return c, fmt.Errorf("STANDBY_AGENT_URL requires STANDBY_APP_URL")
	}
	for name, value := range map[string]string{"APP_URL": c.PrimaryApp, "AGENT_URL": c.PrimaryAgent, "STANDBY_APP_URL": c.StandbyApp, "STANDBY_AGENT_URL": c.StandbyAgent} {
		if value == "" {
			continue
		}
		u, e := url.Parse(value)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return c, fmt.Errorf("%s must be an HTTP origin", name)
		}
	}
	if c.Enabled {
		if c.StandbyApp == "" || len(c.Token) < 16 {
			return c, fmt.Errorf("scaling requires a standby app URL and CONTROL_TOKEN of at least 16 characters")
		}
		if !c.WakeEnabled {
			return c, nil
		}
		mac, e := net.ParseMAC(c.MAC)
		if e != nil || len(mac) != 6 || mac[0]&1 != 0 || bytes.Equal(mac, make([]byte, 6)) {
			return c, fmt.Errorf("STANDBY_MAC must be a six-byte unicast Wi-Fi MAC")
		}
		ip := net.ParseIP(c.Broadcast)
		if c.StandbyApp == "" || len(c.Token) < 16 || ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsMulticast() {
			return c, fmt.Errorf("scaling requires standby URLs, CONTROL_TOKEN of at least 16 characters, and IPv4 WOL_BROADCAST")
		}
	}
	return c, nil
}
