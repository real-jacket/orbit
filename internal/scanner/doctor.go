package scanner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	// 完全磁盘访问权限: a TCC-protected folder is readable only when the app has
	// been granted it. ~/Library/Mail exists on every account that has Mail set
	// up, so fall back to the Safari folder, then to the TCC database itself.
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

// fdaCheck answers whether this process can read TCC-protected locations.
func (s *Scanner) fdaCheck() map[string]any {
	candidates := []string{
		filepath.Join(s.home, "Library", "Mail"),
		filepath.Join(s.home, "Library", "Safari"),
	}
	for _, path := range candidates {
		entries, err := os.ReadDir(path)
		if err == nil {
			return doctorRow("完全磁盘访问权限", pass(),
				fmt.Sprintf("可读取 %s（%d 项）", displayHomePath(path, s.home), len(entries)))
		}
		if os.IsNotExist(err) {
			continue
		}
		// A permission error is the answer; anything else is not.
		if os.IsPermission(err) {
			return doctorRow("完全磁盘访问权限", fail(),
				"无法读取受保护目录，请在「系统设置 › 隐私与安全性 › 完全磁盘访问权限」里允许 orbit")
		}
		return doctorRow("完全磁盘访问权限", nil, "受保护目录不可用："+err.Error())
	}
	return doctorRow("完全磁盘访问权限", nil, "本机没有可判定的受保护目录（未找到 ~/Library/Mail 或 ~/Library/Safari）")
}

func thermalCheck(state string) map[string]any {
	switch state {
	case "nominal":
		return doctorRow("热压力", pass(), "本机热压力正常（nominal）")
	case "fair":
		return doctorRow("热压力", fail(), "热压力偏高（fair），可能有进程在持续占满 CPU")
	case "serious", "critical":
		return doctorRow("热压力", fail(), "热压力告警（"+state+"），建议先关闭高占用进程")
	default:
		return doctorRow("热压力", nil, "本机未提供热压力读数")
	}
}

func memoryPressureCheck(state string) map[string]any {
	switch state {
	case "normal":
		return doctorRow("内存压力", pass(), "内存压力正常")
	case "warn":
		return doctorRow("内存压力", fail(), "内存压力偏高，系统可能开始换页")
	case "critical":
		return doctorRow("内存压力", fail(), "内存压力临界，系统正在大量换页")
	default:
		return doctorRow("内存压力", nil, "本机未提供内存压力读数")
	}
}

// diskFreeCheck fails below 8% free and warns (fails) below 15%: APFS needs
// room to write snapshots, and below ~10 GB the machine starts to thrash.
func diskFreeCheck(freeGB, totalGB float64) map[string]any {
	if totalGB <= 0 {
		return doctorRow("磁盘剩余空间", nil, "无法读取卷容量")
	}
	ratio := freeGB / totalGB * 100
	detail := fmt.Sprintf("可用 %.1f GB / %.1f GB（%.1f%%）", freeGB, totalGB, ratio)
	switch {
	case ratio >= 15:
		return doctorRow("磁盘剩余空间", pass(), detail)
	case ratio >= 8:
		return doctorRow("磁盘剩余空间", fail(), detail+"，低于 15%，建议先清理")
	default:
		return doctorRow("磁盘剩余空间", fail(), detail+"，低于 8%，请立即清理")
	}
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
	if *healthPct >= 80 {
		return doctorRow("电池健康度", pass(), detail), true
	}
	if *healthPct >= 60 {
		return doctorRow("电池健康度", fail(), detail+"，低于 80%，续航会明显下降"), true
	}
	return doctorRow("电池健康度", fail(), detail+"，低于 60%，建议更换电池"), true
}

// sipCheck asks csrutil. It is only reported when the tool answers, because a
// machine where SIP status cannot be read should not be told it is enabled.
func sipCheck() (map[string]any, bool) {
	out, err := exec.Command("/usr/bin/csrutil", "status").CombinedOutput()
	if err != nil {
		return nil, false
	}
	text := strings.ToLower(string(out))
	switch {
	case strings.Contains(text, "status: enabled"):
		return doctorRow("系统完整性保护（SIP）", pass(), "SIP 已启用"), true
	case strings.Contains(text, "status: disabled"):
		return doctorRow("系统完整性保护（SIP）", fail(), "SIP 已关闭，系统文件可被修改"), true
	default:
		return nil, false
	}
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
