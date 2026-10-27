//go:build darwin

package telemetry

import (
	"encoding/binary"
	"math"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Host is one machine-wide CPU and memory reading. The status page asks for it
// on every telemetry tick, so everything here comes from kernel calls: no
// subprocesses, no IOKit, nothing that allocates a process.
type Host struct {
	// CPUPercent is the share of all cores that was busy (0-100) between this
	// call and the previous one. CPUReady is false when there is no usable
	// previous reading (first call, or the kernel counters wrapped), and then
	// the caller should keep the value it already shows instead of publishing a
	// made-up zero.
	CPUPercent float64
	CPUReady   bool

	// MemUsedGB is what the machine reports as used: active + wired + compressed
	// pages, capped at 96% of installed memory so a leaky accounting can never
	// draw a bar past the end. MemAvailableGB is the memory a new process could
	// claim before swapping: free + inactive + speculative pages.
	//
	// They are two different accounting views, not halves of one pie: they do
	// not add up to MemTotalGB.
	MemUsedGB      float64
	MemAvailableGB float64
	MemTotalGB     float64
	MemReady       bool
}

// mach host calls. HOST_CPU_LOAD_INFO is the aggregate per-state tick counter
// (user, system, idle, nice) and HOST_VM_INFO64 the 64-bit VM statistics. The
// MIB for kern.cp_time is gone from current macOS releases — sysctl(8) answers
// "unknown oid" — so CPU time has to come from the mach host port instead.
const (
	hostCPULoadInfo = 3
	hostVMInfo64    = 4

	// HOST_CPU_LOAD_INFO_COUNT and HOST_VM_INFO64_COUNT are the number of
	// natural_t words the kernel writes for each flavour.
	hostCPULoadInfoCount = 4
	hostVMInfo64Count    = 38

	gibibyte = 1024 * 1024 * 1024
)

// vmStatistics64 mirrors struct vm_statistics64 from <mach/vm_statistics.h>.
// The 64-bit fields force padding after the first four counters, so the field
// order here is load-bearing: changing it silently shifts every later offset.
type vmStatistics64 struct {
	freeCount     uint32
	activeCount   uint32
	inactiveCount uint32
	wireCount     uint32

	zeroFillCount uint64
	reactivations uint64
	pageins       uint64
	pageouts      uint64
	faults        uint64
	cowFaults     uint64
	lookups       uint64
	hits          uint64
	purges        uint64

	purgeableCount   uint32
	speculativeCount uint32

	decompressions uint64
	compressions   uint64
	swapins        uint64
	swapouts       uint64

	compressorPageCount uint32
	throttledCount      uint32
	externalPageCount   uint32
	internalPageCount   uint32

	totalUncompressedPagesInCompressor uint64
}

var (
	hostOnce sync.Once
	hostErr  error

	machHostSelf      func() uint32
	hostStatistics    func(uint32, int32, unsafe.Pointer, *uint32) int32
	hostStatistics64  func(uint32, int32, unsafe.Pointer, *uint32) int32
	hostStateMu       sync.Mutex
	lastCPUTicks      [4]uint64
	lastCPUTicksValid bool

	memOnce     sync.Once
	memTotalGB  float64
	memPageSize float64
	memInfoOK   bool
)

// loadHostSymbols resolves the mach entry points once. Both functions live in
// libSystem and are reachable without cgo through purego.
func loadHostSymbols() error {
	hostOnce.Do(func() {
		libSystem, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW)
		if err != nil {
			hostErr = err
			return
		}
		purego.RegisterLibFunc(&machHostSelf, libSystem, "mach_host_self")
		purego.RegisterLibFunc(&hostStatistics, libSystem, "host_statistics")
		purego.RegisterLibFunc(&hostStatistics64, libSystem, "host_statistics64")
	})
	return hostErr
}

// sysctlUint64 reads one integer sysctl. syscall.Sysctl returns the kernel's
// native-endian bytes with trailing NULs trimmed, so the value is padded back to
// eight bytes before decoding; every integer sysctl this file reads is narrower
// than that, and the trimmed bytes are zeroes by construction.
func sysctlUint64(name string) (uint64, bool) {
	value, err := syscall.Sysctl(name)
	if err != nil || len(value) == 0 || len(value) > 8 {
		return 0, false
	}
	var buf [8]byte
	copy(buf[:], value)
	return binary.LittleEndian.Uint64(buf[:]), true
}

