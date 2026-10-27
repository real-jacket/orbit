package trayicon

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

// decode rasterizes the mark and decodes it back, so each test exercises the
// full PNG round trip rather than poking at internal state.
func decode(t *testing.T, pixels int) image.Image {
	t.Helper()

	data, err := PNG(pixels)
	if err != nil {
		t.Fatalf("PNG(%d) returned error: %v", pixels, err)
	}
	if len(data) == 0 {
		t.Fatalf("PNG(%d) returned empty data", pixels)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode(PNG(%d)) failed: %v", pixels, err)
	}
	return img
}

// alphaAt returns the alpha of the pixel at (x, y). It goes through
// color.NRGBAModel because the stdlib PNG decoder returns *image.NRGBA for
// non-opaque images, not *image.RGBA.
func alphaAt(img image.Image, x, y int) uint8 {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A
}

// rgbAt returns the unpremultiplied colour channels of the pixel at (x, y).
func rgbAt(img image.Image, x, y int) (r, g, b uint8) {
	c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	return c.R, c.G, c.B
}

// alphaStats counts fully clear, fully opaque and anti-aliased (intermediate)
// pixels.
func alphaStats(img image.Image) (clear, opaque, partial int) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			switch a := alphaAt(img, x, y); {
			case a == 0:
				clear++
			case a == 255:
				opaque++
			default:
				partial++
			}
		}
	}
	return clear, opaque, partial
}

// TestPNGDecodesAtRequestedSize checks the default 36px mark is a decodable PNG
// of exactly 36x36.
func TestPNGDecodesAtRequestedSize(t *testing.T) {
	img := decode(t, 36)

	b := img.Bounds()
	if b.Dx() != 36 || b.Dy() != 36 {
		t.Fatalf("image is %dx%d, want 36x36", b.Dx(), b.Dy())
	}
}

// TestEveryPixelIsPureBlack proves template-image correctness: macOS re-tints
// template images, so any non-zero colour channel would tint wrong and read as a
// solid blob instead of a crisp mark.
func TestEveryPixelIsPureBlack(t *testing.T) {
	img := decode(t, 36)
	b := img.Bounds()

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl := rgbAt(img, x, y)
			if r != 0 || g != 0 || bl != 0 {
				t.Fatalf("pixel (%d,%d) is rgb(%d,%d,%d), want pure black (0,0,0)", x, y, r, g, bl)
			}
		}
	}
}

// TestCentreIsOpaque checks the middle of the sphere is solid. Design (32,32)
// with +y up maps to image (18,18) at 36px, well inside the filled sphere, so it
// must be fully opaque rather than merely covered.
func TestCentreIsOpaque(t *testing.T) {
	img := decode(t, 36)

	if got := alphaAt(img, 18, 18); got != 255 {
		t.Errorf("centre pixel (18,18) alpha = %d, want 255 (inside the sphere)", got)
	}
}

// TestOutsideCornerIsTransparent checks pixels well outside the mark are clear.
func TestOutsideCornerIsTransparent(t *testing.T) {
	img := decode(t, 36)

	for _, p := range [][2]int{{1, 1}, {0, 0}, {35, 0}, {0, 35}} {
		if got := alphaAt(img, p[0], p[1]); got != 0 {
			t.Errorf("pixel (%d,%d) alpha = %d, want 0 (outside the mark)", p[0], p[1], got)
		}
	}
}

// TestCompanionIsOpaque is the y-flip check. The companion disc sits at design
// (50,21) with radius 4.4; with +y up that is low-and-right, so at 36px:
//
//	x = 50 * 36/64 = 28.1 -> 28
//	y = 36 - 21 * 36/64 = 24.2 -> 24
//
// Without the flip the companion would land near row 12 instead of row 24, so
// this assertion (and its mirror twin) fails if the flip is dropped or doubled.
func TestCompanionIsOpaque(t *testing.T) {
	img := decode(t, 36)

	if got := alphaAt(img, 28, 24); got <= 200 {
		t.Errorf("companion pixel (28,24) alpha = %d, want > 200 (missing y flip would put it near row 12)", got)
	}
	if got := alphaAt(img, 28, 12); got > 100 {
		t.Errorf("pixel (28,12) alpha = %d, want <= 100 (mark must not be vertically mirrored)", got)
	}
}

// TestRingIsOpaque checks the orbit ring is inked at its true extreme extent.
//
// The ring's x extent is NOT 32+26 design units. Rotating an ellipse with
// semi-axes 26 x 10.5 by 26 degrees gives an x extent of
// hypot(26*cos26, 10.5*sin26) = 23.82 units, so the curve's rightmost point is
// design (55.82, 41.36) -> image pixel (31,12); its leftmost counterpart is
// design (8.18, 22.64) -> image pixel (5,23). Throwing away one semi-axis and
// keeping the other overstates the extent by ~2.2 units, which at 36px is more
// than a whole pixel, so the naive pixel lands outside the stroke entirely.
func TestRingIsOpaque(t *testing.T) {
	img := decode(t, 36)

	if got := alphaAt(img, 31, 12); got <= 100 {
		t.Errorf("ring pixel (31,12) alpha = %d, want > 100 (rightmost extent of the rotated ring)", got)
	}
	if got := alphaAt(img, 5, 23); got <= 100 {
		t.Errorf("ring pixel (5,23) alpha = %d, want > 100 (leftmost extent of the rotated ring)", got)
	}
}

