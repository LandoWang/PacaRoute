import SwiftUI
import BridgeCore

struct SourcesView: View {
    @EnvironmentObject var store: AppStore
    @State private var editing: ModelSource?
    @State private var creating = false
    var body: some View {
        Card {
            HStack {
                Label("GitHub Copilot", systemImage: "person.crop.circle").font(.headline)
                Spacer()
                Text(store.loggedIn ? "已登录" : "待登录").foregroundStyle(.secondary)
                Button("管理登录") { store.page = .account }
            }
            Text("模型与 Copilot 额度由 GitHub 账号提供。").font(.caption).foregroundStyle(.secondary)
            SourceUsageSummary(sourceID: "copilot")
        }
        ForEach(store.sources) { source in
            Card {
                HStack {
                    Label(source.name, systemImage: "cloud").font(.headline)
                    Spacer()
                    Text("Azure OpenAI").font(.caption).foregroundStyle(.secondary)
                    Button("编辑") { editing = source }.disabled(store.serviceState != "stopped")
                }
                Text(source.endpoint).font(.system(.caption, design: .monospaced)).textSelection(.enabled)
                Text(source.deployments.joined(separator: " · ")).font(.callout)
                Text(source.authMode == "azure-cli" ? "使用 Azure CLI 登录身份，令牌自动刷新。" : "API Key 保存在本机 Keychain。")
                    .font(.caption).foregroundStyle(.secondary)
                SourceUsageSummary(sourceID: source.id)
            }
        }
        Button("添加 Azure OpenAI 来源", systemImage: "plus") { creating = true }
            .disabled(store.serviceState != "stopped" || !store.ready)
        if store.serviceState != "stopped" {
            Text("停止 Bridge 后可以编辑来源；已有请求使用启动时的来源配置。").font(.caption).foregroundStyle(.secondary)
        }
        Text("来源管理凭据、endpoint 与模型部署；在 Harness 页为 Codex、Claude Code 分别选择来源。Azure 展示本机用量，账单与配额请以 Azure 为准。")
            .font(.caption).foregroundStyle(.secondary)
            .sheet(item: $editing) { source in SourceEditor(original: source).environmentObject(store) }
            .sheet(isPresented: $creating) { SourceEditor(original: nil).environmentObject(store) }
    }
}

struct SourceEditor: View {
    @EnvironmentObject var store: AppStore
    @Environment(\.dismiss) private var dismiss
    let original: ModelSource?
    @State private var sourceID = ""
    @State private var name = ""
    @State private var endpoint = ""
    @State private var authMode = "azure-cli"
    @State private var subscription = ""
    @State private var deployments = ""
    @State private var key = ""
    @State private var error: String?
    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text(original == nil ? "添加 Azure OpenAI 来源" : "编辑来源").font(.title2).bold()
            Form {
                TextField("名称", text: $name)
                TextField("来源 ID", text: $sourceID).disabled(original != nil)
                TextField("资源 Endpoint", text: $endpoint)
                Picker("认证方式", selection: $authMode) {
                    Text("Azure CLI · az login").tag("azure-cli")
                    Text("API Key · Keychain").tag("api-key")
                }
                if authMode == "azure-cli" {
                    TextField("订阅 ID（可选）", text: $subscription)
                    Text("使用本机 Azure CLI 已登录的账号。身份过期时请在终端运行 az login。").font(.caption).foregroundStyle(.secondary)
                } else {
                    SecureField(original == nil ? "API Key" : "新 API Key（留空保留）", text: $key)
                    Text("密钥仅写入 Keychain，不会写入客户端配置。").font(.caption).foregroundStyle(.secondary)
                }
                Text("模型 Deployment（每行一个）")
                TextEditor(text: $deployments).font(.system(.body, design: .monospaced)).frame(height: 110)
            }.textFieldStyle(.roundedBorder)
            if let error { Text(error).font(.caption).foregroundStyle(.red) }
            HStack {
                Button("取消") { key = ""; dismiss() }.keyboardShortcut(.cancelAction)
                if let original {
                    Button("删除来源", role: .destructive) {
                        do { try store.removeSource(original); dismiss() } catch { self.error = error.localizedDescription }
                    }
                }
                Spacer()
                Button("保存") {
                    do {
                        if original == nil && store.sources.contains(where: { $0.id == sourceID }) {
                            throw AppFailure(message: "来源 ID 已存在，请使用其他 ID。")
                        }
                        let source = ModelSource(id: sourceID, name: name, endpoint: endpoint, authMode: authMode,
                            subscriptionId: subscription.isEmpty ? nil : subscription,
                            deployments: deployments.split(whereSeparator: \.isNewline).map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty })
                        try store.saveSource(source, key: key); key = ""; dismiss()
                    } catch { self.error = error.localizedDescription }
                }.buttonStyle(.borderedProminent).keyboardShortcut(.defaultAction)
            }
        }.padding(28).frame(width: 650)
        .onAppear {
            if let original {
                sourceID = original.id; name = original.name; endpoint = original.endpoint
                authMode = original.authMode; subscription = original.subscriptionId ?? ""
                deployments = original.deployments.joined(separator: "\n")
            } else { sourceID = "azure-" + UUID().uuidString.prefix(8).lowercased() }
        }
    }
}


struct SourceUsageSummary: View {
    @EnvironmentObject var store: AppStore
    let sourceID: String
    private var totals: UsageTotals {
        UsageTotals(store.activity.filter { ($0.source ?? "copilot") == sourceID && Calendar.current.isDateInToday($0.timestamp) })
    }
    var body: some View {
        Divider()
        HStack(spacing: 18) {
            VStack(alignment: .leading, spacing: 5) {
                Text("今天 · 本机用量").font(.caption).foregroundStyle(.secondary)
                Text("\(totals.requests.formatted()) 次请求 · 输入 \(totals.input.formatted()) · 输出 \(totals.output.formatted()) token")
                    .font(.callout).monospacedDigit()
            }
            Spacer()
            Button("查看来源用量") { store.usageSource = sourceID; store.page = .usage }
        }
    }
}
