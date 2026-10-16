#!/usr/bin/env swift
//
//  generate-app-icon.swift
//  orbit — app icon generator
//
//  Renders the app icon from code rather than shipping an opaque bitmap, so every
//  size is drawn natively (crisp at 16px, detailed at 1024px) and the palette stays
//  locked to the CSS custom properties in Resources/src-ui/index.html.
//
//  Usage:
//    swift scripts/generate-app-icon.swift iconset <out.iconset>
//    swift scripts/generate-app-icon.swift icns    <out.icns>
//    swift scripts/generate-app-icon.swift sheet   <out.png>
//    swift scripts/generate-app-icon.swift single  <out.png> <size> [concept]
//
import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

// MARK: - Palette (mirrors the CSS custom properties in Resources/src-ui/index.html)

enum Brand {
    static func hex(_ value: UInt32, _ alpha: CGFloat = 1) -> CGColor {
        CGColor(
            red: CGFloat((value >> 16) & 0xFF) / 255,
            green: CGFloat((value >> 8) & 0xFF) / 255,
            blue: CGFloat(value & 0xFF) / 255,
            alpha: alpha
        )
    }

    // Warm Obsidian surfaces
    static let surfaceTop = hex(0x272320)
    static let surfaceMid = hex(0x161514)
    static let surfaceBottom = hex(0x0A0A09)

    // --accent-primary
    static let terracotta = hex(0xD97757)
    static let terracottaLight = hex(0xFFD9C4)
    static let terracottaDeep = hex(0x59240F)

    // --text-primary
    static let alabaster = hex(0xF4F2ED)
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
    mutating func range(_ lo: Double, _ hi: Double) -> Double { lo + next() * (hi - lo) }
}

/// A point on a tilted ellipse: `centre + R(tilt) * (a·cos t, b·sin t)`.
func orbitPoint(centre: CGPoint, a: CGFloat, b: CGFloat, tilt: CGFloat,
                _ t: CGFloat) -> CGPoint {
    let x = a * cos(t)
    let y = b * sin(t)
    return CGPoint(
        x: centre.x + x * cos(tilt) - y * sin(tilt),
        y: centre.y + x * sin(tilt) + y * cos(tilt)
    )
}

func orbitArc(centre: CGPoint, a: CGFloat, b: CGFloat, tilt: CGFloat,
              from: CGFloat, to: CGFloat, steps: Int = 240) -> CGPath {
    let path = CGMutablePath()
    for i in 0...steps {
        let t = from + (to - from) * CGFloat(i) / CGFloat(steps)
        let p = orbitPoint(centre: centre, a: a, b: b, tilt: tilt, t)
        if i == 0 { path.move(to: p) } else { path.addLine(to: p) }
    }
    return path
}

func linearGradient(_ colors: [CGColor], _ locations: [CGFloat]) -> CGGradient {
    CGGradient(colorsSpace: CGColorSpaceCreateDeviceRGB(),
               colors: colors as CFArray,
               locations: locations)!
}

func radialGradient(_ colors: [CGColor], _ locations: [CGFloat]) -> CGGradient {
    linearGradient(colors, locations)
}

// MARK: - Icon concepts

enum Concept: String, CaseIterable {
    case orbit   // ringed planet — the repository name, made literal
    case sweep   // a cleaning arc with a spark
    case system  // a star with several orbiting bodies
}

// MARK: - Shared layers

func drawBody(in ctx: CGContext, size: Int, plain: Bool = false) {
    let centre = CGPoint(x: canvas / 2, y: canvas / 2)
    let mask = squircle(center: centre, radius: bodyRadius)

    ctx.saveGState()
    ctx.addPath(mask)
    ctx.clip()

    // Warm obsidian vertical gradient. Small sizes get a lifted floor, otherwise the
    // shrunken body reads as a plain black square next to the warm large sizes.
    let bg = linearGradient(
        plain
            ? [Brand.hex(0x3A322C), Brand.hex(0x1F1C19), Brand.hex(0x0E0E0D)]
            : [Brand.surfaceTop, Brand.surfaceMid, Brand.surfaceBottom],
        [0.0, 0.55, 1.0]
    )
    ctx.drawLinearGradient(
        bg,
        start: CGPoint(x: centre.x, y: canvas),
        end: CGPoint(x: centre.x, y: 0),
        options: []
    )

    // Warm terracotta bloom in the upper-left, acting as the light source. Skipped at
    // icon sizes, where it only washes out the silhouette.
    if !plain {
        let bloom = radialGradient(
            [Brand.terracotta.copy(alpha: 0.22)!, Brand.terracotta.copy(alpha: 0.0)!],
            [0.0, 1.0]
        )
        ctx.drawRadialGradient(
            bloom,
            startCenter: CGPoint(x: 300, y: 800), startRadius: 0,
            endCenter: CGPoint(x: 300, y: 800), endRadius: 780,
            options: []
        )
    }

    ctx.restoreGState()

    // Hairline inner edge, matching the app's 1px border tokens.
    ctx.saveGState()
    ctx.addPath(squircle(center: centre, radius: bodyRadius - 1.5))
    ctx.setStrokeColor(Brand.alabaster.copy(alpha: plain ? 0.16 : 0.10)!)
    ctx.setLineWidth(plain ? 5 : 3)
    ctx.strokePath()
    ctx.restoreGState()
}

