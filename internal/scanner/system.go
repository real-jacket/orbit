package scanner

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// queryDiskSpace reports the free and total space of the volume the home
// directory is on, in gigabytes.
func (s *Scanner) queryDiskSpace() (freeGB, totalGB float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.home, &stat); err != nil {
		return 0, 0
	}
	free := float64(stat.Bavail) * float64(stat.Bsize) / 1_000_000_000
	total := float64(stat.Blocks) * float64(stat.Bsize) / 1_000_000_000
	return free, total
}

// queryMemoryUsage reports how much memory is in use and how much the machine
// has, in gibibytes.
func (s *Scanner) queryMemoryUsage() (usedGB *float64, totalGB float64) {
	totalBytes := physicalMemory()
	totalGB = float64(totalBytes) / 1_073_741_824
	if totalGB <= 0 {
		return nil, 0
	}
	pageSize, usedPages, ok := vmStatistics()
	if !ok {
		// Report nothing rather than a made-up 55%: a fabricated figure is
		// indistinguishable from a real one once it reaches the UI.
		return nil, totalGB
	}
	used := float64(usedPages) * float64(pageSize) / 1_073_741_824
	if used > totalGB {
		return nil, totalGB
	}
	return &used, totalGB
}

// physicalMemory is the machine's installed memory, in bytes.
func physicalMemory() int64 {
	value, err := syscall.Sysctl("hw.memsize")
	if err != nil {
		return 0
	}
	// Sysctl returns the raw bytes of a C int rather than its decimal form,
	// and it drops the trailing NUL bytes, so read whatever came back and
	// treat the rest as the zeros they were: 32 GB arrives as five bytes.
	var total int64
	for i := 0; i < len(value) && i < 8; i++ {
		total |= int64(value[i]) << (8 * i)
	}
	return total
}

// vmStatistics reports the page size and application + wired + compressed
// pages, excluding reusable file-backed and purgeable pages.
func vmStatistics() (pageSize, usedPages int, ok bool) {
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0, 0, false
	}
	return parseVMStatistics(string(out))
}

func parseVMStatistics(output string) (pageSize, usedPages int, ok bool) {
	pageSize = 16384
	active, inactive, speculative, wired, compressor, external, purgeable := 0, 0, 0, 0, 0, 0, 0
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSuffix(strings.TrimSpace(value), ".")
		if key == "Mach Virtual Memory Statistics" {
			// The header carries the page size in parentheses.
			if open := strings.Index(line, "page size of "); open >= 0 {
				rest := line[open+len("page size of "):]
				if end := strings.Index(rest, " bytes"); end > 0 {
					if size, err := strconv.Atoi(strings.TrimSpace(rest[:end])); err == nil && size > 0 {
						pageSize = size
					}
				}
			}
			continue
		}
		pages, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		switch key {
		case "Pages active":
			active = pages
		case "Pages inactive":
			inactive = pages
		case "Pages speculative":
			speculative = pages
		case "Pages wired down":
			wired = pages
		case "Pages occupied by compressor":
			compressor = pages
		case "File-backed pages":
			external = pages
		case "Pages purgeable":
			purgeable = pages
		}
	}
	if active == 0 && wired == 0 && compressor == 0 {
		return 0, 0, false
	}
	usedPages = active + inactive + speculative + wired + compressor - external - purgeable
	return pageSize, usedPages, usedPages >= 0
}

// queryCPUModel names the processor, as the status page shows it.
func (s *Scanner) queryCPUModel() string {
	cores := runtime.NumCPU()
	if brand, err := syscall.Sysctl("machdep.cpu.brand_string"); err == nil {
		if brand = strings.TrimSpace(brand); brand != "" {
			return brand + " (" + itoa64(int64(cores)) + "核)"
		}
	}
	return "Apple Silicon (" + itoa64(int64(cores)) + "核)"
}

