import XCTest
@testable import MoleStudio

/// Guards the split of `RealMacSystemScanner` into extension files: the orphan
/// scan, the catalogue scans and the full payload must all still resolve and
/// return their documented shapes.
final class ScannerRefactorTests: XCTestCase {
    func testOrphanSignaturesAreNonEmptyAndUnique() {
        let signatures = OrphanSignature.all(home: NSHomeDirectory())
        XCTAssertFalse(signatures.isEmpty, "the orphan catalogue must not be empty")
        let ids = signatures.map(\.id)
        XCTAssertEqual(Set(ids).count, ids.count, "orphan signature ids must be unique")
    }

    func testFullScanPayloadKeepsItsKeys() {
        let payload = RealMacSystemScanner.shared.buildFullScanPayload()
        for key in ["orphanLeftoversList", "cleanGroups", "appsList"] {
            XCTAssertNotNil(payload[key], "buildFullScanPayload must still emit \(key)")
        }
    }

    func testOrphanRowsCarryTheFieldsTheUIBinds() {
        let rows = RealMacSystemScanner.shared.scanOrphanedLeftovers()
        for row in rows {
            XCTAssertNotNil(row["id"])
            XCTAssertNotNil(row["name"])
            XCTAssertNotNil(row["leftoverSizeGb"])
            XCTAssertNotNil(row["subpaths"])
        }
    }
}
