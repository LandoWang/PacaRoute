import AppKit

/// The approved Aurora mark, shared by Dock, sidebar and adaptive menu-bar artwork.
/// Canonical design: apps/macos/Design/AgentLink-aurora.svg (256-point canvas).
enum AgentLinkMark {
    private static let violet = rgb(0xB196FF)
    private static let mint = rgb(0x67EBC6)
    private static let light = rgb(0xDCFFF5)
    private static let wire = rgb(0xA9BEDB)
    private static let nodes = [CGPoint(x: 88, y: 122), CGPoint(x: 168, y: 122), CGPoint(x: 128, y: 181)]

    static func image(size: CGFloat = 24, colored: Bool = false) -> NSImage {
        let image = NSImage(size: NSSize(width: size, height: size), flipped: false) { rect in
            draw(in: rect, color: colored ? nil : .black)
            return true
        }
        image.isTemplate = !colored
        image.accessibilityDescription = "AgentLink · Aurora llama"
        return image
    }

    /// Tightly fitted glyph for the sidebar and monochrome status item.
    static func draw(in rect: NSRect, color: NSColor? = nil) {
        guard let context = NSGraphicsContext.current?.cgContext else { return }
        context.saveGState(); defer { context.restoreGState() }
        let scale = min(rect.width / 148, rect.height / 202)
        context.translateBy(x: rect.midX - 128 * scale, y: rect.midY + 113 * scale)
        context.scaleBy(x: scale, y: -scale)
        paint(context, compact: min(rect.width, rect.height) <= 32, color: color, glow: false)
    }

    static func drawAppIcon(in rect: NSRect) {
        guard let context = NSGraphicsContext.current?.cgContext else { return }
        context.saveGState(); defer { context.restoreGState() }
        context.translateBy(x: rect.minX, y: rect.maxY)
        context.scaleBy(x: rect.width / 256, y: -rect.height / 256)
        context.saveGState()
        context.addPath(CGPath(roundedRect: CGRect(x: 12, y: 12, width: 232, height: 232), cornerWidth: 54, cornerHeight: 54, transform: nil))
        context.clip()
        let gradient = CGGradient(colorsSpace: CGColorSpace(name: CGColorSpace.sRGB),
            colors: [rgb(0x15252A).cgColor, rgb(0x0C111D).cgColor] as CFArray, locations: [0, 1])!
        context.drawLinearGradient(gradient, start: CGPoint(x: 12, y: 12), end: CGPoint(x: 197.6, y: 244),
                                   options: [.drawsBeforeStartLocation, .drawsAfterEndLocation])
        context.restoreGState()
        context.translateBy(x: -5.12, y: 7)
        context.scaleBy(x: 1.04, y: 1.04)
        let compact = min(rect.width, rect.height) <= 32
        paint(context, compact: compact, color: nil, glow: !compact)
    }

    private static func paint(_ context: CGContext, compact: Bool, color: NSColor?, glow: Bool) {
        context.setLineCap(.round); context.setLineJoin(.round)
        // Hollow clearance around solid nodes keeps the heavy dashes distinct.
        context.saveGState()
        context.addRect(CGRect(x: 0, y: 0, width: 256, height: 256))
        for node in nodes { context.addEllipse(in: circle(node, radius: 10.5)) }
        context.clip(using: .evenOdd)
        context.setStrokeColor((color ?? wire.withAlphaComponent(0.82)).cgColor)
        context.setLineWidth(compact ? 3.2 : 2.8)
        context.setLineDash(phase: 0, lengths: [4.8, 4.8])
        context.addPath(links); context.strokePath()
        context.restoreGState()

        for (path, ink) in [(upper, violet), (jaw, violet), (visor, mint)] {
            context.saveGState()
            if glow {
                let scale = hypot(context.ctm.a, context.ctm.b)
                context.setShadow(offset: .zero, blur: 4.8 * scale, color: ink.withAlphaComponent(0.46).cgColor)
            }
            context.setStrokeColor((color ?? ink).cgColor)
            context.setLineWidth(compact ? 15.2 : 10.4)
            context.addPath(path); context.strokePath()
            context.restoreGState()
        }
        context.setFillColor((color ?? light).cgColor)
        for node in nodes { context.fillEllipse(in: circle(node, radius: compact ? 9.15 : 8.1)) }
    }

    private static func circle(_ center: CGPoint, radius: CGFloat) -> CGRect {
        CGRect(x: center.x - radius, y: center.y - radius, width: radius * 2, height: radius * 2)
    }
    private static func rgb(_ value: UInt32) -> NSColor {
        NSColor(srgbRed: CGFloat((value >> 16) & 255) / 255,
                green: CGFloat((value >> 8) & 255) / 255, blue: CGFloat(value & 255) / 255, alpha: 1)
    }

