package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	machineWide = false
	os.Exit(m.Run())
}

// installOwners puts every AI tool's app bundle and launcher into home, for tests
// that check data stays protected while its owner is installed.
func installOwners(t *testing.T, home string) {
	t.Helper()
	for _, o := range personalOwners {
		for _, app := range o.apps {
			if err := os.MkdirAll(filepath.Join(home, "Applications", app+".app"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		for _, cmd := range o.cmds {
			writeFile(t, filepath.Join(home, ".local/bin", cmd), "launcher")
		}
	}
}
