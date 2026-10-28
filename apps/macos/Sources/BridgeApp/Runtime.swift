import Foundation
import CryptoKit
import Darwin
import BridgeCore

struct AppFailure: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}

final class Runtime {
    let root: URL
    let directory: URL
    let executable: URL
    private var lockFD: Int32 = -1

    init(root explicitRoot: URL? = nil, resources explicitResources: URL? = nil) throws {
        let fm = FileManager.default
        if let explicitRoot {
            root = explicitRoot
        } else if let override = ProcessInfo.processInfo.environment["COPILOT_BRIDGE_APP_HOME"] {
            root = URL(fileURLWithPath: override, isDirectory: true)
        } else {
            root = try fm.url(for: .applicationSupportDirectory, in: .userDomainMask,
                              appropriateFor: nil, create: true).appendingPathComponent("Copilot Bridge")
        }
        directory = root.appendingPathComponent("runtime")
        executable = directory.appendingPathComponent("copilot-bridge")
        try fm.createDirectory(at: directory, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        lockFD = Darwin.open(root.appendingPathComponent("app.lock").path, O_CREAT | O_RDWR, 0o600)
        guard lockFD >= 0, flock(lockFD, LOCK_EX | LOCK_NB) == 0 else {
            throw AppFailure(message: "Copilot Bridge 已在运行。请从 Dock 或菜单栏打开已有窗口。")
        }
        guard let resources = explicitResources ?? Bundle.main.resourceURL else { throw AppFailure(message: "应用资源不可用。") }
        let source = resources.appendingPathComponent("bridge")
        let sourceBinary = source.appendingPathComponent("copilot-bridge")
        guard fm.isExecutableFile(atPath: sourceBinary.path) else {
            throw AppFailure(message: "缺少内置 bridge。请运行 scripts/build-macos-app.sh 构建完整应用。")
        }
        let digest = SHA256.hash(data: try Data(contentsOf: sourceBinary)).map { String(format: "%02x", $0) }.joined()
        let marker = directory.appendingPathComponent("app-build")
        if (try? String(contentsOf: marker, encoding: .utf8)) != digest || !fm.fileExists(atPath: executable.path) {
            for name in ["copilot-bridge", "copilot-updater"] {
                let bytes = try Data(contentsOf: source.appendingPathComponent(name))
                let target = directory.appendingPathComponent(name)
                try bytes.write(to: target, options: .atomic)
                try fm.setAttributes([.posixPermissions: 0o700], ofItemAtPath: target.path)
            }
            try digest.write(to: marker, atomically: true, encoding: .utf8)
        }
        // Desktop owns this isolated runtime config; never copy a user's credential files.
        let settings = try Data(contentsOf: source.appendingPathComponent("appsettings.json"))
        try settings.write(to: directory.appendingPathComponent("appsettings.json"), options: .atomic)
    }

    deinit { if lockFD >= 0 { flock(lockFD, LOCK_UN); Darwin.close(lockFD) } }

    func environment(mode: ProxyMode, proxy: String) -> [String: String] {
        var env = ProcessInfo.processInfo.environment
        env["COPILOT_BRIDGE_DESKTOP"] = "1"
        env["COPILOT_BRIDGE_ACCOUNTS_FILE"] = root.appendingPathComponent("accounts.json").path
        let sourceFile = root.appendingPathComponent("sources.json")
        if FileManager.default.fileExists(atPath: sourceFile.path) { env["COPILOT_BRIDGE_SOURCES_FILE"] = sourceFile.path }
        else { env.removeValue(forKey: "COPILOT_BRIDGE_SOURCES_FILE") }
        env["COPILOT_BRIDGE_PROXY_MODE"] = mode.rawValue
        if mode == .custom { env["COPILOT_BRIDGE_PROXY_URL"] = proxy }
        else { env.removeValue(forKey: "COPILOT_BRIDGE_PROXY_URL") }
        // Azure CLI inherits the same explicit proxy choice as the HTTP transport.
        let proxyKeys = ["HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"]
        if mode == .direct { for key in proxyKeys { env.removeValue(forKey: key) } }
        if mode == .custom { for key in proxyKeys { env[key] = proxy } }
        // Local client traffic must never leave the loopback interface.
        env["NO_PROXY"] = "localhost,127.0.0.1,::1"
        env["no_proxy"] = "localhost,127.0.0.1,::1"
        return env
    }
}

/// Both pipes are drained concurrently. Cancellation targets only this Process instance.
final class BridgeProcess: @unchecked Sendable {
    private let process = Process()
    private let output = Pipe()
    private let errors = Pipe()
    private var input: Pipe?
    private let queue = DispatchQueue(label: "bridge.process.output")
    private let readers = DispatchGroup()
    private var buffer = JSONLineBuffer()
    private var errorText = ""
    private var lastReply: Reply?
    private var completion: ((Result<Reply?, Error>) -> Void)?
    private var onReply: ((Reply) -> Void)?
    private var onText: ((String) -> Void)?
    private var timer: DispatchWorkItem?
    private var timedOut = false
    var running: Bool { process.isRunning }

