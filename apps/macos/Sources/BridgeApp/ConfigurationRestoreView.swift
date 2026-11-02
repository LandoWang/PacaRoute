import SwiftUI
import BridgeCore

struct RestoreConfigurationButton: View {
    @EnvironmentObject var store: AppStore
    let clientID: String
    @State private var presented = false

    var body: some View {
        Button("还原配置…") { presented = true }
            .disabled(store.busy != nil || store.runtime == nil || store.signingIn)
            .sheet(isPresented: $presented) {
                ConfigurationRestoreView(clientID: clientID).environmentObject(store)
            }
    }
}

struct ConfigurationRestoreView: View {
    @EnvironmentObject var store: AppStore
    @Environment(\.dismiss) private var dismiss
    let clientID: String
    @State private var backups: [ConfigurationBackup] = []
    @State private var selected = ""
    @State private var target = ""
    @State private var preview: Reply?
    @State private var loading = false
    @State private var error: String?

    private var chosen: ConfigurationBackup? { backups.first { $0.id == selected } }

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text("还原 \(clientID == "codex" ? "Codex" : "Claude Code") 配置")
                .font(.title2).fontWeight(.semibold)
            Text(target).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
            Text("选择接入 Bridge 前的备份，即可恢复原来的连接。还原会替换整个配置文件，之后新增的设置也会撤销；当前文件会另存为备份。")
                .font(.callout)
            if loading { ProgressView().controlSize(.small) }
            if backups.isEmpty && !loading {
                ContentUnavailableView("没有可还原的备份", systemImage: "clock.arrow.circlepath",
                    description: Text("应用 Agent 配置后会自动保存备份。早期版本未记录的状态无法自动找回。"))
                    .frame(height: 130)
            } else if !backups.isEmpty {
                Picker("还原到", selection: $selected) {
                    ForEach(backups) { backup in
                        Text(label(backup)).tag(backup.id).disabled(!backup.valid)
                    }
                }.disabled(loading)
                    .onChange(of: selected) { preview = nil; error = nil }
                if chosen?.bridgeConfigured == true {
                    Label("这份备份仍使用 Bridge；若要解除连接，请选择更早的备份。", systemImage: "info.circle")
                        .font(.caption).foregroundStyle(.orange)
                }
            }
            if let preview {
                VStack(alignment: .leading, spacing: 10) {
                    ForEach(Array((preview.changes ?? []).enumerated()), id: \.offset) { _, change in
                        Text(change).font(.callout).textSelection(.enabled)
                    }
                }.padding().frame(maxWidth: .infinity, alignment: .leading)
                    .background(.quaternary, in: RoundedRectangle(cornerRadius: 10))
            }
            if let error { Text(error).font(.callout).foregroundStyle(.red).textSelection(.enabled) }
            HStack {
                Button("取消", role: .cancel) { dismiss() }.keyboardShortcut(.cancelAction).disabled(loading)
                Button("刷新备份") { Task { await load() } }.disabled(loading)
                Spacer()
                if let preview {
                    Button("备份当前配置并还原", role: .destructive) { Task { await restore(preview) } }
                        .buttonStyle(.borderedProminent).disabled(loading || chosen?.valid != true)
                } else {
                    Button("预览还原") { Task { await prepare() } }
                        .buttonStyle(.borderedProminent).disabled(loading || chosen?.valid != true)
                }
            }
        }.padding(28).frame(width: 660)
            .interactiveDismissDisabled(loading)
            .task { await load() }
    }

    private func label(_ backup: ConfigurationBackup) -> String {
        let formatter = ISO8601DateFormatter()
        let stamp = formatter.date(from: backup.createdAt)?.formatted(date: .abbreviated, time: .standard) ?? backup.createdAt
        let state = !backup.valid ? "备份无效" : backup.missingFile ? "原先没有配置文件"
            : backup.bridgeConfigured ? "仍连接 Bridge" : "未连接 Bridge"
        return "\(stamp) · \(state)"
    }

    private func load() async {
        guard store.busy == nil else { return }
        loading = true; store.busy = "正在读取配置备份"; error = nil; preview = nil
        defer { loading = false; store.busy = nil }
        do {
            let reply = try await store.execute(["desktop", "config-backups", "--client", clientID])
            target = reply.path ?? ""; backups = reply.backups ?? []
            selected = backups.first { $0.valid && !$0.bridgeConfigured }?.id
                ?? backups.first { $0.valid }?.id ?? ""
        } catch { self.error = error.localizedDescription }
    }

    private func prepare() async {
        guard store.busy == nil else { return }
        loading = true; store.busy = "正在预览还原"; error = nil
        defer { loading = false; store.busy = nil }
        do {
            preview = try await store.execute(["desktop", "restore-config", "--client", clientID, "--backup", selected])
        } catch { self.error = error.localizedDescription }
    }

    private func restore(_ plan: Reply) async {
        guard store.busy == nil, let fingerprint = plan.fingerprint else { return }
        loading = true; store.busy = "正在还原配置"; error = nil
        do {
            let result = try await store.execute(["desktop", "restore-config", "--client", clientID,
                "--backup", selected, "--apply", "--fingerprint", fingerprint])
            store.notice = "配置已还原。请新建客户端会话以使用恢复后的配置。"
                + (result.backupPath.map { "\n还原前的配置已备份：\($0)" } ?? "")
            store.busy = nil; loading = false
            await store.refresh()
            dismiss()
        } catch {
            store.busy = nil; loading = false; preview = nil
            self.error = error.localizedDescription
        }
    }
}
