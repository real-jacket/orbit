//go:build darwin

package telemetry

import (
	"os/exec"
	"strconv"
	"strings"
)

// diskCounters is the total number of bytes the block storage drivers moved.
type diskCounters struct {
	read  uint64
	write uint64
}

// readDiskCounters sums the byte counters of every IOBlockStorageDriver. That is
// the same source iostat reads, and summing the drivers gives the machine-wide
// total the status page claims to show.
func readDiskCounters() (diskCounters, bool) {
	output, err := ioregOutput("IOBlockStorageDriver")
	if err != nil {
		return diskCounters{}, false
	}
	return diskCountersFromIOReg(output)
}

// diskCountersFromIOReg sums the byte counters in one IOBlockStorageDriver dump.
func diskCountersFromIOReg(output string) (diskCounters, bool) {
	read, okRead := sumProperty(output, "Bytes (Read)")
	write, okWrite := sumProperty(output, "Bytes (Write)")
	if !okRead && !okWrite {
		return diskCounters{}, false
	}
	return diskCounters{read: read, write: write}, true
}

// readGPUPercent reports the busiest accelerator's utilisation, which is what
// the GPU graph on the status page shows. Apple GPUs publish it in
// IOAccelerator's PerformanceStatistics as a percentage.
func readGPUPercent() (float64, bool) {
	output, err := ioregOutput("IOAccelerator")
	if err != nil {
		return 0, false
	}
	return gpuPercentFromIOReg(output)
}

// gpuPercentFromIOReg reads the busiest accelerator's utilisation out of one
// IOAccelerator dump.
func gpuPercentFromIOReg(output string) (float64, bool) {
	best, found := 0.0, false
	for _, value := range propertyValues(output, "Device Utilization %") {
		percent, err := strconv.ParseFloat(value, 64)
		if err != nil || percent < 0 || percent > 100 {
			continue
		}
		if !found || percent > best {
			best, found = percent, true
		}
	}
	return best, found
}

// ioregOutput runs ioreg for one IOClass. -r keeps the subtree (the counters
// live inside a child dictionary), -w 0 stops the line wrapping that would
// otherwise split long property lines, and -d 1 bounds the depth.
func ioregOutput(class string) (string, error) {
	output, err := exec.Command("/usr/sbin/ioreg", "-r", "-d", "1", "-w", "0", "-c", class).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// propertyValues returns the raw value text of every occurrence of a quoted
// property name, in output order.
func propertyValues(output, name string) []string {
	marker := `"` + name + `"`
	values := make([]string, 0, 4)
	rest := output
	for {
		index := strings.Index(rest, marker)
		if index < 0 {
			return values
		}
		rest = rest[index+len(marker):]
		equals := strings.IndexByte(rest, '=')
		if equals < 0 {
			return values
		}
		rest = rest[equals+1:]
		start := 0
		for start < len(rest) && (rest[start] == ' ' || rest[start] == '\t') {
			start++
		}
		end := start
		for end < len(rest) && (rest[end] == '-' || (rest[end] >= '0' && rest[end] <= '9')) {
			end++
		}
		if end == start {
			continue
		}
		values = append(values, strings.TrimSpace(rest[start:end]))
		rest = rest[end:]
	}
}

// sumProperty adds up every occurrence of a numeric property.
func sumProperty(output, name string) (uint64, bool) {
	var total uint64
	found := false
	for _, raw := range propertyValues(output, name) {
		value, err := strconv.ParseUint(strings.TrimPrefix(raw, "-"), 10, 64)
		if err != nil {
			continue
		}
		total += value
		found = true
	}
	return total, found
}
