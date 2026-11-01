import SwiftUI
import WebKit

struct GatewayAdminView: View {
    @EnvironmentObject var store: AppStore
    var body: some View {
        if store.running, let runtime = store.runtime {
            GatewayWebView(port: store.appliedPort, root: runtime.root)
                .frame(minHeight: 680)
                .clipShape(RoundedRectangle(cornerRadius: 16))
        } else {
            VStack(alignment: .leading, spacing: 16) {
                Text("启动 PacaRoute 后管理 Client 路由").font(.headline)
                Text("每个 Client 有独立密钥，可为 Codex 和 Claude Code 指定来源或来源池。")
                    .foregroundStyle(.secondary)
                Button("启动服务") { store.startService() }.disabled(!store.ready || store.busy != nil)
            }.padding(24)
        }
    }
}

private struct GatewayWebView: NSViewRepresentable {
    let port: Int
    let root: URL
    func makeCoordinator() -> Coordinator { Coordinator(port: port) }
    func makeNSView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        let view = WKWebView(frame: .zero, configuration: configuration)
        view.navigationDelegate = context.coordinator
        let address = "http://127.0.0.1:\(port)"
        if let key = try? String(contentsOf: root.appendingPathComponent("admin.key"), encoding: .utf8),
           let cookie = HTTPCookie(properties: [.domain: "127.0.0.1", .path: "/api", .name: "pacaroute_admin",
               .value: key, HTTPCookiePropertyKey("HttpOnly"): "TRUE"]) {
            configuration.websiteDataStore.httpCookieStore.setCookie(cookie) {
                view.load(URLRequest(url: URL(string: address)!))
            }
        } else { view.load(URLRequest(url: URL(string: address)!)) }
        return view
    }
    func updateNSView(_ view: WKWebView, context: Context) {}
    final class Coordinator: NSObject, WKNavigationDelegate {
        let port: Int
        init(port: Int) { self.port = port }
        func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                     decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
            guard let url = action.request.url else { decisionHandler(.cancel); return }
            decisionHandler(url.scheme == "http" && url.host == "127.0.0.1" && url.port == port ? .allow : .cancel)
        }
    }
}
