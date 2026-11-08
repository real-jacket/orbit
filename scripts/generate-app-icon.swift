#!/usr/bin/env swift
//
//  generate-app-icon.swift
//  orbit — app icon generator
//
//  Renders the Orbit Studio brand master mark (spec 四.4.1 row 0) as a macOS
//  squircle plate, from code, at every size `iconutil` needs.
//
//  The mark is deliberately built from four elements and nothing else:
//
//    1  obsidian squircle plate: one 145° ramp and one lit top edge
//    2  a terracotta sphere, lit from the upper left, with a soft terminator
//    3  a gold orbit rising to the right, drawn behind the sphere for its far
//       half and in front of it for its near half (三.3 真实空间遮挡关系). It
//       tapers continuously with depth: thinner and dimmer far away, thicker
//       and brighter near, so there is no visible step at the seams.
//    4  an ice-blue companion threaded on the far arc, from 32pt up. Its centre
//       is computed on the ring's own centreline (same ellipse, tilt and
//       stretch) and the band is painted over its middle, so the ball shows
//       above and below the band and the ring plainly runs through it. From
//       128pt a thin dark seam edges the band and the band throws a soft shadow
//       onto the ball; below that the bare band is the cue. Lit from the same
//       upper-left key as the sphere (a pale face, a deeper limb and, from
//       64pt, one tiny specular kept clear of the band)
//
//  Refinement pass: the ring carries a soft highlight along its near arc (a
//  gold is never one gold), and its shadow on the sphere is narrow and soft, so
//  the sphere below the ring stays warm instead of reading as a dark cap.
//
//  One subtle addition over the previous revision, justified by meaning: from
//  128pt up the near arc casts a soft shadow onto the sphere. It is the only
//  cue that says the ring is in front of the planet and not merely over it, and
//  it is dropped below 128pt where it would turn into a dirty smudge.
//
//  Everything an earlier revision carried — a second orbit, a dashed airlock
//  ring, corner gauge ticks, a warm bloom, a vignette, a specular hotspot, limb
//  darkening, bounce light, an inner hairline, a bottom shade, a dark outer rim
//  and a highlight tube along the ring — was removed. Each looked defensible on
//  its own; together they made the mark noisy and expensive-looking. The rule
//  that replaces them: add an element only if it changes what the mark *means*
//  at 32pt.
//
//  Geometry is expressed in the same 64×64 space as the brand SVG in
//  Resources/src-ui/index.html, then mapped into Apple's 824pt icon body.
//
//  Usage:
//    swift scripts/generate-app-icon.swift iconset <out.iconset>
//    swift scripts/generate-app-icon.swift icns    <out.icns>
//    swift scripts/generate-app-icon.swift sheet   <out.png>
//    swift scripts/generate-app-icon.swift single  <out.png> <size>
//    swift scripts/generate-app-icon.swift zoom    <out.png> <size> <factor>
//
import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

// MARK: - Palette

enum Brand {
    static func hex(_ value: UInt32, _ alpha: CGFloat = 1) -> CGColor {
        CGColor(
            red: CGFloat((value >> 16) & 0xFF) / 255,
            green: CGFloat((value >> 8) & 0xFF) / 255,
            blue: CGFloat(value & 0xFF) / 255,
            alpha: alpha
        )
    }

    // Plate: the spec's three-stop ramp (四.4.1 row 0), with the terracotta kept
    // to the last fifth so it reads as obsidian with a warm corner, not as brown.
    static let plateTop = hex(0x303036)
    static let plateMid = hex(0x0D0D10)
    static let plateEnd = hex(0x6A2A12)

    // Sphere: one terracotta family, three steps from the key light to the limb.
    static let sphereLight = hex(0xFFC58A)
    static let sphereWarm = hex(0xF97316)
    static let sphereDeep = hex(0xC2410C)
    static let sphereLimb = hex(0xB93A0B)

    // Orbit: one gold and its darker far-side twin, so the ring reads as a
    // single object crossing behind the sphere rather than two different bands.
    static let ringGold = hex(0xF7C24A)
    static let ringGoldDim = hex(0x9C6B12)

