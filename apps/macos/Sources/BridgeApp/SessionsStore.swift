import Foundation
import LocalSessions

@MainActor final class SessionsStore: ObservableObject {
    @Published var agent: SessionAgent = .codex
    @Published var search = ""
    @Published var sessions: [LocalSession] = []
    @Published var hasMore = false
    @Published var selected: LocalSession?
    @Published var items: [SessionItem] = []
    @Published var older: SessionCursor?
    @Published var notice: String?
    @Published var error: String?
    @Published var recordedStatus: String?
    @Published var loading = false
    @Published var reading = false
    @Published var analysis: String?
    @Published var analysisAttribution: String?
    @Published var analyzing = false
    @Published var question = "这个会话在做什么？已经完成了什么，接下来该做什么？"
    private var listTask: Task<Void, Never>?
    private var readTask: Task<Void, Never>?
    private var analysisTask: Task<Void, Never>?
    private var listEpoch = UUID(), detailEpoch = UUID()

    var root: URL {
        let setting = UserDefaults.standard.string(forKey: "sessionRoot.\(agent.rawValue)")
        let env = ProcessInfo.processInfo.environment[agent == .codex ? "CODEX_HOME" : "CLAUDE_CONFIG_DIR"]
        return URL(fileURLWithPath: setting ?? env ?? FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(agent == .codex ? ".codex" : ".claude").path)
    }
    func setRoot(_ root: URL?) {
        UserDefaults.standard.set(root?.path, forKey: "sessionRoot.\(agent.rawValue)")
        reload()
    }
    func reload(more: Bool = false) {
        listTask?.cancel(); listEpoch = UUID()
        let epoch = listEpoch, reader = SessionReader(root: root, agent: agent), search = search
        let offset = more ? sessions.count : 0
        if !more { select(nil); sessions = []; hasMore = false }
        loading = true; error = nil; notice = nil
        listTask = Task {
            let work = Task.detached(priority: .utility) { try reader.catalog(search: search, offset: offset) }
            let result = await withTaskCancellationHandler { await work.result } onCancel: { work.cancel() }
            guard !Task.isCancelled, epoch == listEpoch else { return }
            loading = false
            switch result {
            case .success(let value):
                var seen = Set<String>()
                sessions = (more ? sessions + value.sessions : value.sessions).filter { seen.insert($0.id).inserted }
                hasMore = value.hasMore; notice = value.notice
            case .failure(let failure): error = failure.localizedDescription
            }
        }
    }
    func select(_ session: LocalSession?) {
        readTask?.cancel(); analysisTask?.cancel(); detailEpoch = UUID()
        selected = session; items = []; older = nil; recordedStatus = nil; analysis = nil; analysisAttribution = nil
        analyzing = false; reading = false; error = nil; notice = nil
        if session != nil { read() }
    }
    func read(more: Bool = false) {
        guard let selected else { return }
        readTask?.cancel()
        let epoch = detailEpoch, reader = SessionReader(root: root, agent: agent), cursor = more ? older : nil
        reading = true; error = nil
        readTask = Task {
            let work = Task.detached(priority: .utility) { try reader.page(selected, before: cursor) }
            let result = await withTaskCancellationHandler { await work.result } onCancel: { work.cancel() }
            guard !Task.isCancelled, epoch == detailEpoch else { return }
            reading = false
            switch result {
            case .success(let page):
                var seen = Set<String>()
                let existing = Set(items.map(\.id))
                items = (more ? page.items.filter { !existing.contains($0.id) } + items : page.items).filter { seen.insert($0.id).inserted }
                older = page.olderCursor; notice = page.notice
                analysis = nil; analysisAttribution = nil
                if !more { recordedStatus = page.recordedStatus }
            case .failure(let failure): error = failure.localizedDescription
            }
        }
    }
    var analysisInput: SessionAnalysisInput? {
        selected.map { SessionAnalysisInput(session: $0, items: items, hasOlder: older != nil) }
    }
    func cancelAnalysis() { analysisTask?.cancel(); analyzing = false }
    func analyze(source: String, sourceName: String, model: String, port: Int) {
        guard let input = analysisInput, !items.isEmpty, !model.isEmpty, (1...65535).contains(port),
              source.range(of: "^[a-z][a-z0-9-]{0,62}$", options: .regularExpression) != nil else { return }
        analysisTask?.cancel()
        let epoch = detailEpoch, question = question
        let prefix = source == "copilot" ? "" : "/sources/\(source)"
        guard let url = URL(string: "http://127.0.0.1:\(port)\(prefix)/codex/responses") else { return }
        analyzing = true; analysis = nil; error = nil
        analysisTask = Task {
            do {
                var request = URLRequest(url: url); request.httpMethod = "POST"; request.timeoutInterval = 180
                request.setValue("application/json", forHTTPHeaderField: "Content-Type")
                request.httpBody = try input.request(model: model, question: question)
                let config = URLSessionConfiguration.ephemeral
                config.connectionProxyDictionary = [:]; config.timeoutIntervalForResource = 180
                // No redirects: transcripts must stay on the selected loopback route.
                let session = URLSession(configuration: config, delegate: NoSessionRedirect(), delegateQueue: nil)
                defer { session.invalidateAndCancel() }
                let (data, response) = try await session.data(for: request)
                guard let response = response as? HTTPURLResponse, (200..<300).contains(response.statusCode) else {
                    throw AppFailure(message: "会话分析请求失败（HTTP \((response as? HTTPURLResponse)?.statusCode ?? 0)）。请查看 Bridge 错误页。")
                }
                let text = try SessionAnalysisInput.response(data)
                guard !Task.isCancelled, epoch == detailEpoch else { return }
                analysis = text; analysisAttribution = "\(sourceName) · \(model)\n\(input.coverage)"
            } catch {
                guard !Task.isCancelled, epoch == detailEpoch else { return }
                self.error = error.localizedDescription
            }
            if epoch == detailEpoch { analyzing = false }
        }
    }
}

private final class NoSessionRedirect: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) { completionHandler(nil) }
}
