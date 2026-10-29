import SwiftUI

/// Aurora's neon colors are reserved for dark surfaces. The light appearance uses
/// deeper inks from the same palette so controls and small labels remain readable.
enum AuroraTheme {
    static let mint = Color(red: 103 / 255, green: 235 / 255, blue: 198 / 255)
    static let violet = Color(red: 177 / 255, green: 150 / 255, blue: 255 / 255)
    static let deep = Color(red: 12 / 255, green: 17 / 255, blue: 29 / 255)
    static let heroText = Color(red: 220 / 255, green: 255 / 255, blue: 245 / 255)
    static let heroSecondary = Color(red: 169 / 255, green: 190 / 255, blue: 219 / 255)
    static let accent = adaptive("accent", light: 0x176B59, dark: 0x67EBC6)
    static let accentText = adaptive("accent-text", light: 0xFFFFFF, dark: 0x0C111D)
    static let violetInk = adaptive("violet", light: 0x6951A8, dark: 0xB196FF)
    static let canvas = adaptive("canvas", light: 0xF3F6F8, dark: 0x0C111D)
    static let sidebar = adaptive("sidebar", light: 0xEDF0F7, dark: 0x101727)
    static let surface = adaptive("surface", light: 0xFFFFFF, dark: 0x151D2A)
    static let border = adaptive("border", light: 0xDCE1EB, dark: 0x303C50)
    static let softMint = adaptive("soft-mint", light: 0xE3F4ED, dark: 0x1B3636)
    static let softViolet = adaptive("soft-violet", light: 0xEEEAF8, dark: 0x28243F)
    static let hero = LinearGradient(colors: [color(0x15252A), color(0x1B3036), color(0x28233E)],
                                     startPoint: .topLeading, endPoint: .bottomTrailing)

    private static func color(_ hex: UInt32) -> Color { Color(nsColor: ink(hex)) }
    private static func ink(_ hex: UInt32) -> NSColor {
        NSColor(srgbRed: CGFloat((hex >> 16) & 255) / 255,
                green: CGFloat((hex >> 8) & 255) / 255, blue: CGFloat(hex & 255) / 255, alpha: 1)
    }
    private static func adaptive(_ name: String, light: UInt32, dark: UInt32) -> Color {
        Color(nsColor: NSColor(name: NSColor.Name("Aurora.\(name)")) { appearance in
            ink(appearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua ? dark : light)
        })
    }
}

extension View {
    /// Keep native button behavior, focus and disabled treatment with readable ink.
    func auroraProminent(onDarkSurface: Bool = false) -> some View {
        modifier(AuroraProminentModifier(onDarkSurface: onDarkSurface))
    }
}

private struct AuroraProminentModifier: ViewModifier {
    @Environment(\.isEnabled) private var enabled
    @Environment(\.controlActiveState) private var activeState
    let onDarkSurface: Bool

    private var label: Color {
        if !enabled { return .secondary }
        // Native prominent buttons lose their tint when the window is inactive.
        if activeState == .inactive { return onDarkSurface ? AuroraTheme.heroText : .primary }
        return onDarkSurface ? AuroraTheme.deep : AuroraTheme.accentText
    }
    func body(content: Content) -> some View {
        content.buttonStyle(.borderedProminent)
            .tint(onDarkSurface ? AuroraTheme.mint : AuroraTheme.accent).foregroundStyle(label)
    }
}

struct AuroraNavigationRow: View {
    let page: Page
    let selected: Bool
    let action: () -> Void
    @State private var hovering = false

    var body: some View {
        Button(action: action) {
            HStack(spacing: 11) {
                Image(systemName: page.icon).frame(width: 20)
                    .foregroundStyle(selected ? AuroraTheme.violetInk : .secondary)
                Text(page.rawValue).fontWeight(selected ? .semibold : .regular)
                Spacer(minLength: 0)
                if selected { Capsule().fill(AuroraTheme.violetInk).frame(width: 3, height: 15) }
            }
            .font(.system(size: 13)).padding(.horizontal, 12).padding(.vertical, 10)
            .foregroundStyle(selected ? AuroraTheme.violetInk : .primary)
            .background(selected ? AuroraTheme.softViolet : hovering ? AuroraTheme.surface.opacity(0.65) : .clear,
                        in: RoundedRectangle(cornerRadius: 9))
            .contentShape(RoundedRectangle(cornerRadius: 9))
        }
        .buttonStyle(.plain).onHover { hovering = $0 }
        .accessibilityAddTraits(selected ? .isSelected : [])
    }
}