// TestNaiveRightEdgePixelIsOutsideRing pins down the geometry correction above:
// the axis-aligned guess x=(32+26)*36/64, y=18 is ~5.6 design units away from the
// stroked band, whose half-width is only 2.1 units, so that pixel is empty. This
// test exists so the claim in TestRingIsOpaque is enforced rather than asserted
// in a comment.
func TestNaiveRightEdgePixelIsOutsideRing(t *testing.T) {
	img := decode(t, 36)

	if got := alphaAt(img, 32, 18); got > 100 {
		t.Errorf("pixel (32,18) alpha = %d; the rotated ellipse's x extent is 23.82 units, so this pixel is outside the ring band (want <= 100)", got)
	}
}

// TestRingIsCutAtTheSphere checks the ERASE step left a visible seam: scanning
// right along the row through the mark centre, alpha must drop to nearly zero
// between the sphere and the right-hand arc of the ring, then come back up.
func TestRingIsCutAtTheSphere(t *testing.T) {
	img := decode(t, 36)

	// Row 18 passes through the sphere centre. The sphere edge is at design
	// x = 32+13.5 = 45.5 -> image x = 25.6, and the seam extends ~2.4 units
	// further, so a near-zero pixel must exist in x = 25..29.
	foundGap := false
	for x := 25; x <= 29; x++ {
		if a := alphaAt(img, x, 18); a < 40 {
			foundGap = true
			break
		}
	}
	if !foundGap {
		t.Error("no seam found between the sphere and the ring on row 18; the ERASE step is missing")
	}

	// The ring must still be inked further right on that row.
	foundRing := false
	for x := 29; x <= 34; x++ {
		if a := alphaAt(img, x, 18); a > 100 {
			foundRing = true
			break
		}
	}
	if !foundRing {
		t.Error("no ring ink to the right of the seam on row 18")
	}
}

// TestAntiAliasing checks the output is genuinely anti-aliased and not a
// hard-edged, all-0-or-255 bitmap, and that the mark neither vanishes nor fills
// the canvas.
func TestAntiAliasing(t *testing.T) {
	img := decode(t, 36)
	clear, opaque, partial := alphaStats(img)

	t.Logf("alpha histogram at 36px: clear=%d opaque=%d antialiased=%d (total=%d)",
		clear, opaque, partial, 36*36)

	if partial == 0 {
		t.Fatal("no anti-aliased pixels: every alpha is 0 or 255 (degenerate, stair-stepped output)")
	}
	if partial < 20 {
		t.Errorf("only %d anti-aliased pixels, want >= 20 for smooth edges", partial)
	}
	if opaque < 100 {
		t.Errorf("only %d fully opaque pixels, want >= 100 for a solid mark", opaque)
	}
	if clear < 100 {
		t.Errorf("only %d fully clear pixels, want >= 100 (mark must not fill the canvas)", clear)
	}
}

// TestInvalidSizes checks the documented bounds are enforced.
func TestInvalidSizes(t *testing.T) {
	for _, pixels := range []int{0, -1, -36, -1024, 1025, 2048, math.MaxInt32} {
		t.Run(fmt.Sprintf("%d", pixels), func(t *testing.T) {
			data, err := PNG(pixels)
			if err == nil {
				t.Fatalf("PNG(%d) returned no error, want error", pixels)
			}
			if data != nil {
				t.Errorf("PNG(%d) returned %d bytes alongside the error, want nil", pixels, len(data))
			}
		})
	}
}

// TestValidSizes checks the accepted range, including both bounds, produces a
// square template PNG of the requested size.
func TestValidSizes(t *testing.T) {
	for _, pixels := range []int{1, 2, 17, 36, 64, 512, 1024} {
		t.Run(fmt.Sprintf("%d", pixels), func(t *testing.T) {
			img := decode(t, pixels)

			b := img.Bounds()
			if b.Dx() != pixels || b.Dy() != pixels {
				t.Fatalf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), pixels, pixels)
			}

			// A template image at any size must stay pure black.
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					if r, g, bl := rgbAt(img, x, y); r != 0 || g != 0 || bl != 0 {
						t.Fatalf("pixel (%d,%d) is not pure black at size %d", x, y, pixels)
					}
				}
			}
		})
	}
}

// TestSizeReturnsRecommended checks the 18pt-at-2x recommendation.
func TestSizeReturnsRecommended(t *testing.T) {
	if got := Size(); got != 36 {
		t.Errorf("Size() = %d, want 36 (18 points at 2x)", got)
	}
}

// BenchmarkPNG tracks the cost of the default rendering, expected to sit far
// below the ~50ms budget.
func BenchmarkPNG(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := PNG(Size()); err != nil {
			b.Fatal(err)
		}
	}
}
