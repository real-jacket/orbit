package scanner

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

// psOutput is the shape of `ps -Aceo pid,%cpu,rss,comm -r`, padding included.
const psOutput = `  PID  %CPU    RSS COMM
    1   0.0  21500 /sbin/launchd
   71   0.0  12044 /usr/libexec/logd
  320   0.3  48000 /Applications/Safari.app/Contents/MacOS/Safari
  412   1.2  90000 /Applications/Visual Studio Code.app/Contents/MacOS/Electron
  455   2.5  30000 kernel_task
  480   0.0  10000 ps
  501   0.0    512 /usr/bin/tiny
  502   4.0  64000 /usr/libexec/one
  503   1.0  64000 /usr/libexec/two
  504   1.0  64000 /usr/libexec/three
  505   1.0  64000 /usr/libexec/four
  506   1.0  64000 /usr/libexec/five
  507   1.0  64000 /usr/libexec/six
  508   1.0  64000 /usr/libexec/seven
  509   1.0  64000 /usr/libexec/eight
  510   1.0  64000 /usr/libexec/nine
  511   1.0  64000 /usr/libexec/ten
  512   1.0  64000 /usr/libexec/eleven
  513   1.0  64000 /usr/libexec/twelve
`

func TestParseProcessesSkipsRowsItCannotUse(t *testing.T) {
	_, processes := parseProcesses(psOutput, 8)

	for _, p := range processes {
		pid := p["pid"].(int)
		name := p["name"].(string)
		if pid <= 1 {
			t.Errorf("kept pid %d", pid)
		}
		if name == "ps" || name == "kernel_task" {
			t.Errorf("kept %q", name)
		}
	}
}

func TestParseProcessesKeepsTheFullSampleForMemorySorting(t *testing.T) {
	_, processes := parseProcesses(psOutput, 8)

	if len(processes) != 16 {
		t.Fatalf("got %d processes, want all 16 usable rows: %v", len(processes), processes)
	}
	if processes[len(processes)-1]["name"] != "twelve" {
		t.Fatal("a low-CPU process must remain available for memory sorting and search")
	}
	// The list keeps the order ps printed, so the rows after launchd lead.
	if name := processes[0]["name"].(string); name != "logd" {
		t.Errorf("first process = %q, want logd", name)
	}
	if name := processes[2]["name"].(string); name != "Electron" {
		t.Errorf("third process = %q, want Electron", name)
	}
}

func TestParseProcessesKeepsSpacesInCommandPaths(t *testing.T) {
	// The command column is the last one, so a path with spaces survives.
	_, processes := parseProcesses(psOutput, 8)

	var electron map[string]any
	for _, p := range processes {
		if p["name"].(string) == "Electron" {
			electron = p
		}
	}
	if electron == nil {
		t.Fatal("Electron is missing from the sample")
	}
	want := "/Applications/Visual Studio Code.app/Contents/MacOS/Electron"
	if got := electron["path"].(string); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	// 90000 KB, from the rss column.
	if got := electron["memory_mb"].(int); got != 87 {
		t.Errorf("memory_mb = %d, want 87", got)
	}
	if got := electron["cpu"].(float64); got != 1.2 {
		t.Errorf("cpu = %v, want 1.2", got)
	}
	if got := electron["pinned"].(bool); got {
		t.Error("pinned = true, want false")
	}
}

func TestParseProcessesNeverReportsZeroMemory(t *testing.T) {
	_, processes := parseProcesses(psOutput, 8)

	for _, p := range processes {
		if memory := p["memory_mb"].(int); memory < 1 {
			t.Errorf("%v has memory_mb %d, want at least 1", p["name"], memory)
		}
	}
}

func TestParseProcessesReportsTheRealTotal(t *testing.T) {
	// The Swift app floored the total at 2.5%, which made an idle machine look
	// busy; the real sum of this fixture is 16.5% over 8 cores.
	if cpu, _ := parseProcesses(psOutput, 8); cpu != 2.0625 {
		t.Errorf("cpu = %v, want 2.0625", cpu)
	}
	// A busy one still stops short of 100%.
	busy := "PID %CPU RSS COMM\n" + strings.Repeat("  99 100.0 1000 /usr/bin/busy\n", 20)
	if cpu, _ := parseProcesses(busy, 2); cpu != 99.9 {
		t.Errorf("cpu = %v, want 99.9", cpu)
	}
}

func TestParseProcessesOnEmptyOutput(t *testing.T) {
	cpu, processes := parseProcesses("", 8)
	if cpu != 0 {
		t.Errorf("cpu = %v, want 0", cpu)
	}
	if len(processes) != 0 {
		t.Errorf("got %d processes, want none", len(processes))
	}
}

