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
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: 0xd9, G: 0x77, B: 0x57, A: 0xff})
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

// TestICNSPNGPrefersAReadableSize proves the extractor picks the 128 px rep over
// the 512 px one, so sixteen icons stay small in the scan payload.
func TestICNSPNGPrefersAReadableSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AppIcon.icns")
	large := testPNG(t, 64) // stand-in for a big rep: content size is what matters here
	small := testPNG(t, 8)
	if err := os.WriteFile(path, testICNS([2]any{"ic09", large}, [2]any{"ic07", small}), 0o644); err != nil {
		t.Fatal(err)
	}
	got := icnsPNG(path)
	if !bytes.Equal(got, small) {
		t.Fatalf("expected the ic07 payload, got %d bytes (small=%d large=%d)", len(got), len(small), len(large))
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