    // Companion: the brand's cool accent, with a slightly deeper rim.
    static let companion = hex(0x38BDF8)
    static let companionRim = hex(0x0284C7)
}

// MARK: - Geometry helpers

let canvas: CGFloat = 1024
/// Apple's macOS icon grid: the artwork body is 824x824 inside a 1024 canvas.
let bodyRadius: CGFloat = 412

/// Superellipse ("squircle") approximation of Apple's continuous-corner mask.
func squircle(center: CGPoint, radius: CGFloat, exponent: CGFloat = 5, samples: Int = 1440) -> CGPath {
    let path = CGMutablePath()
    for i in 0...samples {
        let t = CGFloat(i) / CGFloat(samples) * 2 * .pi
        let c = cos(t)
        let s = sin(t)
        let x = center.x + radius * (c < 0 ? -1 : 1) * pow(abs(c), 2 / exponent)
        let y = center.y + radius * (s < 0 ? -1 : 1) * pow(abs(s), 2 / exponent)
        if i == 0 {
            path.move(to: CGPoint(x: x, y: y))
        } else {
            path.addLine(to: CGPoint(x: x, y: y))
        }
    }
    path.closeSubpath()
    return path
}

/// Deterministic pseudo-random source so every regeneration is byte-identical.
struct LCG {
    var state: UInt64
    mutating func next() -> Double {
        state = state &* 6_364_136_223_846_793_005 &+ 1_442_695_040_888_963_407
        return Double((state >> 11) & 0xFFFF_FFFF) / Double(0xFFFF_FFFF)
    }
}

func gradient(_ colors: [CGColor], _ locations: [CGFloat]) -> CGGradient {
    CGGradient(colorsSpace: CGColorSpaceCreateDeviceRGB(),
               colors: colors as CFArray,
               locations: locations)!
}

// MARK: - Brand mark coordinate space

/// Maps the 64×64 brand grid into the 824pt icon body, with y flipped so the
/// numbers here read the same way as the SVG in the shipping UI.
enum OrbitMark {
    static let unit: CGFloat = 824 / 64

    /// Orbit: the ellipse from 三.3, shared with the sidebar brand SVG.
    static let orbitRx: CGFloat = 26.5
    static let orbitRy: CGFloat = 10.2
    /// Degrees, counter-clockwise on screen: the right end of the ring rises.
    static let orbitTilt: CGFloat = 25

    /// Parameter (radians) of the companion on the far arc, just short of the
    /// right-hand end of the major axis. Its centre is (52.65, 18.15), the same
    /// circle the sidebar SVG draws; it is always taken from orbitPoint, never
    /// placed by eye.
    static let companionParam: CGFloat = -22 * .pi / 180

    /// A point on the orbit ellipse, in 64-grid SVG coordinates (y down).
    /// `t` is the ellipse parameter; sin(t) > 0 is the near (lower) half.
    static func orbitPoint(_ t: CGFloat, stretch: CGFloat = 1, rx: CGFloat = orbitRx) -> CGPoint {
        let phi = orbitTilt * .pi / 180
        let lx = rx * cos(t)
        let ly = orbitRy * stretch * sin(t)
        return CGPoint(x: 32 + lx * cos(phi) + ly * sin(phi),
                       y: 32 - lx * sin(phi) + ly * cos(phi))
    }

    static func point(_ x: CGFloat, _ y: CGFloat) -> CGPoint {
        CGPoint(x: 100 + x * unit, y: 924 - y * unit)
    }

    static func length(_ svgUnits: CGFloat) -> CGFloat { svgUnits * unit }
}

// MARK: - Per-size optical profile

/// Three drawings of one mark. A 16pt icon is a silhouette test, a 512pt icon
/// is a geometry test, and 32–64pt sits between them; each tier only thickens
/// what that size cannot resolve.
struct IconProfile {
    let planetRadius: CGFloat
    /// Ring width on the near (front) side and on the far side, in 64-grid units.
    let ringWidth: CGFloat
    let backRingWidth: CGFloat
    let orbitStretch: CGFloat
    let showsShading: Bool
    /// The near arc's shadow on the sphere (128pt and up only).
    let castsShadow: Bool
    /// Radius of the ice-blue companion on the 64-grid. Zero means "do not draw
    /// it": at 16pt a companion is a stray dot beside the sphere, not a symbol.
    let companionRadius: CGFloat
    let plateTop: CGColor
    let plateMid: CGColor
    let plateEnd: CGColor

