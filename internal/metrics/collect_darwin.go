//go:build darwin

package metrics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func macCommand(ctx context.Context, path string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return string(data), nil
}

var macIdle = regexp.MustCompile(`([0-9.]+)% idle`)
var macPageSize = regexp.MustCompile(`page size of ([0-9]+) bytes`)
var macBootSeconds = regexp.MustCompile(`sec = ([0-9]+)`)

func macCPU(raw string) (float64, error) {
	matches := macIdle.FindAllStringSubmatch(raw, -1)
	if len(matches) < 2 {
		return 0, fmt.Errorf("missing interval CPU sample")
	}
	idle, err := strconv.ParseFloat(matches[len(matches)-1][1], 64)
	if err != nil || idle < 0 || idle > 100 {
		return 0, fmt.Errorf("invalid CPU idle value")
	}
	return 100 - idle, nil
}

func macMemory(raw string, total uint64) (float64, error) {
	match := macPageSize.FindStringSubmatch(raw)
	if len(match) != 2 || total == 0 {
		return 0, fmt.Errorf("missing VM page size or physical memory")
	}
	pageSize, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil || pageSize == 0 {
		return 0, fmt.Errorf("invalid VM page size")
	}
	var available uint64
	found := 0
	for _, line := range strings.Split(raw, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok || (name != "Pages free" && name != "Pages inactive" && name != "Pages speculative") {
			continue
		}
		pages, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(value), "."), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid VM page count: %w", err)
		}
		available += pages * pageSize
		found++
	}
	if found != 3 || available > total {
		return 0, fmt.Errorf("incomplete or inconsistent VM memory counters")
	}
	return 100 * float64(total-available) / float64(total), nil
}

func macNetwork(raw string) (uint64, uint64, error) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 2 {
		return 0, 0, fmt.Errorf("missing network counters")
	}
	columns := strings.Fields(lines[0])
	in, out := -1, -1
	for i, column := range columns {
		if column == "Ibytes" {
			in = i
		}
		if column == "Obytes" {
			out = i
		}
	}
	if in < 0 || out < 0 {
		return 0, 0, fmt.Errorf("missing network byte columns")
	}
	seen := map[string]bool{}
	var rx, tx uint64
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) <= in || len(fields) <= out || len(fields) < 3 || strings.HasPrefix(fields[0], "lo") || !strings.HasPrefix(fields[2], "<Link#") || seen[fields[0]] {
			continue
		}
		// Tunnel interfaces have an empty Address column; Fields omits it.
		missing := len(columns) - len(fields)
		if missing < 0 || missing > 1 {
			return 0, 0, fmt.Errorf("unexpected network row layout")
		}
		a, e1 := strconv.ParseUint(fields[in-missing], 10, 64)
		b, e2 := strconv.ParseUint(fields[out-missing], 10, 64)
		if e1 != nil || e2 != nil {
			return 0, 0, fmt.Errorf("invalid network byte counters")
		}
		rx += a
		tx += b
		seen[fields[0]] = true
	}
	if len(seen) == 0 {
		return 0, 0, fmt.Errorf("no interface byte counters")
	}
	return rx, tx, nil
}

// Collect uses macOS built-in tools and statfs; no third-party installation is needed.
func Collect(ctx context.Context, diskPath string) (Sample, error) {
	var sample Sample
	top, err := macCommand(ctx, "/usr/bin/top", "-l", "2", "-n", "0", "-s", "1")
	if err != nil {
		return sample, err
	}
	if sample.CPUPercent, err = macCPU(top); err != nil {
		return sample, err
	}
	mem, err := macCommand(ctx, "/usr/sbin/sysctl", "-n", "hw.memsize")
	if err != nil {
		return sample, err
	}
	total, err := strconv.ParseUint(strings.TrimSpace(mem), 10, 64)
	if err != nil {
		return sample, err
	}
	vm, err := macCommand(ctx, "/usr/bin/vm_stat")
	if err != nil {
		return sample, err
	}
	if sample.MemoryPercent, err = macMemory(vm, total); err != nil {
		return sample, err
	}
	var fs syscall.Statfs_t
	if err = syscall.Statfs(diskPath, &fs); err != nil {
		return sample, err
	}
	if fs.Blocks == 0 {
		return sample, fmt.Errorf("empty filesystem capacity")
	}
	sample.DiskPercent = 100 * float64(fs.Blocks-fs.Bfree) / float64(fs.Blocks)
	boot, err := macCommand(ctx, "/usr/sbin/sysctl", "-n", "kern.boottime")
	if err != nil {
		return sample, err
	}
	seconds := macBootSeconds.FindStringSubmatch(boot)
	if len(seconds) != 2 {
		return sample, fmt.Errorf("invalid boot timestamp")
	}
	started, err := strconv.ParseInt(seconds[1], 10, 64)
	if err != nil || started > time.Now().Unix() {
		return sample, fmt.Errorf("invalid boot time")
	}
	sample.UptimeSeconds = uint64(time.Now().Unix() - started)
	network, err := macCommand(ctx, "/usr/sbin/netstat", "-ibn")
	if err != nil {
		return sample, err
	}
	if sample.NetworkRXBytes, sample.NetworkTXBytes, err = macNetwork(network); err != nil {
		return sample, err
	}
	sample.Hostname, err = os.Hostname()
	sample.SampledAt = time.Now().UTC()
	return sample, err
}
