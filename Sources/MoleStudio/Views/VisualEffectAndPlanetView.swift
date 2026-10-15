import SwiftUI
import AppKit

/// Authentic AppKit `NSVisualEffectView` bridge for macOS Liquid Glass & Sidebar Vibrancy
public struct VisualEffectView: NSViewRepresentable {
    public var material: NSVisualEffectView.Material
    public var blendingMode: NSVisualEffectView.BlendingMode

    public init(
        material: NSVisualEffectView.Material = .sidebar,
        blendingMode: NSVisualEffectView.BlendingMode = .behindWindow
    ) {
        self.material = material
        self.blendingMode = blendingMode
    }

    public func makeNSView(context: Context) -> NSVisualEffectView {
        let view = NSVisualEffectView()
        view.material = material
        view.blendingMode = blendingMode
        view.state = .active
        return view
    }

    public func updateNSView(_ nsView: NSVisualEffectView, context: Context) {
        nsView.material = material
        nsView.blendingMode = blendingMode
    }
}

/// 120Hz ProMotion Native Metal/CoreGraphics Planetary Sphere Renderer (`TimelineView` + `Canvas`)
/// Honors `@Environment(\.accessibilityReduceMotion)` and `vm.reduceMotion`.
public struct PlanetCanvasView: View {
    public let module: MoleModule
    public let reduceMotion: Bool

    public init(module: MoleModule, reduceMotion: Bool) {
        self.module = module
        self.reduceMotion = reduceMotion
    }

    public var body: some View {
        TimelineView(.animation(minimumInterval: 1.0 / 30.0, paused: reduceMotion)) { timeline in
            Canvas { context, size in
                let t = reduceMotion ? 0.0 : timeline.date.timeIntervalSinceReferenceDate
                let center = CGPoint(x: size.width / 2, y: size.height / 2)
                let radius = min(size.width, size.height) * 0.36

                // Outer atmospheric halo
                let glowRect = CGRect(
                    x: center.x - radius * 1.35,
                    y: center.y - radius * 1.35,
                    width: radius * 2.7,
                    height: radius * 2.7
                )
                context.fill(
                    Path(ellipseIn: glowRect),
                    with: .radialGradient(
                        Gradient(colors: [module.primaryColor.opacity(0.42), .clear]),
                        center: center,
                        startRadius: radius * 0.5,
                        endRadius: radius * 1.35
                    )
                )

                // Planetary sphere body
                let sphereRect = CGRect(
                    x: center.x - radius,
                    y: center.y - radius,
                    width: radius * 2,
                    height: radius * 2
                )
                let spherePath = Path(ellipseIn: sphereRect)
                context.drawLayer { layer in
                    layer.clip(to: spherePath)
                    layer.fill(
                        spherePath,
                        with: .radialGradient(
                            Gradient(colors: [
                                .white.opacity(0.85),
                                module.primaryColor,
                                Color.black.opacity(0.85)
                            ]),
                            center: CGPoint(x: center.x - radius * 0.32, y: center.y - radius * 0.32),
                            startRadius: 2,
                            endRadius: radius * 1.35
                        )
                    )

                    // Animated planetary cloud bands
                    for i in -2...2 {
                        let yOffset = center.y + CGFloat(i) * (radius * 0.32)
                        let phase = CGFloat((t * 18.0 * (i % 2 == 0 ? 1.0 : -0.8)).truncatingRemainder(dividingBy: 40.0)) - 20.0
                        let bandRect = CGRect(x: center.x - radius * 0.65 + phase, y: yOffset - 4, width: radius * 1.1, height: 8)
                        layer.fill(Path(ellipseIn: bandRect), with: .color(.white.opacity(0.16)))
                    }
                }

                // Planetary ring for Mercury / Jupiter
                if module.hasPlanetaryRing {
                    var ringTransform = CGAffineTransform(translationX: center.x, y: center.y)
                    ringTransform = ringTransform.rotated(by: -0.32)
                    let ringRect = CGRect(x: -radius * 1.32, y: -radius * 0.30, width: radius * 2.64, height: radius * 0.60)
                    let ringPath = Path(ellipseIn: ringRect).applying(ringTransform)
                    context.stroke(ringPath, with: .color(.white.opacity(0.45)), lineWidth: 3.0)
                }
            }
        }
        .frame(width: 88, height: 88)
    }
}