    /// 128pt and up: the mark at full detail.
    static let full = IconProfile(
        planetRadius: 14, ringWidth: 3.8, backRingWidth: 2.6, orbitStretch: 1.0,
        showsShading: true, castsShadow: true, companionRadius: 4.0,
        plateTop: Brand.plateTop, plateMid: Brand.plateMid, plateEnd: Brand.plateEnd
    )

    /// 32–64pt: the same drawing, a touch heavier so it survives downscaling.
    static let compact = IconProfile(
        planetRadius: 14.4, ringWidth: 4.4, backRingWidth: 3.4, orbitStretch: 1.06,
        showsShading: true, castsShadow: false, companionRadius: 4.6,
        plateTop: Brand.hex(0x36363C), plateMid: Brand.hex(0x0E0E11), plateEnd: Brand.hex(0x4E230F)
    )

    /// 16pt: a pictogram. One uniform ring, opened far enough to clear the
    /// sphere, because at this size a thin occluded band degenerates into mush.
    static let mini = IconProfile(
        planetRadius: 14, ringWidth: 4.6, backRingWidth: 4.6, orbitStretch: 1.46,
        showsShading: false, castsShadow: false, companionRadius: 0,
        plateTop: Brand.hex(0x3E3E45), plateMid: Brand.hex(0x17171A), plateEnd: Brand.hex(0x4A2110)
    )

    static func forSize(_ size: Int) -> IconProfile {
        switch size {
        case ...16: return .mini
        case ...64: return .compact
        default: return .full
        }
    }
}

// MARK: - Element 1: the plate

/// One gradient and one lit edge. The plate is a container, not a subject:
/// every extra treatment here competes with the mark it is holding.
func drawPlate(in ctx: CGContext, profile: IconProfile) {
    let centre = CGPoint(x: canvas / 2, y: canvas / 2)
    ctx.saveGState()
    ctx.addPath(squircle(center: centre, radius: bodyRadius))
    ctx.clip()

    // 145deg in CSS points down-right. In this y-up canvas that is (+sin, +cos).
    let angle = 145 * CGFloat.pi / 180
    let direction = CGPoint(x: sin(angle), y: cos(angle))
    let reach = bodyRadius * 1.15
    ctx.drawLinearGradient(
        gradient([profile.plateTop, profile.plateMid, Brand.hex(0x150E0B), profile.plateEnd], [0, 0.56, 0.87, 1]),
        start: CGPoint(x: centre.x - direction.x * reach, y: centre.y - direction.y * reach),
        end: CGPoint(x: centre.x + direction.x * reach, y: centre.y + direction.y * reach),
        options: [.drawsBeforeStartLocation, .drawsAfterEndLocation]
    )

    // A single lit edge along the top, so the squircle reads as a raised tile.
    let band = bodyRadius * 0.09
    ctx.drawLinearGradient(
        gradient([Brand.hex(0xFFFFFF, 0.28), Brand.hex(0xFFFFFF, 0)], [0, 1]),
        start: CGPoint(x: centre.x, y: centre.y + bodyRadius),
        end: CGPoint(x: centre.x, y: centre.y + bodyRadius - band),
        options: []
    )
    ctx.restoreGState()
}

// MARK: - Element 2: the sphere

