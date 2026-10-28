import AppKit

/// One vector mark for the app icon, sidebar and monochrome menu-bar image.
enum AgentLinkMark {
    static func image(size: CGFloat = 24) -> NSImage {
        let image = NSImage(size: NSSize(width: size, height: size), flipped: false) { rect in
            draw(in: rect, color: .black)
            return true
        }
        image.isTemplate = true
        image.accessibilityDescription = "AgentLink · llama robot"
        return image
    }

    static func draw(in rect: NSRect, color: NSColor) {
        let transform = AffineTransform(translationByX: rect.minX, byY: rect.maxY)
            .scaled(x: rect.width / 100, y: -rect.height / 100)
        func stroke(_ path: NSBezierPath, width: CGFloat) {
            path.transform(using: transform)
            path.lineWidth = width * min(rect.width, rect.height) / 100
            path.lineCapStyle = .round; path.lineJoinStyle = .round
            color.setStroke(); path.stroke()
        }
        func node(_ x: CGFloat, _ y: CGFloat, radius: CGFloat) {
            let path = NSBezierPath(ovalIn: NSRect(x: x - radius, y: y - radius, width: radius * 2, height: radius * 2))
            path.transform(using: transform); color.setFill(); path.fill()
        }
        let compact = min(rect.width, rect.height) < 28
        // Slim upright ears and a tapered face preserve the inverted-triangle silhouette.
        let head = NSBezierPath()
        head.move(to: NSPoint(x: 28, y: 39))
        head.line(to: NSPoint(x: 23, y: 16))
        head.curve(to: NSPoint(x: 32, y: 13), controlPoint1: NSPoint(x: 21, y: 7), controlPoint2: NSPoint(x: 29, y: 6))
        head.line(to: NSPoint(x: 39, y: 33))
        head.line(to: NSPoint(x: 61, y: 33))
        head.line(to: NSPoint(x: 68, y: 13))
        head.curve(to: NSPoint(x: 77, y: 16), controlPoint1: NSPoint(x: 71, y: 6), controlPoint2: NSPoint(x: 79, y: 7))
        head.line(to: NSPoint(x: 72, y: 39))
        head.line(to: NSPoint(x: 77, y: 44))
        head.curve(to: NSPoint(x: 78, y: 54), controlPoint1: NSPoint(x: 81, y: 47), controlPoint2: NSPoint(x: 81, y: 49))
        head.line(to: NSPoint(x: 61, y: 78))
        head.curve(to: NSPoint(x: 54, y: 82), controlPoint1: NSPoint(x: 59, y: 81), controlPoint2: NSPoint(x: 57, y: 82))
        head.line(to: NSPoint(x: 46, y: 82))
        head.curve(to: NSPoint(x: 39, y: 78), controlPoint1: NSPoint(x: 43, y: 82), controlPoint2: NSPoint(x: 41, y: 81))
        head.line(to: NSPoint(x: 22, y: 54))
        head.curve(to: NSPoint(x: 23, y: 44), controlPoint1: NSPoint(x: 19, y: 49), controlPoint2: NSPoint(x: 19, y: 47))
        head.close()
        stroke(head, width: compact ? 6.0 : 4.8)

        // The original three linked nodes become the robot's eyes and muzzle.
        let link = NSBezierPath()
        link.move(to: NSPoint(x: 35, y: 49))
        link.line(to: NSPoint(x: 50, y: 66))
        link.line(to: NSPoint(x: 65, y: 49))
        stroke(link, width: compact ? 4.2 : 3.2)
        node(35, 49, radius: compact ? 5.2 : 4.6)
        node(65, 49, radius: compact ? 5.2 : 4.6)
        node(50, 66, radius: 4.1)
    }

    static func drawAppIcon(in rect: NSRect) {
        let tile = rect.insetBy(dx: rect.width * 0.06, dy: rect.height * 0.06)
        let background = NSBezierPath(roundedRect: tile, xRadius: rect.width * 0.21, yRadius: rect.height * 0.21)
        NSGradient(starting: NSColor(red: 0.19, green: 0.08, blue: 0.34, alpha: 1),
                   ending: NSColor(red: 0.57, green: 0.27, blue: 0.87, alpha: 1))!.draw(in: background, angle: -45)
        draw(in: rect.insetBy(dx: rect.width * 0.07, dy: rect.height * 0.07), color: NSColor(red: 0.96, green: 0.92, blue: 1, alpha: 1))
    }

    static func png(pixels: Int, appIcon: Bool) -> Data {
        let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels,
            bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
            colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
        let rect = NSRect(x: 0, y: 0, width: pixels, height: pixels)
        if appIcon { drawAppIcon(in: rect) }
        else { draw(in: rect, color: NSColor(red: 0.59, green: 0.36, blue: 0.91, alpha: 1)) }
        NSGraphicsContext.restoreGraphicsState()
        return bitmap.representation(using: .png, properties: [:])!
    }
}

private extension AffineTransform {
    func scaled(x: CGFloat, y: CGFloat) -> AffineTransform {
        var result = self; result.scale(x: x, y: y); return result
    }
}
