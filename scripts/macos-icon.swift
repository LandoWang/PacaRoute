import AppKit

let destination = URL(fileURLWithPath: CommandLine.arguments[1]).appendingPathComponent("AppIcon.iconset")
try FileManager.default.createDirectory(at: destination, withIntermediateDirectories: true)
for size in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let pixels = size * scale
        let image = NSImage(size: NSSize(width: pixels, height: pixels))
        image.lockFocus()
        let rect = NSRect(x: Double(pixels) * 0.06, y: Double(pixels) * 0.06, width: Double(pixels) * 0.88, height: Double(pixels) * 0.88)
        let path = NSBezierPath(roundedRect: rect, xRadius: Double(pixels) * 0.21, yRadius: Double(pixels) * 0.21)
        NSGradient(starting: NSColor(red: 0.10, green: 0.31, blue: 0.29, alpha: 1), ending: NSColor(red: 0.18, green: 0.61, blue: 0.50, alpha: 1))!.draw(in: path, angle: -45)
        let points = [(0.29, 0.29), (0.50, 0.70), (0.73, 0.37)]
        let line = NSBezierPath()
        line.move(to: NSPoint(x: points[0].0 * Double(pixels), y: points[0].1 * Double(pixels)))
        for point in points.dropFirst() { line.line(to: NSPoint(x: point.0 * Double(pixels), y: point.1 * Double(pixels))) }
        line.lineWidth = Double(pixels) * 0.055
        line.lineCapStyle = .round
        NSColor.white.withAlphaComponent(0.9).setStroke(); line.stroke()
        for point in points {
            NSColor.white.setFill()
            NSBezierPath(ovalIn: NSRect(x: point.0 * Double(pixels) - Double(pixels) * 0.06, y: point.1 * Double(pixels) - Double(pixels) * 0.06, width: Double(pixels) * 0.12, height: Double(pixels) * 0.12)).fill()
        }
        image.unlockFocus()
        let bitmap = NSBitmapImageRep(data: image.tiffRepresentation!)!
        let filename = "icon_\(size)x\(size)\(scale == 2 ? "@2x" : "").png"
        try bitmap.representation(using: .png, properties: [:])!.write(to: destination.appendingPathComponent(filename))
    }
}
