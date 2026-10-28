import AppKit
import Combine
import BridgeCore

/// Native status item supports left/right click without activating the main window.
@MainActor final class StatusMenuController: NSObject, NSMenuDelegate {
    let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    let menu = NSMenu()
    private let store: AppStore
    private let showWindow: () -> Void
    private var observer: AnyCancellable?
    init(store: AppStore, showWindow: @escaping () -> Void) {
        self.store = store; self.showWindow = showWindow
        super.init()
        item.button?.image = NSImage(systemSymbolName: "point.3.connected.trianglepath.dotted", accessibilityDescription: "AgentLink")
        item.button?.toolTip = "AgentLink · 左键或右键打开快捷菜单"
        item.button?.target = self; item.button?.action = #selector(openMenu)
        item.button?.sendAction(on: [.leftMouseUp, .rightMouseUp])
        menu.delegate = self
        observer = store.$serviceState.sink { [weak self] state in
            self?.item.button?.toolTip = "AgentLink · " + (state == "running" ? "运行中" : "已停止 / 正在切换")
        }
    }
    @objc private func openMenu() {
        guard let button = item.button else { return }
        menu.popUp(positioning: nil, at: NSPoint(x: 0, y: button.bounds.height + 5), in: button)
    }
    func menuWillOpen(_ menu: NSMenu) {
        menu.removeAllItems()
        add("AgentLink · \(store.serviceLabel) · :\(store.appliedPort)", action: nil)
        let today = UsageTotals(store.activity.filter { Calendar.current.isDateInToday($0.timestamp) })
        add("今天：\(today.requests) 次 · \((today.input + today.output).formatted()) token", action: nil)
        menu.addItem(.separator())
        add(store.running ? "停止服务" : "启动服务", action: #selector(toggleService),
            enabled: store.ready && store.busy == nil && !store.signingIn && (store.running || store.canStart) && ["running", "stopped"].contains(store.serviceState))
        add("刷新状态与用量", action: #selector(refresh), enabled: store.ready && store.busy == nil)
        menu.addItem(.separator())
        for page in [Page.overview, .account, .sources, .proxies, .clients, .usage, .errors, .network] {
            let title = page == .overview ? "打开 AgentLink" : page.rawValue
            let row = add(title, action: #selector(navigate)); row.representedObject = page.rawValue
        }
        menu.addItem(.separator())
        add("退出 AgentLink", action: #selector(quit))
    }
    @discardableResult private func add(_ title: String, action: Selector?, enabled: Bool = true) -> NSMenuItem {
        let row = NSMenuItem(title: title, action: action, keyEquivalent: "")
        row.target = self; row.isEnabled = action != nil && enabled; menu.addItem(row); menu.autoenablesItems = false
        return row
    }
    @objc private func toggleService() { if store.running { store.stopService() } else { store.startService() } }
    @objc private func refresh() { Task { await store.refresh() } }
    @objc private func navigate(_ sender: NSMenuItem) {
        if let raw = sender.representedObject as? String, let page = Page(rawValue: raw) { store.page = page }
        showWindow()
    }
    @objc private func quit() { NSApp.terminate(nil) }
}
