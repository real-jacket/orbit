//go:build !darwin

package scanner

import "errors"

func renameWithoutOverwrite(string, string) error {
	return errors.New("原生废纸篓与不覆盖还原只支持 macOS")
}