func drawSphere(in ctx: CGContext, profile: IconProfile) {
    let radius = OrbitMark.length(profile.planetRadius)
    let centre = OrbitMark.point(32, 32)
    let extent = CGRect(x: centre.x - radius, y: centre.y - radius, width: radius * 2, height: radius * 2)
    // Key light at 34% / 28% of the bounding box, as in the sidebar SVG.
    let svg = profile.planetRadius
    let light = OrbitMark.point((32 - svg) + 0.34 * svg * 2, (32 - svg) + 0.28 * svg * 2)

    ctx.saveGState()
    ctx.addEllipse(in: extent)
    ctx.clip()
    if profile.showsShading {
        // Five eased stops: a broad soft key light, then a gentle terminator
        // that only deepens in the last fifth. startRadius 0 matters: a positive
        // radius leaves a transparent pinhole at the centre of the highlight.
        ctx.drawRadialGradient(
            gradient([Brand.sphereLight, Brand.hex(0xFC9A4E), Brand.sphereWarm, Brand.sphereDeep, Brand.sphereLimb],
                     [0, 0.24, 0.55, 0.93, 1]),
            startCenter: light, startRadius: 0,
            endCenter: light, endRadius: radius * 1.65,
            options: [.drawsAfterEndLocation]
        )
    } else {
        // 16pt: one flat terracotta disc reads better than a gradient that the
        // rasteriser turns into two muddy bands.
        ctx.setFillColor(Brand.sphereWarm)
        ctx.fill(extent)
    }
    ctx.restoreGState()
}

// MARK: - Element 3: the orbit

/// The orbit as depth-tapered segments, split at the major-axis ends.
///
/// sin(t) > 0 is the near half. Each short segment carries width and colour
/// from one smoothstep of depth, so far -> near is a continuous ramp: the seams
/// sit exactly on the ends of the major axis and nothing steps there. 三.3 asks
/// for the far half behind the sphere and the near half in front of it.
struct OrbitSegment { let path: CGPath; let width: CGFloat; let color: CGColor; let at: CGPoint }

func orbitSegments(profile: IconProfile, near: Bool, steps: Int = 360) -> [OrbitSegment] {
    // A gold ring is not one gold: the near arc brightens toward the key light
    // (upper left of its run) and settles back to the base tone at both seams.
    func mix(_ a: UInt32, _ b: UInt32, _ k: CGFloat, lift: CGFloat = 0) -> CGColor {
        func ch(_ v: UInt32, _ s: UInt32) -> CGFloat { CGFloat((v >> s) & 0xFF) }
        func c(_ s: UInt32) -> CGFloat {
            let base = ch(a, s) + (ch(b, s) - ch(a, s)) * k
            return (base + (ch(0xFFD770, s) - base) * lift) / 255        // toward a warm, saturated gold (never cream)
        }
        return CGColor(red: c(16), green: c(8), blue: c(0), alpha: 1)
    }
    var out: [OrbitSegment] = []
    let lo = near ? 0 : steps / 2, hi = near ? steps / 2 : steps
    for index in lo..<hi {
        let t0 = CGFloat(index) / CGFloat(steps) * 2 * .pi
        let t1 = CGFloat(index + 1) / CGFloat(steps) * 2 * .pi
        let depth = sin((t0 + t1) / 2)
        // Butt caps, so each piece overlaps its neighbour by a hair instead of
        // a round cap printing a disc where the colour ramp changes.
        let pad = 0.4 * (t1 - t0)                      // -1 far ... +1 near
        let x = max(0, min(1, (depth + 0.7) / 1.4))
        let k = x * x * (3 - 2 * x)
        let path = CGMutablePath()
        path.move(to: OrbitMark.point(OrbitMark.orbitPoint(t0 - pad, stretch: profile.orbitStretch).x,
                                      OrbitMark.orbitPoint(t0 - pad, stretch: profile.orbitStretch).y))
        path.addLine(to: OrbitMark.point(OrbitMark.orbitPoint(t1 + pad, stretch: profile.orbitStretch).x,
                                         OrbitMark.orbitPoint(t1 + pad, stretch: profile.orbitStretch).y))
        let mid = OrbitMark.orbitPoint((t0 + t1) / 2, stretch: profile.orbitStretch)
        out.append(OrbitSegment(
            path: path,
            width: OrbitMark.length(profile.backRingWidth + (profile.ringWidth - profile.backRingWidth) * k),
            color: near
                ? mix(0x9C6B12, 0xF7C24A, k, lift: 0.34 * sin(.pi * CGFloat(index - lo) / CGFloat(hi - lo)) * (1 - 0.35 * CGFloat(index - lo) / CGFloat(hi - lo)))
                : mix(0x9C6B12, 0xF7C24A, k),
            at: OrbitMark.point(mid.x, mid.y)))
    }
    return out
}

