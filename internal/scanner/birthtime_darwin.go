package scanner

import (
	"os"
	"syscall"
)

// birthTime is the creation time in seconds, false when the system has none.
func birthTime(info os.FileInfo) (int64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int64(st.Birthtimespec.Sec), true
}
