#!/usr/bin/env swift
//
//  generate-app-icon.swift
//  orbit — app icon generator
//
//  Renders the Dock icon from the v10 brand mark: a three-stop squircle plate,
//  a terracotta planet, front/back occluding rings, and an ice-blue companion.
//  Geometry matches the sidebar brand SVG in Resources/src-ui/index.html.
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

    // v10 brand plate and planet, matching the sidebar SVG.
    static let plateTop = hex(0x27272A)
    static let plateMid = hex(0x09090B)
    static let plateEnd = hex(0x7C2D12)
    static let planetHighlight = hex(0xFFF7ED)
    static let planetGold = hex(0xFDBA74)
    static let planetWarm = hex(0xEA580C)
    static let companion = hex(0x38BDF8)
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

func linearGradient(_ colors: [CGColor], _ locations: [CGFloat]) -> CGGradient {
    CGGradient(colorsSpace: CGColorSpaceCreateDeviceRGB(),
               colors: colors as CFArray,
               locations: locations)!
}

func radialGradient(_ colors: [CGColor], _ locations: [CGFloat]) -> CGGradient {
    linearGradient(colors, locations)
}

// MARK: - v10 brand mark

/// Maps the sidebar SVG's 64×64 viewBox into the 824px icon body.
enum OrbitMark {
    static let unit: CGFloat = 824 / 64

    static func point(_ x: CGFloat, _ y: CGFloat) -> CGPoint {
        CGPoint(x: 100 + x * unit, y: 924 - y * unit)
    }
}

func svgArcPath(from start: CGPoint, to end: CGPoint,
                rx rawRx: CGFloat, ry rawRy: CGFloat,
                rotationDegrees: CGFloat, largeArc: Bool, sweep: Bool,
                steps: Int = 220) -> CGPath {
    let path = CGMutablePath()
    let phi = rotationDegrees * .pi / 180
    let cosPhi = cos(phi)
    let sinPhi = sin(phi)
    let dx = (start.x - end.x) / 2
    let dy = (start.y - end.y) / 2
    let x1p = cosPhi * dx + sinPhi * dy
    let y1p = -sinPhi * dx + cosPhi * dy

    var rx = abs(rawRx)
    var ry = abs(rawRy)
    let lambda = (x1p * x1p) / (rx * rx) + (y1p * y1p) / (ry * ry)
    if lambda > 1 {
        let fit = sqrt(lambda)
        rx *= fit
        ry *= fit
    }

    let numerator = max(0, rx * rx * ry * ry - rx * rx * y1p * y1p - ry * ry * x1p * x1p)
    let denominator = max(0.000_001, rx * rx * y1p * y1p + ry * ry * x1p * x1p)
    let sign: CGFloat = largeArc == sweep ? -1 : 1
    let coef = sign * sqrt(numerator / denominator)
    let cxp = coef * rx * y1p / ry
    let cyp = coef * -ry * x1p / rx
    let cx = cosPhi * cxp - sinPhi * cyp + (start.x + end.x) / 2
    let cy = sinPhi * cxp + cosPhi * cyp + (start.y + end.y) / 2

    func signedAngle(_ ux: CGFloat, _ uy: CGFloat, _ vx: CGFloat, _ vy: CGFloat) -> CGFloat {
        atan2(ux * vy - uy * vx, ux * vx + uy * vy)
    }
    let theta1 = signedAngle(1, 0, (x1p - cxp) / rx, (y1p - cyp) / ry)
    var delta = signedAngle(
        (x1p - cxp) / rx, (y1p - cyp) / ry,
        (-x1p - cxp) / rx, (-y1p - cyp) / ry
    )
    if !sweep && delta > 0 { delta -= 2 * .pi }
    if sweep && delta < 0 { delta += 2 * .pi }

    for i in 0...steps {
        let theta = theta1 + delta * CGFloat(i) / CGFloat(steps)
        let x = cx + rx * cos(theta) * cosPhi - ry * sin(theta) * sinPhi
        let y = cy + rx * cos(theta) * sinPhi + ry * sin(theta) * cosPhi
        let mapped = OrbitMark.point(x, y)
        if i == 0 { path.move(to: mapped) } else { path.addLine(to: mapped) }
    }
    return path
}

