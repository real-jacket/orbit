package scanner

import (
	"os"
	"runtime"
	"time"
)

// Build identity. "点了没反应" and "CPU 显示未知" were both reported against a
// binary built before the fix; without a visible build stamp there is no way to
// tell a bug from a stale copy of the app. These three values are filled from
// the executable itself at startup (see app.go), so the number the user reads in
// the 体检 panel is the number of the code that actually ran. BuildVersion is
// "dev" until app.go reads the bundle's Info.plist: a plain `go run` has no
// bundle to read, and a packaged build always replaces this.
var (
	BuildVersion = "dev"
	BuildCommit  = "unknown"
	BuildTime    = ""
)

// buildCheck reports which build is running. It is informational: a build stamp
// can never "fail", so the row carries a nil verdict.
func buildCheck() map[string]any {
	detail := "orbit " + BuildVersion + " · Go " + runtime.Version()
	if BuildCommit != "" && BuildCommit != "unknown" {
		detail += " · " + BuildCommit
	}
	if BuildTime != "" {
		detail += " · 构建于 " + BuildTime
	}
	return doctorRow("应用构建", nil, detail)
}

// DescribeExecutable reads the running binary's modification time, which is the
// only build timestamp available at runtime without linker flags.
func DescribeExecutable() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	info, err := os.Stat(executable)
	if err != nil {
		return ""
	}
	return info.ModTime().Format(time.DateTime)
}
