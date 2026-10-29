import SwiftUI
import Charts
import BridgeCore

private let accent = AuroraTheme.accent

struct RootView: View {
    @EnvironmentObject var store: AppStore
    @StateObject private var sessions = SessionsStore()
    var body: some View {
        NavigationSplitView {
            VStack(spacing: 0) {
                HStack(spacing: 10) {
                    Image(nsImage: AgentLinkMark.image(size: 34, colored: true)).renderingMode(.original)
                        .accessibilityLabel("AgentLink")
                    VStack(alignment: .leading, spacing: 2) {
                        Text("AgentLink").font(.headline)
                        Text("一个入口，连接你的工具").font(.caption2).foregroundStyle(.secondary)
                    }
                    Spacer()
                }.padding(.horizontal, 18).padding(.vertical, 24)
                ScrollView {
                    VStack(spacing: 4) {
                        ForEach(Page.allCases) { page in
                            AuroraNavigationRow(page: page, selected: store.page == page) { store.page = page }
                        }
                    }.padding(.horizontal, 12).padding(.bottom, 12)
                }
                Divider()
                HStack(spacing: 10) {
                    Image(systemName: store.loggedIn ? "person.crop.circle.fill" : "person.crop.circle")
                        .font(.title2).foregroundStyle(store.loggedIn ? accent : .secondary)
                    VStack(alignment: .leading, spacing: 3) {
                        Text("账号与模型来源").font(.caption).fontWeight(.medium)
                        Text("本地运行 · macOS").font(.caption2).foregroundStyle(.secondary)
                    }
                    Spacer()
                }.padding(18)
            }.background(AuroraTheme.sidebar).navigationSplitViewColumnWidth(min: 205, ideal: 220, max: 250)
        } detail: {
            ScrollView {
                VStack(alignment: .leading, spacing: 24) {
                    HStack {
                        VStack(alignment: .leading, spacing: 6) {
                            Text(store.page.rawValue).font(.system(size: 28, weight: .bold))
                            Text(subtitle).font(.subheadline).foregroundStyle(.secondary)
                        }
                        Spacer()
                        if let busy = store.busy { ProgressView().controlSize(.small); Text(busy).font(.caption).foregroundStyle(.secondary) }
                    }
                    if let error = store.runtimeError { EmptyCard(icon: "exclamationmark.triangle", title: "应用尚未就绪", text: error) }
                    switch store.page {
                    case .sources: SourcesView()
                    case .proxies: ProxyView()
                    case .overview: OverviewView()
                    case .account: AccountView()
                    case .clients: ClientsView()
                    case .sessions: SessionsView(store: sessions)
                    case .usage: UsageView()
                    case .errors: ErrorsView()
                    case .network: NetworkView()
                    }
                }.padding(30).frame(maxWidth: 1050, alignment: .leading).frame(maxWidth: .infinity)
            }.background(AuroraTheme.canvas)
        }
        .tint(accent)
        .frame(minWidth: 930, minHeight: 650)
        .toolbar {
            ToolbarItem(placement: .automatic) {
                HStack(spacing: 8) {
                    Circle().fill(store.running ? accent : Color.secondary.opacity(0.5)).frame(width: 7, height: 7)
                    Text(store.serviceLabel).font(.caption)
                    Text(":\(String(store.appliedPort))").font(.system(.caption, design: .monospaced)).foregroundStyle(.secondary)
                }
            }
            ToolbarItem(placement: .automatic) {
                Button { store.page = .network } label: { Label("服务配置", systemImage: "slider.horizontal.3").labelStyle(.titleAndIcon) }
                    .help("Bridge 服务配置：端口与服务启停")
            }
            ToolbarItem(placement: .automatic) {
                Button { Task { await store.refresh() } } label: { Image(systemName: "arrow.clockwise") }
                    .help("刷新本地状态").disabled(store.busy != nil || !store.ready)
            }
        }
        .sheet(item: $store.preview) { preview in ConfigurationSheet(plan: preview).environmentObject(store) }
        .alert("AgentLink", isPresented: Binding(get: { store.notice != nil }, set: { if !$0 { store.notice = nil } })) {
            Button("好", role: .cancel) { store.notice = nil }
        } message: { Text(store.notice ?? "") }
        .task { await store.refresh() }
    }

