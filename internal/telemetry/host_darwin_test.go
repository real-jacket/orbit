//go:build darwin

package telemetry

import (
	"math"
	"testing"
	"time"
	"unsafe"
)

func TestHostSampleMemoryAccounting(t *testing.T) {
	if err := loadHostSymbols(); err != nil {
		t.Fatal(err)
	}
	totalGB, pageSize, ok := loadMemInfo()
	if !ok {
		t.Fatal("installed memory is unavailable")
	}
	totalPages := totalGB * gibibyte / pageSize
	original := hostStatistics64
	t.Cleanup(func() { hostStatistics64 = original })
	for _, test := range []struct {
		name  string
		stats vmStatistics64
	}{
		{"above 96 percent", vmStatistics64{activeCount: uint32(totalPages * 0.99)}},
		{"inactive apps and reusable cache", vmStatistics64{
			activeCount: uint32(totalPages * 0.20), inactiveCount: uint32(totalPages * 0.40),
			speculativeCount: uint32(totalPages * 0.02), wireCount: uint32(totalPages * 0.10),
			compressorPageCount: uint32(totalPages * 0.05), externalPageCount: uint32(totalPages * 0.25),
			purgeableCount: uint32(totalPages * 0.05),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			hostStatistics64 = func(_ uint32, _ int32, output unsafe.Pointer, _ *uint32) int32 {
				*(*vmStatistics64)(output) = test.stats
				return 0
			}
			stats := test.stats
			pages := float64(stats.activeCount) + float64(stats.inactiveCount) + float64(stats.speculativeCount) +
				float64(stats.wireCount) + float64(stats.compressorPageCount) - float64(stats.externalPageCount) - float64(stats.purgeableCount)
			want := pages * pageSize / gibibyte
			host := HostSample()
			if !host.MemReady || math.Abs(host.MemUsedGB-want) > 0.000001 {
				t.Fatalf("used memory = %v GiB, want %v GiB without a display cap or cached-file overlap", host.MemUsedGB, want)
			}
			if host.Memory == nil || math.Abs(host.Memory.AppGB+host.Memory.WiredGB+host.Memory.CompressedGB-host.MemUsedGB) > 0.000001 || math.Abs(host.MemUsedGB+host.MemAvailableGB-totalGB) > 0.000001 {
				t.Fatal("memory components overlap or fail to account for installed capacity")
			}
		})
	}
}

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
// which is a different kernel interface for the free list.
//
// The two do not count the same pages: host_statistics64 publishes
// vm_page_free_count + speculative_count as free_count (xnu, osfmk/kern/host.c
// in vm_stats), while the sysctl exposes vm_page_free_count alone (xnu,
// bsd/vm/vm_unix.c). Subtracting the speculative count before comparing matters
// on small machines, where the speculative cache is a sizeable share of RAM:
// the raw difference on a 7 GB CI runner was several percent of installed
// memory, which says nothing about the struct layout.
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
	// Remaining capacity includes reusable file caches; it complements used
	// memory instead of adding the cached pages to the occupied components.
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
	// host_statistics64's free_count is vm_page_free_count + speculative_count,
	// so subtract the speculative pages before comparing against the sysctl,
	// which reports the free list alone. Pages move between the lists while the
	// two readings are taken, so a couple of percent of installed memory of slop
	// is expected; a layout mistake would be off by a factor, not by a page
	// migration.
	if stats.freeCount < stats.speculativeCount {
		t.Fatalf("free_count = %d pages < speculative_count = %d, impossible", stats.freeCount, stats.speculativeCount)
	}
	freeList := uint64(stats.freeCount - stats.speculativeCount)
	if diff := float64(freeList) - float64(freeSysctl); diff < -totalPages*0.02 || diff > totalPages*0.02 {
		t.Errorf("free_count - speculative_count = %d pages, vm.page_free_count = %d: layout or read is wrong", freeList, freeSysctl)
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
