import AppKit
import Foundation
import BridgeCore
import Darwin

enum Page: String, CaseIterable, Identifiable {
    case overview = "导览", account = "账号", sources = "来源", clients = "Agent", routing = "Client 路由", sessions = "会话", usage = "用量", proxies = "Proxy", network = "PacaRoute 服务配置", errors = "错误"
    var id: String { rawValue }
    var icon: String {
        switch self {
        case .sources: return "server.rack"
        case .proxies: return "network"
        case .overview: return "square.grid.2x2"
        case .account: return "person.crop.circle"
        case .clients: return "slider.horizontal.3"
        case .routing: return "arrow.triangle.branch"
        case .sessions: return "bubble.left.and.bubble.right"
        case .usage: return "chart.bar.xaxis"
        case .errors: return "exclamationmark.bubble"
        case .network: return "network"
        }
    }
}

struct ConfigPreview: Identifiable {
    let id = UUID()
    let client: String
    let source: String
    let model: String
    let port: Int
    let reply: Reply
}

struct LocalIssue: Identifiable {
    let id = UUID()
    let date = Date()
    let message: String
}

@MainActor final class AppStore: ObservableObject {
    @Published var page: Page = .overview
    @Published var loggedIn = false
    @Published var checkingAccounts = true
    @Published var loadingModelSources: Set<String> = []
    @Published var modelErrors: [String: String] = [:]
    @Published var login: String?
    @Published var clients: [ClientState] = []
    @Published var accounts: [AccountProfile] = [.copilot, .localAzure]
    @Published var accountStates: [String: Reply] = [:]
    @Published var sourceModels: [String: [BridgeModel]] = [:]
    @Published var signingInAccount: String?
    @Published var sources: [ModelSource] = []
    @Published var pausedSourceIDs: Set<String> = []
    @Published var sourcePools: [SourcePool] = []
    @Published var poolError: String?
    @Published var proxies = ProxyConfiguration()
    @Published var codexSource = "copilot"
    @Published var claudeSource = "copilot"
    @Published var usageSource = "all"
    @Published var models: [BridgeModel] = []
    @Published var quota: AccountQuota?
    @Published var quotaError: String?
    @Published var quotaUpdated: Date?
    @Published var activity: [Activity] = []
    @Published var issues: [LocalIssue] = []
    @Published var preview: ConfigPreview?
    @Published var notice: String?
    @Published var busy: String?
    @Published var signingIn = false
    @Published var deviceCode: String?
    @Published var deviceURL: URL?
    @Published var deviceExpires: Date?
    @Published var serviceState = "stopped"
    @Published var portText: String
    @Published var selectedCodex = ""
    @Published var selectedClaude = ""
    @Published var runtimeError: String?
    @Published var usageDays = 1
    @Published var usageStart = Calendar.current.startOfDay(for: Date())
    @Published var usageEnd = Date()
    private(set) var runtime: Runtime?
    private var service: BridgeProcess?
    private var loginProcess: BridgeProcess?
    private var jobs: [UUID: BridgeProcess] = [:]
    private let activityReader = ActivityLogReader()
    private var refreshingActivity = false
    private var refreshTimer: Timer?
    private(set) var appliedPort: Int
    let defaults: UserDefaults
    let credentials: SourceCredentialCache
    var modelCatalogs: [String: SavedModelCatalog] = [:]
    var modelEpochs: [String: Int] = [:]

