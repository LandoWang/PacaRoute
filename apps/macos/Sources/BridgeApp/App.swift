import SwiftUI

@MainActor final class AppDelegate: NSObject, NSApplicationDelegate {
    weak var store: AppStore?
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
        Window("Copilot Bridge", id: "main") {
            RootView().environmentObject(store).onAppear { delegate.store = store }
        }.defaultSize(width: 1060, height: 760)
            .commands {
                CommandGroup(replacing: .newItem) {}
                CommandMenu("Bridge") {
                    Button("显示主窗口") { openWindow(id: "main"); NSApp.activate(ignoringOtherApps: true) }
                    Button(store.running ? "停止服务" : "启动服务") { if store.running { store.stopService() } else { store.startService() } }
                        .disabled(!store.loggedIn)
                }
            }
        MenuBarExtra("Copilot Bridge", systemImage: store.running ? "point.3.connected.trianglepath.dotted" : "point.3.filled.connected.trianglepath.dotted") {
            Text("Bridge · \(store.serviceLabel)")
            Button("打开 Copilot Bridge") { openWindow(id: "main"); NSApp.activate(ignoringOtherApps: true) }
            Divider()
            Button(store.running ? "停止服务" : "启动服务") { if store.running { store.stopService() } else { store.startService() } }
                .disabled(!store.loggedIn || store.signingIn || store.serviceState == "starting" || store.serviceState == "stopping")
            Divider()
            Button("退出") { NSApp.terminate(nil) }.keyboardShortcut("q")
        }
    }
}
