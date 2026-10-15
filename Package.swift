// swift-tools-version: 5.10
import PackageDescription

let package = Package(
    name: "MoleStudio",
    defaultLocalization: "zh-Hans",
    platforms: [.macOS(.v13)],
    products: [.executable(name: "MoleStudio", targets: ["MoleStudio"])],
    targets: [
        .executableTarget(
            name: "MoleStudio",
            path: "Sources/MoleStudio",
            linkerSettings: [
                .linkedFramework("Cocoa"),
                .linkedFramework("WebKit"),
                .linkedFramework("SwiftUI"),
                .linkedFramework("IOKit")
            ]
        ),
        .testTarget(
            name: "MoleStudioTests",
            dependencies: ["MoleStudio"],
            path: "Tests/MoleStudioTests"
        )
    ]
)
