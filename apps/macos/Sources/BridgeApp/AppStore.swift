import AppKit
import Foundation
import BridgeCore
import Darwin

enum Page: String, CaseIterable, Identifiable {
    case overview = "导览", account = "账号", sources = "来源", clients = "Agent", usage = "用量", errors = "错误", network = "Bridge 服务配置"
    var id: String { rawValue }
    var icon: String {
        switch self {
        case .sources: return "server.rack"
        case .overview: return "square.grid.2x2"
        case .account: return "person.crop.circle"
        case .clients: return "slider.horizontal.3"
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
    @Published var login: String?
    @Published var clients: [ClientState] = []
    @Published var accounts: [AccountProfile] = [.copilot, .localAzure]
    @Published var accountStates: [String: Reply] = [:]
    @Published var sourceModels: [String: [BridgeModel]] = [:]
    @Published var signingInAccount: String?
    @Published var sources: [ModelSource] = []
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
    @Published var proxyMode: ProxyMode
    @Published var proxyAddress: String
    @Published var portText: String
    @Published var selectedCodex = ""
    @Published var selectedClaude = ""
    @Published var runtimeError: String?
    @Published var usageDays = 1
    private(set) var runtime: Runtime?
    private var service: BridgeProcess?
    private var loginProcess: BridgeProcess?
    private var jobs: [UUID: BridgeProcess] = [:]
    private var activityCache: [String: (Int, Date, [Activity])] = [:]
    private var refreshTimer: Timer?
    private var appliedMode: ProxyMode
    private var appliedProxy: String
    private(set) var appliedPort: Int

    init() {
        let defaults = UserDefaults.standard
        let mode = ProxyMode(rawValue: defaults.string(forKey: "proxyMode") ?? "system") ?? .system
        let address = defaults.string(forKey: "proxyAddress") ?? "http://127.0.0.1:7890"
        let port = defaults.integer(forKey: "port")
        proxyMode = mode; appliedMode = mode
        proxyAddress = address; appliedProxy = address
        appliedPort = (1024...65535).contains(port) ? port : 8765
        portText = String(appliedPort)
        do {
            runtime = try Runtime()
            if let file = runtime?.root.appendingPathComponent("accounts.json"), FileManager.default.fileExists(atPath: file.path) {
                let saved = try JSONDecoder().decode([AccountProfile].self, from: Data(contentsOf: file)).map { try $0.validated() }
                accounts += saved
                guard Set(accounts.map(\.id)).count == accounts.count else { throw SourceFailure.invalid }
            }
            if let file = runtime?.root.appendingPathComponent("sources.json"), FileManager.default.fileExists(atPath: file.path) {
                sources = try ModelSource.decode(Data(contentsOf: file))
            }
        }
        catch { runtimeError = error.localizedDescription }
        refreshTimer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.refreshActivity() }
        }
    }

    var ready: Bool { runtime != nil && runtimeError == nil }
    var canStart: Bool { loggedIn || !sources.isEmpty }
    var running: Bool { serviceState == "running" }
    var serviceLabel: String {
        switch serviceState { case "starting": return "正在启动"; case "running": return "运行中"; case "stopping": return "正在停止"; default: return "已停止" }
    }
    var filteredActivity: [Activity] {
        let start = Calendar.current.date(byAdding: .day, value: -(usageDays - 1), to: Calendar.current.startOfDay(for: Date()))!
        return activity.filter { $0.timestamp >= start && (usageSource == "all" || ($0.source ?? "copilot") == usageSource) }
    }
    var totals: UsageTotals { UsageTotals(filteredActivity) }
    var errors: [Activity] { activity.filter(\.failed) }
    var networkChanged: Bool { proxyMode != appliedMode || proxyAddress != appliedProxy || portText != String(appliedPort) }
    var networkValid: Bool { proxyMode.validate(proxyAddress) && Int(portText).map { (1024...65535).contains($0) } == true }

    func showError(_ error: Error) {
        let message = BridgeProcess.clean(error.localizedDescription)
        issues.insert(LocalIssue(message: message), at: 0)
        if issues.count > 100 { issues.removeLast() }
        notice = message
    }

    func refresh() async {
        guard ready, busy == nil else { return }
        busy = "正在刷新"
        defer { busy = nil }
        do { try await refreshStatus() } catch { showError(error) }
        await refreshAccountStatuses()
        await refreshActivity()
    }