func TestPhysicalMemory(t *testing.T) {
	total := physicalMemory()
	if total <= 0 {
		t.Fatalf("physicalMemory = %d, want the installed memory", total)
	}
	if total < 1<<30 {
		t.Errorf("physicalMemory = %d, want at least a gigabyte", total)
	}
}

func TestVMStatistics(t *testing.T) {
	if testing.Short() {
		t.Skip("runs vm_stat")
	}
	pageSize, usedPages, ok := vmStatistics()
	if !ok {
		t.Skip("vm_stat is not available here")
	}
	if pageSize < 4096 {
		t.Errorf("pageSize = %d, want a plausible page size", pageSize)
	}
	if usedPages <= 0 {
		t.Errorf("usedPages = %d, want a positive count", usedPages)
	}
}

func TestVMStatisticsMemoryAccountingMatchesNativePages(t *testing.T) {
	pageSize, pages, ok := parseVMStatistics(`Mach Virtual Memory Statistics: (page size of 4096 bytes)
Pages active: 20.
Pages inactive: 40.
Pages speculative: 2.
Pages wired down: 10.
Pages occupied by compressor: 5.
File-backed pages: 25.
Pages purgeable: 5.
`)
	if !ok || pageSize != 4096 || pages != 47 {
		t.Fatalf("memory fallback = %d-byte pages, %d used, ready=%v", pageSize, pages, ok)
	}
	if _, _, ready := parseVMStatistics("not a memory reading"); ready {
		t.Fatal("a missing reading became an idle sample")
	}
}

func TestTelemetryIncludesDiskCapacityAndMemoryBreakdown(t *testing.T) {
	reading := Shared.BuildTelemetry(true)
	if reading.TotalDiskGb <= 0 || reading.FreeDiskGb > reading.TotalDiskGb {
		t.Fatalf("disk capacity = %v free / %v total", reading.FreeDiskGb, reading.TotalDiskGb)
	}
	if reading.Memory == nil || reading.MemUsedGb == nil {
		t.Fatal("native memory breakdown was not included in telemetry")
	}
	parts := reading.Memory.AppGB + reading.Memory.WiredGB + reading.Memory.CompressedGB
	if difference := parts - *reading.MemUsedGb; difference < -0.01 || difference > 0.01 {
		t.Fatalf("memory parts %v GiB do not match used %v GiB", parts, *reading.MemUsedGb)
	}
}

// A tick inside the process cache window must not resend the ~64 KB table the
// page already has, but a page that just opened asks for it explicitly.
func TestTelemetryOmitsCachedProcessesUnlessAsked(t *testing.T) {
	scanner := &Scanner{home: t.TempDir()}
	if first := scanner.BuildTelemetry(false); first.Processes == nil {
		t.Skip("ps produced no process table on this machine")
	}
	tick, _ := json.Marshal(scanner.BuildTelemetry(false))
	if strings.Contains(string(tick), `"processes"`) {
		t.Fatal("a cached process table was resent on a plain tick")
	}
	opened, _ := json.Marshal(scanner.BuildTelemetry(true))
	if !strings.Contains(string(opened), `"processes":[`) {
		t.Fatal("a page that just opened did not get the process table")
	}
}

func TestQueryCPUModel(t *testing.T) {
	model := Shared.queryCPUModel()
	if model == "" {
		t.Fatal("queryCPUModel returned nothing")
	}
	if !strings.HasSuffix(model, "核)") {
		t.Errorf("model = %q, want it to end with the core count", model)
	}
	if want := "(" + itoa64(int64(runtime.NumCPU())) + "核)"; !strings.Contains(model, want) {
		t.Errorf("model = %q, want it to mention %s", model, want)
	}
}

func TestSplitFields(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want []string
	}{
		{"padded columns", "  512   1.0  64000 /usr/bin/thing", 4, []string{"512", "1.0", "64000", "/usr/bin/thing"}},
		{"inner spaces survive the last field", "  9  0.0  1  /Applications/Foo  Bar.app/x", 4,
			[]string{"9", "0.0", "1", "/Applications/Foo  Bar.app/x"}},
		{"tabs split too", "7\t2.0\t99\t/usr/bin/y", 4, []string{"7", "2.0", "99", "/usr/bin/y"}},
		{"too few fields", "12  0.0", 4, []string{"12", "0.0"}},
		{"trailing padding is dropped", "12 0.0 5 /bin/z   ", 4, []string{"12", "0.0", "5", "/bin/z"}},
		{"blank line", "   ", 4, nil},
	}
	for _, c := range cases {
		got := splitFields(c.in, c.n)
		if len(got) != len(c.want) {
			t.Errorf("%s: splitFields(%q) = %q, want %q", c.name, c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: field %d = %q, want %q (whole: %q)", c.name, i, got[i], c.want[i], got)
			}
		}
	}
}
