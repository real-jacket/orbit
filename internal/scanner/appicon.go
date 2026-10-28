package scanner

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image/png"
	"os"
	"os/exec"
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

// icnsPNGRank orders the ICNS chunk types by size, largest first. A 38 px row
// only needs 76 physical pixels, but bundles are inconsistent: some ship a blank
// or placeholder 128 px rep while the real artwork lives in the 256/512 ones
// (Adobe's cleaner tool did exactly that and rendered as an empty white square).
// Prefer 256 first — sharp at 2×, still small enough that sixteen icons do not
// bloat the scan payload — then 512, then 128, and only then the tiny reps.
// Types not listed (RLE, JPEG 2000, masks) carry no PNG payload and are ignored.
var icnsPNGRank = map[string]int{
	"ic08": 0,  // 256×256
	"ic13": 1,  // 128×128@2x
	"ic09": 2,  // 512×512
	"ic14": 3,  // 256×256@2x
	"ic07": 4,  // 128×128
	"ic10": 5,  // 512×512@2x
	"ic12": 6,  // 32×32@2x
	"ic11": 7,  // 16×16@2x
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
		path := filepath.Join(resources, name)
		if png := icnsPNG(path); png != nil && !pngLooksBlank(png) {
			return pngDataURL(png)
		}
		if png := icnsPNGViaSips(path); png != nil && !pngLooksBlank(png) {
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
		path := filepath.Join(resources, entry.Name())
		if png := icnsPNG(path); png != nil && !pngLooksBlank(png) {
			return pngDataURL(png)
		}
		if png := icnsPNGViaSips(path); png != nil && !pngLooksBlank(png) {
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
	// Walk the container once, then pick by rank: the highest-ranked payload that
	// is not blank, and the highest-ranked one at all as a last resort.
	type candidate struct {
		rank    int
		payload []byte
	}
	var all []candidate
	for offset := 8; offset+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[offset+4:]))
		if size < 8 || offset+size > len(data) {
			break
		}
		kind := string(data[offset : offset+4])
		payload := data[offset+8 : offset+size]
		if rank, ok := icnsPNGRank[kind]; ok && isPNG(payload) {
			all = append(all, candidate{rank: rank, payload: payload})
		}
		offset += size
	}
	// Best-ranked non-blank rep wins; the best-ranked rep at all is the fallback so
	// a bundle whose every rep is a flat placeholder still gets its square shown.
	bestNonBlankRank := 1 << 30
	var bestNonBlank []byte
	bestRank := 1 << 30
	var best []byte
	for _, c := range all {
		if c.rank < bestRank {
			best, bestRank = c.payload, c.rank
		}
		if c.rank < bestNonBlankRank && !pngLooksBlank(c.payload) {
			bestNonBlank, bestNonBlankRank = c.payload, c.rank
		}
	}
	if bestNonBlank != nil {
		return bestNonBlank
	}
	return best
}

// pngLooksBlank reports whether a PNG rep carries no artwork worth showing: a
// fully transparent canvas, or a single flat colour (the placeholder squares some
// installers ship). Only used to choose between representations, never to drop
// the icon entirely — a blank best candidate still beats no icon.
func pngLooksBlank(data []byte) bool {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}
	bounds := img.Bounds()
	if bounds.Empty() {
		return true
	}
	stepX := bounds.Dx() / 16
	if stepX < 1 {
		stepX = 1
	}
	stepY := bounds.Dy() / 16
	if stepY < 1 {
		stepY = 1
	}
	total, opaque, first := 0, 0, uint32(0)
	same := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r, g, b, a := img.At(x, y).RGBA()
			total++
			if a < 8<<8 {
				continue
			}
			opaque++
			pack := (r>>8)<<16 | (g>>8)<<8 | (b >> 8)
			if first == 0 {
				first = pack | 1<<24 // sentinel so a pure black pixel is not "unset"
			} else if pack|1<<24 == first {
				same++
			}
		}
	}
	if total == 0 || opaque == 0 {
		return true
	}
	// Flat placeholders: almost every sampled pixel is the same colour.
	return float64(same)/float64(opaque) > 0.92
}

// icnsPNGViaSips converts an ICNS container that carries no PNG payload. Some
// bundles store their only large reps as JPEG 2000 (Adobe's cleaner tool does:
// ic08/ic09 start with the jP magic), which the standard library cannot decode;
// /usr/bin/sips can, and the result is cached like any other icon.
func icnsPNGViaSips(icnsPath string) []byte {
	if _, err := os.Stat("/usr/bin/sips"); err != nil {
		return nil
	}
	out, err := os.CreateTemp("", "orbit-icon-*.png")
	if err != nil {
		return nil
	}
	name := out.Name()
	_ = out.Close()
	defer os.Remove(name)
	cmd := exec.Command("/usr/bin/sips", "-s", "format", "png", "-Z", "256", icnsPath, "--out", name)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Run(); err != nil {
		return nil
	}
	data, err := os.ReadFile(name)
	if err != nil || !isPNG(data) {
		return nil
	}
	return data
}

// isPNG reports whether data starts with the PNG signature.
func isPNG(data []byte) bool {
	return len(data) > 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n"
}

// pngDataURL wraps PNG bytes in the form an <img src> accepts.
func pngDataURL(png []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}
