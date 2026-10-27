//go:build windows

package metrics

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	getSystemTimes     = kernel32.NewProc("GetSystemTimes")
	globalMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")
	getDiskFreeSpace   = kernel32.NewProc("GetDiskFreeSpaceExW")
	getTickCount       = kernel32.NewProc("GetTickCount64")
	getIfEntry         = syscall.NewLazyDLL("iphlpapi.dll").NewProc("GetIfEntry2")
)

type windowsMemory struct {
	Length, Load                                                                                         uint32
	TotalPhys, AvailPhys, TotalPageFile, AvailPageFile, TotalVirtual, AvailVirtual, AvailExtendedVirtual uint64
}

// ifRow2 matches the Windows SDK MIB_IF_ROW2 layout on Windows amd64/arm64.
// GUIDs are four-byte aligned in the SDK; their offsets here also follow uint32s.
type ifRow2 struct {
	LUID                                                                            uint64
	Index                                                                           uint32
	GUID                                                                            [16]byte
	Alias, Description                                                              [257]uint16
	AddressLength                                                                   uint32
	Address, PermanentAddress                                                       [32]byte
	MTU, Type, TunnelType, MediaType, PhysicalMediumType, AccessType, DirectionType uint32
	Flags                                                                           uint8
	OperStatus, AdminStatus, MediaConnectState                                      uint32
	NetworkGUID                                                                     [16]byte
	ConnectionType                                                                  uint32
	TransmitSpeed, ReceiveSpeed                                                     uint64
	InOctets, InUcastPkts, InNUcastPkts, InDiscards, InErrors, InUnknownProtos      uint64
	InUcastOctets, InMulticastOctets, InBroadcastOctets                             uint64
	OutOctets, OutUcastPkts, OutNUcastPkts, OutDiscards, OutErrors                  uint64
	OutUcastOctets, OutMulticastOctets, OutBroadcastOctets, OutQLen                 uint64
}

// Compile-time ABI guards also run during cross-builds. A layout mismatch must
// fail the build rather than let GetIfEntry2 overwrite a smaller Go buffer.
var (
	_ [1352 - unsafe.Sizeof(ifRow2{})]byte
	_ [unsafe.Sizeof(ifRow2{}) - 1352]byte
	_ [1208 - unsafe.Offsetof(ifRow2{}.InOctets)]byte
	_ [unsafe.Offsetof(ifRow2{}.InOctets) - 1208]byte
	_ [1280 - unsafe.Offsetof(ifRow2{}.OutOctets)]byte
	_ [unsafe.Offsetof(ifRow2{}.OutOctets) - 1280]byte
)

func windowsCPUTimes() (idle, total uint64, err error) {
	var i, k, u syscall.Filetime
	ok, _, callErr := getSystemTimes.Call(uintptr(unsafe.Pointer(&i)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u)))
	if ok == 0 {
		return 0, 0, fmt.Errorf("GetSystemTimes: %w", callErr)
	}
	value := func(f syscall.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	return value(i), value(k) + value(u), nil
}

// Collect samples whole-machine Windows CPU and reads native memory, volume,
// uptime, and physical-interface counters. Unsupported temperature is omitted.
func Collect(ctx context.Context, diskPath string) (Sample, error) {
	idle1, total1, err := windowsCPUTimes()
	if err != nil {
		return Sample{}, err
	}
	select {
	case <-ctx.Done():
		return Sample{}, ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	idle2, total2, err := windowsCPUTimes()
	if err != nil {
		return Sample{}, err
	}
	if total2 <= total1 || idle2 < idle1 || idle2-idle1 > total2-total1 {
		return Sample{}, fmt.Errorf("invalid Windows CPU interval")
	}
	mem := windowsMemory{}
	mem.Length = uint32(unsafe.Sizeof(mem))
	if ok, _, e := globalMemoryStatus.Call(uintptr(unsafe.Pointer(&mem))); ok == 0 {
		return Sample{}, fmt.Errorf("GlobalMemoryStatusEx: %w", e)
	}
	if mem.TotalPhys == 0 || mem.AvailPhys > mem.TotalPhys {
		return Sample{}, fmt.Errorf("invalid Windows memory reading")
	}
	if diskPath == "" || diskPath == "/" {
		diskPath = os.Getenv("SystemDrive") + `\`
		if diskPath == `\` {
			diskPath = `C:\`
		}
	}
	path, err := syscall.UTF16PtrFromString(diskPath)
	if err != nil {
		return Sample{}, fmt.Errorf("disk path: %w", err)
	}
	var available, total, free uint64
	if ok, _, e := getDiskFreeSpace.Call(uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free))); ok == 0 {
		return Sample{}, fmt.Errorf("GetDiskFreeSpaceEx: %w", e)
	}
	if total == 0 || free > total {
		return Sample{}, fmt.Errorf("invalid Windows disk reading")
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return Sample{}, fmt.Errorf("network interfaces: %w", err)
	}
	var rx, tx uint64
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		row := ifRow2{Index: uint32(iface.Index)}
		if code, _, _ := getIfEntry.Call(uintptr(unsafe.Pointer(&row))); code != 0 {
			return Sample{}, fmt.Errorf("GetIfEntry2 %s: %w", iface.Name, syscall.Errno(code))
		}
		if row.Flags&1 == 0 || (row.Type != 6 && row.Type != 71) {
			continue
		}
		rx += row.InOctets
		tx += row.OutOctets
	}
	uptime, _, _ := getTickCount.Call()
	host, err := os.Hostname()
	if err != nil {
		return Sample{}, fmt.Errorf("hostname: %w", err)
	}
	return Sample{Hostname: host, SampledAt: time.Now().UTC(), CPUPercent: 100 * (1 - float64(idle2-idle1)/float64(total2-total1)), MemoryPercent: 100 * float64(mem.TotalPhys-mem.AvailPhys) / float64(mem.TotalPhys), DiskPercent: 100 * float64(total-free) / float64(total), NetworkRXBytes: rx, NetworkTXBytes: tx, UptimeSeconds: uint64(uptime) / 1000}, nil
}
