import SwiftUI
import BridgeCore

struct AccountView: View {
    @EnvironmentObject var store: AppStore
    @State private var adding = false
    var body: some View {
        HStack {
            Text("账号 → 资源来源 → 模型").font(.headline)
            Spacer()
            Button("添加 GitHub 登录", systemImage: "person.badge.plus") { Task { await store.addGitHubLogin() } }
                .disabled(store.signingIn || store.busy != nil || !store.ready)
            Button("添加 Azure 账号", systemImage: "plus") { adding = true }
                .disabled(store.signingIn || !store.ready)
        }
        Text("可同时保存多个 GitHub 和 Azure 账号。Azure 账号下可添加多个 AOAI 资源，在 Agent 页分别选择来源 × 模型。")
            .font(.callout).foregroundStyle(.secondary)
        if store.signingIn {
            Card {
                Text("浏览器授权").font(.headline)
                if let code = store.deviceCode {
                    Text(code).font(.system(size: 30, weight: .medium, design: .monospaced)).textSelection(.enabled)
                    HStack {
                        Button("复制验证码") { NSPasteboard.general.clearContents(); NSPasteboard.general.setString(code, forType: .string) }
                        Button("打开授权页") { if let url = store.deviceURL { NSWorkspace.shared.open(url) } }
                    }
                } else { ProgressView("正在请求验证码…") }
                Button("取消登录") { store.cancelSignIn() }
            }
        }
        ForEach(store.accounts) { account in AccountCard(account: account) }
        if store.serviceState != "stopped" {
            Text("账号与来源支持热更新，当前请求会继续完成。")
                .font(.caption).foregroundStyle(.secondary)
        }
        Text("GitHub 凭据按账号加密保存；新增 Azure 账号使用独立 CLI 登录目录；API Key 存在 Keychain。Azure 登录需要本机安装 Azure CLI。")
            .font(.caption).foregroundStyle(.secondary)
            .sheet(isPresented: $adding) { AccountEditor().environmentObject(store) }
    }
}