func drawStarfield(in ctx: CGContext, size: Int) {
    guard size > 64 else { return }

    var rng = LCG(state: 0x5EED_1234)
    let centre = CGPoint(x: canvas / 2, y: canvas / 2)

    ctx.saveGState()
    ctx.addPath(squircle(center: centre, radius: bodyRadius - 6))
    ctx.clip()

    for _ in 0..<26 {
        let x = rng.range(140, 900)
        let y = rng.range(140, 900)
        // Keep the middle clear so the mark stays legible.
        guard hypot(x - 512, y - 500) > 260 else { continue }
        let r = rng.range(1.6, 4.2)
        let a = rng.range(0.10, 0.42)
        ctx.setFillColor(Brand.alabaster.copy(alpha: CGFloat(a))!)
        ctx.fillEllipse(in: CGRect(x: x - r, y: y - r, width: r * 2, height: r * 2))
    }

    ctx.restoreGState()
}

/// Traces a band along the orbit ellipse with a tapered alpha, used for both concepts.
func strokeOrbit(_ ctx: CGContext, centre: CGPoint, a: CGFloat, b: CGFloat, tilt: CGFloat,
                 from: CGFloat, to: CGFloat, width: CGFloat, colour: CGColor, glow: Bool) {
    let path = orbitArc(centre: centre, a: a, b: b, tilt: tilt, from: from, to: to)

    if glow {
        ctx.saveGState()
        ctx.setLineCap(.round)
        ctx.addPath(path)
        ctx.setStrokeColor(colour.copy(alpha: colour.alpha * 0.22)!)
        ctx.setLineWidth(width * 4.4)
        ctx.strokePath()
        ctx.restoreGState()
    }

    ctx.saveGState()
    ctx.setLineCap(.round)
    ctx.addPath(path)
    ctx.setStrokeColor(colour)
    ctx.setLineWidth(width)
    ctx.strokePath()
    ctx.restoreGState()
}

// MARK: - Concept: orbit (ringed planet)

/// A Saturn-style ring drawn as concentric bands so it reads as a solid disc rather
/// than a hairline. `from`/`to` let the caller split it around the planet.
func drawRingBand(_ ctx: CGContext, centre: CGPoint, a: CGFloat, b: CGFloat, tilt: CGFloat,
                  from: CGFloat, to: CGFloat, detailed: Bool, tiny: Bool) {
    if tiny {
        // At 16–32px only two bands survive; anything finer turns to mud.
        strokeOrbit(ctx, centre: centre, a: a, b: b, tilt: tilt, from: from, to: to,
                    width: 54, colour: Brand.alabaster.copy(alpha: 0.36)!, glow: false)
        strokeOrbit(ctx, centre: centre, a: a * 0.90, b: b * 0.90, tilt: tilt, from: from, to: to,
                    width: 17, colour: Brand.alabaster.copy(alpha: 0.95)!, glow: false)
        return
    }
    if detailed {
        strokeOrbit(ctx, centre: centre, a: a, b: b, tilt: tilt, from: from, to: to,
                    width: 78, colour: Brand.terracotta.copy(alpha: 0.12)!, glow: false)
    }
    strokeOrbit(ctx, centre: centre, a: a, b: b, tilt: tilt, from: from, to: to,
                width: 42, colour: Brand.alabaster.copy(alpha: 0.16)!, glow: false)
    strokeOrbit(ctx, centre: centre, a: a * 0.962, b: b * 0.962, tilt: tilt, from: from, to: to,
                width: 16, colour: Brand.alabaster.copy(alpha: 0.38)!, glow: false)
    strokeOrbit(ctx, centre: centre, a: a * 0.910, b: b * 0.910, tilt: tilt, from: from, to: to,
                width: 6, colour: Brand.alabaster.copy(alpha: 0.92)!, glow: false)
}

