import AppKit
import SwiftUI
import BridgeCore
import LocalSessions

struct SessionsView: View {
    @EnvironmentObject var app: AppStore
    @ObservedObject var store: SessionsStore
    @State private var section = "概览"
    @State private var source = "copilot"
    @State private var model = ""
    @State private var historySearch = ""
    @State private var preview = false
    private var models: [BridgeModel] { app.availableModels(source).filter(\.codex) }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                Picker("客户端", selection: $store.agent) { ForEach(SessionAgent.allCases) { Text($0.rawValue).tag($0) } }
                    .pickerStyle(.segmented).frame(width: 235)
                    .onChange(of: store.agent) { _, _ in store.reload() }
                Spacer()
                Menu("历史目录") {
                    Button("选择 \(store.agent.rawValue) 目录…") { chooseRoot() }
                    Button("恢复默认目录") { store.setRoot(nil) }
                }
                Button { store.reload() } label: { Label("刷新", systemImage: "arrow.clockwise") }.disabled(store.loading || store.analyzing)
            }
            Text(store.root.path).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
            if let error = store.error { Label(error, systemImage: "exclamationmark.triangle").font(.callout).foregroundStyle(.red).textSelection(.enabled) }
            if let notice = store.notice { Text(notice).font(.caption).foregroundStyle(.secondary) }
            HSplitView {
                VStack(spacing: 10) {
                    TextField("搜索标题或项目", text: $store.search).textFieldStyle(.roundedBorder).onSubmit { store.reload() }
                    HStack {
                        Text("\(store.sessions.count) 个会话").font(.caption).foregroundStyle(.secondary)
                        Spacer()
                        if store.loading { ProgressView().controlSize(.small) }
                        Button("搜索") { store.reload() }.controlSize(.small)
                    }
                    List(selection: Binding(get: { store.selected?.id }, set: { id in
                        store.select(store.sessions.first { $0.id == id }); historySearch = ""
                    })) {
                        ForEach(store.sessions) { session in
                            VStack(alignment: .leading, spacing: 5) {
                                Text(session.title).font(.subheadline).lineLimit(2)
                                Text(URL(fileURLWithPath: session.cwd).lastPathComponent).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                                HStack {
                                    Text(session.updated, style: .date)
                                    if session.archived { Text("已归档") }
                                }.font(.caption2).foregroundStyle(.secondary)
                            }.padding(.vertical, 5).tag(session.id)
                        }
                    }.listStyle(.inset).clipShape(RoundedRectangle(cornerRadius: 10))
                    if store.hasMore { Button("加载更多会话") { store.reload(more: true) }.disabled(store.loading) }
                }.frame(minWidth: 200, idealWidth: 240, maxWidth: 280).padding(.trailing, 10)
                ScrollView {
                    VStack(alignment: .leading, spacing: 18) {
                        if let selected = store.selected {
                            Text(selected.title).font(.title3).fontWeight(.semibold).textSelection(.enabled)
                            Picker("会话内容", selection: $section) {
                                ForEach(["概览", "历史", "理解"], id: \.self) { Text($0) }
                            }.pickerStyle(.segmented)
                            if store.reading { ProgressView("读取本地历史…").font(.caption) }
                            switch section {
                            case "历史": history
                            case "理解": understanding
                            default: overview(selected)
                            }
                        } else {
                            ContentUnavailableView(store.sessions.isEmpty ? "暂无会话" : "选择一个会话", systemImage: "bubble.left.and.bubble.right",
                                description: Text("读取本地保存的聊天和工具记录。浏览历史不需要启动 Bridge。"))
                        }
                    }.padding(.leading, 14).padding(.vertical, 5).frame(maxWidth: .infinity, alignment: .leading)
                }.frame(minWidth: 350)
            }.frame(height: 630)
            Text("历史记录按需读取，不会恢复或控制原会话。自动遮盖常见密钥格式，分析前仍可预览发送内容。")
                .font(.caption).foregroundStyle(.secondary)
        }
        .task {
            source = app.codexSource; selectModel()
            if store.sessions.isEmpty && !store.loading { store.reload() }
        }
        .sheet(isPresented: $preview) {
            VStack(alignment: .leading, spacing: 14) {
                Text("将发送到：\(app.sourceName(source)) · \(model)").font(.headline)
                Text(store.analysisInput?.coverage ?? "").font(.caption)
                ScrollView { Text(store.analysisInput?.evidence ?? "").font(.system(.caption, design: .monospaced)).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading) }
                HStack { Spacer(); Button("关闭") { preview = false }.keyboardShortcut(.cancelAction) }
            }.padding(24).frame(width: 760, height: 560)
        }
    }

    private func overview(_ session: LocalSession) -> some View {
        VStack(alignment: .leading, spacing: 16) {
            Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 9) {
                metadata("客户端", session.agent.rawValue)
                metadata("项目", session.cwd)
                metadata("记录的来源", session.providerLabel)
                metadata("记录的模型", session.model ?? store.items.last(where: { $0.model != nil })?.model ?? "未记录")
                if let tokens = session.tokens { metadata("客户端 token", tokens.formatted() + "（索引记录值）") }
                metadata("最后一轮", statusLabel)
            }.font(.caption).textSelection(.enabled)
            Text("这些是历史记录，不代表客户端当前进程状态，也不保证已读取 Agent 页的新配置。")
                .font(.caption).foregroundStyle(.secondary)
            Divider()
            HStack {
                Text("已加载 \(store.items.count) 条记录").font(.subheadline).fontWeight(.medium)
                Spacer()
                Button("刷新历史") { store.read() }.disabled(store.reading || store.analyzing)
            }
            Text("对话 \(store.items.filter { $0.kind == .user || $0.kind == .assistant }.count) · 工具 \(store.items.filter { $0.kind == .tool || $0.kind == .result }.count) · 错误 \(store.items.filter { $0.kind == .error }.count)")
                .font(.caption).foregroundStyle(.secondary)
            if store.older != nil { Text("当前为最近一段历史，可在历史页加载更早记录。").font(.caption).foregroundStyle(.secondary) }
            excerpt("最近的用户消息", item: store.items.last { $0.kind == .user })
            excerpt("最近的回复（原文摘录）", item: store.items.last { $0.kind == .assistant })
            excerpt("最近的错误", item: store.items.last { $0.kind == .error })
            Button("理解这个会话") { section = "理解" }.auroraProminent().disabled(store.items.isEmpty)
        }
    }

    private var history: some View {
        VStack(alignment: .leading, spacing: 14) {
            TextField("搜索已加载的聊天和工具记录", text: $historySearch).textFieldStyle(.roundedBorder)
            if store.older != nil { Button("加载更早记录") { store.read(more: true) }.disabled(store.reading || store.analyzing) }
            LazyVStack(alignment: .leading, spacing: 14) {
                ForEach(Array(store.items.enumerated()), id: \.element.id) { index, item in
                    if historySearch.isEmpty || (item.title + item.text).localizedCaseInsensitiveContains(historySearch) {
                        VStack(alignment: .leading, spacing: 7) {
                            HStack {
                                Text("E\(index + 1)").font(.system(.caption2, design: .monospaced)).foregroundStyle(.secondary)
                                Text(item.title).font(.caption).fontWeight(.semibold)
                                Spacer()
                                if let date = item.timestamp { Text(date, style: .time).font(.caption2).foregroundStyle(.secondary) }
                            }
                            if item.kind == .user || item.kind == .assistant {
                                Text(item.text).font(.callout).textSelection(.enabled)
                            } else {
                                DisclosureGroup {
                                    Text(item.text).font(.system(.caption, design: .monospaced)).textSelection(.enabled)
                                } label: { Text(String(item.text.prefix(110))).font(.caption).lineLimit(2).foregroundStyle(.secondary) }
                            }
                        }.padding(12).frame(maxWidth: .infinity, alignment: .leading)
                            .background(item.kind == .error ? Color.red.opacity(0.06) : AuroraTheme.surface, in: RoundedRectangle(cornerRadius: 10))
                    }
                }
            }
        }
    }

    private var understanding: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("分析目标、决策、完成证据和待办；也可以针对历史提问。").font(.callout)
            Picker("分析来源", selection: $source) {
                Text("GitHub Copilot").tag("copilot")
                ForEach(app.sources) { Text(app.sourceName($0.id)).tag($0.id) }
            }.onChange(of: source) { _, _ in selectModel() }.disabled(store.analyzing)
            Picker("分析模型", selection: $model) {
                if models.isEmpty { Text("请先加载可用模型").tag("") }
                ForEach(models) { Text($0.name).tag($0.id) }
            }.disabled(store.analyzing)
            if models.isEmpty {
                Button("加载模型") { Task { await app.loadModels(for: source, force: true); selectModel() } }
                    .disabled(app.loadingModelSources.contains(source))
            }
            TextField("想了解什么", text: $store.question, axis: .vertical).textFieldStyle(.roundedBorder).lineLimit(2...5).disabled(store.analyzing)
            Text(store.analysisInput?.coverage ?? "").font(.caption).foregroundStyle(.secondary)
            Text("点击分析会将预览中的历史发送到所选模型来源，并产生模型用量。分析结果仅保留在当前窗口。")
                .font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("预览发送内容") { preview = true }.disabled(store.items.isEmpty)
                Spacer()
                if store.analyzing {
                    ProgressView().controlSize(.small)
                    Button("取消") { store.cancelAnalysis() }
                } else {
                    Button("分析会话") { store.analyze(source: source, sourceName: app.sourceName(source), model: model, port: app.appliedPort) }
                        .auroraProminent().disabled(!app.running || store.items.isEmpty || model.isEmpty || store.reading)
                }
            }
            if !app.running { Text("启动 Bridge 后即可分析；本地浏览仍可使用。").font(.caption).foregroundStyle(.secondary) }
            if let result = store.analysis {
                Divider()
                Text(store.analysisAttribution ?? "").font(.caption).foregroundStyle(.secondary)
                Text("引用 [E编号] 对应历史页中的记录；模型判断可能有误，请核对原文。").font(.caption).foregroundStyle(.secondary)
                Text(result).textSelection(.enabled).font(.callout)
                Button("复制分析") { NSPasteboard.general.clearContents(); NSPasteboard.general.setString(result, forType: .string) }
            }
        }
    }
    private var statusLabel: String {
        switch store.recordedStatus {
        case "completed": return "已结束（历史记录）"
        case "failed": return "失败（历史记录）"
        case "interrupted": return "已中断（历史记录）"
        case "inProgress": return "未记录结束；当前状态未知"
        default: return "未记录"
        }
    }
    private func metadata(_ label: String, _ text: String) -> some View {
        GridRow { Text(label).foregroundStyle(.secondary); Text(text).fixedSize(horizontal: false, vertical: true) }
    }
    @ViewBuilder private func excerpt(_ label: String, item: SessionItem?) -> some View {
        if let item {
            VStack(alignment: .leading, spacing: 6) {
                Text(label).font(.caption).foregroundStyle(.secondary)
                Text(String(item.text.prefix(700)) + (item.text.count > 700 ? "…" : "")).font(.callout).textSelection(.enabled)
            }
        }
    }
    private func selectModel() {
        model = models.contains { $0.id == app.selectedCodex } ? app.selectedCodex : models.first?.id ?? ""
    }
    private func chooseRoot() {
        let panel = NSOpenPanel(); panel.canChooseDirectories = true; panel.canChooseFiles = false
        panel.showsHiddenFiles = true; panel.allowsMultipleSelection = false; panel.directoryURL = store.root
        panel.message = "选择包含 sessions / projects 或会话数据库的客户端目录"
        if panel.runModal() == .OK, let url = panel.url { store.setRoot(url) }
    }
}
