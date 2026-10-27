//go:build darwin

package telemetry

import (
	"os"
	"testing"
)

// The fixtures below are trimmed copies of real ioreg output: the property
// lines are verbatim, only the unrelated keys around them are dropped.
const storageDriverOutput = `+-o IOBlockStorageDriver  <class IOBlockStorageDriver, id 0x100000a4c, registered, matched, active, busy 0 (75 ms), retain 7>
    {
      "Statistics" = {"Operations (Read)"=3938687,"Latency Time (Read)"=10553052,"Total Time (Write)"=17989370,"Total Time (Read)"=35809189,"Operations (Write)"=2424639,"Latency Time (Write)"=12652951,"Bytes (Write)"=498639101952,"Bytes (Read)"=683249713152,"Errors (Read)"=0,"Retries (Read)"=0}
    }

+-o IOBlockStorageDriver  <class IOBlockStorageDriver, id 0x100000b11, registered, matched, active, busy 0 (3 ms), retain 6>
    {
      "Statistics" = {"Operations (Read)"=394,"Latency Time (Read)"=61,"Total Time (Write)"=0,"Total Time (Read)"=88,"Operations (Write)"=243,"Latency Time (Write)"=0,"Bytes (Write)"=635392,"Bytes (Read)"=2196471,"Errors (Read)"=0,"Retries (Read)"=0}
    }
`

const acceleratorOutput = `+-o AGXAcceleratorG13X  <class AGXAcceleratorG13X, id 0x100000c22, registered, matched, active, busy 0 (106 ms), retain 7>
    {
      "PerformanceStatistics" = {"In use system memory (driver)"=0,"Alloc system memory"=1906749440,"Tiled system memory (driver)"=0,"Device Utilization %"=69,"Renderer Utilization %"=63,"Tiler Utilization %"=41,"SplitSceneCount"=0,"recoveryCount"=0,"GPU Busy"=69}
    }
`

func TestDiskCountersSumEveryStorageDriver(t *testing.T) {
	counters, ok := diskCountersFromIOReg(storageDriverOutput)
	if !ok {
		t.Fatal("diskCountersFromIOReg refused a real dump")
	}
	wantRead := uint64(683249713152 + 2196471)
	wantWrite := uint64(498639101952 + 635392)
	if counters.read != wantRead || counters.write != wantWrite {
		t.Errorf("counters = %+v, want read=%d write=%d", counters, wantRead, wantWrite)
	}
}

func TestDiskCountersRefuseADumpWithoutStatistics(t *testing.T) {
	if _, ok := diskCountersFromIOReg(`+-o IOBlockStorageDriver  <class IOBlockStorageDriver>`); ok {
		t.Fatal("a dump without Statistics must not look like a zero reading")
	}
}

func TestGPUPercentReadsDeviceUtilization(t *testing.T) {
	percent, ok := gpuPercentFromIOReg(acceleratorOutput)
	if !ok {
		t.Fatal("gpuPercentFromIOReg refused a real dump")
	}
	if percent != 69 {
		t.Errorf("gpuPercent = %v, want 69", percent)
	}
}

func TestGPUPercentRejectsImpossibleValues(t *testing.T) {
	dump := `"PerformanceStatistics" = {"Device Utilization %"=421}`
	if _, ok := gpuPercentFromIOReg(dump); ok {
		t.Fatal("a utilisation above 100 must be refused, not displayed")
	}
}

// TestInterfaceNameReadsTheLinkSockaddr builds the record the kernel returns:
// a 160-byte if_msghdr2 followed by an AF_LINK sockaddr_dl carrying the name.
func TestInterfaceNameReadsTheLinkSockaddr(t *testing.T) {
	record := make([]byte, 180)
	record[3] = rtmIfinfo2
	link := record[ifmHeaderSize:]
	link[0] = 20   // sdl_len
	link[1] = 0x12 // AF_LINK
	link[5] = 3    // sdl_nlen
	copy(record[ifmHeaderSize+sockaddrDLExt:], "en0")

	if got := interfaceName(record); got != "en0" {
		t.Fatalf("interfaceName = %q, want %q", got, "en0")
	}
}

func TestInterfaceNameRejectsRecordsWithoutALinkSockaddr(t *testing.T) {
	record := make([]byte, 180)
	record[3] = rtmIfinfo2
	if got := interfaceName(record); got != "" {
		t.Fatalf("interfaceName = %q, want empty", got)
	}
}

// TestLiveIORegistryCounters reads the real machine. It only runs when asked,
// because it depends on the hardware and on ioreg being allowed.
func TestLiveIORegistryCounters(t *testing.T) {
	if os.Getenv("MOLE_TELEMETRY_E2E") != "1" {
		t.Skip("set MOLE_TELEMETRY_E2E=1 to read this machine's IORegistry")
	}
	disk, ok := readDiskCounters()
	if !ok {
		t.Fatal("the IORegistry reports no IOBlockStorageDriver counters")
	}
	if disk.read == 0 || disk.write == 0 {
		t.Fatalf("a running Mac has written and read bytes: %+v", disk)
	}
	if percent, ok := readGPUPercent(); ok && (percent < 0 || percent > 100) {
		t.Fatalf("GPU utilisation %.1f is outside 0..100", percent)
	}
	counters := readNetCounters()
	if len(counters) == 0 {
		t.Fatal("the routing sysctl reported no interfaces")
	}
}
