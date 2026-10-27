//go:build darwin

package scanner

import (
	"os"
	"path/filepath"
)

// moveToTrash moves path into ~/.Trash and reports where it landed. It is the
// same place Finder deletes to, so the user can put the item back by hand as
// well as through the app's own restore.
func moveToTrash(path string) (string, error) {
	trash, err := trashDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(trash, filepath.Base(path))
	if exists(target) {
		// Never overwrite what is already in the Trash: give the item a
		// unique name instead, the way Finder does.
		target = uniqueSibling(target)
	}
	if err := os.Rename(path, target); err != nil {
		return "", err
	}
	return target, nil
}
