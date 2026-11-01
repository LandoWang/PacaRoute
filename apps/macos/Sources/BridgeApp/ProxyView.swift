import SwiftUI
import BridgeCore

struct SourceProxyPicker: View {
    @EnvironmentObject var store: AppStore
    let sourceID: String
    var body: some View {
        HStack(spacing: 8) {
            Label("Proxy", systemImage: "network")
                .font(.caption).foregroundStyle(.secondary)
            Picker("Proxy", selection: Binding(get: { store.proxies.sourceProxy(sourceID) }, set: { store.bindProxy($0, source: sourceID) })) {
                ForEach(store.proxies.choices) { Text($0.name).tag($0.id) }
            }
            .labelsHidden().pickerStyle(.menu).controlSize(.small)
            .frame(width: 190).help(store.proxyName(for: sourceID))
            .disabled(store.signingIn || store.busy != nil)
            Spacer(minLength: 0)
        }.frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct ProxyView: View {
    @EnvironmentObject var store: AppStore
    @State private var editing: ProxyProfile?
    @State private var adding = false
    var body: some View {
        Card {
            Label("来源 × Proxy", systemImage: "network").font(.headline)
            Text("在这里管理可复用的代理，再到“来源”选择。不同来源可以同时使用不同代理；修改后下次启动服务生效。")
            HStack {
                ForEach(ProxyProfile.builtins) { profile in Label(profile.name, systemImage: profile.id == "direct" ? "arrow.right" : "globe"); Spacer() }
            }.font(.callout).foregroundStyle(.secondary)
        }
        ForEach(store.proxies.profiles) { profile in
            Card {
                HStack {
                    Text(profile.name).font(.headline); Spacer()
                    Button("编辑") { editing = profile }
                    Button("删除", role: .destructive) {
                        do {
                            guard !store.proxies.bindings.values.contains(profile.id), store.proxies.accountProxyId != profile.id else {
                                throw AppFailure(message: "此 Proxy 仍被来源或账号操作使用，请先更换绑定。")
                            }
                            var next = store.proxies; next.profiles.removeAll { $0.id == profile.id }; try store.saveProxies(next)
                        } catch { store.showError(error) }
                    }
                }
                Text(profile.address ?? ProxyMode(rawValue: profile.mode)?.label ?? "").font(.system(.callout, design: .monospaced))
                let names = store.proxies.bindings.filter { $0.value == profile.id }.map { store.sourceName($0.key) }.sorted()
                Text(names.isEmpty ? "尚未绑定来源" : "来源：" + names.joined(separator: "、")).font(.caption).foregroundStyle(.secondary)
            }.disabled(locked)
        }
        Button("添加 Proxy", systemImage: "plus") { adding = true }.disabled(locked)
        Card {
            Text("账号登录与资源发现").font(.headline)
            Picker("使用 Proxy", selection: Binding(get: { store.proxies.accountProxyId }, set: { id in
                do { var next = store.proxies; next.accountProxyId = id; try store.saveProxies(next) } catch { store.showError(error) }
            })) {
                ForEach(store.proxies.choices) { Text($0.name).tag($0.id) }
            }.disabled(locked)
            Text("用于 device login、账号额度和 Azure 资源发现。模型查询、请求和令牌刷新遵循各来源的 Proxy。跟随系统会使用启动应用时的网络环境；直连会绕过代理。")
                .font(.caption).foregroundStyle(.secondary)
        }
        if locked { Text("请等待账号操作完成，再修改 Proxy。").font(.caption).foregroundStyle(.secondary) }
        Text("HTTP、HTTPS、SOCKS5 地址不包含用户名、密码或路径。此设置只影响 AgentLink，不修改 macOS 系统代理。")
            .font(.caption).foregroundStyle(.secondary)
            .sheet(isPresented: $adding) { ProxyEditor(original: nil).environmentObject(store) }
            .sheet(item: $editing) { ProxyEditor(original: $0).environmentObject(store) }
    }
    private var locked: Bool { store.signingIn || store.busy != nil }
}

struct ProxyEditor: View {
    @EnvironmentObject var store: AppStore
    @Environment(\.dismiss) private var dismiss
    let original: ProxyProfile?
    @State private var name = ""
    @State private var address = "http://127.0.0.1:7890"
    @State private var error: String?
    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Text(original == nil ? "添加 Proxy" : "编辑 Proxy").font(.title2).bold()
            TextField("名称", text: $name)
            TextField("代理地址", text: $address)
            Text("例如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080").font(.caption).foregroundStyle(.secondary)
            if let error { Text(error).foregroundStyle(.red).font(.caption) }
            HStack {
                Button("取消") { dismiss() }.keyboardShortcut(.cancelAction); Spacer()
                Button("保存") {
                    do {
                        let profile = ProxyProfile(id: original?.id ?? "proxy-" + UUID().uuidString.prefix(8).lowercased(), name: name, mode: "custom", address: address)
                        var next = store.proxies; next.profiles.removeAll { $0.id == profile.id }; next.profiles.append(try profile.validated())
                        try store.saveProxies(next); dismiss()
                    } catch { self.error = error.localizedDescription }
                }.auroraProminent().keyboardShortcut(.defaultAction)
            }
        }.padding(28).frame(width: 520).textFieldStyle(.roundedBorder)
            .onAppear { if let original { name = original.name; address = original.address ?? address } }
    }
}