/// The 16pt pictogram: one closed ring, stroked flat, drawn clear of the sphere.
func orbitPictogram(stretch: CGFloat, steps: Int = 360) -> CGPath {
    let path = CGMutablePath()
    for index in 0...steps {
        let t = CGFloat(index) / CGFloat(steps) * 2 * .pi
        let p = OrbitMark.orbitPoint(t, stretch: stretch, rx: OrbitMark.orbitRx * 0.94)
        let point = OrbitMark.point(p.x, p.y)
        if index == 0 { path.move(to: point) } else { path.addLine(to: point) }
    }
    path.closeSubpath()
    return path
}

func stroke(_ ctx: CGContext, path: CGPath, width: CGFloat, color: CGColor, cap: CGLineCap = .round) {
    ctx.saveGState()
    ctx.setStrokeColor(color)
    ctx.setLineWidth(width)
    ctx.setLineCap(cap)
    ctx.addPath(path)
    ctx.strokePath()
    ctx.restoreGState()
}

func drawOrbit(in ctx: CGContext, profile: IconProfile) {
    if !profile.showsShading {
        drawSphere(in: ctx, profile: profile)
        stroke(ctx, path: orbitPictogram(stretch: profile.orbitStretch),
               width: OrbitMark.length(profile.ringWidth), color: Brand.ringGold)
        return
    }
    // Far half first, then the sphere, then the near half: the sphere paints
    // over the far arc, which is what produces the occlusion seam.
    // The far arc, minus the stretch that threads the companion: that stretch is
    // painted once, over the ball, so no clip edge prints a hairline on the band.
    let (back, through) = orbitSegments(profile: profile, near: false).reduce(into: ([OrbitSegment](), [OrbitSegment]())) { acc, segment in
        let c = companionFrame(profile: profile)
        let near = c.radius > 0 && hypot(segment.at.x - c.centre.x, segment.at.y - c.centre.y) < c.radius * 1.5
        if near { acc.1.append(segment) } else { acc.0.append(segment) }
    }
    for segment in back {
        stroke(ctx, path: segment.path, width: segment.width, color: segment.color, cap: .butt)
    }
    drawCompanion(in: ctx, profile: profile, ring: through)
    drawSphere(in: ctx, profile: profile)
    let front = orbitSegments(profile: profile, near: true)
    if profile.castsShadow { drawRingShadow(in: ctx, profile: profile, front: front) }
    for segment in front {
        stroke(ctx, path: segment.path, width: segment.width, color: segment.color, cap: .butt)
    }
}

/// The near arc's shadow on the sphere: the ring itself, shifted away from the
/// key light and blurred, clipped to the sphere so it never touches the plate.
func drawRingShadow(in ctx: CGContext, profile: IconProfile, front: [OrbitSegment]) {
    let radius = OrbitMark.length(profile.planetRadius)
    let centre = OrbitMark.point(32, 32)
    ctx.saveGState()
    ctx.addEllipse(in: CGRect(x: centre.x - radius, y: centre.y - radius, width: radius * 2, height: radius * 2))
    ctx.clip()
    ctx.setShadow(offset: CGSize(width: OrbitMark.length(0.25), height: -OrbitMark.length(0.6)),
                  blur: OrbitMark.length(2.2), color: Brand.hex(0x3A0F00, 0.3))
    for segment in front { stroke(ctx, path: segment.path, width: segment.width, color: segment.color, cap: .butt) }
    ctx.restoreGState()
}

// MARK: - Element 4: the ice-blue companion

/// A small lit ball: pale upper-left face, brand blue, deeper limb, a darker rim,
/// and one tiny specular from 64pt. It borrows the sphere's key light so the two
/// read as one scene; below 64pt it stays a flat disc.
/// The companion sits exactly on the ring's centreline: the same ellipse, the
/// same tilt and the same stretch the ring segments use.
func companionFrame(profile: IconProfile) -> (home: CGPoint, centre: CGPoint, radius: CGFloat) {
    let home = OrbitMark.orbitPoint(OrbitMark.companionParam, stretch: profile.orbitStretch)
    return (home, OrbitMark.point(home.x, home.y), OrbitMark.length(profile.companionRadius))
}

