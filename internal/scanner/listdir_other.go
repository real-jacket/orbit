//go:build !darwin

package scanner

func listDirItems(dir string) ([]dirItem, error) { return listDirStd(dir) }
