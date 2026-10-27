// Package telemetry samples the machine-wide counters the status page shows on
// top of the per-process numbers: network throughput, disk throughput, GPU
// utilisation, CPU temperature and fan speeds.
//
// Throughput is a rate, so it takes two readings; the Sampler keeps the
// previous one. Everything is published through public or long-stable kernel
// interfaces: the routing sysctl for network counters, the IORegistry for the
// storage drivers and accelerators, and AppleSMC for temperatures and fans.
package telemetry

import (
	"strings"
	"sync"
	"time"

	"github.com/mymoles/mole-studio/internal/smc"
)

// How often each source is re-read. Network counters are a cheap sysctl and are
// read on every call, because the status page shows them moving. ioreg spawns a
// process (~25 ms each), so disk and GPU are sampled every diskEvery; SMC calls
// are cheap after the connection is open but temperatures move slowly.
const (
	diskEvery = 3 * time.Second
	gpuEvery  = 3 * time.Second
	smcEvery  = 5 * time.Second
)

// Net is bytes per second across all interfaces.
type Net struct {
	RxBytesPerSec float64 `json:"rxBytesPerSec"`
	TxBytesPerSec float64 `json:"txBytesPerSec"`
}

// DiskIO is bytes per second across all block storage drivers.
type DiskIO struct {
	ReadBytesPerSec  float64 `json:"readBytesPerSec"`
	WriteBytesPerSec float64 `json:"writeBytesPerSec"`
}

// Sample is one reading of the machine. Every field is omitted when this
// machine does not expose it, so the frontend can tell "no sensor" from "zero".
type Sample struct {
	Net      *Net     `json:"net,omitempty"`
	DiskIO   *DiskIO  `json:"diskIo,omitempty"`
	GPUPct   *float64 `json:"gpuPct,omitempty"`
	CPUTempC *float64 `json:"cpuTempC,omitempty"`
	Fans     []int    `json:"fans,omitempty"`
}

// Sampler keeps the previous counters so it can report rates. The zero value is
// not usable; call New or use Shared.
type Sampler struct {
	mu sync.Mutex

	netPrev   netCounters
	netPrevAt time.Time
	netOK     bool

	diskPrev   diskCounters
	diskPrevAt time.Time
	diskOK     bool

	diskCache   diskCounters
	diskCacheAt time.Time
	diskCacheOK bool

	gpuCache   float64
	gpuCacheAt time.Time
	gpuCacheOK bool

	smcOnce sync.Once
	smc     *smc.Client

	smcAt       time.Time
	smcTemp     *float64
	smcFans     []int
	smcReadOnce bool
}

// New returns a sampler with no previous reading: the first Snapshot reports
// temperatures, fan speeds and utilisation, but throughput rates start at zero
// and become meaningful on the second call.
func New() *Sampler { return &Sampler{} }

// Shared is the process-wide sampler, so consecutive telemetry ticks share the
// previous counters.
var Shared = New()

// Snapshot reads every source that is due and returns the current sample.
func (s *Sampler) Snapshot() Sample {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	sample := Sample{}
	s.prefetchIOReg(now)
	sample.Net = s.netRate(now)
	sample.DiskIO = s.diskRate(now)
	if v, ok := s.gpuPercent(now); ok {
		sample.GPUPct = &v
	}
	temp, fans := s.smcReading(now)
	sample.CPUTempC = temp
	sample.Fans = fans
	return sample
}

// netRate is the network throughput since the previous call, summed over the
// interfaces that count (see countsForNet).
func (s *Sampler) netRate(now time.Time) *Net {
	counters := readNetCounters()
	if counters == nil {
		s.netOK = false
		return nil
	}
	net := &Net{}
	if s.netOK && !s.netPrevAt.IsZero() {
		rx, tx := netDelta(s.netPrev, counters)
		elapsed := now.Sub(s.netPrevAt)
		net.RxBytesPerSec = rate(0, rx, elapsed)
		net.TxBytesPerSec = rate(0, tx, elapsed)
	}
	s.netPrev, s.netPrevAt, s.netOK = counters, now, true
	return net
}

// netDelta sums the growth of every counting interface between two readings. A
// counter that went backwards wrapped (the kernel reports 32-bit values in the
// routing sysctl), and that interface contributes nothing for the interval
// instead of a negative spike.
func netDelta(previous, current netCounters) (rx, tx uint64) {
	for name, now := range current {
		if !countsForNet(name) {
			continue
		}
		before, ok := previous[name]
		if !ok {
			continue
		}
		if now.rx >= before.rx {
			rx += now.rx - before.rx
		}
		if now.tx >= before.tx {
			tx += now.tx - before.tx
		}
	}
	return rx, tx
}

