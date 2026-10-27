package scanner

import (
	"sync"
	"time"

	"github.com/mymoles/mole-studio/internal/telemetry"
)

// The status page asks for a telemetry tick every 1.5 seconds. Forking `vm_stat`
// and `ps` on that cadence was the single most expensive thing the app did (two
// processes, ~20 ms of wall time, every tick). CPU and memory now come straight
// from the kernel through the mach host port, and the process table — which
// genuinely needs `ps` — is reused for a few seconds.

// processTableTTL bounds how stale the process list may be. The list is ranked
// by CPU, so a few seconds of lag costs nothing perceptible.
const processTableTTL = 5 * time.Second

// telemetryState carries the sampling state that has to survive between ticks.
type telemetryState struct {
	mu          sync.Mutex
	lastCPU     float64
	processes   []map[string]any
	processesAt time.Time
}

// cpuShare returns the busy share of all cores since the previous call. When the
// kernel has no usable previous reading yet (first tick), the last published
// value is repeated instead of flashing 0%.
func (s *Scanner) cpuShare(host telemetry.Host) float64 {
	s.sample.mu.Lock()
	defer s.sample.mu.Unlock()
	if host.CPUReady {
		s.sample.lastCPU = host.CPUPercent
	}
	return s.sample.lastCPU
}

// memoryUsage returns used and installed memory in GB. The mach path is exact;
// the forked `vm_stat` reading stays as a fallback for the rare case where the
// host statistics port cannot be opened.
func (s *Scanner) memoryUsage(host telemetry.Host) (float64, float64) {
	if host.MemReady && host.MemTotalGB > 0 {
		return host.MemUsedGB, host.MemTotalGB
	}
	return s.queryMemoryUsage()
}

// cachedProcesses reuses the last process table for processTableTTL.
func (s *Scanner) cachedProcesses() []map[string]any {
	s.sample.mu.Lock()
	if s.sample.processes != nil && time.Since(s.sample.processesAt) < processTableTTL {
		processes := s.sample.processes
		s.sample.mu.Unlock()
		return processes
	}
	s.sample.mu.Unlock()

	_, processes := s.sampleRealProcesses()

	s.sample.mu.Lock()
	s.sample.processes = processes
	s.sample.processesAt = time.Now()
	s.sample.mu.Unlock()
	return processes
}
