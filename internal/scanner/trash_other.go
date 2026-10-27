//go:build !darwin

package scanner

import "errors"

// moveToTrash is only meaningful on macOS; elsewhere the app reports the
// failure instead of pretending the file moved.
func moveToTrash(string) (string, error) {
	return "", errors.New("仅支持 macOS")
}