    private func refreshStatus() async throws {
        let reply = try await execute(["desktop", "status", "--port", String(appliedPort)])
        loggedIn = reply.loggedIn ?? false
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

    func refreshModels() async {
        guard busy == nil, !signingIn else { return }
        busy = "正在获取来源模型"
        defer { busy = nil }
        for sourceID in Set([codexSource, claudeSource]) {
            do {
                let reply = try await execute(["desktop", "models", "--source", sourceID])
                sourceModels[sourceID] = reply.models ?? []
                if sourceID == "copilot" { models = reply.models ?? [] }
            } catch { sourceModels[sourceID] = nil; showError(error) }
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
            notice = result.backupPath.map { "配置已保存。原文件备份：\($0)\n请新建客户端会话以使用新配置。" }
                ?? "配置已保存。请新建客户端会话以使用新配置。"
        } catch { showError(error); preview = nil }
    }

    func startService() {
        guard let runtime, ready, canStart, !signingIn, serviceState == "stopped" else { return }
        guard Self.portAvailable(appliedPort) else {
            showError(AppFailure(message: "端口 \(appliedPort) 已被占用。请在 Bridge 服务配置中选择其他端口，再更新客户端配置。")); return
        }
        let child = BridgeProcess()
        service = child; serviceState = "starting"
        do {
            var keys: [String: String] = [:]
            for source in sources where source.kind == "azure-openai" && source.authMode == "api-key" { keys[source.id] = try SourceKeychain.read(source.id) }
            var envelope = try JSONEncoder().encode(keys); envelope.append(10)
            var environment = runtime.environment(mode: appliedMode, proxy: appliedProxy)
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

    func saveNetwork() async {
        guard networkValid, busy == nil, !signingIn else { return }
        busy = "正在应用网络设置"
        let restart = serviceState == "running" || serviceState == "starting"
        if restart {
            stopService()
            for _ in 0..<100 where serviceState != "stopped" { try? await Task.sleep(nanoseconds: 100_000_000) }
            guard serviceState == "stopped" else { busy = nil; showError(AppFailure(message: "Bridge 尚未停止，请稍后重试。")); return }
        }
        appliedMode = proxyMode; appliedProxy = proxyAddress; appliedPort = Int(portText)!
        let defaults = UserDefaults.standard
        defaults.set(appliedMode.rawValue, forKey: "proxyMode")
        defaults.set(appliedProxy, forKey: "proxyAddress")
        defaults.set(appliedPort, forKey: "port")
        do { try await refreshStatus() } catch { showError(error) }
        busy = nil
        if restart { startService() }
        notice = "网络设置已保存。若修改了端口，请到 Agent 页重新应用连接配置。"
    }

    func refreshActivity() async {
        guard let runtime else { return }
        let directory = runtime.directory.appendingPathComponent("desktop-activity")
        let prior = activityCache
        let result = await Task.detached(priority: .utility) { () -> [String: (Int, Date, [Activity])] in
            let fm = FileManager.default
            let files = (try? fm.contentsOfDirectory(at: directory,
                includingPropertiesForKeys: [.fileSizeKey, .contentModificationDateKey])) ?? []
            var next: [String: (Int, Date, [Activity])] = [:]
            for file in files.filter({ $0.pathExtension == "jsonl" }).sorted(by: { $0.path > $1.path }).prefix(31) {
                guard let values = try? file.resourceValues(forKeys: [.fileSizeKey, .contentModificationDateKey]),
                      let size = values.fileSize, let date = values.contentModificationDate else { continue }
                if let cached = prior[file.path], cached.0 == size, cached.1 == date { next[file.path] = cached; continue }
                if let data = try? Data(contentsOf: file) { next[file.path] = (size, date, ActivityReader.decode(data)) }
            }
            return next
        }.value
        activityCache = result
        activity = result.values.flatMap { $0.2 }.sorted { $0.timestamp > $1.timestamp }
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
                    environment: runtime.environment(mode: appliedMode, proxy: appliedProxy), timeout: timeout,
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
        id == "copilot" ? "GitHub Copilot" : sources.first { $0.id == id }.map { source in
            (accounts.first { a in a.id == (source.accountId ?? "azure-local") }?.name ?? "") + " / " + source.name
        } ?? id
    }

    func saveSource(_ candidate: ModelSource, key: String) throws {
        guard let runtime, serviceState == "stopped" else { throw AppFailure(message: "请先停止 Bridge，再修改来源。") }
        let source = try candidate.validated()
        guard let account = accounts.first(where: { $0.id == (source.accountId ?? "azure-local") }),
              account.kind == (source.kind == "copilot" ? "github" : "azure") else { throw SourceFailure.invalid }
        if source.kind == "azure-openai" && source.authMode == "api-key" {
            if !key.isEmpty {
                guard !key.contains(where: { $0.isNewline || $0.isWhitespace }) else { throw AppFailure(message: "API Key 不能含空白字符。") }
                try SourceKeychain.save(key, for: source.id)
            } else { _ = try SourceKeychain.read(source.id) }
        }
        var next = sources.filter { $0.id != source.id }; next.append(source)
        try ModelSource.encode(next).write(to: runtime.root.appendingPathComponent("sources.json"), options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: runtime.root.appendingPathComponent("sources.json").path)
        sources = next
    }

    func removeSource(_ source: ModelSource) throws {
        guard let runtime, serviceState == "stopped" else { throw AppFailure(message: "请先停止 Bridge。") }
        guard !clients.contains(where: { $0.connected && $0.source == source.id }) else {
            throw AppFailure(message: "此来源仍被 Agent 使用，请先为它选择其他来源并应用配置。")
        }
        let next = sources.filter { $0.id != source.id }
        try ModelSource.encode(next).write(to: runtime.root.appendingPathComponent("sources.json"), options: .atomic)
        sources = next
        try SourceKeychain.delete(source.id)
    }

    func shutdown() { refreshTimer?.invalidate(); stopService(); jobs.values.forEach { $0.stop() } }
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