// sampleRealProcesses reports the busiest processes and how much of the CPU
// they use between them.
func (s *Scanner) sampleRealProcesses() (cpuTotal float64, processes []map[string]any) {
	// -c strips executable paths, breaking Finder and native identity checks.
	out, err := exec.Command("/bin/ps", "-Aeo", "pid,ppid,%cpu,rss,comm", "-r").Output()
	if err != nil {
		return 0, nil
	}
	cores := runtime.NumCPU()
	if cores <= 0 {
		cores = 1
	}
	return parseProcesses(string(out), cores)
}

// parseProcesses reads what `ps -Aeo pid,ppid,%cpu,rss,comm -r` printed.
func parseProcesses(out string, cores int) (cpuTotal float64, processes []map[string]any) {
	var cpuSum float64
	parents := map[int]int{}
	appPaths := map[int]string{}
	processes = []map[string]any{}
	for i, line := range strings.Split(out, "\n") {
		if i == 0 {
			// The header line.
			continue
		}
		parts := splitFields(line, 5)
		if len(parts) < 5 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil || pid <= 1 {
			continue
		}
		// The Swift app read a number it could not parse as zero rather
		// than dropping the row.
		parentPID, _ := strconv.Atoi(parts[1])
		cpu, _ := strconv.ParseFloat(parts[2], 64)
		rssKB, _ := strconv.ParseFloat(parts[3], 64)
		comm := parts[4]
		name := filepath.Base(comm)
		if name == "ps" || name == "kernel_task" {
			continue
		}
		parents[pid] = parentPID
		appPaths[pid] = processAppPath(comm)
		cpuSum += cpu
		memoryMB := int(rssKB / 1024)
		if memoryMB < 1 {
			memoryMB = 1
		}
		processes = append(processes, map[string]any{
			"pid":       pid,
			"name":      name,
			"cpu":       cpu,
			"memory_mb": memoryMB,
			"power":     0,
			"pinned":    false,
			"path":      comm,
		})
	}
	for _, process := range processes {
		seen := map[int]bool{}
		for ancestorPID := process["pid"].(int); ancestorPID > 1 && !seen[ancestorPID]; ancestorPID = parents[ancestorPID] {
			seen[ancestorPID] = true
			if appPath := appPaths[ancestorPID]; appPath != "" {
				process["app_path"] = appPath
				process["app_name"] = strings.TrimSuffix(filepath.Base(appPath), filepath.Ext(appPath))
				break
			}
		}
	}
	cpuTotal = cpuSum / float64(cores)
	// No artificial floor: an idle machine really does sit near zero, and a
	// permanent "2.5%" made every idle reading look busy.
	if cpuTotal < 0 {
		cpuTotal = 0
	}
	if cpuTotal > 99.9 {
		cpuTotal = 99.9
	}
	return cpuTotal, processes
}

func processAppPath(path string) string {
	if !filepath.IsAbs(path) {
		return ""
	}
	var appPath string
	for path = filepath.Dir(path); path != "/" && path != "."; path = filepath.Dir(path) {
		if isAppBundle(path) {
			appPath = path
		}
	}
	return appPath
}

// splitFields breaks a line into at most n fields the way Swift's
// split(separator:maxSplits:omittingEmptySubsequences:) does: fields are
// separated by runs of spaces or tabs, and the last field keeps the rest of
// the line, spaces included.
func splitFields(line string, n int) []string {
	var fields []string
	i := 0
	skip := func() {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
	}
	for len(fields) < n-1 {
		skip()
		if i == len(line) {
			return fields
		}
		start := i
		for i < len(line) && line[i] != ' ' && line[i] != '\t' {
			i++
		}
		fields = append(fields, line[start:i])
	}
	skip()
	if i < len(line) {
		fields = append(fields, strings.TrimRight(line[i:], " \t\r"))
	}
	return fields
}

// ageText renders how long ago a file was modified.
func ageText(modified int64) string {
	if modified <= 0 {
		return "修改时间未知"
	}
	now := nowUnix()
	if modified > now {
		return "今天修改"
	}
	days := (now - modified) / 86400
	if days <= 0 {
		return "今天修改"
	}
	return itoa64(days) + " 天未修改"
}
