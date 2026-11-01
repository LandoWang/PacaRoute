import AppKit
import Foundation
import BridgeCore

extension AppStore {
    private func accountData(_ accounts: [AccountProfile]) throws -> Data {
        let encoder = JSONEncoder(); encoder.keyEncodingStrategy = .convertToSnakeCase
        return try encoder.encode(accounts)
    }

    func isCopilot(_ source: String) -> Bool {
        source == "copilot" || sources.first { $0.id == source }?.kind == "copilot"
    }

    func accountSources(_ account: AccountProfile) -> [ModelSource] {
        sources.filter { ($0.accountId ?? "azure-local") == account.id }
    }

    func accountLoggedIn(_ account: AccountProfile) -> Bool {
        account.id == "copilot" ? loggedIn : (accountStates[account.id]?.loggedIn == true || (account.kind == "github" && accountStates[account.id]?.login != nil))
    }

    func availableModels(_ source: String) -> [BridgeModel] {
        if let pool = pool(source) {
            return [BridgeModel(id: pool.model, name: pool.name + "（按权重）", vendor: "Source pool",
                claude: pool.agent == "claude-code", codex: pool.agent == "codex")]
        }
        if source == "copilot" { return models }
        if isCopilot(source) { return sourceModels[source] ?? [] }
        return (sources.first { $0.id == source }?.deployments ?? []).map {
            BridgeModel(id: $0, name: $0, vendor: "Azure OpenAI", claude: true, codex: true)
        }
    }

    func refreshAccountStatuses() async {
        for account in accounts where account.kind == "github" && account.id != "copilot" {
            do {
                let status = try await execute(["desktop", "account-status", "--account", account.id])
                if status.loggedIn != true || accountStates[account.id]?.login == nil { accountStates[account.id] = status }
                if status.loggedIn != true { invalidateModelCatalogs(accountID: account.id) }
            }
            catch { showError(error) }
        }
    }

    func refreshAccount(_ account: AccountProfile) async {
        guard busy == nil, !signingIn else { return }
        busy = "正在读取账号"
        defer { busy = nil }
        do {
            accountStates[account.id] = try await execute(["desktop", "account", "--account", account.id])
            if account.id == "copilot" {
                login = accountStates[account.id]?.login; quota = accountStates[account.id]?.quota
                quotaError = accountStates[account.id]?.quotaError; quotaUpdated = Date()
            }
        } catch { showError(error) }
    }

    func addGitHubLogin() async {
        let account = AccountProfile(id: "github-" + UUID().uuidString.prefix(8).lowercased(), name: "GitHub · 待授权", kind: "github")
        do { try saveAccount(account); await signInAccount(account) } catch { showError(error) }
    }

    func signInAccount(_ account: AccountProfile, reauthorize: Bool = false) async {
        guard !signingIn, busy == nil else { return }
        signingIn = true; signingInAccount = account.id; deviceCode = nil; deviceURL = nil
        defer { signingIn = false; signingInAccount = nil; deviceCode = nil; deviceURL = nil; deviceExpires = nil }
        do {
            _ = try await execute(["desktop", reauthorize && account.kind == "github" ? "reauthorize" : "login", "--account", account.id], timeout: 900, isLogin: true) { [weak self] reply in
                guard reply.event == "device_code" else { return }
                self?.deviceCode = reply.userCode
                self?.deviceExpires = Date().addingTimeInterval(reply.expiresIn ?? 900)
                if let string = reply.verificationUri, let url = URL(string: string), url.scheme == "https",
                   ["github.com", "microsoft.com"].contains(url.host ?? "") {
                    self?.deviceURL = url; NSWorkspace.shared.open(url)
                }
            }
            accountStates[account.id] = try await execute(["desktop", "account-status", "--account", account.id])
            invalidateModelCatalogs(accountID: account.id)
            if account.kind == "github" {
                if account.id == "copilot" { loggedIn = true }
                if !account.builtin && !accountSources(account).contains(where: { $0.kind == "copilot" }) {
                    try saveSource(ModelSource(id: account.id, name: "Copilot", endpoint: "", deployments: [], accountId: account.id, kind: "copilot"), key: "")
                }
                // Account/quota lookup is optional: a network error must not undo saved login.
                let details = try? await execute(["desktop", "account", "--account", account.id])
                if let details { accountStates[account.id] = details }
                if let username = details?.login, !account.builtin, let runtime {
                    var next = accounts
                    if let index = next.firstIndex(where: { $0.id == account.id }) { next[index].name = username }
                    try runtime.saveMetadata("accounts", data: accountData(next.filter { !$0.builtin }))
                    accounts = next
                }
            }
            if account.id == "copilot" {
                loggedIn = true; login = accountStates[account.id]?.login; quota = accountStates[account.id]?.quota
                quotaError = accountStates[account.id]?.quotaError; quotaUpdated = Date()
            }
            notice = account.kind == "github"
                ? "GitHub 登录成功，Copilot 来源已自动创建。前往 Agent 页选择来源和模型。"
                : "\(account.name) 登录成功。接下来可为此账号添加一个或多个 AOAI 资源来源。"
            Task { await self.refreshModels(force: true) }
        } catch { if signingIn { showError(error) } }
    }

    func signOutAccount(_ account: AccountProfile) async {
        guard busy == nil, !signingIn else { return }
        busy = "正在退出此账号"
        defer { busy = nil }
        do {
            accountStates[account.id] = try await execute(["desktop", "logout", "--account", account.id])
            invalidateModelCatalogs(accountID: account.id)
            for source in accountSources(account) { sourceModels[source.id] = nil }
            if account.id == "copilot" { loggedIn = false; login = nil; quota = nil; quotaUpdated = nil; models = [] }
        } catch { showError(error) }
    }

    func saveAccount(_ candidate: AccountProfile) throws {
        guard let runtime, !signingIn else { throw AppFailure(message: "请等待当前操作完成。") }
        let account = try candidate.validated()
        guard !accounts.contains(where: { $0.id == account.id }) else { throw SourceFailure.invalid }
        let next = accounts + [account]
        let file = runtime.root.appendingPathComponent("accounts.json")
        try runtime.saveMetadata("accounts", data: accountData(next.filter { !$0.builtin }))
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: file.path)
        accounts = next
    }

    func removeAccount(_ account: AccountProfile) throws {
        guard let runtime, !account.builtin, !signingIn, busy == nil else {
            throw AppFailure(message: "请等待账号操作完成后再移除账号。")
        }
        guard !accountLoggedIn(account) else { throw AppFailure(message: "请先退出此账号。") }
        guard accountSources(account).isEmpty else { throw AppFailure(message: "请先移除账号下的来源；Agent 使用中的来源需要先切换。") }
        let next = accounts.filter { $0.id != account.id }
        try runtime.saveMetadata("accounts", data: accountData(next.filter { !$0.builtin }))
        accounts = next; accountStates[account.id] = nil
    }
}
