import SwiftUI

@MainActor final class AppDelegate: NSObject, NSApplicationDelegate {
    weak var store: AppStore?
    var statusMenu: StatusMenuController?
    func installMenu(store: AppStore, show: @escaping () -> Void) {
        self.store = store
        if statusMenu == nil { statusMenu = StatusMenuController(store: store, showWindow: show) }
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard let store, store.hasRunningChildren else { return .terminateNow }
        store.shutdown()
        Task { @MainActor in
            for _ in 0..<60 where store.hasRunningChildren { try? await Task.sleep(nanoseconds: 100_000_000) }
            sender.reply(toApplicationShouldTerminate: !store.hasRunningChildren)
        }
        return .terminateLater
    }
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag { sender.windows.first?.makeKeyAndOrderFront(nil) }
        return true
    }
}

@main struct CopilotBridgeApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var delegate
    @StateObject private var store = AppStore()
    @Environment(\.openWindow) private var openWindow

    var body: some Scene {
        Window("AgentLink", id: "main") {
            RootView().environmentObject(store).onAppear {
                delegate.installMenu(store: store) { openWindow(id: "main"); NSApp.activate(ignoringOtherApps: true) }
            }
        }.defaultSize(width: 1060, height: 760)
            .commands {
                CommandGroup(replacing: .newItem) {}
                CommandMenu("Bridge") {
                    Button("显示主窗口") { openWindow(id: "main"); NSApp.activate(ignoringOtherApps: true) }
                    Button(store.running ? "停止服务" : "启动服务") { if store.running { store.stopService() } else { store.startService() } }
                        .disabled(!store.canStart)
                }
            }
    }
}
