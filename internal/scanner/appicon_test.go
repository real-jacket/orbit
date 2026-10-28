package scanner

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testPNG builds a valid PNG of the given size so the ICNS fixtures hold real
// image data instead of magic bytes.
func testPNG(t *testing.T, size int) []byte {
	t.Helper()
	// 不能是纯色：纯色会被 pngLooksBlank 判成占位方块（这正是线上 Adobe 那个 bug）。
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: uint8(0xd9 - x%32), G: uint8(0x77 + y%32), B: uint8(0x57 + (x+y)%48), A: 0xff})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}
	return buffer.Bytes()
}

// testICNS assembles an ICNS container from type → payload pairs.
func testICNS(chunks ...[2]any) []byte {
	var body bytes.Buffer
	for _, chunk := range chunks {
		kind := chunk[0].(string)
		payload := chunk[1].([]byte)
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(payload)+8))
		body.WriteString(kind)
		body.Write(size[:])
		body.Write(payload)
	}
	total := uint32(body.Len() + 8)
	out := make([]byte, 8, int(total))
	copy(out, "icns")
	binary.BigEndian.PutUint32(out[4:], total)
	return append(out, body.Bytes()...)
}

// testFlatPNG builds a single-colour opaque square: the placeholder artwork some
// installers ship in one rep while the real icon lives in another.
func testFlatPNG(t *testing.T, size int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, c)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode flat png: %v", err)
	}
	return buffer.Bytes()
}

// testDetailedPNG builds artwork that is not a flat colour, so the blank check
// treats it as real.
func testDetailedPNG(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 13), B: uint8((x + y) * 3), A: 0xff})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode detailed png: %v", err)
	}
	return buffer.Bytes()
}

// TestICNSPNGPrefersTheLargerRealArtwork pins the current policy: the biggest
// non-blank rep wins, because a 38 px row at 2× still benefits from 256 px and
// some bundles only carry the real icon in the larger reps.
func TestICNSPNGPrefersTheLargerRealArtwork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AppIcon.icns")
	large := testDetailedPNG(t, 64)
	small := testDetailedPNG(t, 16)
	if err := os.WriteFile(path, testICNS([2]any{"ic09", large}, [2]any{"ic07", small}), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := icnsPNG(path); !bytes.Equal(got, large) {
		t.Fatalf("expected the larger ic09 payload, got %d bytes (small=%d large=%d)", len(got), len(small), len(large))
	}
}

// TestICNSPNGSkipsBlankingRepresentations covers the Adobe cleaner-tool case: the
// preferred 256 px rep is a flat white square, so the real artwork further down
// the list must win instead.
func TestICNSPNGSkipsBlankRepresentations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AppIcon.icns")
	blank := testFlatPNG(t, 32, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	real := testDetailedPNG(t, 16)
	// ic08 (rank 0, preferred) is blank; ic07 (rank 4) carries the artwork.
	if err := os.WriteFile(path, testICNS([2]any{"ic08", blank}, [2]any{"ic07", real}), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := icnsPNG(path); !bytes.Equal(got, real) {
		t.Fatalf("expected the non-blank ic07 payload, got %d bytes", len(got))
	}
}

// TestPNGLooksBlank pins the detector's two triggers: no opaque pixel at all, and
// a canvas that is one flat colour.
func TestPNGLooksBlank(t *testing.T) {
	if !pngLooksBlank(testFlatPNG(t, 16, color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff})) {
		t.Fatal("a flat square should read as blank")
	}
	if pngLooksBlank(testDetailedPNG(t, 24)) {
		t.Fatal("varying pixels should read as artwork")
	}
	clear := image.NewRGBA(image.Rect(0, 0, 16, 16))
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, clear); err != nil {
		t.Fatal(err)
	}
	if !pngLooksBlank(buffer.Bytes()) {
		t.Fatal("a fully transparent canvas should read as blank")
	}
}

// TestICNSPNGSkipsNonPNGRepresentations makes sure an old RLE icon does not get
// base64'd into the payload as if it were a PNG.
func TestICNSPNGSkipsNonPNGRepresentations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AppIcon.icns")
	if err := os.WriteFile(path, testICNS([2]any{"ic04", []byte{0x00, 0x01, 0x02, 0x03}}), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := icnsPNG(path); got != nil {
		t.Fatalf("expected no PNG, got %d bytes", len(got))
	}
}

// TestICNSPNGRejectsGarbage covers a truncated or foreign file.
func TestICNSPNGRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string][]byte{
		"empty.icns":     {},
		"short.icns":     []byte("icns"),
		"noticns.icns":   []byte("RIFF....WAVEfmt "),
		"truncated.icns": append([]byte("icns\x00\x00\x00\x40"), []byte("ic07\x00\x00\x00\x20")...),
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := icnsPNG(path); got != nil {
			t.Fatalf("%s: expected nil, got %d bytes", name, len(got))
		}
	}
}

// TestExtractAppIconReadsTheBundlesOwnArtwork is the end-to-end path: a bundle
// whose Info.plist names the icon file yields a data URL of the real PNG.
func TestExtractAppIconReadsTheBundlesOwnArtwork(t *testing.T) {
	app := t.TempDir()
	resources := filepath.Join(app, "Contents", "Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleIdentifier</key><string>com.example.orbit</string>
  <key>CFBundleIconFile</key><string>Orbit</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := testPNG(t, 16)
	if err := os.WriteFile(filepath.Join(resources, "Orbit.icns"), testICNS([2]any{"ic07", payload}), 0o644); err != nil {
		t.Fatal(err)
	}
	got := extractAppIcon(app)
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("expected a png data URL, got %q", got)
	}
	if len(got) <= len("data:image/png;base64,") {
		t.Fatalf("data URL carries no payload: %q", got)
	}
}

// TestExtractAppIconFallsBackToAnyICNS covers bundles whose plist names an icon
// that is not there, which happens with repackaged apps.
func TestExtractAppIconFallsBackToAnyICNS(t *testing.T) {
	app := t.TempDir()
	resources := filepath.Join(app, "Contents", "Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte("<plist><dict></dict></plist>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resources, "whatever.icns"), testICNS([2]any{"ic08", testPNG(t, 12)}), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := extractAppIcon(app); !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("expected the fallback icon, got %q", got)
	}
}

// TestAppIconDataURLIsEmptyForBareBundles keeps the page's fallback icon in play
// when there is no artwork at all.
func TestAppIconDataURLIsEmptyForBareBundles(t *testing.T) {
	if got := appIconDataURL(t.TempDir()); got != "" {
		t.Fatalf("expected no icon for an empty directory, got %q", got)
	}
}
