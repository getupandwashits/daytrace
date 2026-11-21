// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "DayTrace",
    platforms: [.macOS("15.0")],
    products: [
        .executable(name: "DayTrace", targets: ["DayTraceApp"]),
        .executable(name: "daytrace-agent", targets: ["DayTraceAgent"]),
        .library(name: "DayTraceCore", targets: ["DayTraceCore"]),
    ],
    targets: [
        .target(name: "DayTraceCore"),
        .executableTarget(name: "DayTraceApp", dependencies: ["DayTraceCore"]),
        .executableTarget(name: "DayTraceAgent", dependencies: ["DayTraceCore"]),
        .testTarget(name: "DayTraceCoreTests", dependencies: ["DayTraceCore"]),
    ]
)
