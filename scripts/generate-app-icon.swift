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
//    2  a terracotta sphere, lit from the upper left
//    3  a gold orbit, drawn behind the sphere for its far half and in front of
//       it for its near half (三.3 真实空间遮挡关系)
//    4  a flat ice-blue companion riding the orbit, from 32pt up
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
    static let plateTop = hex(0x2E2E33)
    static let plateMid = hex(0x0A0A0C)
    static let plateEnd = hex(0x6E2910)

    // Sphere: one terracotta family, three steps from the key light to the limb.
    static let sphereLight = hex(0xFDBA74)
    static let sphereWarm = hex(0xF97316)
    static let sphereDeep = hex(0xC2410C)

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
    static let orbitRx: CGFloat = 26
    static let orbitRy: CGFloat = 10.5
    static let orbitTilt: CGFloat = -26

    /// The ice-blue companion sits on the orbit where it clears the sphere,
    /// matching the sidebar SVG's circle at (50, 21).
    static let companionHome = CGPoint(x: 50, y: 21)

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
    let ringWidth: CGFloat
    let backRingWidth: CGFloat
    let orbitStretch: CGFloat
    let showsShading: Bool
    /// Radius of the ice-blue companion on the 64-grid. Zero means "do not draw
    /// it": at 16pt a companion is a stray dot beside the sphere, not a symbol.
    let companionRadius: CGFloat
    let plateTop: CGColor
    let plateMid: CGColor
    let plateEnd: CGColor

    /// 128pt and up: the mark at full detail.
    static let full = IconProfile(
        planetRadius: 12.6, ringWidth: 3.2, backRingWidth: 3.2, orbitStretch: 1.0,
        showsShading: true, companionRadius: 3.3,
        plateTop: Brand.plateTop, plateMid: Brand.plateMid, plateEnd: Brand.plateEnd
    )

    /// 32–64pt: the same drawing, a touch heavier so it survives downscaling.
    static let compact = IconProfile(
        planetRadius: 13.0, ringWidth: 3.6, backRingWidth: 3.6, orbitStretch: 1.06,
        showsShading: true, companionRadius: 3.7,
        plateTop: Brand.hex(0x34341F), plateMid: Brand.hex(0x0C0C0E), plateEnd: Brand.hex(0x3A1A0A)
    )

    /// 16pt: a pictogram. One uniform ring, opened far enough to clear the
    /// sphere, because at this size a thin occluded band degenerates into mush.
    static let mini = IconProfile(
        planetRadius: 12.6, ringWidth: 4.2, backRingWidth: 4.2, orbitStretch: 1.46,
        showsShading: false, companionRadius: 0,
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
        gradient([profile.plateTop, profile.plateMid, profile.plateEnd], [0, 0.90, 1]),
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
    // Key light at 35% / 30% of the bounding box, as in the sidebar SVG.
    let svg = profile.planetRadius
    let light = OrbitMark.point((32 - svg) + 0.35 * svg * 2, (32 - svg) + 0.30 * svg * 2)

    ctx.saveGState()
    ctx.addEllipse(in: extent)
    ctx.clip()
    if profile.showsShading {
        ctx.drawRadialGradient(
            gradient([Brand.sphereLight, Brand.sphereWarm, Brand.sphereDeep], [0, 0.62, 1]),
            startCenter: light, startRadius: 1,
            endCenter: light, endRadius: radius * 1.06,
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

/// Splits the orbit at the two points where it crosses the sphere's silhouette.
///
/// For a tilted circle seen in projection the near half is exactly the 180° of
/// parameter space centred on the lowest projected point, so the seams land on
/// the ends of the major axis and the two halves abut with no gap. 三.3 asks for
/// this: the far half runs behind the sphere, the near half in front of it.
func orbitHalves(stretch: CGFloat, steps: Int = 720) -> (front: CGPath, back: CGPath) {
    let phi = OrbitMark.orbitTilt * .pi / 180
    var points: [CGPoint] = []
    points.reserveCapacity(steps)
    for index in 0..<steps {
        let t = CGFloat(index) / CGFloat(steps) * 2 * .pi
        let localX = OrbitMark.orbitRx * cos(t)
        let localY = OrbitMark.orbitRy * stretch * sin(t)
        let x = 32 + localX * cos(phi) + localY * sin(phi)
        let y = 32 - localX * sin(phi) + localY * cos(phi)
        points.append(OrbitMark.point(x, y))
    }

    // This canvas is y-up, so the near half is centred on the smallest y.
    var near = 0
    for index in points.indices where points[index].y < points[near].y { near = index }

    let seamA = ((near - steps / 4) % steps + steps) % steps
    let seamB = (near + steps / 4) % steps

    func walk(from: Int, to: Int) -> CGPath {
        let path = CGMutablePath()
        path.move(to: points[from])
        var index = (from + 1) % steps
        while index != (to + 1) % steps {
            path.addLine(to: points[index])
            index = (index + 1) % steps
        }
        path.addLine(to: points[to])
        return path
    }
    return (walk(from: seamA, to: seamB), walk(from: seamB, to: seamA))
}

/// The 16pt pictogram: one closed ring, stroked flat, drawn clear of the sphere.
func orbitPictogram(stretch: CGFloat, steps: Int = 360) -> CGPath {
    let phi = OrbitMark.orbitTilt * .pi / 180
    let path = CGMutablePath()
    for index in 0...steps {
        let t = CGFloat(index) / CGFloat(steps) * 2 * .pi
        let localX = OrbitMark.orbitRx * 0.94 * cos(t)
        let localY = OrbitMark.orbitRy * stretch * sin(t)
        let x = 32 + localX * cos(phi) + localY * sin(phi)
        let y = 32 - localX * sin(phi) + localY * cos(phi)
        let point = OrbitMark.point(x, y)
        if index == 0 { path.move(to: point) } else { path.addLine(to: point) }
    }
    path.closeSubpath()
    return path
}

func stroke(_ ctx: CGContext, path: CGPath, width: CGFloat, color: CGColor) {
    ctx.saveGState()
    ctx.setStrokeColor(color)
    ctx.setLineWidth(width)
    ctx.setLineCap(.round)
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
    let halves = orbitHalves(stretch: profile.orbitStretch)
    // Far half first, then the sphere, then the near half: the sphere paints
    // over the far arc, which is what produces the occlusion seam.
    stroke(ctx, path: halves.back, width: OrbitMark.length(profile.backRingWidth),
           color: Brand.ringGoldDim)
    drawSphere(in: ctx, profile: profile)
    stroke(ctx, path: halves.front, width: OrbitMark.length(profile.ringWidth),
           color: Brand.ringGold)
}

// MARK: - Element 4: the ice-blue companion

/// A flat disc and a slightly darker rim. No highlight: the companion carries
/// the brand's cool accent, and a second light source inside one 28pt badge
/// would fight the sphere for attention.
func drawCompanion(in ctx: CGContext, profile: IconProfile) {
    guard profile.companionRadius > 0 else { return }
    let centre = OrbitMark.point(OrbitMark.companionHome.x, OrbitMark.companionHome.y)
    let radius = OrbitMark.length(profile.companionRadius)
    let extent = CGRect(x: centre.x - radius, y: centre.y - radius, width: radius * 2, height: radius * 2)

    ctx.saveGState()
    ctx.setFillColor(Brand.companion)
    ctx.fillEllipse(in: extent)
    ctx.restoreGState()

    guard profile.showsShading else { return }
    ctx.saveGState()
    ctx.addEllipse(in: extent.insetBy(dx: radius * 0.09, dy: radius * 0.09))
    ctx.setStrokeColor(Brand.companionRim)
    ctx.setLineWidth(max(1, radius * 0.16))
    ctx.strokePath()
    ctx.restoreGState()
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
    drawCompanion(in: ctx, profile: profile)
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