func drawCompanion(in ctx: CGContext, profile: IconProfile, ring: [OrbitSegment]) {
    guard profile.companionRadius > 0 else { return }
    let (home, centre, radius) = companionFrame(profile: profile)
    let extent = CGRect(x: centre.x - radius, y: centre.y - radius, width: radius * 2, height: radius * 2)

    ctx.saveGState()
    if profile.showsShading {
        // Same key light as the sphere: a pale upper-left, the brand blue, a deeper limb.
        let key = OrbitMark.point(home.x - profile.companionRadius * 0.32, home.y - profile.companionRadius * 0.34)
        ctx.addEllipse(in: extent)
        ctx.clip()
        ctx.drawRadialGradient(
            gradient([Brand.hex(0xA5E8FF), Brand.companion, Brand.hex(0x0EA5E9)], [0, 0.55, 1]),
            startCenter: key, startRadius: 0, endCenter: key, endRadius: radius * 1.5,
            options: [.drawsAfterEndLocation])
    } else {
        ctx.setFillColor(Brand.companion)
        ctx.fillEllipse(in: extent)
    }
    ctx.restoreGState()

    guard profile.showsShading else { return }
    ctx.saveGState()
    ctx.addEllipse(in: extent.insetBy(dx: radius * 0.09, dy: radius * 0.09))
    ctx.setStrokeColor(Brand.companionRim)
    ctx.setLineWidth(max(1, radius * 0.16))
    ctx.strokePath()
    ctx.restoreGState()

    // The tiny highlight: only where it is large enough to be seen (64pt up).
    if profile.companionRadius >= 3.6, radius >= 6 {
        let spec = OrbitMark.point(home.x - profile.companionRadius * 0.3, home.y - profile.companionRadius * 0.5)
        let sr = radius * 0.18
        ctx.setFillColor(Brand.hex(0xFFFFFF, 0.85))
        ctx.fillEllipse(in: CGRect(x: spec.x - sr, y: spec.y - sr, width: sr * 2, height: sr * 2))
    }

    // The ring runs THROUGH the ball: the band is drawn again over the ball's
    // middle, so the ball shows above and below it. From 128pt a thin dark seam
    // edges the band and the band throws a soft shadow onto the ball; below that
    // the bare band is the whole cue.
    if profile.castsShadow {
        ctx.saveGState()
        ctx.addEllipse(in: extent.insetBy(dx: radius * 0.04, dy: radius * 0.04))
        ctx.clip()
        for segment in ring {
            stroke(ctx, path: segment.path, width: segment.width + OrbitMark.length(0.9), color: Brand.hex(0x0B4A73), cap: .butt)
        }
        ctx.restoreGState()
    }
    if profile.castsShadow {
        ctx.saveGState()
        ctx.addEllipse(in: extent)
        ctx.clip()
        ctx.setShadow(offset: CGSize(width: OrbitMark.length(0.2), height: -OrbitMark.length(0.55)),
                      blur: OrbitMark.length(1.1), color: Brand.hex(0x03233A, 0.5))
        // One layer, so the band casts a single shadow instead of one per segment.
        ctx.beginTransparencyLayer(auxiliaryInfo: nil)
        for segment in ring { stroke(ctx, path: segment.path, width: segment.width, color: segment.color, cap: .butt) }
        ctx.endTransparencyLayer()
        ctx.restoreGState()
    }
    for segment in ring { stroke(ctx, path: segment.path, width: segment.width, color: segment.color, cap: .butt) }
}

// MARK: - Composition

func drawMark(in ctx: CGContext, size: Int) {
    let profile = IconProfile.forSize(size)
    drawPlate(in: ctx, profile: profile)

    let centre = CGPoint(x: canvas / 2, y: canvas / 2)
    ctx.saveGState()
    ctx.addPath(squircle(center: centre, radius: bodyRadius - 2))
    ctx.clip()
    drawOrbit(in: ctx, profile: profile)
    ctx.restoreGState()
}

