package scanner

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mymoles/mole-studio/internal/fsread"
	"github.com/mymoles/mole-studio/internal/power"
)

// DoctorChecks reports the pre-flight checks the 体检 page shows. Every row is
// a fact this machine can answer: a check that cannot be decided is reported
// with passed=nil ("未知") instead of being counted as a pass, because a report
// that always says "全部通过" is worth nothing.
func (s *Scanner) DoctorChecks() []map[string]any {
	checks := []map[string]any{
		// 先说清楚「你在跑的是哪一版」：这一行让「是不是旧构建」不再靠猜。
		buildCheck(),
	}

	// Readability probes describe this process's current access, not the TCC
	// switch itself. Check both available locations so one cannot hide another.
	checks = append(checks, s.fdaCheck())

	powerState := power.Take()
	checks = append(checks, thermalCheck(powerState.Thermal))
	checks = append(checks, memoryPressureCheck(powerState.MemPressure))
	freeGB, totalGB := s.queryDiskSpace()
	if totalGB > 0 {
		checks = append(checks, diskFreeCheck(freeGB, totalGB))
	}
	if battery := powerState.Battery; battery != nil {
		if check, ok := batteryHealthCheck(battery.HealthPct, battery.Cycles, battery.Percent); ok {
			checks = append(checks, check)
		}
	}
	if check, ok := sipCheck(); ok {
		checks = append(checks, check)
	}
	return checks
}

// fdaCheck answers whether Full Disk Access is on. The decisive evidence is
// opening TCC.db read-only (what CleanMyMac, Raycast and friends do); the Mail
// and Safari folders are secondary evidence for when TCC.db cannot decide.
func (s *Scanner) fdaCheck() map[string]any {
	checks := []map[string]any{
		s.directoryReadCheck("Mail", "Library/Mail"),
		s.directoryReadCheck("Safari", "Library/Safari"),
	}
	result := doctorRow("完全磁盘访问权限", nil, "无法判断：没有找到可用于核对的受保护文件")
	result["id"] = "fda"
	result["status"] = "missing"
	switch s.probeFullDisk() {
	case "granted":
		result["passed"], result["status"], result["detail"] = true, "granted", "已授权"
	case "denied":
		result["passed"], result["status"], result["detail"] = false, "denied", "未授权：系统拒绝读取受保护数据"
	default:
		readable, denied, unknown := false, false, false
		for _, check := range checks {
			readable = readable || check["status"] == "readable"
			denied = denied || check["status"] == "denied"
			unknown = unknown || check["status"] == "unknown"
		}
		switch {
		case denied:
			result["passed"], result["status"], result["detail"] = false, "denied", "未授权：系统拒绝读取受保护目录"
		case unknown:
			result["status"], result["detail"] = "unknown", "无法判断：部分受保护目录没有完成读取"
		case readable:
			result["passed"], result["status"], result["detail"] = true, "readable", "已授权"
		}
	}
	result["checks"], result["checkedAt"] = checks, time.Now().UnixMilli()
	return result
}

func (s *Scanner) FullDiskReadCheck() map[string]any {
	return s.fdaCheck()
}

func (s *Scanner) directoryReadCheck(title, relativePath string) map[string]any {
	path := filepath.Join(s.home, relativePath)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	entries, err := fsread.ReadDir(ctx, path)
	result := doctorRow(title, nil, "")
	result["path"], result["checkedAt"] = displayHomePath(path, s.home), time.Now().UnixMilli()
	switch {
	case err == nil:
		result["passed"], result["status"] = true, "readable"
		result["detail"] = fmt.Sprintf("可读取目录（%d 项）；未打开文件内容", len(entries))
	case os.IsNotExist(err):
		result["status"], result["detail"] = "missing", "未找到该目录，不能据此判断是否授权"
	case os.IsPermission(err):
		result["passed"], result["status"] = false, "denied"
		result["detail"] = "目录读取被拒绝；请核对系统权限和目录访问控制"
	default:
		result["status"], result["detail"] = "unknown", "读取未完成（可能正在等待系统授权）："+err.Error()
	}
	return result
}

func (s *Scanner) FolderReadChecks() []map[string]any {
	checks := make([]map[string]any, 0, 3)
	for _, folder := range []struct{ id, title, path string }{
		{"desktop", "桌面", "Desktop"}, {"documents", "文稿", "Documents"}, {"downloads", "下载", "Downloads"},
	} {
		check := s.directoryReadCheck(folder.title, folder.path)
		check["id"] = folder.id
		checks = append(checks, check)
	}
	return checks
}

