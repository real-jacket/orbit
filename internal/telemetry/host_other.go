//go:build !darwin

package telemetry

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
	// pages. MemAvailableGB is the memory a new process could claim before
	// swapping. They are two different accounting views and do not add up to
	// MemTotalGB.
	MemUsedGB      float64
	MemAvailableGB float64
	MemTotalGB     float64
	MemReady       bool
}

// HostSample reports no counters on platforms without the mach host statistics
// this app is built for. The zero Host has every Ready flag false, which the
// caller reads as "no reading" rather than "zero usage".
func HostSample() Host { return Host{} }