    private var subtitle: String {
        switch store.page {
        case .proxies: return "管理可复用代理，在来源中独立组合。"
        case .sources: return "管理 Copilot、Azure OpenAI 的连接与可用模型。"
        case .overview: return "将模型来源连接到你的开发工具。"
        case .account: return "通过 device login 连接 GitHub、Azure，并管理账号下的资源。"
        case .clients: return "每个 Agent 独立选择来源和默认模型。"
        case .sessions: return "浏览本地历史，理解任务进展、工具结果与未完成事项。"
        case .usage: return "了解本机请求、token 消耗与账号剩余额度。"
        case .errors: return "集中查看失败请求与应用操作错误。"
        case .network: return "管理本地服务、监听端口和出站代理。"
        }
    }
}

struct Card<Content: View>: View {
    @ViewBuilder var content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 18) { content }
            .padding(22).frame(maxWidth: .infinity, alignment: .leading)
            .background(AuroraTheme.surface, in: RoundedRectangle(cornerRadius: 16))
            .overlay(RoundedRectangle(cornerRadius: 16).stroke(AuroraTheme.border, lineWidth: 1))
    }
}

struct EmptyCard: View {
    let icon: String
    let title: String
    let text: String
    var body: some View {
        Card {
            HStack(alignment: .top, spacing: 16) {
                Image(systemName: icon).font(.title2).foregroundStyle(AuroraTheme.violetInk).frame(width: 32)
                VStack(alignment: .leading, spacing: 7) {
                    Text(title).font(.headline)
                    Text(text).font(.subheadline).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }
}

struct StatCard: View {
    let title: String
    let value: String
    let icon: String
    var tone: Color = AuroraTheme.violetInk
    var body: some View {
        Card {
            HStack { Text(title).font(.caption).foregroundStyle(.secondary); Spacer(); Image(systemName: icon).foregroundStyle(tone) }
            Text(value).font(.system(size: 28, weight: .semibold, design: .rounded)).monospacedDigit()
        }
    }
}

struct OverviewView: View {
    @EnvironmentObject var store: AppStore
    var body: some View {
        VStack(alignment: .leading, spacing: 22) {
            HStack(spacing: 24) {
                VStack(alignment: .leading, spacing: 12) {
                    Label { Text("AGENTLINK") } icon: { Image(nsImage: AgentLinkMark.image(size: 16)).renderingMode(.template) }
                        .font(.system(size: 11, weight: .bold, design: .monospaced)).tracking(2).foregroundStyle(AuroraTheme.mint)
                    Text(store.running ? "你的工具，已连接。" : "从一个简单的连接开始。")
                        .font(.system(size: 27, weight: .semibold))
                    Text(store.hasSource ? "启动本地服务，再为 Agent 选择来源和模型。" : "先添加账号，再连接模型来源，最后配置你的 Agent。")
                        .font(.subheadline).foregroundStyle(AuroraTheme.heroSecondary).fixedSize(horizontal: false, vertical: true)
                    HStack(spacing: 10) {
                        if !store.hasAccount {
                            Button("添加账号") { store.page = .account }.auroraProminent(onDarkSurface: true)
                        } else if !store.hasSource {
                            Button("添加来源") { store.page = .sources }.auroraProminent(onDarkSurface: true)
                        } else {
                            Button(store.running ? "停止 Bridge" : "启动 Bridge") {
                                if store.running { store.stopService() } else { store.startService() }
                            }.auroraProminent(onDarkSurface: true)
                                .disabled(store.serviceState == "starting" || store.serviceState == "stopping")
                            Button("配置 Agent") { store.page = .clients }.buttonStyle(.bordered).tint(.white)
                        }
                    }.padding(.top, 6)
                }
                Spacer(minLength: 0)
                Image(nsImage: AgentLinkMark.image(size: 86, colored: true)).renderingMode(.original).accessibilityHidden(true)
            }.padding(28).foregroundStyle(AuroraTheme.heroText).frame(maxWidth: .infinity, alignment: .leading)
                .background(AuroraTheme.hero, in: RoundedRectangle(cornerRadius: 20))
            let totals = UsageTotals(store.activity.filter { Calendar.current.isDateInToday($0.timestamp) })
            HStack(spacing: 14) {
                StatCard(title: "今日请求", value: totals.requests.formatted(), icon: "arrow.left.arrow.right")
                StatCard(title: "今日输出 token", value: totals.output.formatted(), icon: "text.alignleft")
                StatCard(title: "失败请求", value: totals.failures.formatted(), icon: "exclamationmark.circle", tone: .orange)
            }
            Card {
                Text("四步开始").font(.headline)
                step("1", "添加账号", "使用 device login 登录 GitHub 或 Azure，也可以添加使用 API Key 的 Azure 账号。", done: store.hasAccount, page: .account)
                Divider()
                step("2", "添加模型来源", "GitHub 登录后自动创建 Copilot 来源；Azure 账号下可添加多个 AOAI 资源及其模型部署。", done: store.hasSource, page: .sources)
                Divider()
                step("3", "启动 Bridge", "在服务配置中选择端口，点击启动即可应用。当前已保存端口：\(String(store.appliedPort))。", done: store.running, page: .network)
                Divider()
                step("4", "配置 Agent", "为 Codex、Claude Code 选择来源 × 模型，预览并应用连接配置。", done: store.clients.contains { $0.connected && !$0.drifted }, page: .clients)
            }
            Text("用量统计来自此应用启动的 Bridge；本地聊天历史可在「会话」页按需读取。")
                .font(.caption).foregroundStyle(.secondary)
        }
    }
    private func step(_ number: String, _ title: String, _ subtitle: String, done: Bool, page: Page) -> some View {
        HStack(spacing: 14) {
            ZStack {
                Circle().fill(done ? AuroraTheme.softMint : AuroraTheme.softViolet).frame(width: 30, height: 30)
                if done { Image(systemName: "checkmark").foregroundStyle(accent) }
                else { Text(number).font(.system(.caption, design: .monospaced)).foregroundStyle(AuroraTheme.violetInk) }
            }
            VStack(alignment: .leading, spacing: 4) {
                Text(title).font(.subheadline).fontWeight(.medium)
                Text(subtitle).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            if page != .overview { Button("打开") { store.page = page }.buttonStyle(.borderless) }
        }
    }
}

struct ClientsView: View {
    @EnvironmentObject var store: AppStore
    var body: some View {
        HStack {
            Label("配置作用于全局默认值", systemImage: "doc.text").foregroundStyle(.secondary).font(.subheadline)
            Spacer()
            Button("刷新可用模型") { Task { await store.refreshModels() } }
                .disabled(!store.ready || store.busy != nil || store.signingIn)
        }
        ForEach(["codex", "claude-code"], id: \.self) { id in
            ClientCard(clientID: id)
        }
        Text("应用配置后，请新建客户端会话。会话参数或其他配置层仍可能覆盖默认模型。原有的其他 provider、权限、插件和行为设置会保留。")
            .font(.caption).foregroundStyle(.secondary)
    }
}

struct ClientCard: View {
    @EnvironmentObject var store: AppStore
    let clientID: String
    private var state: ClientState? { store.clients.first { $0.id == clientID } }
    private var selection: Binding<String> { clientID == "codex" ? $store.selectedCodex : $store.selectedClaude }
    private var sourceSelection: Binding<String> {
        Binding(get: { clientID == "codex" ? store.codexSource : store.claudeSource },
                set: { store.selectSource($0, client: clientID) })
    }
    private var available: [BridgeModel] {
        store.availableModels(sourceSelection.wrappedValue).filter { clientID == "codex" ? $0.codex : $0.claude }
    }
    var body: some View {
        Card {
            HStack(spacing: 14) {
                Image(systemName: clientID == "codex" ? "terminal" : "asterisk").font(.title).foregroundStyle(AuroraTheme.violetInk)
                VStack(alignment: .leading, spacing: 5) {
                    Text(clientID == "codex" ? "Codex" : "Claude Code").font(.title3).fontWeight(.semibold)
                    Text(state?.path ?? "正在读取配置路径…").font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
                }
                Spacer()
                Text(state?.drifted == true ? "需要更新连接" : (state?.connected == true ? "已连接" : "未连接"))
                    .font(.caption).padding(.horizontal, 10).padding(.vertical, 5)
                    .background(AuroraTheme.softMint, in: Capsule()).foregroundStyle(accent)
            }
            Divider()
            HStack(spacing: 20) {
                Text("来源").font(.subheadline)
                Picker("来源", selection: sourceSelection) {
                    Text("GitHub Copilot").tag("copilot")
                    ForEach(store.sources) { source in Text(store.sourceName(source.id)).tag(source.id) }
                    if sourceSelection.wrappedValue != "copilot" && !store.sources.contains(where: { $0.id == sourceSelection.wrappedValue }) {
                        Text("来源已移除 · " + sourceSelection.wrappedValue).tag(sourceSelection.wrappedValue)
                    }
                }.labelsHidden()
            }
            HStack(spacing: 20) {
                Text("默认模型").font(.subheadline)
                Picker("默认模型", selection: selection) {
                    Text(store.isCopilot(sourceSelection.wrappedValue) ? "选择模型 / 保留当前默认值" : "选择 Deployment").tag("")
                    if !selection.wrappedValue.isEmpty && !available.contains(where: { $0.id == selection.wrappedValue }) {
                        Text(selection.wrappedValue + "（当前配置）").tag(selection.wrappedValue)
                    }
                    ForEach(available) { model in Text(model.name + " · " + model.id).tag(model.id) }
                }.labelsHidden().frame(maxWidth: .infinity)
            }
            if store.checkingAccounts {
                Text("正在恢复已保存的登录…").font(.caption).foregroundStyle(.secondary)
            } else if store.loadingModelSources.contains(sourceSelection.wrappedValue) {
                ProgressView("正在更新可用模型…").controlSize(.small)
            } else if let error = store.modelErrors[sourceSelection.wrappedValue] {
                Text(error).font(.caption).foregroundStyle(.secondary)
            } else if store.isCopilot(sourceSelection.wrappedValue) && available.isEmpty {
                Text(store.modelAccount(sourceSelection.wrappedValue).map { store.accountLoggedIn($0) } == true
                     ? "暂无可用模型，可点击“刷新可用模型”重试，无需重新登录。"
                     : "此来源的账号尚未登录，请到账号页登录。")
                    .font(.caption).foregroundStyle(.secondary)
            }
            if !store.isCopilot(sourceSelection.wrappedValue) {
                Text("使用 Azure deployment 名称；需要此资源的模型调用权限。客户端工具能力需按部署验证。")
                    .font(.caption).foregroundStyle(.secondary)
            }
            if let error = state?.error { Text(error).font(.caption).foregroundStyle(.red).textSelection(.enabled) }
            HStack {
                Text("localhost:\(String(store.appliedPort))\(sourceSelection.wrappedValue == "copilot" ? "" : "/sources/" + sourceSelection.wrappedValue)/\(clientID == "codex" ? "codex" : "cc")")
                    .font(.system(.caption, design: .monospaced)).foregroundStyle(.secondary)
                Spacer()
                if let path = state?.path, !path.isEmpty {
                    Button("在 Finder 中查看") { NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)]) }.buttonStyle(.borderless)
                }
                Button("预览并应用") { Task { await store.prepareConfig(clientID) } }
                    .auroraProminent().disabled(store.busy != nil || !store.ready || store.signingIn
                        || (selection.wrappedValue.isEmpty && store.loadingModelSources.contains(sourceSelection.wrappedValue)))
            }
            Text("切换来源或模型无需重启 Bridge。应用后，请新建 Agent 会话以使用新配置。")
                .font(.caption).foregroundStyle(.secondary)
        }
        .task(id: sourceSelection.wrappedValue) { await store.loadModels(for: sourceSelection.wrappedValue) }
    }
}

struct ConfigurationSheet: View {
    @EnvironmentObject var store: AppStore
    let plan: ConfigPreview
    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Text("确认客户端配置").font(.title2).fontWeight(.semibold)
            Text(plan.reply.path ?? "").font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
            ScrollView {
                VStack(alignment: .leading, spacing: 10) {
                    ForEach(Array((plan.reply.changes ?? []).enumerated()), id: \.offset) { _, change in
                        Label(change, systemImage: "checkmark.circle").font(.system(.callout, design: .monospaced)).textSelection(.enabled)
                    }
                }.frame(maxWidth: .infinity, alignment: .leading)
            }.frame(maxHeight: 300)
            Text("只更新上述连接与默认模型设置。已有文件会先创建独立备份；若文件在预览后发生变化，本次保存会停止。").font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("取消", role: .cancel) { store.preview = nil }.keyboardShortcut(.cancelAction)
                Spacer()
                Button("备份并应用") { Task { await store.applyConfig(plan) } }.auroraProminent()
                    .keyboardShortcut(.defaultAction).disabled(store.busy != nil)
            }
        }.padding(28).frame(width: 620)
    }
}