// loadMemInfo reads the installed memory and the vm page size once. Both are
// constants for the life of the machine, so paying a syscall per tick would be
// pure waste.
func loadMemInfo() (totalGB, pageSize float64, ok bool) {
	memOnce.Do(func() {
		memsize, ok := sysctlUint64("hw.memsize")
		if !ok {
			return
		}
		pagesize, ok := sysctlUint64("hw.pagesize")
		if !ok {
			return
		}
		memTotalGB = float64(memsize) / gibibyte
		memPageSize = float64(pagesize)
		memInfoOK = memTotalGB > 0 && memPageSize > 0
	})
	return memTotalGB, memPageSize, memInfoOK
}

// readVMStatistics asks the kernel for the machine-wide page counts.
func readVMStatistics() (vmStatistics64, bool) {
	var stats vmStatistics64
	if err := loadHostSymbols(); err != nil {
		return stats, false
	}
	count := uint32(hostVMInfo64Count)
	if kr := hostStatistics64(machHostSelf(), hostVMInfo64, unsafe.Pointer(&stats), &count); kr != 0 {
		return vmStatistics64{}, false
	}
	return stats, true
}

// readCPUTicks reads the aggregate CPU tick counters. The order is the kernel's:
// CPU_STATE_USER, CPU_STATE_SYSTEM, CPU_STATE_IDLE, CPU_STATE_NICE.
func readCPUTicks() ([4]uint64, bool) {
	if err := loadHostSymbols(); err != nil {
		return [4]uint64{}, false
	}
	var ticks [4]uint32
	count := uint32(hostCPULoadInfoCount)
	if kr := hostStatistics(machHostSelf(), hostCPULoadInfo, unsafe.Pointer(&ticks[0]), &count); kr != 0 {
		return [4]uint64{}, false
	}
	return [4]uint64{uint64(ticks[0]), uint64(ticks[1]), uint64(ticks[2]), uint64(ticks[3])}, true
}

// cpuShare turns two tick readings into the percentage of all cores that was
// busy in between. A counter that went backwards means it wrapped (they are
// 32-bit) or the machine rebooted; either way there is no honest percentage for
// this interval, so the next reading becomes the new baseline.
func cpuShare() (float64, bool) {
	ticks, ok := readCPUTicks()
	if !ok {
		return 0, false
	}
	hostStateMu.Lock()
	defer hostStateMu.Unlock()
	previous := lastCPUTicks
	hadPrevious := lastCPUTicksValid
	lastCPUTicks = ticks
	lastCPUTicksValid = true
	if !hadPrevious {
		return 0, false
	}
	var busy, total uint64
	for i, current := range ticks {
		if current < previous[i] {
			return 0, false
		}
		delta := current - previous[i]
		total += delta
		if i != 2 { // everything but CPU_STATE_IDLE
			busy += delta
		}
	}
	if total == 0 {
		return 0, false
	}
	percent := float64(busy) / float64(total) * 100
	return math.Min(100, math.Max(0, percent)), true
}

// HostSample reports the machine-wide CPU and memory counters. The first call
// after process start has no CPU baseline and reports CPUReady false; memory is
// available immediately.
func HostSample() Host {
	var host Host
	if totalGB, pageSize, ok := loadMemInfo(); ok {
		host.MemTotalGB = totalGB
		if stats, ok := readVMStatistics(); ok {
			usedPages := float64(stats.activeCount) + float64(stats.wireCount) +
				float64(stats.compressorPageCount)
			availablePages := float64(stats.freeCount) + float64(stats.inactiveCount) +
				float64(stats.speculativeCount)
			host.MemUsedGB = math.Min(totalGB*0.96, usedPages*pageSize/gibibyte)
			host.MemAvailableGB = availablePages * pageSize / gibibyte
			host.MemReady = true
		}
	}
	host.CPUPercent, host.CPUReady = cpuShare()
	return host
}