// countsForNet reports whether an interface counts towards the network
// throughput the status page shows. Loopback is local traffic, and AirDrop's
// peer-to-peer links are not what a person means by "network".
func countsForNet(name string) bool {
	if strings.HasPrefix(name, "lo") {
		return false
	}
	if strings.HasPrefix(name, "awdl") || strings.HasPrefix(name, "llw") {
		return false
	}
	return true
}

// diskRate is the disk throughput between the last two readings of the
// counters. The counters themselves are re-read at most every diskEvery, so the
// rate is measured between the times the counters were actually sampled, not
// between the times the status page asked.
func (s *Sampler) diskRate(now time.Time) *DiskIO {
	counter, sampledAt, ok := s.diskCounters(now)
	if !ok {
		s.diskOK = false
		return nil
	}
	io := &DiskIO{}
	if s.diskOK && !s.diskPrevAt.IsZero() {
		elapsed := sampledAt.Sub(s.diskPrevAt)
		io.ReadBytesPerSec = rate(s.diskPrev.read, counter.read, elapsed)
		io.WriteBytesPerSec = rate(s.diskPrev.write, counter.write, elapsed)
	}
	s.diskPrev, s.diskPrevAt, s.diskOK = counter, sampledAt, true
	return io
}

// prefetchIOReg refreshes the two IORegistry-backed readings at the same time.
// Each `ioreg` invocation costs ~25 ms, so serialising them would spend a third
// of every 1.5 s telemetry tick waiting on the kernel to describe itself.
func (s *Sampler) prefetchIOReg(now time.Time) {
	needDisk := !s.diskCacheOK || now.Sub(s.diskCacheAt) >= diskEvery
	needGPU := !s.gpuCacheOK || now.Sub(s.gpuCacheAt) >= gpuEvery
	if !needDisk && !needGPU {
		return
	}
	var wait sync.WaitGroup
	if needDisk {
		wait.Add(1)
		go func() {
			defer wait.Done()
			counter, ok := readDiskCounters()
			s.diskCache, s.diskCacheAt, s.diskCacheOK = counter, now, ok
		}()
	}
	if needGPU {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, ok := readGPUPercent()
			s.gpuCache, s.gpuCacheAt, s.gpuCacheOK = value, now, ok
		}()
	}
	wait.Wait()
}

// diskCounters returns the latest counter reading and when it was taken.
func (s *Sampler) diskCounters(now time.Time) (diskCounters, time.Time, bool) {
	if s.diskCacheOK && now.Sub(s.diskCacheAt) < diskEvery {
		return s.diskCache, s.diskCacheAt, true
	}
	counter, ok := readDiskCounters()
	s.diskCache, s.diskCacheAt, s.diskCacheOK = counter, now, ok
	return counter, now, ok
}

func (s *Sampler) gpuPercent(now time.Time) (float64, bool) {
	if !s.gpuCacheOK || now.Sub(s.gpuCacheAt) >= gpuEvery {
		value, ok := readGPUPercent()
		s.gpuCache, s.gpuCacheAt, s.gpuCacheOK = value, now, ok
	}
	return s.gpuCache, s.gpuCacheOK
}

// smcReading reads CPU temperature and fan speeds, at most every smcEvery.
func (s *Sampler) smcReading(now time.Time) (*float64, []int) {
	if s.smcReadOnce && now.Sub(s.smcAt) < smcEvery {
		return s.smcTemp, s.smcFans
	}
	s.smcOnce.Do(func() {
		client, err := smc.Open()
		if err == nil {
			s.smc = client
		}
	})
	s.smcAt = now
	s.smcReadOnce = true
	s.smcTemp, s.smcFans = nil, nil
	if s.smc == nil {
		return nil, nil
	}
	if value, ok := s.smc.CPUTemperature(); ok {
		s.smcTemp = &value
	}
	if fans := s.smc.Fans(); len(fans) > 0 {
		s.smcFans = fans
	}
	return s.smcTemp, s.smcFans
}

// rate turns two cumulative counters into per-second bytes. A counter that
// went backwards means it was reset or wrapped, and the honest answer is zero
// for that interval rather than a negative or absurd spike.
func rate(previous, current uint64, elapsed time.Duration) float64 {
	if elapsed <= 0 || current < previous {
		return 0
	}
	return float64(current-previous) / elapsed.Seconds()
}