func render(size: Int) -> CGImage? {
    let scale = CGFloat(size) / canvas
    guard let ctx = CGContext(
        data: nil, width: size, height: size,
        bitsPerComponent: 8, bytesPerRow: 0,
        space: CGColorSpaceCreateDeviceRGB(),
        bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
    ) else { return nil }

    ctx.interpolationQuality = .high
    ctx.setAllowsAntialiasing(true)
    ctx.scaleBy(x: scale, y: scale)

    drawMark(in: ctx, size: size)
    dither(ctx, size: size)
    return ctx.makeImage()
}

/// An 8-bit gradient across a large, dark surface bands visibly — the obsidian
/// plate shows concentric steps and blotches. A sub-percent noise layer breaks
/// the steps up; it is invisible at 100% but removes the banding entirely.
func dither(_ ctx: CGContext, size: Int) {
    guard size >= 64, let base = ctx.data else { return }
    let rowBytes = ctx.bytesPerRow
    let pixels = base.bindMemory(to: UInt8.self, capacity: rowBytes * size)
    var rng = LCG(state: 0x9E37_79B9_7F4A_7C15)

    for y in 0..<size {
        let row = y * rowBytes
        for x in 0..<size {
            let offset = row + x * 4
            // Skip fully transparent pixels so the squircle edge stays clean.
            guard pixels[offset + 3] > 8 else { continue }
            let noise = Int((rng.next() * 5).rounded()) - 2
            guard noise != 0 else { continue }
            for channel in 0..<3 {
                let value = Int(pixels[offset + channel]) + noise
                pixels[offset + channel] = UInt8(max(0, min(255, value)))
            }
        }
    }
}

@discardableResult
func writePNG(_ image: CGImage, to url: URL) -> Bool {
    guard let dest = CGImageDestinationCreateWithURL(
        url as CFURL, UTType.png.identifier as CFString, 1, nil
    ) else { return false }
    CGImageDestinationAddImage(dest, image, nil)
    return CGImageDestinationFinalize(dest)
}

// MARK: - Commands

/// The exact set `iconutil` expects inside an .iconset directory.
let iconsetEntries: [(name: String, pixels: Int)] = [
    ("icon_16x16.png", 16),
    ("icon_16x16@2x.png", 32),
    ("icon_32x32.png", 32),
    ("icon_32x32@2x.png", 64),
    ("icon_128x128.png", 128),
    ("icon_128x128@2x.png", 256),
    ("icon_256x256.png", 256),
    ("icon_256x256@2x.png", 512),
    ("icon_512x512.png", 512),
    ("icon_512x512@2x.png", 1024)
]

func fail(_ message: String) -> Never {
    FileHandle.standardError.write(Data(("error: " + message + "\n").utf8))
    exit(1)
}

let arguments = Array(CommandLine.arguments.dropFirst())
guard let command = arguments.first else {
    fail("usage: generate-app-icon.swift <iconset|icns|sheet|single|zoom> ...")
}

switch command {
case "iconset":
    guard arguments.count >= 2 else { fail("iconset requires an output directory") }
    let dir = URL(fileURLWithPath: arguments[1])
    try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    for entry in iconsetEntries {
        guard let image = render(size: entry.pixels) else {
            fail("failed to render \(entry.pixels)px")
        }
        let url = dir.appendingPathComponent(entry.name)
        guard writePNG(image, to: url) else { fail("failed to write \(url.path)") }
        print("  \(entry.name)  \(entry.pixels)x\(entry.pixels)")
    }
    print("wrote \(iconsetEntries.count) images to \(dir.path)")

case "icns":
    guard arguments.count >= 2 else { fail("icns requires an output path") }
    let out = URL(fileURLWithPath: arguments[1])
    let staging = URL(fileURLWithPath: NSTemporaryDirectory())
        .appendingPathComponent("AppIcon-\(UUID().uuidString).iconset")
    try? FileManager.default.createDirectory(at: staging, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: staging) }

    for entry in iconsetEntries {
        guard let image = render(size: entry.pixels) else {
            fail("failed to render \(entry.pixels)px")
        }
        guard writePNG(image, to: staging.appendingPathComponent(entry.name)) else {
            fail("failed to stage \(entry.name)")
        }
    }

    let proc = Process()
    proc.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
    proc.arguments = ["-c", "icns", staging.path, "-o", out.path]
    try? proc.run()
    proc.waitUntilExit()
    guard proc.terminationStatus == 0 else { fail("iconutil exited \(proc.terminationStatus)") }
    print("wrote \(out.path)")

