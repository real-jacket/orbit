package scanner

import (
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

func TestParseProcessesKeepsTenBusiest(t *testing.T) {
	_, processes := parseProcesses(psOutput, 8)

	if len(processes) != 10 {
		t.Fatalf("got %d processes, want the first 10: %v", len(processes), processes)
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

func TestParseProcessesClampsTheTotal(t *testing.T) {
	// A quiet machine still reports the floor the Swift app used...
	cpu, _ := parseProcesses(psOutput, 8)
	if cpu < 2.5 {
		t.Errorf("cpu = %v, want at least 2.5", cpu)
	}
	// ...and a busy one stops short of 100%.
	busy := "PID %CPU RSS COMM\n" + strings.Repeat("  99 100.0 1000 /usr/bin/busy\n", 20)
	if cpu, _ := parseProcesses(busy, 2); cpu != 99.9 {
		t.Errorf("cpu = %v, want 99.9", cpu)
	}
}

func TestParseProcessesOnEmptyOutput(t *testing.T) {
	cpu, processes := parseProcesses("", 8)
	if cpu != 2.5 {
		t.Errorf("cpu = %v, want 2.5", cpu)
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