struct QuotaView: View {
    @EnvironmentObject var store: AppStore
    var account: AccountProfile = .copilot
    private var quota: AccountQuota? { account.id == "copilot" ? store.quota : store.accountStates[account.id]?.quota }
    private var quotaError: String? { account.id == "copilot" ? store.quotaError : store.accountStates[account.id]?.quotaError }
    var body: some View {
        Card {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text(account.name + " · Copilot 额度").font(.headline)
                    Text(quota?.copilotPlan.map { "套餐：\($0)" } ?? "来自 GitHub 的账号数据")
                        .font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Button("刷新额度") { Task { await store.refreshAccount(account) } }.disabled(!store.accountLoggedIn(account) || store.busy != nil || store.signingIn)
            }
            if let snapshots = quota?.quotaSnapshots, !snapshots.isEmpty {
                ForEach(snapshots.keys.sorted(), id: \.self) { key in
                    if let quota = snapshots[key] {
                        VStack(alignment: .leading, spacing: 7) {
                            HStack {
                                Text(quotaLabel(key)).font(.subheadline)
                                Spacer()
                                Text(quota.unlimited == true ? "不限量" : quota.available.map { "剩余 \($0.formatted())" } ?? "剩余量未提供")
                                    .font(.subheadline).fontWeight(.medium)
                                if quota.unlimited != true, let total = quota.entitlement { Text("/ \(total.formatted())").foregroundStyle(.secondary) }
                            }
                            if quota.unlimited != true, let percent = quota.percentRemaining {
                                ProgressView(value: min(max(percent, 0), 100), total: 100).tint(accent)
                                Text("剩余 \(percent.formatted(.number.precision(.fractionLength(1))))%").font(.caption).foregroundStyle(.secondary)
                            }
                            if let overage = quota.overageCount, overage > 0 { Text("超额使用：\(overage.formatted())").font(.caption).foregroundStyle(.orange) }
                        }.padding(.vertical, 4)
                    }
                }
                if let reset = quota?.quotaResetDate { Text("额度重置：\(reset)").font(.caption).foregroundStyle(.secondary) }
            } else {
                Text(quotaError ?? (store.accountLoggedIn(account) ? "点击刷新读取账号额度。" : "登录 GitHub 后可查看额度。"))
                    .font(.subheadline).foregroundStyle(.secondary).textSelection(.enabled)
            }
            HStack {
                if account.id == "copilot", let date = store.quotaUpdated { Text("更新于 \(date.formatted(date: .abbreviated, time: .shortened))").font(.caption2).foregroundStyle(.secondary) }
                Spacer()
                Link("在 GitHub 查看用量 ↗", destination: URL(string: "https://github.com/settings/copilot/features")!).font(.caption)
            }
            Text("GitHub 未返回的额度不会显示为 0。此接口受账号权限影响，账单以 GitHub 为准。").font(.caption2).foregroundStyle(.secondary)
        }
    }
    private func quotaLabel(_ key: String) -> String {
        switch key { case "premium_interactions": return "高级请求"; case "chat": return "聊天"; case "completions": return "代码补全"; default: return key }
    }
}

