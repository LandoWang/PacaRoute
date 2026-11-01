import SwiftUI
import BridgeCore

struct SourcePoolsSection: View {
    @EnvironmentObject var store: AppStore
    @State private var creating = false
    @State private var editing: SourcePool?
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                Label("来源池", systemImage: "arrow.triangle.branch").font(.headline)
                Spacer()
                Button("添加来源池", systemImage: "plus") { creating = true }.disabled(!store.ready || store.poolError != nil)
            }
            Text("按会话分配来源和模型。同一会话固定使用一个成员；各成员沿用自己的账号和 Proxy。")
                .font(.caption).foregroundStyle(.secondary)
            if let error = store.poolError {
                HStack { Text(error).foregroundStyle(.red); Button("重新读取") { store.reloadPools() } }
            }
            ForEach(store.sourcePools) { pool in
                Card {
                    HStack {
                        Text(pool.name).font(.headline); Spacer()
                        Text(pool.agentName).foregroundStyle(.secondary)
                        Button("编辑") { editing = pool }
                        Button("配置 Agent") { store.selectSource(pool.connectionID, client: pool.agent); store.page = .clients }
                    }
                    ForEach(pool.members) { member in
                        HStack {
                            Text(store.sourceName(member.sourceId) + " · " + member.model).lineLimit(2)
                            Spacer()
                            Text(proportion(member.weight, in: pool.members)).monospacedDigit().foregroundStyle(AuroraTheme.accent)
                        }.font(.callout)
                    }
                    Text(pool.failover ? "会话固定 · 故障时自动切换 · 修改权重无需重启" : "会话固定 · 故障时保持原来源")
                        .font(.caption).foregroundStyle(.secondary)
                }
            }
        }
        .sheet(isPresented: $creating) { SourcePoolEditor(original: nil).environmentObject(store) }
        .sheet(item: $editing) { SourcePoolEditor(original: $0).environmentObject(store) }
    }
    private func proportion(_ weight: Int, in members: [SourcePoolMember]) -> String {
        weight == 0 ? "仅保留已有会话" : (Double(weight) / Double(max(1, members.reduce(0) { $0 + $1.weight })))
            .formatted(.percent.precision(.fractionLength(0...1)))
    }
}

struct SourcePoolEditor: View {
    @EnvironmentObject var store: AppStore
    @Environment(\.dismiss) private var dismiss
    let original: SourcePool?
    @State private var id = ""
    @State private var name = ""
    @State private var agent = "codex"
    @State private var model = ""
    @State private var failover = true
    @State private var members: [SourcePoolMember] = []
    @State private var error: String?
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text(original == nil ? "添加来源池" : "编辑来源池").font(.title2).bold()
            HStack { TextField("名称", text: $name); TextField("来源池 ID", text: $id).disabled(original != nil) }
            HStack {
                Picker("Agent", selection: $agent) { Text("Codex").tag("codex"); Text("Claude Code").tag("claude-code") }.disabled(original != nil)
                TextField("Agent 显示的模型（留空使用首项）", text: $model).disabled(original != nil)
            }
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    ForEach($members) { $member in
                        HStack(alignment: .top) {
                            PoolMemberEditor(member: $member, agent: agent)
                            Button(role: .destructive) { members.removeAll { $0.id == member.id } } label: { Image(systemName: "minus.circle") }
                                .buttonStyle(.borderless).disabled(members.count <= 1)
                        }
                    }
                }
            }.frame(minHeight: 150, maxHeight: 310)
            Button("添加成员", systemImage: "plus") { addMember() }.disabled(members.count >= 32)
            Text("权重按比例换算，不必合计为 100。已有会话保持原分配；权重设为 0 时不接收新会话。")
                .font(.caption).foregroundStyle(.secondary)
            Toggle("来源故障或额度用完时，允许自动切换", isOn: $failover)
            Text("切换可能改变模型与上下文。已输出的回答不会拼接其他来源；仅有服务端历史引用的请求需要重新建立上下文。")
                .font(.caption).foregroundStyle(.secondary)
            if let error { Text(error).font(.caption).foregroundStyle(.red) }
            HStack {
                Button("取消") { dismiss() }.keyboardShortcut(.cancelAction)
                if let original { Button("删除来源池", role: .destructive) { do { try store.removePool(original); dismiss() } catch { self.error = error.localizedDescription } } }
                Spacer()
                Button("保存") { save() }.auroraProminent().keyboardShortcut(.defaultAction)
            }
        }
        .textFieldStyle(.roundedBorder).padding(28).frame(width: 790)
        .onAppear {
            if let original {
                id = original.id; name = original.name; agent = original.agent; model = original.model
                failover = original.failover; members = original.members
            } else { id = "balance-" + UUID().uuidString.prefix(8).lowercased(); addMember(); addMember() }
        }
        .onChange(of: agent) { _, _ in if original == nil { for index in members.indices { members[index].model = "" } } }
    }
    private func addMember() {
        let selected = agent == "codex" ? store.codexSource : store.claudeSource
        let preferred = selected.hasPrefix("pool:") ? "copilot" : selected
        let source = members.isEmpty ? preferred : (["copilot"] + store.sources.map(\.id))
            .first(where: { candidate in !members.contains(where: { $0.sourceId == candidate }) }) ?? preferred
        members.append(SourcePoolMember(id: "member-" + UUID().uuidString.prefix(8).lowercased(), sourceId: source, model: ""))
    }
    private func save() {
        do {
            if original == nil && store.sourcePools.contains(where: { $0.id == id }) { throw SourcePoolFailure("来源池 ID 已存在。") }
            let alias = model.isEmpty ? members.first?.model ?? "" : model
            try store.savePool(SourcePool(id: id, name: name, agent: agent, model: alias, failover: failover, members: members))
            dismiss()
        } catch { self.error = error.localizedDescription }
    }
}

private struct PoolMemberEditor: View {
    @EnvironmentObject var store: AppStore
    @Binding var member: SourcePoolMember
    let agent: String
    private var models: [BridgeModel] { store.availableModels(member.sourceId).filter { agent == "codex" ? $0.codex : $0.claude } }
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Picker("来源", selection: $member.sourceId) {
                    Text("GitHub Copilot").tag("copilot")
                    ForEach(store.sources) { Text(store.sourceName($0.id)).tag($0.id) }
                }.frame(maxWidth: .infinity)
                Text("权重").font(.caption)
                TextField("权重", value: $member.weight, format: .number).frame(width: 75)
            }
            Picker("模型", selection: $member.model) {
                Text("选择模型 / Deployment").tag("")
                if !member.model.isEmpty && !models.contains(where: { $0.id == member.model }) { Text(member.model + "（已保存）").tag(member.model) }
                ForEach(models) { Text($0.name + " · " + $0.id).tag($0.id) }
            }
        }
        .onChange(of: member.sourceId) { _, _ in member.model = "" }
        .task(id: member.sourceId + "/" + agent) {
            let source = member.sourceId
            await store.loadModels(for: source)
            if member.sourceId == source && member.model.isEmpty { member.model = models.first?.id ?? "" }
        }
    }
}