    init(defaults: UserDefaults = .standard, credentials: SourceCredentialCache = SourceCredentialCache(),
         makeRuntime: () throws -> Runtime = { try Runtime() }) {
        self.defaults = defaults
        self.credentials = credentials
        let mode = ProxyMode(rawValue: defaults.string(forKey: "proxyMode") ?? "system") ?? .system
        let address = defaults.string(forKey: "proxyAddress") ?? "http://127.0.0.1:7890"
        let oldDefaults = UserDefaults(suiteName: "com.landowang.CopilotBridge")
        let port = defaults.object(forKey: "port") != nil ? defaults.integer(forKey: "port") : (oldDefaults?.integer(forKey: "port") ?? 0)
        appliedPort = (1024...65535).contains(port) ? port : 8766
        portText = String(appliedPort)
        do {
            runtime = try makeRuntime()
            if let file = runtime?.root.appendingPathComponent("accounts.json"), FileManager.default.fileExists(atPath: file.path) {
                let accountDecoder = JSONDecoder(); accountDecoder.keyDecodingStrategy = .convertFromSnakeCase
                let saved = try accountDecoder.decode([AccountProfile].self, from: Data(contentsOf: file)).map { try $0.validated() }
                accounts += saved
                guard Set(accounts.map(\.id)).count == accounts.count else { throw SourceFailure.invalid }
            }
            if let file = runtime?.root.appendingPathComponent("sources.json"), FileManager.default.fileExists(atPath: file.path) {
                sources = try ModelSource.decode(Data(contentsOf: file))
            }
            reloadPools()
            if let runtime {
                let file = runtime.root.appendingPathComponent("proxies.json")
                if FileManager.default.fileExists(atPath: file.path) { proxies = try ProxyConfiguration.decode(Data(contentsOf: file)) }
                else {
                    proxies = ProxyConfiguration.migrated(mode: mode, address: address, sourceIDs: sources.map(\.id))
                    try proxies.encoded().write(to: file, options: .atomic)
                    try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: file.path)
                }
            }
        }
        catch { runtimeError = error.localizedDescription }
        restoreModelCatalogs()
        refreshTimer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.refreshActivity() }
        }
    }

    var ready: Bool { runtime != nil && runtimeError == nil }
    var hasAccount: Bool { loggedIn || accounts.contains { !$0.builtin } || accountStates["azure-local"]?.loggedIn == true || !sources.isEmpty }
    var hasSource: Bool {
        loggedIn || sources.contains { source in
            if source.kind != "copilot" { return true }
            guard let account = accounts.first(where: { $0.id == source.accountId }) else { return false }
            return accountLoggedIn(account)
        }
    }
    var canStart: Bool { ready }
    var running: Bool { serviceState == "running" }
    var serviceLabel: String {
        switch serviceState { case "starting": return "正在启动"; case "running": return "运行中"; case "stopping": return "正在停止"; default: return "已停止" }
    }
    var filteredActivity: [Activity] {
        let range = usageInterval
        return activity.filter { $0.timestamp >= range.start && $0.timestamp < range.end
            && (usageSource == "all" || ($0.source ?? "copilot") == usageSource) }
    }
    var usageInterval: DateInterval {
        let cal = Calendar.current
        let start = usageDays == 0 ? cal.startOfDay(for: usageStart)
            : cal.date(byAdding: .day, value: -(usageDays - 1), to: cal.startOfDay(for: Date()))!
        let last = usageDays == 0 ? max(start, cal.startOfDay(for: usageEnd)) : cal.startOfDay(for: Date())
        return DateInterval(start: start, end: cal.date(byAdding: .day, value: 1, to: last)!)
    }
    var usageHourly: Bool { Calendar.current.isDate(usageInterval.start, inSameDayAs: usageInterval.end.addingTimeInterval(-1)) }
    var timeline: [UsageBucket] { UsageTimeline.buckets(activity, start: usageInterval.start, end: usageInterval.end,
        hourly: usageHourly, source: usageSource) }
    var totals: UsageTotals { UsageTotals(filteredActivity) }
    var errors: [Activity] { activity.filter(\.failed) }
    var networkChanged: Bool { portText != String(appliedPort) }
    var networkValid: Bool { Int(portText).map { (1024...65535).contains($0) } == true }

    func showError(_ error: Error) {
        let message = BridgeProcess.clean(error.localizedDescription)
        issues.insert(LocalIssue(message: message), at: 0)
        if issues.count > 100 { issues.removeLast() }
        notice = message
    }

    func refresh() async {
        guard ready, busy == nil else { return }
        reloadMetadata()
        checkingAccounts = true
        busy = "正在刷新"
        do { try await refreshStatus() } catch { showError(error) }
        await refreshAccountStatuses()
        checkingAccounts = false
        busy = nil
        await refreshActivity()
        await refreshModels(force: false)
    }

    private func refreshStatus() async throws {
        let reply = try await execute(["desktop", "status", "--port", String(appliedPort)])
        loggedIn = reply.loggedIn ?? false
        if !loggedIn { invalidateModelCatalogs(accountID: "copilot") }
        clients = reply.clients ?? []
        for client in clients {
            if client.id == "codex" { selectedCodex = client.model ?? ""; codexSource = client.source ?? "copilot" }
            else { selectedClaude = client.model ?? ""; claudeSource = client.source ?? "copilot" }
        }
    }

    func refreshAccount() async {
        guard loggedIn, busy == nil, !signingIn else { return }
        busy = "正在读取账号与额度"
        defer { busy = nil }
        do {
            let reply = try await execute(["desktop", "account"])
            login = reply.login; quota = reply.quota; quotaError = reply.quotaError
            quotaUpdated = reply.quota == nil ? nil : Date()
        } catch { quotaError = error.localizedDescription; showError(error) }
    }

    func refreshModels(force: Bool = true) async {
        guard ready, !signingIn else { return }
        for sourceID in Set([codexSource, claudeSource]) {
            await loadModels(for: sourceID, force: force)
        }
    }

    func signIn() async { await signInAccount(.copilot) }
    func cancelSignIn() { signingIn = false; loginProcess?.stop() }
    func signOut() async { await signOutAccount(.copilot) }

    func prepareConfig(_ client: String) async {
        guard busy == nil, !signingIn else { return }
        let model = client == "codex" ? selectedCodex : selectedClaude
        let source = client == "codex" ? codexSource : claudeSource
        if source != (clients.first { $0.id == client }?.source ?? "copilot") && model.isEmpty {
            showError(AppFailure(message: "切换来源时请选择该来源的默认模型；Copilot 模型可先刷新列表。")); return
        }
        busy = "正在预览配置"
        defer { busy = nil }
        do {
            var args = ["desktop", "configure", "--client", client, "--source", source, "--port", String(appliedPort)]
            if !model.isEmpty { args += ["--model", model] }
            let reply = try await execute(args)
            preview = ConfigPreview(client: client, source: source, model: model, port: appliedPort, reply: reply)
        } catch { showError(error) }
    }

    func applyConfig(_ plan: ConfigPreview) async {
        guard busy == nil, let fingerprint = plan.reply.fingerprint else { return }
        busy = "正在保存配置"
        defer { busy = nil }
        do {
            var args = ["desktop", "configure", "--client", plan.client, "--source", plan.source, "--port", String(plan.port),
                        "--apply", "--fingerprint", fingerprint]
            if !plan.model.isEmpty { args += ["--model", plan.model] }
            let result = try await execute(args)
            preview = nil
            try await refreshStatus()
            defaults.set(plan.model, forKey: "model.\(plan.client).\(plan.source)")
            notice = "来源与模型已应用，Bridge 无需重启。请新建 Agent 会话以使用新配置。"
                + (result.backupPath.map { "\n原文件备份：\($0)" } ?? "")
        } catch { showError(error); preview = nil }
    }

    func startService() {
        guard ready, canStart, !signingIn, busy == nil, serviceState == "stopped" else { return }
        guard networkValid else { showError(AppFailure(message: "请填写有效的端口后再启动。")); return }
        if networkChanged {
            Task { await saveNetwork(startIfStopped: true) }
        } else {
            launchService()
        }
    }

    private func launchService() {
        guard let runtime, ready, canStart, !signingIn, serviceState == "stopped" else { return }
        guard Self.portAvailable(appliedPort) else {
            showError(AppFailure(message: "端口 \(appliedPort) 已被占用。请在 PacaRoute 服务配置中选择其他端口，再更新客户端配置。")); return
        }
        let child = BridgeProcess()
        service = child; serviceState = "starting"
        do {
            var keys: [String: String] = [:]
            for source in sources where source.kind == "azure-openai" && source.authMode == "api-key" {
                if let key = try? credentials.read(source.id) { keys[source.id] = key }
            }
            var envelope = try JSONEncoder().encode(keys); envelope.append(10)
            var environment = runtime.environment(mode: .system, proxy: "")
            environment["COPILOT_BRIDGE_SECRETS_STDIN"] = "1"
            try child.start(runtime: runtime, arguments: ["serve", "--port", String(appliedPort)],
                            environment: environment, timeout: nil, inputData: envelope,
                            onReply: { [weak self] reply in
                if reply.event == "ready" { Task { @MainActor in self?.serviceState = "running" } }
            }, completion: { [weak self] result in
                Task { @MainActor in
                    guard let self else { return }
                    let stoppedByUser = self.serviceState == "stopping"
                    self.serviceState = "stopped"; self.service = nil
                    if case .failure(let error) = result, !stoppedByUser { self.showError(error) }
                    await self.refreshActivity()
                }
            })
            Task { @MainActor [weak self, weak child] in
                try? await Task.sleep(nanoseconds: 90_000_000_000)
                if self?.service === child && self?.serviceState == "starting" {
                    self?.stopService()
                    self?.showError(AppFailure(message: "Bridge 启动超时。请检查登录与代理设置。"))
                }
            }
        } catch { serviceState = "stopped"; service = nil; showError(error) }
    }

    func stopService() { guard service != nil else { return }; serviceState = "stopping"; service?.stop() }

    func saveNetwork(startIfStopped: Bool = false) async {
        guard networkValid, busy == nil, !signingIn, serviceState != "stopping" else { return }
        let nextPort = Int(portText)!
        let restart = serviceState == "running" || serviceState == "starting"
        let shouldStart = restart || startIfStopped
        if shouldStart && (!restart || nextPort != appliedPort) && !Self.portAvailable(nextPort) {
            showError(AppFailure(message: "端口 \(nextPort) 已被占用，请选择其他端口。当前服务与已保存设置保持不变。")); return
        }
        busy = "正在应用网络设置"
        defer { busy = nil }
        if restart {
            stopService()
            for _ in 0..<100 where serviceState != "stopped" { try? await Task.sleep(nanoseconds: 100_000_000) }
            guard serviceState == "stopped" else { showError(AppFailure(message: "Bridge 尚未停止，请稍后重试。")); return }
        }
        appliedPort = nextPort
        defaults.set(appliedPort, forKey: "port")
        do { try await refreshStatus() } catch { showError(error) }
        if shouldStart {
            launchService()
            for _ in 0..<910 where serviceState == "starting" { try? await Task.sleep(nanoseconds: 100_000_000) }
            guard serviceState == "running" else { return }
            notice = "Bridge 已运行于 localhost:\(appliedPort)。若 Agent 仍使用旧端口，请到 Agent 页预览并应用连接配置。"
        } else {
            notice = "设置已保存，当前未启动服务。下次启动将使用 localhost:\(appliedPort)。"
        }
    }

    func refreshActivity() async {
        guard let runtime, !refreshingActivity else { return }
        refreshingActivity = true
        defer { refreshingActivity = false }
        let directory = runtime.directory.appendingPathComponent("desktop-activity")
        let reader = activityReader
        let result = await Task.detached(priority: .utility) {
            await reader.refresh(directory: directory)
        }.value
        if let result { activity = result }
    }

    func execute(_ arguments: [String], timeout: TimeInterval = 90, isLogin: Bool = false,
                         onReply: @escaping @MainActor (Reply) -> Void = { _ in }) async throws -> Reply {
        guard let runtime else { throw AppFailure(message: runtimeError ?? "Runtime is unavailable.") }
        let id = UUID(), child = BridgeProcess()
        jobs[id] = child
        if isLogin { loginProcess = child }
        defer { jobs.removeValue(forKey: id) }
        return try await withCheckedThrowingContinuation { continuation in
            do {
                try child.start(runtime: runtime, arguments: arguments,
                    environment: runtime.environment(mode: .system, proxy: ""), timeout: timeout,
                    onReply: { reply in Task { @MainActor in onReply(reply) } },
                    completion: { result in
                        switch result {
                        case .success(let reply):
                            if let reply { continuation.resume(returning: reply) }
                            else { continuation.resume(throwing: AppFailure(message: "Bridge 没有返回有效结果。")) }
                        case .failure(let error): continuation.resume(throwing: error)
                        }
                    })
            } catch { continuation.resume(throwing: error) }
        }
    }

    func sourceName(_ id: String) -> String {
        if let pool = pool(id) { return "来源池 / " + pool.name }
        return id == "copilot" ? "GitHub Copilot" : sources.first { $0.id == id }.map { source in
            (accounts.first { a in a.id == (source.accountId ?? "azure-local") }?.name ?? "") + " / " + source.name
        } ?? id
    }

    func saveSource(_ candidate: ModelSource, key: String) throws {
        guard let runtime else { throw AppFailure(message: "应用尚未准备好。") }
        let source = try candidate.validated()
        let proposed = sources.filter { $0.id != source.id } + [source]
        for pool in sourcePools { _ = try pool.validated(sources: proposed) }
        guard let account = accounts.first(where: { $0.id == (source.accountId ?? "azure-local") }),
              account.kind == (source.kind == "copilot" ? "github" : "azure") else { throw SourceFailure.invalid }
        if source.kind == "azure-openai" && source.authMode == "api-key" {
            if !key.isEmpty {
                guard !key.contains(where: { $0.isNewline || $0.isWhitespace }) else { throw AppFailure(message: "API Key 不能含空白字符。") }
                try credentials.save(key, for: source.id)
            } else { _ = try credentials.read(source.id) }
        }
        var next = sources.filter { $0.id != source.id }; next.append(source)
        try runtime.saveMetadata("sources", data: ModelSource.encode(next))
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: runtime.root.appendingPathComponent("sources.json").path)
        if sources.first(where: { $0.id == source.id })?.accountId != source.accountId { invalidateModelCatalog(source.id) }
        sources = next
        updateServiceKeys()
    }

    func removeSource(_ source: ModelSource) throws {
        if let pool = sourcePools.first(where: { $0.members.contains(where: { $0.sourceId == source.id }) }) {
            throw AppFailure(message: "此来源被来源池「\(pool.name)」引用，请先编辑该来源池。")
        }
        guard let runtime else { throw AppFailure(message: "应用尚未准备好。") }
        guard !clients.contains(where: { $0.connected && $0.source == source.id }) else {
            throw AppFailure(message: "此来源仍被 Agent 使用，请先为它选择其他来源并应用配置。")
        }
        let next = sources.filter { $0.id != source.id }
        try runtime.saveMetadata("sources", data: ModelSource.encode(next))
        sources = next
        var bindings = proxies; bindings.bindings.removeValue(forKey: source.id); try saveProxies(bindings)
        try credentials.delete(source.id)
        service?.sendInput(try JSONEncoder().encode([source.id: ""]) + Data([10]))
        invalidateModelCatalog(source.id)
    }

    func reloadMetadata() {
        guard let runtime else { return }
        do {
            let decoder = JSONDecoder(); decoder.keyDecodingStrategy = .convertFromSnakeCase
            let accountFile = runtime.root.appendingPathComponent("accounts.json")
            if FileManager.default.fileExists(atPath: accountFile.path) {
                accounts = [.copilot, .localAzure] + (try decoder.decode([AccountProfile].self, from: Data(contentsOf: accountFile)))
            }
            let sourceFile = runtime.root.appendingPathComponent("sources.json")
            if FileManager.default.fileExists(atPath: sourceFile.path) { sources = try ModelSource.decode(Data(contentsOf: sourceFile)) }
            let proxyFile = runtime.root.appendingPathComponent("proxies.json")
            if FileManager.default.fileExists(atPath: proxyFile.path) { proxies = try ProxyConfiguration.decode(Data(contentsOf: proxyFile)) }
            let stateFile = runtime.root.appendingPathComponent("source-state.json")
            if let data = try? Data(contentsOf: stateFile), let value = try? JSONDecoder().decode([String: [String]].self, from: data) {
                pausedSourceIDs = Set(value["paused"] ?? [])
            }
            reloadPools()
        } catch { showError(error) }
    }

    func toggleSourcePause(_ id: String) {
        guard let runtime else { return }
        do {
            var next = pausedSourceIDs
            if next.contains(id) { next.remove(id) } else { next.insert(id) }
            try runtime.saveMetadata("source-state", data: JSONEncoder().encode(["paused": Array(next).sorted()]))
            pausedSourceIDs = next
        } catch { showError(error) }
    }

    private func updateServiceKeys() {
        guard service != nil else { return }
        do {
            var keys: [String: String] = [:]
            for source in sources where source.kind == "azure-openai" && source.authMode == "api-key" {
                if let key = try? credentials.read(source.id) { keys[source.id] = key }
            }
            service?.sendInput(try JSONEncoder().encode(keys) + Data([10]))
        } catch { showError(error) }
    }

    func shutdown() { refreshTimer?.invalidate(); stopService(); jobs.values.forEach { $0.stop() }; credentials.clear() }
    var hasRunningChildren: Bool { service?.running == true || jobs.values.contains { $0.running } }

    private static func portAvailable(_ port: Int) -> Bool {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else { return false }
        defer { Darwin.close(fd) }
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = UInt16(port).bigEndian
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        return withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) == 0 }
        }
    }
}
