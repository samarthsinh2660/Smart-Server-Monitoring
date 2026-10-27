//go:build windows

package metrics

import (
	"context"
	"testing"
	"unsafe"
)

func TestWindowsSDKLayout(t *testing.T) {
	var row ifRow2
	if unsafe.Sizeof(row) != 1352 || unsafe.Offsetof(row.InOctets) != 1208 || unsafe.Offsetof(row.OutOctets) != 1280 {
		t.Fatalf("MIB_IF_ROW2 layout size=%d in=%d out=%d", unsafe.Sizeof(row), unsafe.Offsetof(row.InOctets), unsafe.Offsetof(row.OutOctets))
	}
	if unsafe.Sizeof(windowsMemory{}) != 64 {
		t.Fatal("MEMORYSTATUSEX layout changed")
	}
}

func TestWindowsNativeMetrics(t *testing.T) {
	sample, err := Collect(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if sample.CPUPercent < 0 || sample.CPUPercent > 100 || sample.MemoryPercent <= 0 || sample.MemoryPercent > 100 || sample.DiskPercent < 0 || sample.DiskPercent > 100 || sample.Hostname == "" {
		t.Fatalf("invalid native metrics: %+v", sample)
	}
}