    func start(runtime: Runtime, arguments: [String], environment: [String: String], timeout: TimeInterval?, inputData: Data? = nil,
               onReply: @escaping (Reply) -> Void = { _ in }, onText: @escaping (String) -> Void = { _ in },
               completion: @escaping (Result<Reply?, Error>) -> Void) throws {
        try start(executable: runtime.executable, directory: runtime.directory,
                  arguments: arguments, environment: environment, timeout: timeout, inputData: inputData,
                  onReply: onReply, onText: onText, completion: completion)
    }

    func start(executable: URL, directory: URL, arguments: [String], environment: [String: String], timeout: TimeInterval?, inputData: Data? = nil,
               onReply: @escaping (Reply) -> Void = { _ in }, onText: @escaping (String) -> Void = { _ in },
               completion: @escaping (Result<Reply?, Error>) -> Void) throws {
        self.onReply = onReply; self.onText = onText; self.completion = completion
        process.executableURL = executable
        process.arguments = arguments
        process.currentDirectoryURL = directory
        process.environment = environment
        process.standardOutput = output; process.standardError = errors
        if inputData != nil { input = Pipe(); process.standardInput = input }
        else { process.standardInput = FileHandle.nullDevice }
        process.terminationHandler = { [weak self] process in
            guard let self else { return }
            self.readers.notify(queue: self.queue) {
                self.timer?.cancel()
                let result: Result<Reply?, Error>
                if self.timedOut { result = .failure(AppFailure(message: "操作超时，请检查网络或代理后重试。")) }
                else if process.terminationStatus != 0 || self.lastReply?.ok == false {
                    result = .failure(AppFailure(message: self.lastReply?.error ?? Self.clean(self.errorText)))
                } else { result = .success(self.lastReply) }
                self.completion?(result)
                self.completion = nil
            }
        }
        // Enter before launch: a very short-lived child may exit immediately.
        readers.enter(); readers.enter()
        do { try process.run() }
        catch { readers.leave(); readers.leave(); self.completion = nil; throw error }
        if let inputData, let input {
            DispatchQueue.global(qos: .utility).async {
                try? input.fileHandleForWriting.write(contentsOf: inputData)
                try? input.fileHandleForWriting.close()
            }
        }
        for (handle, isOutput) in [(output.fileHandleForReading, true), (errors.fileHandleForReading, false)] {
            DispatchQueue.global(qos: .utility).async { [self] in
                defer { readers.leave() }
                var bytes = [UInt8](repeating: 0, count: 8192)
                while true {
                    // POSIX read returns available pipe bytes immediately. Foundation's
                    // read(upToCount:) can wait to fill its buffer and hide device codes.
                    let count = Darwin.read(handle.fileDescriptor, &bytes, bytes.count)
                    if count < 0 && errno == EINTR { continue }
                    if count <= 0 { break }
                    let data = Data(bytes.prefix(count))
                    queue.async { [self] in
                        if isOutput { consume(data) }
                        else {
                            let text = String(decoding: data, as: UTF8.self)
                            errorText = String((errorText + text).suffix(16_384))
                            onText(text)
                        }
                    }
                }
            }
        }
        if let timeout {
            let timer = DispatchWorkItem { [weak self] in
                guard let self, self.process.isRunning else { return }
                self.timedOut = true
                self.stop()
            }
            self.timer = timer
            queue.asyncAfter(deadline: .now() + timeout, execute: timer)
        }
    }

    private func consume(_ data: Data) {
        for line in buffer.append(data) {
            if let reply = try? Reply.decode(line) { lastReply = reply; onReply?(reply) }
            else { onText?(String(decoding: line, as: UTF8.self)) }
        }
    }

    func stop() {
        guard process.isRunning else { return }
        process.interrupt()
        queue.asyncAfter(deadline: .now() + 3) { [weak self] in
            guard let self, self.process.isRunning else { return }
            self.process.terminate()
        }
    }

    static func clean(_ text: String) -> String {
        let plain = text.replacingOccurrences(of: "\u{001B}\\[[0-9;]*m", with: "", options: .regularExpression)
            .replacingOccurrences(of: "(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|sk-[A-Za-z0-9_-]+|(?i:Bearer)\\s+\\S+)",
                                  with: "[redacted]", options: .regularExpression)
        return plain.isEmpty ? "Bridge 未能完成操作，请查看错误日志。" : String(plain.suffix(3000))
    }
}
