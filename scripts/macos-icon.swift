import AppKit

@main struct MacOSIconGenerator {
    static func main() throws {
        let output = URL(fileURLWithPath: CommandLine.arguments[1])
        let destination = output.appendingPathComponent("AppIcon.iconset")
        try FileManager.default.createDirectory(at: destination, withIntermediateDirectories: true)
        for size in [16, 32, 128, 256, 512] {
            for scale in [1, 2] {
                let filename = "icon_\(size)x\(size)\(scale == 2 ? "@2x" : "").png"
                try AgentLinkMark.png(pixels: size * scale, appIcon: true)
                    .write(to: destination.appendingPathComponent(filename))
            }
        }
        for pixels in [18, 36, 24, 48, 32, 64] {
            try AgentLinkMark.png(pixels: pixels, appIcon: false).write(to: output.appendingPathComponent("AgentLink-mark-\(pixels).png"))
        }
        try AgentLinkMark.png(pixels: 512, appIcon: true).write(to: output.appendingPathComponent("AgentLink-icon-preview.png"))
        try AgentLinkMark.png(pixels: 512, appIcon: false).write(to: output.appendingPathComponent("AgentLink-mark.png"))
    }
}
