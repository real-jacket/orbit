package scanner

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The uninstall list used to draw every app with a generated squircle and a
// letter, which made a list of real, installed apps look like sample data. The
// bundle already ships the artwork, so extract it instead: pick the PNG entries
// out of the .icns container (modern icons are PNG chunks, no decoding needed)
// and hand the page a data: URL it can put straight into an <img>.
//
// Icons are cached per bundle path: they cannot change while the process runs,
// and decoding sixteen bundles on every 90-second rescan would be pure waste.

// icnsPNGRank orders the ICNS chunk types by how well they suit a 28–34 px row.
// ic07 (128×128) is the sweet spot: sharp on Retina and small enough that
// sixteen of them do not bloat the scan payload. Types not listed (RLE, JPEG
// 2000, masks) carry no PNG payload and are ignored.
var icnsPNGRank = map[string]int{
	"ic07": 0,  // 128×128
	"ic12": 1,  // 32×32@2x
	"ic11": 2,  // 16×16@2x
	"ic13": 3,  // 128×128@2x
	"ic08": 4,  // 256×256
	"ic14": 5,  // 256×256@2x
	"ic09": 6,  // 512×512
	"ic10": 7,  // 512×512@2x
	"icp6": 8,  // 64×64
	"icp5": 9,  // 32×32
	"icp4": 10, // 16×16
}

var (
	appIconMu    sync.Mutex
	appIconCache = map[string]string{}
)

// appIconDataURL returns a data: URL holding the app's real icon, or "" when the
// bundle ships nothing we can read.
func appIconDataURL(appPath string) string {
	appIconMu.Lock()
	cached, ok := appIconCache[appPath]
	appIconMu.Unlock()
	if ok {
		return cached
	}
	icon := extractAppIcon(appPath)
	appIconMu.Lock()
	appIconCache[appPath] = icon
	appIconMu.Unlock()
	return icon
}

// extractAppIcon looks for the icon Info.plist names, then for any .icns the
// bundle happens to ship.
func extractAppIcon(appPath string) string {
	resources := filepath.Join(appPath, "Contents", "Resources")
	var names []string
	if plist, err := parsePlistFile(filepath.Join(appPath, "Contents", "Info.plist")); err == nil {
		if file := plist["CFBundleIconFile"]; file != "" {
			names = append(names, file)
		}
		if file := plist["CFBundleIconName"]; file != "" {
			names = append(names, file+".icns")
		}
	}
	for _, name := range names {
		if !strings.HasSuffix(strings.ToLower(name), ".icns") {
			name += ".icns"
		}
		if png := icnsPNG(filepath.Join(resources, name)); png != nil {
			return pngDataURL(png)
		}
	}
	entries, err := os.ReadDir(resources)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".icns") {
			continue
		}
		if png := icnsPNG(filepath.Join(resources, entry.Name())); png != nil {
			return pngDataURL(png)
		}
	}
	return ""
}

// icnsPNG walks an ICNS container and returns the best PNG payload it holds.
func icnsPNG(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 16 || string(data[:4]) != "icns" {
		return nil
	}
	declared := int(binary.BigEndian.Uint32(data[4:8]))
	if declared > 0 && declared < len(data) {
		data = data[:declared]
	}
	var best []byte
	bestRank := 1 << 30
	for offset := 8; offset+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[offset+4:]))
		if size < 8 || offset+size > len(data) {
			break
		}
		kind := string(data[offset : offset+4])
		payload := data[offset+8 : offset+size]
		if rank, ok := icnsPNGRank[kind]; ok && rank < bestRank && isPNG(payload) {
			best, bestRank = payload, rank
		}
		offset += size
	}
	return best
}

// isPNG reports whether data starts with the PNG signature.
func isPNG(data []byte) bool {
	return len(data) > 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n"
}

// pngDataURL wraps PNG bytes in the form an <img src> accepts.
func pngDataURL(png []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}
