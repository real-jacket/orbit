//go:build darwin

package telemetry

import (
	"testing"
	"time"
	"unsafe"
)

// TestVMStatistics64Layout pins the hand-written struct to the C layout in
// <mach/vm_statistics.h>. host_statistics64 writes raw bytes into it, so a
// reordered field or a wrong Go type would silently produce counts read from the
// middle of a neighbouring field; the offsets below are the ones from the
// header. 152 bytes / 4 = 38 natural_t words, which the kernel is told to write.
func TestVMStatistics64Layout(t *testing.T) {
	var stats vmStatistics64
	if size := unsafe.Sizeof(stats); size != 152 {
		t.Fatalf("sizeof(vm_statistics64) = %d, want 152", size)
	}
	offsets := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"free_count", unsafe.Offsetof(stats.freeCount), 0},
		{"active_count", unsafe.Offsetof(stats.activeCount), 4},
		{"inactive_count", unsafe.Offsetof(stats.inactiveCount), 8},
		{"wire_count", unsafe.Offsetof(stats.wireCount), 12},
		{"zero_fill_count", unsafe.Offsetof(stats.zeroFillCount), 16},
		{"purges", unsafe.Offsetof(stats.purges), 80},
		{"purgeable_count", unsafe.Offsetof(stats.purgeableCount), 88},
		{"speculative_count", unsafe.Offsetof(stats.speculativeCount), 92},
		{"decompressions", unsafe.Offsetof(stats.decompressions), 96},
		{"compressor_page_count", unsafe.Offsetof(stats.compressorPageCount), 128},
		{"total_uncompressed_pages_in_compressor", unsafe.Offsetof(stats.totalUncompressedPagesInCompressor), 144},
	}
	for _, o := range offsets {
		if o.got != o.want {
			t.Errorf("offsetof(%s) = %d, want %d", o.name, o.got, o.want)
		}
	}
	if want := int(unsafe.Sizeof(stats)) / 4; hostVMInfo64Count != want {
		t.Errorf("HOST_VM_INFO64_COUNT = %d, want %d", hostVMInfo64Count, want)
	}
}

// TestHostSampleMemorySane checks that the four page counters are decoded from
// the right places and that the derived numbers are physically possible. The
// free-page counter is cross-checked against the vm.page_free_count sysctl,
// which is a different kernel interface for the same number.
func TestHostSampleMemorySane(t *testing.T) {
	host := HostSample()
	if !host.MemReady {
		t.Fatal("MemReady = false: host_statistics64 or hw.memsize failed")
	}
	if host.MemTotalGB < 0.5 || host.MemTotalGB > 4096 {
		t.Fatalf("MemTotalGB = %v, want a plausible machine size", host.MemTotalGB)
	}
	if host.MemUsedGB <= 0 || host.MemUsedGB > host.MemTotalGB {
		t.Fatalf("MemUsedGB = %v, want 0 < used <= %v", host.MemUsedGB, host.MemTotalGB)
	}
	if host.MemAvailableGB <= 0 || host.MemAvailableGB > host.MemTotalGB {
		t.Fatalf("MemAvailableGB = %v, want 0 < available <= %v", host.MemAvailableGB, host.MemTotalGB)
	}
	// The two views overlap (wired pages are neither free nor reclaimable, but
	// both counters count the file cache differently), so the only invariant is
	// that together they cannot describe more memory than is installed.
	if sum := host.MemUsedGB + host.MemAvailableGB; sum > host.MemTotalGB*1.05 {
		t.Fatalf("used+available = %v GB > total %v GB", sum, host.MemTotalGB)
	}

	totalGB, pageSize, ok := loadMemInfo()
	if !ok || pageSize <= 0 {
		t.Fatal("loadMemInfo failed")
	}
	totalPages := totalGB * gibibyte / pageSize
	stats, ok := readVMStatistics()
	if !ok {
		t.Fatal("readVMStatistics failed")
	}
	for name, count := range map[string]uint32{
		"free_count":            stats.freeCount,
		"active_count":          stats.activeCount,
		"inactive_count":        stats.inactiveCount,
		"wire_count":            stats.wireCount,
		"speculative_count":     stats.speculativeCount,
		"compressor_page_count": stats.compressorPageCount,
		"purgeable_count":       stats.purgeableCount,
		"internal_page_count":   stats.internalPageCount,
		"external_page_count":   stats.externalPageCount,
	} {
		if float64(count) > totalPages {
			t.Errorf("%s = %d pages, more than the %v pages installed", name, count, totalPages)
		}
	}

	freeSysctl, ok := sysctlUint64("vm.page_free_count")
	if !ok {
		t.Skip("vm.page_free_count sysctl unavailable")
	}
	// Pages move between the free lists while the two readings are taken, so a
	// couple of percent of installed memory of slop is expected; a layout
	// mistake would be off by a factor, not by a page migration.
	if diff := float64(stats.freeCount) - float64(freeSysctl); diff < -totalPages*0.02 || diff > totalPages*0.02 {
		t.Errorf("free_count = %d pages, vm.page_free_count = %d: layout or read is wrong", stats.freeCount, freeSysctl)
	}
}

// TestHostSampleCPUReady checks the two-call contract: the first call only seeds
// the baseline, a later one reports the share of all cores that was busy.
func TestHostSampleCPUReady(t *testing.T) {
	sample := HostSample()
	deadline := time.Now().Add(2 * time.Second)
	for !sample.CPUReady && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
		sample = HostSample()
	}
	if !sample.CPUReady {
		t.Fatal("CPUReady stayed false: host_statistics(HOST_CPU_LOAD_INFO) is broken")
	}
	if sample.CPUPercent < 0 || sample.CPUPercent > 100 {
		t.Fatalf("CPUPercent = %v, want 0..100", sample.CPUPercent)
	}
}

// TestHostSampleCheap is the regression guard for the whole point of this file:
// sampling must not fork. A hundred samples cover more than two minutes of the
// 1.5 s telemetry tick; if anything went back to exec-ing vm_stat or ps this
// takes seconds, not milliseconds.
func TestHostSampleCheap(t *testing.T) {
	const samples = 100
	const budget = 2 * time.Millisecond // per sample, averaged
	start := time.Now()
	for i := 0; i < samples; i++ {
		HostSample()
	}
	elapsed := time.Since(start)
	perSample := elapsed / samples
	t.Logf("%d samples in %v (avg %v per sample)", samples, elapsed, perSample)
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("%d samples took %v, want < 1.5s: sampling is forking again", samples, elapsed)
	}
	if perSample > budget {
		t.Fatalf("average sample took %v, want < %v", perSample, budget)
	}
}

// BenchmarkHostSample measures one full CPU+memory sample, which is what the
// telemetry tick pays every 1.5 seconds.
func BenchmarkHostSample(b *testing.B) {
	HostSample() // seed the CPU baseline so the loop measures a real delta
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		HostSample()
	}
}

// BenchmarkHostSampleCPU measures only the CPU half of a sample.
func BenchmarkHostSampleCPU(b *testing.B) {
	cpuShare()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cpuShare()
	}
}

// BenchmarkHostSampleMemory measures only the memory half of a sample.
func BenchmarkHostSampleMemory(b *testing.B) {
	totalGB, pageSize, _ := loadMemInfo()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		stats, ok := readVMStatistics()
		if !ok {
			b.Fatal("readVMStatistics failed")
		}
		_ = totalGB * float64(stats.activeCount+stats.wireCount+stats.compressorPageCount) * pageSize / gibibyte
		_ = float64(stats.freeCount+stats.inactiveCount+stats.speculativeCount) * pageSize / gibibyte
	}
}
