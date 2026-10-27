// Package scanner measures the disk: how large directories are, which apps
// are installed, which files are big, and what a clean, purge or uninstall
// run would touch. It is a port of the app's Swift RealMacSystemScanner, and
// its payloads keep the exact shape the frontend already reads.
package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Scanner measures the disk. Use Shared.
type Scanner struct {
	home string
}

// Shared is the app's scanner.
var Shared = &Scanner{home: homeDir()}

// homeDir returns the user's home directory.
func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}

// Home returns the home directory the scanner measures relative to.
func (s *Scanner) Home() string { return s.home }

// expand replaces a leading ~ with the home directory.
func (s *Scanner) expand(path string) string {
	if path == "~" {
		return s.home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(s.home, path[2:])
	}
	return path
}

// relativize replaces the home directory at the start of path with ~, which is
// what the frontend shows.
func (s *Scanner) relativize(path string) string {
	if path == s.home {
		return "~"
	}
	if strings.HasPrefix(path, s.home+"/") {
		return "~" + strings.TrimPrefix(path, s.home)
	}
	return path
}

// round2 rounds v to two decimal places.
func round2(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return float64(int64(v*100-0.5)) / 100
}

// round1 rounds v to one decimal place.
func round1(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*10+0.5)) / 10
	}
	return float64(int64(v*10-0.5)) / 10
}

// gb converts bytes to gigabytes, rounding to two decimals and never
// reporting less than 0.01 GB.
func gb(bytes int64) float64 {
	value := round2(float64(bytes) / 1_000_000_000)
	if value < 0.01 {
		return 0.01
	}
	return value
}

// plainGB converts bytes to gigabytes, rounding to two decimals without a
// floor. The analyze bridge reports sizes this way.
func plainGB(bytes int64) float64 {
	return round2(float64(bytes) / 1_000_000_000)
}

// formatBytes renders bytes the way the app's lists show them.
func formatBytes(bytes int64) string {
	value := float64(bytes) / 1_000_000_000
	if value >= 0.1 {
		return formatFloat(value, 2) + " GB"
	}
	mb := float64(bytes) / 1_000_000
	if mb < 1 {
		mb = 1
	}
	return formatFloat(mb, 0) + " MB"
}

// formatFloat formats v with the given number of decimals, as printf's %.*f
// does.
func formatFloat(v float64, decimals int) string {
	return trimFloat(v, decimals)
}

// trimFloat is formatFloat without a leading sign split.
func trimFloat(v float64, decimals int) string {
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	scaled := v * mult
	var rounded float64
	if scaled >= 0 {
		rounded = float64(int64(scaled+0.5)) / mult
	} else {
		rounded = float64(int64(scaled-0.5)) / mult
	}
	return fixedFloat(rounded, decimals)
}

// fixedFloat renders v with exactly decimals digits after the point.
func fixedFloat(v float64, decimals int) string {
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	scaled := v * mult
	var n int64
	if scaled >= 0 {
		n = int64(scaled + 0.5)
	} else {
		n = int64(scaled - 0.5)
	}
	if decimals == 0 {
		return itoa64(n)
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	digits := itoa64(n)
	for len(digits) <= decimals {
		digits = "0" + digits
	}
	whole := digits[:len(digits)-decimals]
	frac := digits[len(digits)-decimals:]
	return sign + whole + "." + frac
}

// itoa64 renders n in base 10.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// exists reports whether path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// nowUnix is the current time in seconds since the epoch.
func nowUnix() int64 {
	return time.Now().Unix()
}

// isDir reports whether path is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// dirSize is a directory's size in bytes and how many entries it has.
type dirSize struct {
	bytes int64
	count int
}

// fastDirectorySize measures how large a directory is, walking at most
// maxFiles entries. A path that is not a directory reports its own size and
// one entry, which is what the Swift app did.
func (s *Scanner) fastDirectorySize(path string, maxFiles int) dirSize {
	path = s.expand(path)
	info, err := os.Stat(path)
	if err != nil {
		return dirSize{}
	}
	if !info.IsDir() {
		return dirSize{bytes: info.Size(), count: 1}
	}
	var total dirSize
	walkDir(path, maxFiles, &total)
	return total
}

// walkDir accumulates the sizes of the regular files under dir.
func walkDir(dir string, maxFiles int, total *dirSize) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if maxFiles > 0 && total.count >= maxFiles {
			return
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total.count++
		if entry.IsDir() {
			walkDir(path, maxFiles, total)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		total.bytes += allocatedSize(info)
	}
}

// allocatedSize is a file's size on disk, blocks times block size, which is
// what FileManager's totalFileAllocatedSize reported. It falls back to the
// file's length for a file whose stat has no block count.
func allocatedSize(info os.FileInfo) int64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if allocated := int64(stat.Blocks) * 512; allocated > 0 {
			return allocated
		}
	}
	if size := info.Size(); size > 0 {
		return size
	}
	return 0
}
