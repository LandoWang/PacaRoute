import Foundation
import BridgeCore

@main struct ProcessChecks {
    static func main() throws {
        let complete = DispatchSemaphore(value: 0)
        let code = DispatchSemaphore(value: 0)
        let child = BridgeProcess()
        let began = Date()
        var result: Result<Reply?, Error>?
        try child.start(executable: URL(fileURLWithPath: "/bin/sh"), directory: URL(fileURLWithPath: "/tmp"),
            arguments: ["-c", "printf '%s\\n' '{\"ok\":true,\"event\":\"device_code\",\"user_code\":\"TEST-1234\"}'; sleep 2; printf '%s\\n' '{\"ok\":true,\"event\":\"result\"}'"],
            environment: ProcessInfo.processInfo.environment, timeout: 10,
            onReply: { reply in if reply.event == "device_code" { code.signal() } },
            completion: { value in result = value; complete.signal() })
        guard code.wait(timeout: .now() + 1) == .success else { throw AppFailure(message: "Device code was buffered until process exit") }
        guard Date().timeIntervalSince(began) < 1.5 else { throw AppFailure(message: "Device code was delayed") }
        guard complete.wait(timeout: .now() + 5) == .success else { throw AppFailure(message: "Process did not complete") }
        guard case .success(let reply) = result, reply?.event == "result" else { throw AppFailure(message: "Final reply was lost") }

        let pipeFinished = DispatchSemaphore(value: 0)
        let pipeChild = BridgeProcess()
        var pipeOK = false
        try pipeChild.start(executable: URL(fileURLWithPath: "/bin/cat"), directory: URL(fileURLWithPath: "/tmp"),
            arguments: [], environment: ProcessInfo.processInfo.environment, timeout: 5,
            inputData: Data("{\"ok\":true,\"event\":\"result\"}\n".utf8),
            completion: { result in if case .success(let reply) = result { pipeOK = reply?.ok == true }; pipeFinished.signal() })
        guard pipeFinished.wait(timeout: .now() + 6) == .success && pipeOK else {
            throw AppFailure(message: "Owned stdin pipe did not deliver and close")
        }

        let cancelled = DispatchSemaphore(value: 0)
        let timeoutChild = BridgeProcess()
        var failed = false
        try timeoutChild.start(executable: URL(fileURLWithPath: "/bin/sleep"), directory: URL(fileURLWithPath: "/tmp"),
            arguments: ["30"], environment: ProcessInfo.processInfo.environment, timeout: 0.2,
            completion: { value in if case .failure = value { failed = true }; cancelled.signal() })
        guard cancelled.wait(timeout: .now() + 5) == .success, failed, !timeoutChild.running else {
            throw AppFailure(message: "Timed-out process was not reaped")
        }
        print("PASS: device codes stream before process exit; final JSON survives pipe draining; stdin delivers and closes; timeout terminates only the owned child")
    }
}