case "single":
    guard arguments.count >= 3, let size = Int(arguments[2]) else {
        fail("single requires <out.png> <size>")
    }
    guard let image = render(size: size) else { fail("render failed") }
    guard writePNG(image, to: URL(fileURLWithPath: arguments[1])) else { fail("write failed") }
    print("wrote \(arguments[1]) (\(size)px)")

case "zoom":
    // 三.4 双尺度自适应检验: a true-pixel render blown up with nearest-neighbour,
    // so a 16pt or 32pt mark can be inspected without judging an upscaler.
    guard arguments.count >= 4, let size = Int(arguments[2]), let factor = Int(arguments[3]),
          factor >= 1, factor <= 32 else {
        fail("zoom requires <out.png> <size> <factor 1-32>")
    }
    guard let image = render(size: size) else { fail("render failed") }
    let side = size * factor
    guard let ctx = CGContext(
        data: nil, width: side, height: side,
        bitsPerComponent: 8, bytesPerRow: 0,
        space: CGColorSpaceCreateDeviceRGB(),
        bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
    ) else { fail("no zoom context") }
    ctx.interpolationQuality = .none
    ctx.draw(image, in: CGRect(x: 0, y: 0, width: side, height: side))
    guard let zoomed = ctx.makeImage() else { fail("no zoom image") }
    guard writePNG(zoomed, to: URL(fileURLWithPath: arguments[1])) else { fail("write failed") }
    print("wrote \(arguments[1]) (\(size)px at \(factor)x)")

case "sheet":
    guard arguments.count >= 2 else { fail("sheet requires an output path") }
    // The shipped mark the way it is actually seen: true pixel sizes at 1:1 on a dark
    // plate and a light plate, so both backdrops can be judged side by side.
    let sizes = [16, 32, 64, 128, 256]
    let gap: CGFloat = 26
    let pad: CGFloat = 34
    let rowHeight: CGFloat = 168

    let strip = sizes.reduce(CGFloat(0)) { $0 + CGFloat($1) } + gap * CGFloat(sizes.count - 1)
    let width = Int(strip + pad * 2)
    let height = Int(rowHeight * 2 + pad)

    guard let ctx = CGContext(
        data: nil, width: width, height: height,
        bitsPerComponent: 8, bytesPerRow: 0,
        space: CGColorSpaceCreateDeviceRGB(),
        bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
    ) else { fail("no contact-sheet context") }

    ctx.setFillColor(CGColor(red: 0.10, green: 0.10, blue: 0.11, alpha: 1))
    ctx.fill(CGRect(x: 0, y: CGFloat(height) / 2, width: CGFloat(width), height: CGFloat(height) / 2))
    ctx.setFillColor(CGColor(red: 0.93, green: 0.92, blue: 0.90, alpha: 1))
    ctx.fill(CGRect(x: 0, y: 0, width: CGFloat(width), height: CGFloat(height) / 2))

    for centreY in [CGFloat(height) * 0.75, CGFloat(height) * 0.25] {
        var x = pad
        for size in sizes {
            let side = CGFloat(size)
            if let image = render(size: size) {
                ctx.draw(image, in: CGRect(x: x, y: centreY - side / 2, width: side, height: side))
            }
            x += side + gap
        }
    }

    guard let sheet = ctx.makeImage() else { fail("no sheet image") }
    guard writePNG(sheet, to: URL(fileURLWithPath: arguments[1])) else { fail("sheet write failed") }
    print("wrote \(arguments[1])")

default:
    fail("unknown command '\(command)'")
}