func drawOrbit(in ctx: CGContext, size: Int) {
    let tiny = size <= 32
    let detailed = size > 64

    drawBody(in: ctx, size: size, plain: tiny)
    if !tiny { drawStarfield(in: ctx, size: size) }

    let tilt: CGFloat = 0.34
    let centre = CGPoint(x: 512, y: 492)
    // Small sizes get a larger planet and a chunkier ring so the silhouette survives.
    let planetR: CGFloat = tiny ? 250 : 226
    let ringA: CGFloat = tiny ? 358 : 372
    let ringB: CGFloat = tiny ? 126 : 122

    // Far half of the ring disappears behind the planet.
    drawRingBand(ctx, centre: centre, a: ringA, b: ringB, tilt: tilt,
                 from: 0, to: .pi, detailed: detailed, tiny: tiny)

    // Atmospheric halo behind the sphere.
    if !tiny {
        let halo = radialGradient(
            [Brand.terracotta.copy(alpha: 0.42)!, Brand.terracotta.copy(alpha: 0.0)!],
            [0.0, 1.0]
        )
        ctx.drawRadialGradient(
            halo,
            startCenter: centre, startRadius: planetR * 0.88,
            endCenter: centre, endRadius: planetR * 1.45,
            options: []
        )
    }

    // Warm terracotta sphere, lit from the upper left.
    ctx.saveGState()
    ctx.addEllipse(in: CGRect(x: centre.x - planetR, y: centre.y - planetR,
                              width: planetR * 2, height: planetR * 2))
    ctx.clip()
    let sphere = radialGradient(
        [Brand.terracottaLight, Brand.hex(0xE88A63), Brand.terracotta, Brand.terracottaDeep],
        [0.0, 0.32, 0.62, 1.0]
    )
    let light = CGPoint(x: centre.x - planetR * 0.40, y: centre.y + planetR * 0.42)
    ctx.drawRadialGradient(
        sphere,
        startCenter: light, startRadius: 2,
        endCenter: light, endRadius: planetR * 2.05,
        options: [.drawsAfterEndLocation]
    )
    ctx.restoreGState()

    // Near half of the ring crosses in front of the planet.
    drawRingBand(ctx, centre: centre, a: ringA, b: ringB, tilt: tilt,
                 from: .pi, to: 2 * .pi, detailed: detailed, tiny: tiny)
}

// MARK: - Concept: sweep (cleaning arc)

func drawSweep(in ctx: CGContext, size: Int) {
    drawBody(in: ctx, size: size)
    drawStarfield(in: ctx, size: size)

    let detailed = size > 64
    let centre = CGPoint(x: 512, y: 500)
    let tilt: CGFloat = -0.34

    // Two concentric cleaning arcs of decreasing weight.
    strokeOrbit(ctx, centre: centre, a: 330, b: 250, tilt: tilt,
                from: .pi * 0.12, to: .pi * 0.94,
                width: 30, colour: Brand.terracotta.copy(alpha: 0.92)!, glow: detailed)
    strokeOrbit(ctx, centre: centre, a: 232, b: 168, tilt: tilt,
                from: .pi * 0.20, to: .pi * 0.80,
                width: 20, colour: Brand.terracottaLight.copy(alpha: 0.72)!, glow: false)

    // The point being swept: a bright spark.
    let spark = CGPoint(x: 742, y: 706)
    let glow = radialGradient(
        [Brand.alabaster.copy(alpha: 0.85)!, Brand.terracotta.copy(alpha: 0.0)!],
        [0.0, 1.0]
    )
    ctx.drawRadialGradient(
        glow,
        startCenter: spark, startRadius: 4,
        endCenter: spark, endRadius: 132,
        options: []
    )
    ctx.setFillColor(Brand.alabaster)
    ctx.fillEllipse(in: CGRect(x: spark.x - 32, y: spark.y - 32, width: 64, height: 64))

    // Two smaller motes, like dust lifting away.
    for (dx, dy, r, a) in [(-104.0, 78.0, 15.0, 0.55), (-176.0, 24.0, 9.0, 0.34)] {
        ctx.setFillColor(Brand.alabaster.copy(alpha: CGFloat(a))!)
        ctx.fillEllipse(in: CGRect(x: spark.x + dx - r, y: spark.y + dy - r,
                                   width: r * 2, height: r * 2))
    }
}