    static func png(pixels: Int, appIcon: Bool) -> Data {
        let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels,
            bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
            colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
        let rect = NSRect(x: 0, y: 0, width: pixels, height: pixels)
        if appIcon { drawAppIcon(in: rect) } else { draw(in: rect) }
        NSGraphicsContext.restoreGraphicsState()
        return bitmap.representation(using: .png, properties: [:])!
    }

    private static let upper: CGPath = {
        let p = CGMutablePath()
        p.move(to: CGPoint(x: 74, y: 97))
        p.addCurve(to: CGPoint(x: 82, y: 78), control1: CGPoint(x: 67, y: 90), control2: CGPoint(x: 71, y: 79))
        p.addLine(to: CGPoint(x: 75, y: 67))
        p.addCurve(to: CGPoint(x: 66, y: 19), control1: CGPoint(x: 66, y: 47), control2: CGPoint(x: 61, y: 30))
        p.addCurve(to: CGPoint(x: 91, y: 65), control1: CGPoint(x: 79, y: 33), control2: CGPoint(x: 85, y: 49))
        p.addCurve(to: CGPoint(x: 112, y: 61), control1: CGPoint(x: 99, y: 55), control2: CGPoint(x: 107, y: 56))
        p.addCurve(to: CGPoint(x: 139, y: 60), control1: CGPoint(x: 114, y: 47), control2: CGPoint(x: 132, y: 45))
        p.addCurve(to: CGPoint(x: 165, y: 67), control1: CGPoint(x: 149, y: 53), control2: CGPoint(x: 160, y: 55))
        p.addCurve(to: CGPoint(x: 190, y: 19), control1: CGPoint(x: 173, y: 48), control2: CGPoint(x: 179, y: 31))
        p.addCurve(to: CGPoint(x: 175, y: 78), control1: CGPoint(x: 196, y: 33), control2: CGPoint(x: 187, y: 65))
        p.addCurve(to: CGPoint(x: 182, y: 97), control1: CGPoint(x: 185, y: 78), control2: CGPoint(x: 192, y: 87))
        return p
    }()

    private static let jaw: CGPath = {
        let p = CGMutablePath()
        p.move(to: CGPoint(x: 77, y: 149))
        p.addCurve(to: CGPoint(x: 112, y: 198), control1: CGPoint(x: 86, y: 167), control2: CGPoint(x: 97, y: 184))
        p.addQuadCurve(to: CGPoint(x: 144, y: 198), control: CGPoint(x: 128, y: 211))
        p.addCurve(to: CGPoint(x: 179, y: 149), control1: CGPoint(x: 159, y: 184), control2: CGPoint(x: 170, y: 167))
        return p
    }()

    private static let visor: CGPath = {
        let p = CGMutablePath()
        p.move(to: CGPoint(x: 75, y: 100))
        p.addLine(to: CGPoint(x: 104, y: 100))
        p.addLine(to: CGPoint(x: 116, y: 108))
        p.addLine(to: CGPoint(x: 140, y: 108))
        p.addLine(to: CGPoint(x: 152, y: 100))
        p.addLine(to: CGPoint(x: 181, y: 100))
        p.addQuadCurve(to: CGPoint(x: 193, y: 112), control: CGPoint(x: 193, y: 100))
        p.addLine(to: CGPoint(x: 193, y: 129))
        p.addQuadCurve(to: CGPoint(x: 183, y: 141), control: CGPoint(x: 193, y: 138))
        p.addLine(to: CGPoint(x: 161, y: 146))
        p.addQuadCurve(to: CGPoint(x: 147, y: 139), control: CGPoint(x: 153, y: 148))
        p.addLine(to: CGPoint(x: 136, y: 127))
        p.addLine(to: CGPoint(x: 120, y: 127))
        p.addLine(to: CGPoint(x: 109, y: 139))
        p.addQuadCurve(to: CGPoint(x: 95, y: 146), control: CGPoint(x: 104, y: 148))
        p.addLine(to: CGPoint(x: 73, y: 141))
        p.addQuadCurve(to: CGPoint(x: 63, y: 129), control: CGPoint(x: 63, y: 138))
        p.addLine(to: CGPoint(x: 63, y: 112))
        p.addQuadCurve(to: CGPoint(x: 75, y: 100), control: CGPoint(x: 63, y: 100))
        p.closeSubpath()
        return p
    }()

    private static let links: CGPath = {
        let p = CGMutablePath()
        p.move(to: CGPoint(x: 88, y: 122))
        p.addLine(to: CGPoint(x: 168, y: 122))
        p.addLine(to: CGPoint(x: 128, y: 181))
        p.closeSubpath()
        return p
    }()
}