func svgRotatedEllipse(cx: CGFloat, cy: CGFloat, rx: CGFloat, ry: CGFloat,
                       rotationDegrees: CGFloat, steps: Int = 240) -> CGPath {
    let path = CGMutablePath()
    let phi = rotationDegrees * .pi / 180
    for i in 0...steps {
        let t = CGFloat(i) / CGFloat(steps) * 2 * .pi
        let localX = rx * cos(t)
        let localY = ry * sin(t)
        let x = cx + localX * cos(phi) + localY * sin(phi)
        let y = cy - localX * sin(phi) + localY * cos(phi)
        let mapped = OrbitMark.point(x, y)
        if i == 0 { path.move(to: mapped) } else { path.addLine(to: mapped) }
    }
    path.closeSubpath()
    return path
}

func drawOrbitPlate(in ctx: CGContext, size: Int) {
    let tiny = size <= 32
    let centre = CGPoint(x: canvas / 2, y: canvas / 2)
    ctx.saveGState()
    ctx.addPath(squircle(center: centre, radius: bodyRadius))
    ctx.clip()

    // 145deg in CSS points down-right. In this y-up canvas that is (+sin, +cos).
    let angle = 145 * CGFloat.pi / 180
    let direction = CGPoint(x: sin(angle), y: cos(angle))
    let reach = bodyRadius * 1.2
    let plate = linearGradient(
        [
            tiny ? Brand.hex(0x3F3F46) : Brand.plateTop,
            tiny ? Brand.hex(0x1C1917) : Brand.plateMid,
            Brand.plateEnd
        ],
        [0, 0.65, 1]
    )
    ctx.drawLinearGradient(
        plate,
        start: CGPoint(x: centre.x - direction.x * reach, y: centre.y - direction.y * reach),
        end: CGPoint(x: centre.x + direction.x * reach, y: centre.y + direction.y * reach),
        options: [.drawsBeforeStartLocation, .drawsAfterEndLocation]
    )

    let highlight = linearGradient(
        [Brand.hex(0xFFFFFF, tiny ? 0.22 : 0.42), Brand.hex(0xFFFFFF, 0)],
        [0, 1]
    )
    ctx.drawLinearGradient(
        highlight,
        start: CGPoint(x: centre.x, y: centre.y + bodyRadius),
        end: CGPoint(x: centre.x, y: centre.y + bodyRadius * 0.62),
        options: []
    )
    let shade = linearGradient(
        [Brand.hex(0x000000, 0), Brand.hex(0x000000, tiny ? 0.22 : 0.35)],
        [0, 1]
    )
    ctx.drawLinearGradient(
        shade,
        start: CGPoint(x: centre.x, y: centre.y - bodyRadius * 0.42),
        end: CGPoint(x: centre.x, y: centre.y - bodyRadius),
        options: []
    )
    ctx.restoreGState()

    ctx.saveGState()
    ctx.addPath(squircle(center: centre, radius: bodyRadius - 1.5))
    ctx.setStrokeColor(Brand.hex(0xFFFFFF, tiny ? 0.20 : 0.12))
    ctx.setLineWidth(tiny ? 6 : 3)
    ctx.strokePath()
    ctx.restoreGState()
}

func paintRing(_ ctx: CGContext, path: CGPath, width: CGFloat, alpha: CGFloat, shadow: Bool) {
    ctx.saveGState()
    if shadow {
        ctx.setShadow(
            offset: CGSize(width: 0, height: -2.5 * OrbitMark.unit),
            blur: 2.2 * OrbitMark.unit,
            color: Brand.hex(0x000000, 0.48)
        )
    }
    let outline = path.copy(strokingWithWidth: width, lineCap: .round, lineJoin: .round, miterLimit: 4)
    ctx.addPath(outline)
    ctx.clip()
    ctx.setAlpha(alpha)
    ctx.drawLinearGradient(
        linearGradient(
            [Brand.hex(0xFEF08A), Brand.hex(0xFFFFFF), Brand.hex(0xFB923C)],
            [0, 0.5, 1]
        ),
        start: OrbitMark.point(6, 44),
        end: OrbitMark.point(58, 20),
        options: [.drawsBeforeStartLocation, .drawsAfterEndLocation]
    )
    ctx.restoreGState()
}