struct UsageView: View {
    @EnvironmentObject var store: AppStore
    private var counts: [(String, Int)] {
        Dictionary(grouping: store.filteredActivity.filter { $0.kind == "request" }, by: { store.sourceName($0.source ?? "copilot") + " / " + ($0.model ?? "未知模型") })
            .map { ($0.key, $0.value.count) }.sorted { $0.1 > $1.1 }
    }
    var body: some View {
        HStack {
            Text(store.usageSource == "all" ? "整体用量" : store.sourceName(store.usageSource) + " 用量").font(.headline)
            Spacer()
            Picker("来源", selection: $store.usageSource) {
                Text("全部来源").tag("all"); Text("GitHub Copilot").tag("copilot")
                ForEach(store.sources) { source in Text(store.sourceName(source.id)).tag(source.id) }
            }.frame(width: 210)
            Picker("时间范围", selection: $store.usageDays) { Text("今天").tag(1); Text("7 天").tag(7); Text("30 天").tag(30); Text("自定义").tag(0) }
                .pickerStyle(.segmented).frame(width: 210)
        }
        HStack(spacing: 14) {
            StatCard(title: "请求数", value: store.totals.requests.formatted(), icon: "arrow.left.arrow.right")
            StatCard(title: "输入 token", value: store.totals.input.formatted(), icon: "arrow.down.left")
            StatCard(title: "输出 token", value: store.totals.output.formatted(), icon: "arrow.up.right")
        }
        if store.usageDays == 0 {
            HStack {
                DatePicker("开始", selection: $store.usageStart, in: ...Date(), displayedComponents: .date)
                DatePicker("结束", selection: $store.usageEnd, in: store.usageStart...max(store.usageStart, Date()), displayedComponents: .date)
            }.onChange(of: store.usageStart) { _, value in if store.usageEnd < value { store.usageEnd = value } }
        }
        UsageTimelineView()
        if !counts.isEmpty {
            Card {
                Text("按来源与模型统计请求").font(.headline)
                Chart(counts.prefix(8), id: \.0) { item in
                    BarMark(x: .value("请求", item.1), y: .value("模型", item.0)).foregroundStyle(accent.gradient).cornerRadius(3)
                }.frame(height: CGFloat(max(counts.prefix(8).count, 2)) * 34)
                Text("缓存读取：\(store.totals.cached.formatted()) token · 失败：\(store.totals.failures)")
                    .font(.caption).foregroundStyle(.secondary)
            }
        } else { EmptyCard(icon: "chart.bar", title: "还没有请求记录", text: "启动 Bridge 并通过已连接的客户端发起请求后，这里会自动更新。") }
        Text("仅统计此应用管理的 Bridge；count_tokens 预估不计入请求量。输入与缓存按上游原始值分别展示，不相加。\(store.totals.missingUsage > 0 ? "有 \(store.totals.missingUsage) 次请求缺少完整 token 数据，总量可能不完整。" : "")")
            .font(.caption).foregroundStyle(.secondary)
        ForEach(store.accounts.filter { account in
            account.kind == "github" && (store.usageSource == "all" ||
                (store.usageSource == "copilot" && account.id == "copilot") ||
                store.sources.contains { $0.id == store.usageSource && $0.accountId == account.id })
        }) { account in QuotaView(account: account) }
        if store.usageSource != "all" && !store.isCopilot(store.usageSource) {
            Card {
                Text("Azure 账号用量").font(.headline)
                Text("以上是本机请求统计；Azure 配额与账单以 Azure Portal 为准。").font(.caption).foregroundStyle(.secondary)
                Link("打开 Azure Portal ↗", destination: URL(string: "https://portal.azure.com/")!)
            }
        }
    }
}

