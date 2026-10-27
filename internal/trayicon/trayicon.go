// Package trayicon rasterizes the Mole Studio menu-bar mark into a macOS
// template PNG image.
//
// The mark is a port of the runtime AppKit drawing in
// Sources/MoleStudio/MoleStudioApp.swift, which used an 18x18 point image and a
// 64-unit design grid. In drawing order the mark is:
//
//  1. the orbit ring: an ellipse of radii 26 x 10.5 design units centred at
//     (32,32), rotated +26 degrees counter-clockwise, stroked with a line width
//     of 4.2 design units;
//  2. an ERASE stroke (compositingOperation = .clear) of a 4.2*1.15 wide band
//     around the circle of radius 13.5, cutting a seam where the ring would
//     otherwise cross the sphere;
//  3. the sphere: a filled disc of radius 13.5 at (32,32), which re-covers the
//     inner half of that seam so only the outer half reads as separation;
//  4. the companion: a filled disc of radius 4.4 at (50,21).
//
// The output is a template image: every pixel is pure black and only the alpha
// channel carries the shape, because macOS re-tints template images (black on a
// light menu bar, white on a dark one) and any colour left in the pixels would
// render as a solid blob.
package trayicon

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
)

// Design-grid constants, in design units (the 64-unit grid). These mirror the
// numeric literals in the Swift original one-for-one.
const (
	gridSize    = 64.0 // design grid extent
	centreX     = 32.0 // mark centre
	centreY     = 32.0 // mark centre
	sphereR     = 13.5 // sphere radius
	ringLine    = 4.2  // orbit ring stroke width
	ringA       = 26.0 // orbit ellipse semi-axis along x
	ringB       = 10.5 // orbit ellipse semi-axis along y
	ringDegrees = 26.0 // orbit rotation, counter-clockwise, in AppKit's +y-up space
	gapFactor   = 1.15 // seam stroke width = ringLine * gapFactor
	compR       = 4.4  // companion disc radius
	compX       = 50.0 // companion centre
	compY       = 21.0 // companion centre
)

// minPixels and maxPixels bound the accepted image side length.
const (
	minPixels = 1
	maxPixels = 1024
)

// Size returns the recommended PNG side length in pixels for the menu-bar mark:
// the Swift original drew an 18-point image, and menu bars are 2x on Retina, so
// 18 * 2 = 36. This is the side length PNG is normally called with.
func Size() int { return 36 }

// PNG returns the menu-bar mark as a template PNG image with side pixels on a
// side (square), black pixels where the mark is opaque.
//
// The image is a template image: R, G and B are zero everywhere and only alpha
// encodes the mark. Edges are anti-aliased by evaluating an analytic signed
// distance to each shape and converting that distance to fractional coverage
// over one output pixel, so the result is smooth rather than stair-stepped at
// any size.
//
// pixels must be in [1, 1024]; values outside that range return an error.
func PNG(pixels int) ([]byte, error) {
	if pixels < minPixels || pixels > maxPixels {
		return nil, fmt.Errorf("trayicon: pixels must be between %d and %d, got %d",
			minPixels, maxPixels, pixels)
	}

	img := image.NewRGBA(image.Rect(0, 0, pixels, pixels))

	// One output pixel spans this many design units; also the width of the
	// anti-aliasing ramp, which gives edges a one-pixel soft border.
	unitsPerPixel := gridSize / float64(pixels)

	for py := 0; py < pixels; py++ {
		for px := 0; px < pixels; px++ {
			// Sample the pixel centre, then convert to design space.
			x := (float64(px) + 0.5) * unitsPerPixel
			// AppKit's origin is bottom-left with +y up; image.Image is
			// top-left with +y down, so flip y to avoid a mirrored mark.
			y := gridSize - (float64(py)+0.5)*unitsPerPixel

			a := coverage(x, y, unitsPerPixel)

			i := py*img.Stride + px*4
			// Template image: black everywhere, alpha carries the shape.
			img.Pix[i+0] = 0
			img.Pix[i+1] = 0
			img.Pix[i+2] = 0
			img.Pix[i+3] = a
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("trayicon: encode PNG: %w", err)
	}
	return buf.Bytes(), nil
}

// coverage composites the four drawing steps at design-space point (x, y) and
// returns the resulting alpha in [0,255]. steps maps one output pixel to design
// units and therefore controls the width of the anti-aliasing ramp.
//
// The composition order is load-bearing and matches the Swift original: the ring
// is stroked, the seam is cleared, then the sphere and companion are filled over
// the result. Composing with source-over keeps anti-aliased edges correct where
// the steps overlap.
func coverage(x, y, steps float64) uint8 {
	dx := x - centreX
	dy := y - centreY

	// --- 1. Stroke the rotated orbit ellipse -------------------------------
	var a float64
	if d, ok := ringDistance(dx, dy); ok {
		a = coverBand(d, ringLine/2, steps)
	}

	// --- 2. ERASE the seam around the sphere (compositingOperation = .clear) --
	// Clearing reduces the accumulated alpha towards zero, so scale it by the
	// inverse of the seam's coverage.
	r := math.Hypot(dx, dy)
	seam := coverBand(r-sphereR, ringLine*gapFactor/2, steps)
	a *= 1 - seam

	// --- 3. Fill the sphere ------------------------------------------------
	a = sourceOver(a, coverInside(r-sphereR, steps))

	// --- 4. Fill the companion ---------------------------------------------
	cr := math.Hypot(x-compX, y-compY)
	a = sourceOver(a, coverInside(cr-compR, steps))

	if a <= 0 {
		return 0
	}
	if a >= 1 {
		return 255
	}
	return uint8(math.Round(a * 255))
}

// sourceOver composites an opaque source of coverage s over a backdrop of
// coverage a, returning the result in [0,1].
func sourceOver(a, s float64) float64 {
	return a + s*(1-a)
}

// coverInside returns the coverage of a filled shape from its signed distance
// field value d (negative inside, zero on the boundary), with a one-pixel-wide
// ramp centred on the boundary.
func coverInside(d, steps float64) float64 {
	return clamp01(0.5 - d/steps)
}

// coverBand returns the coverage of a stroke of half-width half centred on a
// boundary whose signed distance is d, with a one-pixel-wide ramp.
func coverBand(d, half, steps float64) float64 {
	return clamp01(0.5 - (math.Abs(d)-half)/steps)
}

func clamp01(v float64) float64 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 1
	}
	return v
}

// ringDistance returns the signed distance in design units from the point
// (dx, dy), expressed relative to the mark centre, to the boundary of the orbit
// ellipse, and reports whether the distance could be evaluated.
//
// The ellipse has semi-axes ringA and ringB and is rotated ringDegrees
// counter-clockwise in AppKit's +y-up space, so the point is un-rotated by the
// same angle before being measured in the ellipse's own frame.
func ringDistance(dx, dy float64) (float64, bool) {
	rad := ringDegrees * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)

	// Inverse rotation: bring the point into the ellipse's axis-aligned frame.
	ex := dx*cos + dy*sin
	ey := -dx*sin + dy*cos

	// Implicit ellipse distance: first-order approximation, which is exact on
	// the boundary (the gradient there is the true normal) and good enough
	// nearby for a one-pixel coverage ramp.
	k := math.Hypot(ex/ringA, ey/ringB)
	if k == 0 {
		return -math.Min(ringA, ringB), true
	}
	grad := math.Hypot(ex/(ringA*ringA), ey/(ringB*ringB)) / k
	if grad == 0 {
		return 0, false
	}
	return (k - 1) / grad, true
}
