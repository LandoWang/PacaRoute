import AppKit
import Foundation
import BridgeCore

@main struct MenuChecks {
    @MainActor static func main() async throws {
        _ = NSApplication.shared
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("agentlink-menu-" + UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: root) }
        let suite = "agentlink-menu-" + UUID().uuidString
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        let resources = URL(fileURLWithPath: CommandLine.arguments[1])
        let store = AppStore(defaults: defaults, makeRuntime: { try Runtime(root: root, resources: resources) })
        precondition(store.ready)
        let controller = StatusMenuController(store: store, showWindow: {})
        let events = NSEvent.EventTypeMask(rawValue: UInt64(controller.item.button!.sendAction(on: [.leftMouseUp, .rightMouseUp])))
        precondition(events.contains(.leftMouseUp) && events.contains(.rightMouseUp), "Both mouse buttons must open the menu")
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 200, height: 100), styleMask: [.titled, .closable], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false; window.close()
        controller.menuWillOpen(controller.menu)
        precondition(controller.item.isVisible, "Closing the window must not remove the menu item")
        precondition(controller.menu.items.contains { $0.title.hasPrefix("今天：") })
        let usage = controller.menu.items.firstIndex { $0.title == "用量" }!
        controller.menu.performActionForItem(at: usage)
        precondition(store.page == .usage, "Shortcut must choose the requested page")
        let source = ModelSource(id: "fixture", name: "Fixture", endpoint: "https://example.openai.azure.com", deployments: ["fixture"])
        try store.saveSource(source, key: "")
        // A loopback socket reserves a free port, then releases it just before startup.
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        var addr = sockaddr_in(); addr.sin_family = sa_family_t(AF_INET); addr.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        addr.sin_addr.s_addr = inet_addr("127.0.0.1")
        withUnsafePointer(to: &addr) { p in _ = p.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) } }
        var size = socklen_t(MemoryLayout<sockaddr_in>.size)
        withUnsafeMutablePointer(to: &addr) { p in _ = p.withMemoryRebound(to: sockaddr.self, capacity: 1) { getsockname(fd, $0, &size) } }
        close(fd); store.portText = String(UInt16(bigEndian: addr.sin_port))
        controller.menuWillOpen(controller.menu)
        controller.menu.performActionForItem(at: controller.menu.items.firstIndex { $0.title == "启动服务" }!)
        for _ in 0..<150 where !store.running || store.busy != nil { try await Task.sleep(nanoseconds: 100_000_000) }
        precondition(store.running && store.busy == nil, "Menu start must work with the main window closed")
        controller.menuWillOpen(controller.menu)
        controller.menu.performActionForItem(at: controller.menu.items.firstIndex { $0.title == "停止服务" }!)
        for _ in 0..<100 where store.hasRunningChildren { try await Task.sleep(nanoseconds: 100_000_000) }
        precondition(!store.hasRunningChildren, "Menu stop must end the owned child")
        store.shutdown()
        NSStatusBar.system.removeStatusItem(controller.item)
        print("PASS: native left/right mouse masks, persistent status item, usage shortcut, real AOT start/stop with main window closed")
    }
}