struct ErrorsView: View {
    @EnvironmentObject var store: AppStore
    var body: some View {
        HStack {
            Text("最近 31 个日志日").font(.caption).foregroundStyle(.secondary)
            Spacer()
            Button("打开日志目录") {
                if let directory = store.runtime?.directory { NSWorkspace.shared.open(directory.appendingPathComponent("log")) }
            }.disabled(!store.ready)
        }
        if store.errors.isEmpty && store.issues.isEmpty {
            EmptyCard(icon: "checkmark.shield", title: "暂时没有错误", text: "启动失败、网络问题和模型请求错误会显示在这里。")
        }
        ForEach(store.issues) { issue in
            Card {
                HStack { Label("应用操作", systemImage: "exclamationmark.circle").foregroundStyle(.orange); Spacer(); Text(issue.date.formatted(date: .abbreviated, time: .standard)).font(.caption).foregroundStyle(.secondary) }
                Text(issue.message).font(.system(.caption, design: .monospaced)).textSelection(.enabled)
            }
        }
        ForEach(Array(store.errors.prefix(100))) { event in
            Card {
                HStack {
                    Label(store.sourceName(event.source ?? "copilot") + " · " + (event.client ?? "Bridge"), systemImage: "exclamationmark.circle").foregroundStyle(.orange)
                    if let status = event.status { Text("HTTP \(status)").font(.system(.caption, design: .monospaced)) }
                    Spacer()
                    Text(event.timestamp.formatted(date: .abbreviated, time: .standard)).font(.caption).foregroundStyle(.secondary)
                }
                if let model = event.model { Text(model).font(.caption).foregroundStyle(.secondary) }
                Text(event.error ?? "请求失败").font(.system(.caption, design: .monospaced)).textSelection(.enabled)
            }
        }
        Text("显示脱敏摘要，不收集提示词或响应正文。查看原始日志可获得更完整的排查信息。").font(.caption).foregroundStyle(.secondary)
    }
}

