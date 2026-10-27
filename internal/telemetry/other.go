//go:build !darwin

package telemetry

// netCounters is how many bytes each interface has moved.
type netCounters map[string]netPair

// netPair is one interface's byte counters.
type netPair struct {
	rx uint64
	tx uint64
}

// diskCounters is the total number of bytes the block storage drivers moved.
type diskCounters struct {
	read  uint64
	write uint64
}

// readNetCounters reports that the routing sysctl is not available here.
func readNetCounters() netCounters { return nil }

// readDiskCounters reports that the IORegistry is not available here.
func readDiskCounters() (diskCounters, bool) { return diskCounters{}, false }

// readGPUPercent reports that the IORegistry is not available here.
func readGPUPercent() (float64, bool) { return 0, false }
