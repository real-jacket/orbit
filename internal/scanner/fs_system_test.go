package scanner

import "testing"

func TestIsSystemProtected(t *testing.T) {
	for path, want := range map[string]bool{
		"/Users/a/Library/Caches/com.apple.homed":                 true,
		"/Users/a/Library/Caches/familycircled/x":                 true,
		"/Users/a/Library/Caches/com.apple.Safari":                false,
		"/Users/a/Library/Caches/com.apple.Safari.SafeBrowsing/x": false,
		"/Users/a/Library/Caches/pnpm":                            false,
	} {
		if got := isSystemProtected(path); got != want {
			t.Errorf("%s: got %v want %v", path, got, want)
		}
	}
}
