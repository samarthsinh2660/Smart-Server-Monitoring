//go:build linux

package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Collect reads a short CPU interval and the current Linux host counters.
func Collect(ctx context.Context, diskPath string) (Sample, error) {
	first, err := cpuTicks()
	if err != nil {
		return Sample{}, err
	}
	select {
	case <-ctx.Done():
		return Sample{}, ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	second, err := cpuTicks()
	if err != nil {
		return Sample{}, err
	}
	if second.total <= first.total {
		return Sample{}, fmt.Errorf("cpu ticks did not advance")
	}
	used := (second.total - first.total) - (second.idle - first.idle)
	memory, err := memoryPercent()
	if err != nil {
		return Sample{}, err
	}
	disk, err := diskPercent(diskPath)
	if err != nil {
		return Sample{}, err
	}
	rx, tx, err := networkBytes()
	if err != nil {
		return Sample{}, err
	}
	uptime, err := uptimeSeconds()
	if err != nil {
		return Sample{}, err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return Sample{}, fmt.Errorf("hostname: %w", err)
	}
	return Sample{Hostname: hostname, SampledAt: time.Now().UTC(), CPUPercent: 100 * float64(used) / float64(second.total-first.total), MemoryPercent: memory, DiskPercent: disk, NetworkRXBytes: rx, NetworkTXBytes: tx, UptimeSeconds: uptime, TemperatureC: temperature()}, nil
}

type ticks struct{ total, idle uint64 }

func cpuTicks() (ticks, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return ticks{}, fmt.Errorf("open cpu stats: %w", err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if !s.Scan() {
		return ticks{}, fmt.Errorf("read cpu stats: %v", s.Err())
	}
	fields := strings.Fields(s.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return ticks{}, fmt.Errorf("invalid cpu stats")
	}
	var result ticks
	for i, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return ticks{}, fmt.Errorf("parse cpu stats: %w", err)
		}
		result.total += value
		if i == 3 || i == 4 {
			result.idle += value
		}
	}
	return result, nil
}

func memoryPercent() (float64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("open memory stats: %w", err)
	}
	defer f.Close()
	var total, available uint64
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = value
		case "MemAvailable:":
			available = value
		}
	}
	if err := s.Err(); err != nil {
		return 0, fmt.Errorf("scan memory stats: %w", err)
	}
	if total == 0 || available > total {
		return 0, fmt.Errorf("invalid memory stats")
	}
	return 100 * float64(total-available) / float64(total), nil
}

func diskPercent(path string) (float64, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return 0, fmt.Errorf("disk stats for %s: %w", path, err)
	}
	if stats.Blocks == 0 {
		return 0, fmt.Errorf("disk has no blocks")
	}
	return 100 * float64(stats.Blocks-stats.Bfree) / float64(stats.Blocks), nil
}

func networkBytes() (uint64, uint64, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, fmt.Errorf("open network stats: %w", err)
	}
	defer f.Close()
	var rx, tx uint64
	s := bufio.NewScanner(f)
	for s.Scan() {
		parts := strings.SplitN(s.Text(), ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "lo" {
			continue
		}
		name := strings.TrimSpace(parts[0])
		// Virtual interfaces can mirror traffic and inflate totals.
		if strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			return 0, 0, fmt.Errorf("invalid network stats for %s", name)
		}
		in, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parse rx for %s: %w", name, err)
		}
		out, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parse tx for %s: %w", name, err)
		}
		rx += in
		tx += out
	}
	if err := s.Err(); err != nil {
		return 0, 0, fmt.Errorf("scan network stats: %w", err)
	}
	return rx, tx, nil
}

func uptimeSeconds() (uint64, error) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("read uptime: %w", err)
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, fmt.Errorf("invalid uptime")
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid uptime: %q", fields[0])
	}
	return uint64(value), nil
}

func temperature() *float64 {
	paths, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		millidegrees, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err == nil && millidegrees > 0 && millidegrees < 150000 {
			value := millidegrees / 1000
			return &value
		}
	}
	return nil
}