// MARK: - Concept: system (a star with orbiting bodies)

func drawSystem(in ctx: CGContext, size: Int) {
    drawBody(in: ctx, size: size)
    drawStarfield(in: ctx, size: size)

    let detailed = size > 64
    let centre = CGPoint(x: 512, y: 500)

    // Star.
    let halo = radialGradient(
        [Brand.terracotta.copy(alpha: 0.55)!, Brand.terracotta.copy(alpha: 0.0)!],
        [0.0, 1.0]
    )
    ctx.drawRadialGradient(
        halo,
        startCenter: centre, startRadius: 40,
        endCenter: centre, endRadius: 220,
        options: []
    )
    let star = radialGradient([Brand.alabaster, Brand.terracottaLight, Brand.terracotta], [0.0, 0.6, 1.0])
    let starR: CGFloat = 96
    ctx.saveGState()
    ctx.addEllipse(in: CGRect(x: centre.x - starR, y: centre.y - starR,
                              width: starR * 2, height: starR * 2))
    ctx.clip()
    ctx.drawRadialGradient(
        star,
        startCenter: CGPoint(x: centre.x - 26, y: centre.y + 30), startRadius: 2,
        endCenter: CGPoint(x: centre.x - 26, y: centre.y + 30), endRadius: starR * 2,
        options: [.drawsAfterEndLocation]
    )
    ctx.restoreGState()

    // Three orbits, each carrying one body.
    let orbits: [(a: CGFloat, b: CGFloat, tilt: CGFloat, t: CGFloat, r: CGFloat, colour: CGColor)] = [
        (300, 116, 0.42, 0.55, 34, Brand.hex(0x4B83F0)),   // Earth
        (368, 150, -0.22, 3.55, 28, Brand.hex(0xE05263)),  // Mars
        (432, 190, 0.12, 1.25, 22, Brand.hex(0xD99B26))    // Jupiter
    ]

    for o in orbits {
        strokeOrbit(ctx, centre: centre, a: o.a, b: o.b, tilt: o.tilt,
                    from: 0, to: 2 * .pi,
                    width: detailed ? 5 : 7, colour: Brand.alabaster.copy(alpha: 0.26)!, glow: false)

        let p = orbitPoint(centre: centre, a: o.a, b: o.b, tilt: o.tilt, o.t)
        ctx.setFillColor(o.colour)
        ctx.fillEllipse(in: CGRect(x: p.x - o.r, y: p.y - o.r, width: o.r * 2, height: o.r * 2))
        if detailed {
            ctx.setFillColor(Brand.alabaster.copy(alpha: 0.28)!)
            ctx.fillEllipse(in: CGRect(x: p.x - o.r * 0.42, y: p.y - o.r * 0.10,
                                       width: o.r * 0.62, height: o.r * 0.62))
        }
    }
}

// MARK: - Rendering

func render(concept: Concept, size: Int) -> CGImage? {
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

    switch concept {
    case .orbit: drawOrbit(in: ctx, size: size)
    case .sweep: drawSweep(in: ctx, size: size)
    case .system: drawSystem(in: ctx, size: size)
    }

    dither(ctx, size: size)
    return ctx.makeImage()
}

/// An 8-bit gradient across a large, dark surface bands visibly — the obsidian
/// background shows concentric steps and blotches. A sub-percent noise layer breaks
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
    fail("usage: generate-app-icon.swift <iconset|icns|sheet|single> ...")
}

let concept = Concept.orbit

switch command {
case "iconset":
    guard arguments.count >= 2 else { fail("iconset requires an output directory") }
    let dir = URL(fileURLWithPath: arguments[1])
    try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    for entry in iconsetEntries {
        guard let image = render(concept: concept, size: entry.pixels) else {
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
        guard let image = render(concept: concept, size: entry.pixels) else {
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
        fail("single requires <out.png> <size> [concept]")
    }
    let chosen = arguments.count >= 4 ? (Concept(rawValue: arguments[3]) ?? concept) : concept
    guard let image = render(concept: chosen, size: size) else { fail("render failed") }
    guard writePNG(image, to: URL(fileURLWithPath: arguments[1])) else { fail("write failed") }
    print("wrote \(arguments[1]) (\(size)px, \(chosen.rawValue))")

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
            if let image = render(concept: concept, size: size) {
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