struct NetworkView: View {
    @EnvironmentObject var store: AppStore
    var body: some View {
        Card {
            Label("本地服务", systemImage: "desktopcomputer").font(.headline)
            HStack {
                Text("监听端口")
                Spacer()
                TextField("8765", text: $store.portText).textFieldStyle(.roundedBorder).frame(width: 120)
            }
            Text("只监听 localhost。修改后点击“保存并启动”即可使用新端口；已连接的 Agent 需另行预览并应用新地址。")
                .font(.caption).foregroundStyle(.secondary)
            Text("已保存端口：\(String(store.appliedPort))\(store.running ? " · 服务运行中" : " · 服务未启动")")
                .font(.caption).foregroundStyle(.secondary)
            Divider()
            Button("管理来源 Proxy", systemImage: "network") { store.page = .proxies }
            Text("出站代理在 Proxy 页管理，由每个来源独立选择。").font(.caption).foregroundStyle(.secondary)
            if !store.networkValid { Text("请填写 1024–65535 之间的端口。").font(.caption).foregroundStyle(.orange) }
            HStack {
                if store.networkChanged { Text("有未应用的修改").font(.caption).foregroundStyle(.secondary) }
                Spacer()
                Button(store.running ? "应用并重启 Bridge" : "应用网络设置") { Task { await store.saveNetwork() } }
                    .auroraProminent().disabled(!store.networkValid || store.busy != nil || store.signingIn || !store.ready)
            }
        }
        Card {
            HStack {
                VStack(alignment: .leading, spacing: 5) {
                    Text("Bridge 服务").font(.headline)
                    Text("\(store.serviceLabel) · localhost:\(String(store.appliedPort))").font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Button(store.running ? "停止" : store.networkChanged ? "保存并启动" : "启动") { if store.running { store.stopService() } else { store.startService() } }
                    .disabled(!store.canStart || store.busy != nil || (!store.running && !store.networkValid) || store.signingIn || store.serviceState == "starting" || store.serviceState == "stopping")
            }
        }
    }
}