func drawOrbit(in ctx: CGContext, size: Int) {
    let tiny = size <= 32
    let boost: CGFloat = size <= 16 ? 2.6 : (tiny ? 1.75 : 1)
    drawOrbitPlate(in: ctx, size: size)

    let centre = CGPoint(x: canvas / 2, y: canvas / 2)
    ctx.saveGState()
    ctx.addPath(squircle(center: centre, radius: bodyRadius - 2))
    ctx.clip()

    if !tiny {
        ctx.saveGState()
        ctx.setStrokeColor(Brand.hex(0xFED7AA, 0.32))
        ctx.setLineWidth(2.2 * OrbitMark.unit)
        ctx.setLineCap(.round)
        ctx.addPath(svgRotatedEllipse(cx: 32, cy: 32, rx: 24, ry: 9.5, rotationDegrees: 32))
        ctx.strokePath()
        ctx.restoreGState()
    }

    let back = svgArcPath(
        from: CGPoint(x: 10, y: 41), to: CGPoint(x: 54, y: 23),
        rx: 26, ry: 10.5, rotationDegrees: -26, largeArc: false, sweep: true
    )
    let front = svgArcPath(
        from: CGPoint(x: 54, y: 23), to: CGPoint(x: 10, y: 41),
        rx: 26, ry: 10.5, rotationDegrees: -26, largeArc: false, sweep: true
    )
    paintRing(ctx, path: back, width: 3.6 * OrbitMark.unit * boost, alpha: 0.45, shadow: false)

    let planetSVG: CGFloat = tiny ? 16.2 : 13.5
    let planetR = planetSVG * OrbitMark.unit
    let planet = OrbitMark.point(32, 32)
    let light = OrbitMark.point(
        (32 - planetSVG) + 0.35 * planetSVG * 2,
        (32 - planetSVG) + 0.30 * planetSVG * 2
    )
    ctx.saveGState()
    if !tiny {
        ctx.setShadow(
            offset: CGSize(width: 0, height: -2.5 * OrbitMark.unit),
            blur: 2.2 * OrbitMark.unit,
            color: Brand.hex(0x000000, 0.48)
        )
    }
    ctx.addEllipse(in: CGRect(x: planet.x - planetR, y: planet.y - planetR,
                              width: planetR * 2, height: planetR * 2))
    ctx.clip()
    ctx.drawRadialGradient(
        radialGradient(
            [Brand.planetHighlight, Brand.planetGold, Brand.planetWarm, Brand.plateEnd],
            [0, 0.32, 0.70, 1]
        ),
        startCenter: light, startRadius: 1,
        endCenter: light, endRadius: 0.68 * planetSVG * 2 * OrbitMark.unit,
        options: [.drawsAfterEndLocation]
    )
    ctx.restoreGState()

    paintRing(ctx, path: front, width: 4.2 * OrbitMark.unit * boost, alpha: 1, shadow: !tiny)

    let star = OrbitMark.point(50, 21)
    let outer = (size <= 16 ? 8.4 : (tiny ? 6.4 : 4.8)) * OrbitMark.unit
    let inner = (size <= 16 ? 4.8 : (tiny ? 3.6 : 2.6)) * OrbitMark.unit
    ctx.saveGState()
    if !tiny {
        ctx.setShadow(
            offset: CGSize(width: 0, height: -2.5 * OrbitMark.unit),
            blur: 2.2 * OrbitMark.unit,
            color: Brand.hex(0x000000, 0.48)
        )
    }
    ctx.setFillColor(Brand.hex(0xFFFFFF))
    ctx.fillEllipse(in: CGRect(x: star.x - outer, y: star.y - outer, width: outer * 2, height: outer * 2))
    ctx.restoreGState()
    ctx.setFillColor(Brand.companion)
    ctx.fillEllipse(in: CGRect(x: star.x - inner, y: star.y - inner, width: inner * 2, height: inner * 2))

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

    drawOrbit(in: ctx, size: size)
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
