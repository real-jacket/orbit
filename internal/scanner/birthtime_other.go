//go:build !darwin

package scanner

import "os"

// birthTime has no source off macOS, so the Mail guard that needs it keeps everything.
func birthTime(os.FileInfo) (int64, bool) { return 0, false }
