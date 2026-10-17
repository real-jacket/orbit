import XCTest
@testable import MoleStudio

/// Guards the invariants the safety model depends on. These are cheap to assert and expensive to
/// regress: a protected tier that becomes selectable would let the UI delete hardware-level caches.
final class SafetyModelTests: XCTestCase {

    func testProtectedTierIsDistinctFromOtherTiers() {
        XCTAssertNotEqual(SafetyTier.protectedSystem, SafetyTier.safeRegenerable)
        XCTAssertNotEqual(SafetyTier.protectedSystem, SafetyTier.reviewOnly)
    }

    func testEveryTierHasBothLocalizations() {
        for tier in [SafetyTier.safeRegenerable, .reviewOnly, .protectedSystem] {
            XCTAssertFalse(tier.label(isZH: true).isEmpty, "\(tier) missing zh label")
            XCTAssertFalse(tier.label(isZH: false).isEmpty, "\(tier) missing en label")
        }
    }

    func testEveryModuleHasBothLocalizationsAndABadge() {
        for module in MoleModule.allCases {
            XCTAssertFalse(module.sidebarTitle(isZH: true).isEmpty, "\(module) missing zh sidebar title")
            XCTAssertFalse(module.sidebarTitle(isZH: false).isEmpty, "\(module) missing en sidebar title")
            XCTAssertFalse(module.headerTitle(isZH: true).isEmpty, "\(module) missing zh header")
            XCTAssertFalse(module.headerTitle(isZH: false).isEmpty, "\(module) missing en header")
            XCTAssertFalse(module.sidebarBadge.isEmpty, "\(module) missing badge")
            XCTAssertFalse(module.cliBadgeText.isEmpty, "\(module) missing CLI badge")
        }
    }

    func testModuleIdentifiersAreUnique() {
        let ids = MoleModule.allCases.map(\.id)
        XCTAssertEqual(Set(ids).count, ids.count, "duplicate MoleModule ids: \(ids)")
    }

    func testSafetyTierRawValuesAreStable() {
        // These strings are persisted in scan payloads; changing them breaks compatibility.
        XCTAssertEqual(SafetyTier.safeRegenerable.rawValue, "safe")
        XCTAssertEqual(SafetyTier.reviewOnly.rawValue, "review")
        XCTAssertEqual(SafetyTier.protectedSystem.rawValue, "protected")
    }

    @MainActor
    func testProtectedCleanItemsAreNotSelectable() {
        let vm = MoleViewModel()
        vm.cleanItems = [
            CleanCategoryItem(
                id: "protected",
                category: "保护项",
                name: "受保护目录",
                path: "/System",
                sizeGB: 1,
                itemCount: 1,
                filesCountText: "1",
                lastModifiedText: "现在",
                rebuildTimeText: "不可重建",
                cliCommand: "mo clean",
                impactDescription: "受保护",
                tier: .protectedSystem,
                isSelected: false,
                subpaths: []
            )
        ]

        vm.toggleCleanSelection(id: "protected")

        XCTAssertFalse(vm.cleanItems[0].isSelected)
    }

    @MainActor
    func testDryRunIsOnByDefault() {
        XCTAssertTrue(MoleViewModel().dryRunMode, "dry-run must default to true")
    }

    @MainActor
    func testTogglingAnUnprotectedItemFlipsSelection() throws {
        let vm = MoleViewModel()
        vm.cleanItems = [
            CleanCategoryItem(
                id: "cache",
                category: "缓存",
                name: "可重建缓存",
                path: "~/Library/Caches",
                sizeGB: 1,
                itemCount: 1,
                filesCountText: "1",
                lastModifiedText: "现在",
                rebuildTimeText: "可重建",
                cliCommand: "mo clean",
                impactDescription: "可重建",
                tier: .safeRegenerable,
                isSelected: true,
                subpaths: []
            )
        ]
        let item = try XCTUnwrap(vm.cleanItems.first { $0.tier == .safeRegenerable })
        let before = item.isSelected

        vm.toggleCleanSelection(id: item.id)

        XCTAssertEqual(vm.cleanItems.first { $0.id == item.id }?.isSelected, !before)
    }

    @MainActor
    func testSelectionTotalsOnlyCountSelectedRows() {
        let vm = MoleViewModel()
        let expected = vm.cleanItems.filter(\.isSelected).reduce(0.0) { $0 + $1.sizeGB }
        XCTAssertEqual(vm.totalSelectedCleanGB, expected, accuracy: 0.0001)
    }
}
