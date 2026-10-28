// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "CopilotBridgeMac",
    platforms: [.macOS(.v14)],
    products: [.executable(name: "CopilotBridgeMac", targets: ["BridgeApp"])],
    targets: [
        .target(name: "BridgeCore"),
        .executableTarget(name: "BridgeApp", dependencies: ["BridgeCore"]),
        .executableTarget(name: "BridgeCoreChecks", dependencies: ["BridgeCore"], path: "Tests/BridgeCoreTests"),
    ]
)
