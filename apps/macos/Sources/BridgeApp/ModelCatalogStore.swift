import Foundation
import BridgeCore

/// Public picker metadata only. Authentication always comes from the credential facade.
struct SavedModelCatalog: Codable {
    let accountID: String
    let fetchedAt: Date
    let models: [BridgeModel]
}

extension AppStore {
    func modelAccount(_ source: String) -> AccountProfile? {
        let id = source == "copilot" ? "copilot" : sources.first { $0.id == source }?.accountId
        return accounts.first { $0.id == id }
    }

    func restoreModelCatalogs() {
        guard let runtime,
              let data = try? Data(contentsOf: runtime.root.appendingPathComponent("model-catalogs.json")),
              let saved = try? JSONDecoder().decode([String: SavedModelCatalog].self, from: data) else { return }
        modelCatalogs = saved.filter { isCopilot($0.key) && modelAccount($0.key)?.id == $0.value.accountID }
        for (id, catalog) in modelCatalogs { sourceModels[id] = catalog.models }
        models = modelCatalogs["copilot"]?.models ?? []
    }

    private func persistModelCatalogs() {
        guard let runtime else { return }
        do {
            let file = runtime.root.appendingPathComponent("model-catalogs.json")
            try JSONEncoder().encode(modelCatalogs).write(to: file, options: .atomic)
            try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: file.path)
        } catch { modelErrors["cache"] = "模型列表无法缓存：\(error.localizedDescription)" }
    }

    func invalidateModelCatalog(_ source: String) {
        modelEpochs[source, default: 0] += 1
        modelCatalogs[source] = nil; sourceModels[source] = nil; modelErrors[source] = nil
        if source == "copilot" { models = [] }
        persistModelCatalogs()
    }

    func invalidateModelCatalogs(accountID: String) {
        for id in Set(modelCatalogs.keys).union(sourceModels.keys).union(loadingModelSources)
            where modelAccount(id)?.id == accountID { invalidateModelCatalog(id) }
    }

    func loadModels(for source: String, force: Bool = false) async {
        guard ready, !checkingAccounts, !signingIn, isCopilot(source),
              let account = modelAccount(source), accountLoggedIn(account),
              !loadingModelSources.contains(source) else { return }
        if !force, let saved = modelCatalogs[source], !saved.models.isEmpty,
           Date().timeIntervalSince(saved.fetchedAt) < 3600 { return }
        loadingModelSources.insert(source); modelErrors[source] = nil
        let epoch = modelEpochs[source, default: 0]
        defer { loadingModelSources.remove(source) }
        do {
            let reply = try await execute(["desktop", "models", "--source", source])
            guard modelEpochs[source, default: 0] == epoch, accountLoggedIn(account) else { return }
            let loaded = reply.models ?? []
            modelCatalogs[source] = SavedModelCatalog(accountID: account.id, fetchedAt: Date(), models: loaded)
            sourceModels[source] = loaded
            if source == "copilot" { models = loaded }
            persistModelCatalogs()
            if codexSource == source && selectedCodex.isEmpty { selectedCodex = preferredModel(source, client: "codex") }
            if claudeSource == source && selectedClaude.isEmpty { selectedClaude = preferredModel(source, client: "claude-code") }
        } catch {
            guard modelEpochs[source, default: 0] == epoch else { return }
            modelErrors[source] = "模型列表暂时无法更新。" + (availableModels(source).isEmpty ? "" : "仍可选择已保存的模型。")
                + " " + BridgeProcess.clean(error.localizedDescription)
        }
    }

    private func preferredModel(_ source: String, client: String) -> String {
        let available = availableModels(source).filter { client == "codex" ? $0.codex : $0.claude }
        let remembered = defaults.string(forKey: "model.\(client).\(source)") ?? ""
        return available.contains { $0.id == remembered } ? remembered : available.first?.id ?? ""
    }

    func selectSource(_ source: String, client: String) {
        let oldSource = client == "codex" ? codexSource : claudeSource
        let oldModel = client == "codex" ? selectedCodex : selectedClaude
        if !oldModel.isEmpty { defaults.set(oldModel, forKey: "model.\(client).\(oldSource)") }
        if client == "codex" { codexSource = source; selectedCodex = preferredModel(source, client: client) }
        else { claudeSource = source; selectedClaude = preferredModel(source, client: client) }
    }
}
