// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "CopilotBridgeMac",
    platforms: [.macOS(.v14)],
    products: [.executable(name: "CopilotBridgeMac", targets: ["BridgeApp"])],
    targets: [
        .systemLibrary(name: "CSQLite", path: "Sources/CSQLite"),
        .target(name: "LocalSessions", dependencies: ["CSQLite"]),
        .target(name: "BridgeCore", dependencies: ["CSQLite"]),
        .executableTarget(name: "BridgeApp", dependencies: ["BridgeCore", "LocalSessions"]),
        .executableTarget(name: "LocalSessionChecks", dependencies: ["LocalSessions", "CSQLite"], path: "Tests/LocalSessionChecks"),
        .executableTarget(name: "BridgeCoreChecks", dependencies: ["BridgeCore"], path: "Tests/BridgeCoreTests"),
    ]
)