struct AccountCard: View {
    @EnvironmentObject var store: AppStore
    let account: AccountProfile
    @State private var importing = false
    @State private var manual = false
    @State private var editing: ModelSource?
    @State private var showQuota = false
    private var mutable: Bool { store.busy == nil && !store.signingIn }
    var body: some View {
        Card {
            HStack {
                Label(account.name, systemImage: account.kind == "github" ? "person.crop.circle" : "cloud").font(.title3).bold()
                Spacer()
                Text(account.kind == "github" ? "GitHub Copilot" : account.kind == "azure" ? "Azure OpenAI" : "API Key").font(.caption).foregroundStyle(.secondary)
            }
            if let login = store.accountStates[account.id]?.login ?? (account.id == "copilot" ? store.login : nil) {
                Text(login).textSelection(.enabled)
            }
            Text(store.checkingAccounts ? "正在恢复已保存的登录…" : store.accountLoggedIn(account) ? "已保存登录 · 重开应用自动恢复" : account.kind == "azure" ? "刷新可检查登录状态；API Key 来源不需要 CLI 登录。" : "待登录")
                .font(.caption).foregroundStyle(.secondary)
            HStack {
                if account.id != "azure-local" && account.kind != "api-key" {
                    if !store.accountLoggedIn(account) {
                        Button("登录") { Task { await store.signInAccount(account) } }
                            .auroraProminent().disabled(!mutable || store.checkingAccounts)
                    } else {
                        Menu("账号操作") {
                            Button("重新授权…") { Task { await store.signInAccount(account, reauthorize: true) } }
                            Button("退出登录") { Task { await store.signOutAccount(account) } }
                        }.disabled(!mutable)
                    }
                }
                Button(account.kind == "github" ? "读取账号与额度" : "刷新登录状态") {
                    showQuota = account.kind == "github"
                    Task { await store.refreshAccount(account) }
                }.disabled(store.busy != nil || store.signingIn)
                Spacer()
                if !account.builtin {
                    Button("移除账号", role: .destructive) {
                        do { try store.removeAccount(account) } catch { store.showError(error) }
                    }.disabled(!mutable)
                }
            }
            if account.id == "azure-local" {
                Text("兼容已有 az login；此处不会退出或更换终端账号。添加新 Azure 账号可独立登录。")
                    .font(.caption).foregroundStyle(.secondary)
            }
            Divider()
            if account.id == "copilot" { SourceUsageSummary(sourceID: "copilot") }
            ForEach(store.accountSources(account)) { source in
                HStack {
                    VStack(alignment: .leading, spacing: 5) {
                        Text(source.name).fontWeight(.medium)
                        Text(source.kind == "copilot" ? "此账号的 Copilot 模型" : source.deployments.joined(separator: " · "))
                            .font(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    if source.kind == "azure-openai" { Button("编辑") { editing = source }.disabled(!mutable) }
                    Button("移除", role: .destructive) {
                        do { try store.removeSource(source) } catch { store.showError(error) }
                    }.disabled(!mutable)
                }
            }
            if account.kind == "azure" {
                HStack {
                    Button("从 Azure 选择 AOAI 资源", systemImage: "cloud") { importing = true }.disabled(!mutable)
                    Button("手动添加资源 / API Key") { manual = true }.disabled(!mutable)
                }
            } else if account.kind == "github" && account.id != "copilot" && store.accountSources(account).isEmpty && store.accountLoggedIn(account) {
                Button("添加 Copilot 来源") {
                    do { try store.saveSource(ModelSource(id: account.id, name: "Copilot", endpoint: "", deployments: [], accountId: account.id, kind: "copilot"), key: "") }
                    catch { store.showError(error) }
                }.disabled(!mutable)
            } else if account.kind == "github" && !store.accountLoggedIn(account) {
                Text("完成 GitHub 授权后会自动创建 Copilot 来源，无需手动添加。")
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
        .sheet(isPresented: $importing) { AzureResourcePicker(account: account).environmentObject(store) }
        .sheet(isPresented: $manual) { SourceEditor(original: nil, account: account).environmentObject(store) }
        .sheet(item: $editing) { SourceEditor(original: $0, account: account).environmentObject(store) }
        if showQuota { QuotaView(account: account) }
    }
}

struct AccountEditor: View {
    @EnvironmentObject var store: AppStore
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var kind = "azure"
    @State private var error: String?
    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Text("添加账号").font(.title2).bold()
            TextField("账号名称，例如工作 / 个人", text: $name).textFieldStyle(.roundedBorder)
            Text("Azure OpenAI / Foundry").foregroundStyle(.secondary)
            Text("保存后在账号卡片登录。Azure 账号可添加多个 AOAI 资源；API Key 也可在资源内单独保存。")
                .font(.caption).foregroundStyle(.secondary)
            if let error { Text(error).foregroundStyle(.red) }
            HStack {
                Button("取消") { dismiss() }.keyboardShortcut(.cancelAction)
                Spacer()
                Button("添加") {
                    do {
                        try store.saveAccount(AccountProfile(id: kind + "-" + UUID().uuidString.prefix(8).lowercased(), name: name, kind: kind))
                        dismiss()
                    } catch { self.error = error.localizedDescription }
                }.auroraProminent().keyboardShortcut(.defaultAction)
            }
        }.padding(28).frame(width: 520)
    }
}

struct AzureResourcePicker: View {
    @EnvironmentObject var store: AppStore
    @Environment(\.dismiss) private var dismiss
    let account: AccountProfile
    @State private var subscriptions: [AzureSubscription] = []
    @State private var resources: [AzureResource] = []
    @State private var subscription = ""
    @State private var resource = ""
    @State private var search = ""
    @State private var loading = false
    @State private var error: String?
    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text(account.name + " · 添加 AOAI 资源").font(.title2).bold()
            Picker("订阅", selection: $subscription) {
                Text("选择订阅").tag("")
                ForEach(subscriptions) { Text($0.name).tag($0.id) }
            }.disabled(loading)
                .onChange(of: subscription) { resources = []; resource = ""; Task { await loadResources() } }
            TextField("搜索资源名称", text: $search).textFieldStyle(.roundedBorder)
            List(selection: $resource) {
                ForEach(resources.filter { search.isEmpty || $0.name.localizedCaseInsensitiveContains(search) }) { item in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(item.name)
                        Text(item.endpoint).font(.caption).foregroundStyle(.secondary)
                    }.tag(item.id)
                }
            }.frame(height: 240).disabled(loading)
            Text("将读取选中资源的 OpenAI deployments。订阅可见不代表拥有推理权限；添加后仍需资源授权。")
                .font(.caption).foregroundStyle(.secondary)
            if loading { ProgressView("正在读取 Azure…").controlSize(.small) }
            if let error { Text(error).font(.caption).foregroundStyle(.red).textSelection(.enabled) }
            HStack {
                Button("取消") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("刷新订阅") { Task { await loadSubscriptions() } }.disabled(loading)
                Spacer()
                Button("添加资源及模型") { Task { await importResource() } }
                    .auroraProminent().disabled(loading || resource.isEmpty)
            }
        }.padding(28).frame(width: 660)
        .task { await loadSubscriptions() }
    }
    private func loadSubscriptions() async {
        loading = true; error = nil
        defer { loading = false }
        do {
            let reply = try await store.execute(["desktop", "azure-subscriptions", "--account", account.id])
            subscriptions = reply.subscriptions ?? []; store.accountStates[account.id] = reply
            if subscriptions.isEmpty { error = "此账号没有可用订阅，请先登录。" }
        } catch { self.error = error.localizedDescription }
    }
    private func loadResources() async {
        guard !subscription.isEmpty else { return }
        loading = true; error = nil
        defer { loading = false }
        do {
            let reply = try await store.execute(["desktop", "azure-resources", "--account", account.id, "--subscription", subscription])
            resources = reply.resources ?? []
            if resources.isEmpty { error = "此订阅没有可见的 OpenAI / AI Services 资源。" }
        } catch { self.error = error.localizedDescription }
    }
    private func importResource() async {
        guard let item = resources.first(where: { $0.id == resource }) else { return }
        loading = true; error = nil
        defer { loading = false }
        do {
            let reply = try await store.execute(["desktop", "azure-deployments", "--account", account.id,
                "--subscription", subscription, "--resource", item.id])
            let deployments = reply.models?.map(\.id) ?? []
            guard !deployments.isEmpty else { throw AppFailure(message: "资源中没有可用的 OpenAI deployment。") }
            let candidate = try ModelSource(id: "azure-" + UUID().uuidString.prefix(8).lowercased(), name: item.name,
                endpoint: item.endpoint, subscriptionId: subscription, deployments: deployments, accountId: account.id).validated()
            guard !store.sources.contains(where: { ($0.accountId ?? "azure-local") == account.id && $0.endpoint == candidate.endpoint }) else {
                throw AppFailure(message: "此账号已添加这个资源，可在资源卡片编辑模型。")
            }
            try store.saveSource(candidate, key: ""); dismiss()
        } catch { self.error = error.localizedDescription }
    }
}
