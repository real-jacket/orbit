package scanner

import (
	"archive/zip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// installerLocation is a folder Mole's installer scan looks in, with the label
// the page shows for files found there. Labels match by path prefix in order.
type installerLocation struct{ path, label string }

// Ported from tw93/Mole V1.70.0 bin/installer.sh INSTALLER_SCAN_PATHS and get_source_display
var installerLocations = []installerLocation{
	{"~/Downloads", "Downloads"},
	{"~/Desktop", "Desktop"},
	{"~/Documents", "Documents"},
	{"~/Public", "Public"},
	{"~/Library/Downloads", "Library"},
	{"/Users/Shared", "Shared"},
	{"/Users/Shared/Downloads", "Shared"},
	{"~/Library/Caches/Homebrew", "Homebrew"},
	{"~/Library/Mobile Documents/com~apple~CloudDocs/Downloads", "iCloud"},
	{"~/Library/Containers/com.apple.mail/Data/Library/Mail Downloads", "Mail"},
	{"~/Library/Application Support/Telegram Desktop", "Telegram"},
	{"~/Downloads/Telegram Desktop", "Telegram"},
}

const (
	installerDepth        = 2 // files directly in a location or one folder below it
	installerZipEntries   = 50
	installerBudget       = 30 * time.Second
	maxInstallerItems     = 200
	installerZipReviewMsg = "压缩包里含有安装程序，但也可能是你想留的资料，请确认后再清理"
)

var (
	installerExts     = map[string]bool{".dmg": true, ".pkg": true, ".mpkg": true, ".iso": true, ".xip": true}
	installerZipEntry = regexp.MustCompile(`\.(app|pkg|dmg|xip)(/|$)`)
	homebrewHash      = regexp.MustCompile(`^[0-9a-f]{64}--`)
)

// Ported from tw93/Mole V1.70.0 bin/installer.sh is_installer_zip
// A zip is an installer when one of its first 50 entries is an app, pkg, dmg or
// xip. A corrupt or unreadable archive is an ordinary file, not an installer.
func isInstallerZip(path string) bool {
	r, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer r.Close()
	for i, f := range r.File {
		if i >= installerZipEntries {
			break
		}
		if installerZipEntry.MatchString(f.Name) {
			return true
		}
	}
	return false
}

// Ported from tw93/Mole V1.70.0 bin/installer.sh get_source_display
func (s *Scanner) installerSource(file string) string {
	dir := filepath.Dir(file)
	for _, l := range installerLocations {
		if strings.HasPrefix(dir, s.expand(l.path)) {
			return l.label
		}
	}
	if strings.Contains(dir, "Telegram Desktop") {
		return "Telegram"
	}
	return filepath.Base(dir)
}

// Ported from tw93/Mole V1.70.0 bin/installer.sh handle_candidate_file
// installerKind says whether a file is an installer and whether it only counts
// because of what is inside a zip.
func installerKind(path string, bytes int64) (installer, zipArchive bool) {
	ext := strings.ToLower(filepath.Ext(path))
	if installerExts[ext] {
		return true, false
	}
	// A zero allocated size means an iCloud placeholder: opening it would
	// download the whole file.
	if ext == ".zip" && bytes > 0 && isInstallerZip(path) {
		return true, true
	}
	return false, false
}

// scanInstallerPackages lists the installers found in Mole's installer
// locations, two levels deep, never following symlinks. Zips are listed for
// review and never pre-checked. The row's size and mtime are the identity the
// move to the Trash re-checks.
func (s *Scanner) scanInstallerPackages() []map[string]any {
	type candidate struct {
		path     string
		bytes    int64
		modified int64
		size     int64
		zip      bool
	}
	deadline := time.Now().Add(installerBudget)
	seen := map[string]bool{}
	var found []candidate
	visit := func(dir string, item dirItem) {
		file := filepath.Join(dir, item.name)
		if item.kind != itemFile || seen[file] {
			return
		}
		installer, zipArchive := installerKind(file, item.bytes)
		if !installer {
			return
		}
		seen[file] = true
		if _, protected := s.scanProtection(file); protected {
			return
		}
		info, err := os.Lstat(file)
		if err != nil || !info.Mode().IsRegular() {
			return
		}
		found = append(found, candidate{file, item.bytes, item.mtime, info.Size(), zipArchive})
	}
	for _, l := range installerLocations {
		root := s.expand(l.path)
		if time.Now().After(deadline) {
			warnPath := root
			if !strings.HasPrefix(root, s.home+"/") {
				warnPath = s.home
			}
			recordReadWarning(warnPath, "安装包查找达到时间上限；只统计已访问路径")
			break
		}
		items, err := listDirGuarded(root)
		if err != nil {
			continue
		}
		for _, item := range items {
			if item.kind == itemDir && !isBundleDir(item.name) {
				sub := filepath.Join(root, item.name)
				if subItems, err := listDirItems(sub); err == nil {
					for _, subItem := range subItems {
						visit(sub, subItem)
					}
				}
				continue
			}
			visit(root, item)
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].bytes != found[j].bytes {
			return found[i].bytes > found[j].bytes
		}
		return found[i].path < found[j].path
	})
	if len(found) > maxInstallerItems {
		found = found[:maxInstallerItems]
	}
	items := []map[string]any{}
	for i, c := range found {
		source := s.installerSource(c.path)
		name := filepath.Base(c.path)
		if source == "Homebrew" {
			name = homebrewHash.ReplaceAllString(name, "")
		}
		item := map[string]any{
			"id":       "installer_" + itoa64(int64(i)),
			"name":     name,
			"location": filepath.Dir(c.path),
			"path":     s.relativize(c.path),
			"source":   source,
			"sizeGb":   gb(c.bytes),
			"size":     c.size,
			"mtime":    c.modified,
			"age":      ageText(c.modified),
			"checked":  !c.zip,
			"tier":     "safe",
		}
		if c.zip {
			item["tier"], item["reason"] = "review", installerZipReviewMsg
		}
		items = append(items, item)
	}
	return items
}
