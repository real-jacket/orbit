package scanner

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeZipFile writes a real zip whose entries have the given names.
func writeZipFile(t *testing.T, path string, entries ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, name := range entries {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte("x"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestInstallerDiscovery(t *testing.T) {
	home := fixtureHome(t)
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	writeTestFile(t, filepath.Join(home, "Downloads/App.dmg"), 10)
	writeTestFile(t, filepath.Join(home, "Downloads/.Hidden.pkg"), 10)
	writeTestFile(t, filepath.Join(home, "Desktop/Disk.ISO"), 10)
	writeTestFile(t, filepath.Join(home, "Documents/deep/er/Too.dmg"), 10) // below depth 2
	writeTestFile(t, filepath.Join(home, "Library/Caches/Homebrew/"+hash+"--tool-1.0.pkg"), 10)
	writeTestFile(t, filepath.Join(home, "Downloads/notes.txt"), 10)
	writeZipFile(t, filepath.Join(home, "Downloads/Tool.zip"), "docs/readme.txt", "Tool.app/Contents/Info.plist")
	writeZipFile(t, filepath.Join(home, "Downloads/photos.zip"), "a.jpg", "b.jpg")
	writeTestFile(t, filepath.Join(home, "Downloads/broken.zip"), 100) // not a zip: skipped
	// Only entries past the first 50 name an app: Mole does not look that far.
	late := make([]string, 0, 51)
	for i := 0; i < 50; i++ {
		late = append(late, "f"+itoa64(int64(i)))
	}
	writeZipFile(t, filepath.Join(home, "Downloads/late.zip"), append(late, "Late.app/x")...)
	if err := os.Symlink(filepath.Join(home, "Downloads/App.dmg"), filepath.Join(home, "Desktop/Link.dmg")); err != nil {
		t.Fatal(err)
	}

	rows := map[string]map[string]any{}
	for _, row := range (&Scanner{home: home}).scanInstallerPackages() {
		rows[row["name"].(string)] = row
	}
	for _, want := range []string{"App.dmg", ".Hidden.pkg", "Disk.ISO", "tool-1.0.pkg", "Tool.zip"} {
		if rows[want] == nil {
			t.Errorf("%s should be listed: %v", want, rows)
		}
	}
	for _, not := range []string{"Too.dmg", "notes.txt", "photos.zip", "broken.zip", "late.zip", "Link.dmg"} {
		if rows[not] != nil {
			t.Errorf("%s must not be listed", not)
		}
	}
	if rows["tool-1.0.pkg"]["source"] != "Homebrew" || rows["Disk.ISO"]["source"] != "Desktop" || rows["App.dmg"]["source"] != "Downloads" {
		t.Errorf("wrong source labels: %v", rows)
	}
	if zipRow := rows["Tool.zip"]; zipRow["checked"] != false || zipRow["tier"] != "review" || zipRow["reason"] == "" {
		t.Errorf("an installer zip is a review row, never pre-checked: %v", zipRow)
	}
}

func TestTrashPathsCheckedRefusesChangedFile(t *testing.T) {
	home := fixtureHome(t)
	t.Setenv("HOME", home)
	file := filepath.Join(home, "Downloads/App.dmg")
	writeTestFile(t, file, 10)
	s := &Scanner{home: home}
	got := s.TrashPathsChecked([]string{file}, map[string]FileIdentity{file: {Size: 11, Mtime: 1}})
	if len(got.Skipped) != 1 || len(got.Moved) != 0 || !exists(file) {
		t.Fatalf("a file that no longer matches the scan must stay put: %+v", got)
	}
}
