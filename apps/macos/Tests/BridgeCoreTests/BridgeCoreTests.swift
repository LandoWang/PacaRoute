import Foundation
import BridgeCore

final class BridgeCoreTests {
    func testSplitUTF8AndPartialJSONAreNotLost() throws {
        var buffer = JSONLineBuffer()
        let bytes = Data("{\"ok\":true,\"event\":\"result\",\"error\":\"测试\"}\n".utf8)
        expectTrue(buffer.append(bytes.prefix(bytes.count - 5)).isEmpty)
        let lines = buffer.append(bytes.suffix(5))
        expectEqual(lines.count, 1)
        expectEqual(try Reply.decode(lines[0]).error, "测试")
    }

    func testQuotaMissingIsUnknownNotZero() throws {
        let reply = try Reply.decode(Data(#"{"ok":true,"event":"result","quota":{"quota_snapshots":{"premium_interactions":{"unlimited":false}}}}"#.utf8))
        expectNil(reply.quota?.quotaSnapshots?["premium_interactions"]?.available)
        expectNil(reply.quota?.quotaSnapshots?["premium_interactions"]?.percentRemaining)
    }

    func testEstimatesAndErrorsDoNotInflateUsageOrDoubleCountCache() {
        let json = """
        {"id":"a","timestamp":"2026-10-28T10:00:00.1234567+00:00","kind":"request","input_tokens":100,"output_tokens":20,"cache_read_tokens":80,"status":200}
        {"id":"b","timestamp":"2026-10-28T10:01:00Z","kind":"estimate","input_tokens":999,"status":200}
        {"id":"c","timestamp":"2026-10-28T10:02:00Z","kind":"error","error":"offline"}
        {"id":"d","timestamp":"2026-10-28T10:03:00Z","kind":"request","status":500}
        broken line
        {"id":"unfinished"
        """
        let events = ActivityReader.decode(Data(json.utf8))
        expectEqual(events.count, 4)
        let totals = UsageTotals(events)
        expectEqual(totals.requests, 2)
        expectEqual(totals.failures, 1)
        expectEqual(totals.input, 100)
        expectEqual(totals.output, 20)
        expectEqual(totals.cached, 80)
        expectEqual(totals.missingUsage, 1)
    }

    func testProxyValidationRefusesCredentialsAndInvalidDestinations() {
        for value in ["file:///tmp/x", "http://user:secret@localhost:7890", "http://localhost:7890/path", "localhost:7890", "http://localhost:99999"] {
            expectFalse(ProxyMode.custom.validate(value), value)
        }
        expectTrue(ProxyMode.custom.validate("http://127.0.0.1:7890"))
        expectTrue(ProxyMode.custom.validate("socks5://[::1]:1080"))
        expectTrue(ProxyMode.direct.validate(""))
    }
}

func expectTrue(_ value: Bool, _ message: String = "", file: StaticString = #file, line: UInt = #line) {
    guard value else { fatalError("Expected true: " + message, file: file, line: line) }
}
func expectFalse(_ value: Bool, _ message: String = "", file: StaticString = #file, line: UInt = #line) {
    guard !value else { fatalError("Expected false: " + message, file: file, line: line) }
}
func expectEqual<T: Equatable>(_ a: T, _ b: T, file: StaticString = #file, line: UInt = #line) {
    guard a == b else { fatalError("Values differ: \(a) vs \(b)", file: file, line: line) }
}
func expectNil<T>(_ value: T?, file: StaticString = #file, line: UInt = #line) {
    guard value == nil else { fatalError("Expected unknown, got \(String(describing: value))", file: file, line: line) }
}
@main struct ContractChecks {
    static func main() throws {
        let tests = BridgeCoreTests()
        try tests.testSplitUTF8AndPartialJSONAreNotLost()
        try tests.testQuotaMissingIsUnknownNotZero()
        tests.testEstimatesAndErrorsDoNotInflateUsageOrDoubleCountCache()
        tests.testProxyValidationRefusesCredentialsAndInvalidDestinations()
        print("Passed 4 Swift contract checks")
    }
}
