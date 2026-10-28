//go:build darwin

package metrics

import (
	"context"
	"testing"
	"time"
)

func TestMacParsersRejectMissingSamples(t *testing.T) {
	if _, err := macCPU("CPU usage: 50% idle"); err == nil {
		t.Fatal("accepted initial CPU sample without interval")
	}
	if v, err := macCPU("CPU usage: 50% idle\nCPU usage: 75% idle"); err != nil || v != 25 {
		t.Fatalf("CPU=%v error=%v", v, err)
	}
	if v, err := macMemory("Mach Virtual Memory Statistics: (page size of 4096 bytes)\nPages free: 1.\nPages inactive: 1.\nPages speculative: 0.\n", 16384); err != nil || v != 50 {
		t.Fatalf("memory=%v error=%v", v, err)
	}
	if _, err := macMemory("", 16384); err == nil {
		t.Fatal("missing memory accepted")
	}
	if rx, tx, err := macNetwork("Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes\nen0 1500 <Link#1> ab:cd 1 0 100 2 0 200\nen0 1500 10.0.0 10.0.0.1 1 0 100 2 0 200\n"); err != nil || rx != 100 || tx != 200 {
		t.Fatalf("network=%d/%d error=%v", rx, tx, err)
	}
}

func TestMacNativeMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s, err := Collect(ctx, "/System/Volumes/Data")
	if err != nil {
		t.Fatal(err)
	}
	if s.CPUPercent < 0 || s.CPUPercent > 100 || s.MemoryPercent <= 0 || s.MemoryPercent > 100 || s.DiskPercent <= 0 || s.DiskPercent > 100 || s.Hostname == "" || s.UptimeSeconds == 0 {
		t.Fatalf("invalid native metrics: %+v", s)
	}
	t.Logf("CPU %.2f memory %.2f disk %.2f uptime %d", s.CPUPercent, s.MemoryPercent, s.DiskPercent, s.UptimeSeconds)
}