func thermalCheck(state string) map[string]any {
	var row map[string]any
	switch state {
	case "nominal":
		row = doctorRow("热压力", pass(), "本机热压力正常（nominal）")
	case "fair":
		row = doctorRow("热压力", fail(), "热压力偏高（fair），可能有进程在持续占满 CPU")
	case "serious", "critical":
		row = doctorRow("热压力", fail(), "热压力告警（"+state+"），建议先关闭高占用进程")
	default:
		row = doctorRow("热压力", nil, "本机未提供热压力读数")
	}
	row["id"] = "thermal"
	return row
}

func memoryPressureCheck(state string) map[string]any {
	var row map[string]any
	switch state {
	case "normal":
		row = doctorRow("内存压力", pass(), "内存压力正常")
	case "warn":
		row = doctorRow("内存压力", fail(), "内存压力偏高，系统可能开始换页")
	case "critical":
		row = doctorRow("内存压力", fail(), "内存压力临界，系统正在大量换页")
	default:
		row = doctorRow("内存压力", nil, "本机未提供内存压力读数")
	}
	row["id"] = "memory"
	return row
}

// diskFreeCheck fails below 8% free and warns (fails) below 15%: APFS needs
// room to write snapshots, and below ~10 GB the machine starts to thrash.
func diskFreeCheck(freeGB, totalGB float64) map[string]any {
	if totalGB <= 0 {
		row := doctorRow("磁盘剩余空间", nil, "无法读取卷容量")
		row["id"] = "disk"
		return row
	}
	ratio := freeGB / totalGB * 100
	shownRatio := math.Round(ratio*10) / 10
	// Never print "8.0%，低于 8%" or "15.0%，低于 15%" due to rounding up at the boundary.
	if ratio < 8 && shownRatio >= 8 {
		shownRatio = 7.9
	} else if ratio < 15 && shownRatio >= 15 {
		shownRatio = 14.9
	}
	detail := fmt.Sprintf("可用 %.1f GB / %.1f GB（%.1f%%）", freeGB, totalGB, shownRatio)
	var row map[string]any
	switch {
	case ratio >= 15:
		row = doctorRow("磁盘剩余空间", pass(), detail)
	case ratio >= 8:
		row = doctorRow("磁盘剩余空间", fail(), detail+"，低于 15%，建议先清理")
	default:
		row = doctorRow("磁盘剩余空间", fail(), detail+"，低于 8%，请立即清理")
	}
	row["id"] = "disk"
	return row
}

// batteryHealthCheck reports capacity loss, not charge level. A machine without
// a battery reports ok=false so the row is left out instead of showing "未知".
func batteryHealthCheck(healthPct, cycles, percent *int) (map[string]any, bool) {
	if healthPct == nil {
		return nil, false
	}
	detail := fmt.Sprintf("健康度 %d%%", *healthPct)
	if cycles != nil {
		detail += fmt.Sprintf("，循环 %d 次", *cycles)
	}
	if percent != nil {
		detail += fmt.Sprintf("，当前电量 %d%%", *percent)
	}
	var row map[string]any
	if *healthPct >= 80 {
		row = doctorRow("电池健康度", pass(), detail)
	} else if *healthPct >= 60 {
		row = doctorRow("电池健康度", fail(), detail+"，低于 80%，续航会明显下降")
	} else {
		row = doctorRow("电池健康度", fail(), detail+"，低于 60%，建议更换电池")
	}
	row["id"] = "battery"
	return row, true
}

// sipCheck asks csrutil. It is only reported when the tool answers, because a
// machine where SIP status cannot be read should not be told it is enabled.
func sipCheck() (map[string]any, bool) {
	out, err := exec.Command("/usr/bin/csrutil", "status").CombinedOutput()
	if err != nil {
		return nil, false
	}
	text := strings.ToLower(string(out))
	var row map[string]any
	switch {
	case strings.Contains(text, "status: enabled"):
		row = doctorRow("系统完整性保护（SIP）", pass(), "SIP 已启用")
	case strings.Contains(text, "status: disabled"):
		row = doctorRow("系统完整性保护（SIP）", fail(), "SIP 已关闭，系统文件可被修改")
	default:
		return nil, false
	}
	row["id"] = "sip"
	return row, true
}

// doctorRow is one line of the 体检 list. passed is a pointer so "unknown" is
// representable: the page prints ⚪ 未知 for it instead of a green tick.
func doctorRow(title string, passed *bool, detail string) map[string]any {
	row := map[string]any{
		"title":  title,
		"detail": detail,
		"passed": nil,
	}
	if passed != nil {
		row["passed"] = *passed
	}
	return row
}

func pass() *bool { yes := true; return &yes }

func fail() *bool { no := false; return &no }

// displayHomePath shortens a path under home to ~/… for display.
func displayHomePath(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
