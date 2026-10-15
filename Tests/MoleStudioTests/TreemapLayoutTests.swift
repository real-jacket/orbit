import XCTest
@testable import MoleStudio

/// The treemap layout is the one piece of non-trivial pure math in the app, so it gets the most
/// coverage. The contract `computeTreemapRects` promises: every entry gets a rect, the rects tile
/// the unit square exactly, and each rect's area tracks its share of the total byte count.
final class TreemapLayoutTests: XCTestCase {

    private func makeEntries(_ sizes: [Int64]) -> [DiskAnalyzeEntry] {
        sizes.enumerated().map { index, size in
            DiskAnalyzeEntry(
                name: "entry-\(index)",
                path: "/tmp/entry-\(index)",
                size: size,
                is_dir: true,
                scan_status: "Complete"
            )
        }
    }

    private var runner: MoleCLIRunner { MoleCLIRunner.shared }

    func testEmptyInputProducesNoRects() {
        XCTAssertTrue(runner.computeTreemapRects(entries: []).isEmpty)
    }

    func testSingleEntryFillsTheUnitSquare() throws {
        let rects = runner.computeTreemapRects(entries: makeEntries([1_000]))

        XCTAssertEqual(rects.count, 1)
        let box = try XCTUnwrap(rects.first).normalizedRect
        XCTAssertEqual(box.minX, 0, accuracy: 0.0001)
        XCTAssertEqual(box.minY, 0, accuracy: 0.0001)
        XCTAssertEqual(box.width, 1, accuracy: 0.0001)
        XCTAssertEqual(box.height, 1, accuracy: 0.0001)
    }

    func testEveryEntryReceivesExactlyOneRect() {
        let entries = makeEntries([500, 400, 300, 200, 100])
        let rects = runner.computeTreemapRects(entries: entries)

        XCTAssertEqual(rects.count, entries.count)
        XCTAssertEqual(Set(rects.map(\.id)), Set(entries.map(\.path)))
    }

    func testRectsTileTheUnitSquareWithoutOverlap() {
        let entries = makeEntries([9_000, 4_000, 2_500, 1_200, 600, 300, 90])
        let rects = runner.computeTreemapRects(entries: entries)

        // Total area must be exactly 1.0: the layout splits a box into two disjoint halves at
        // every step, so no area is lost or duplicated.
        let totalArea = rects.reduce(0.0) { $0 + Double($1.normalizedRect.width * $1.normalizedRect.height) }
        XCTAssertEqual(totalArea, 1.0, accuracy: 0.0001)

        // No rect may escape the unit square.
        for rect in rects {
            let box = rect.normalizedRect
            XCTAssertGreaterThanOrEqual(box.minX, -0.0001, "rect \(rect.id) escapes on minX")
            XCTAssertGreaterThanOrEqual(box.minY, -0.0001, "rect \(rect.id) escapes on minY")
            XCTAssertLessThanOrEqual(box.maxX, 1.0001, "rect \(rect.id) escapes on maxX")
            XCTAssertLessThanOrEqual(box.maxY, 1.0001, "rect \(rect.id) escapes on maxY")
        }
    }

    func testLargestEntryGetsTheLargestRect() {
        let entries = makeEntries([8_000, 1_500, 400, 100])
        let rects = runner.computeTreemapRects(entries: entries)

        let areas = rects.map { Double($0.normalizedRect.width * $0.normalizedRect.height) }
        let largestArea = areas.max() ?? 0
        let biggest = rects.first { Double($0.normalizedRect.width * $0.normalizedRect.height) == largestArea }

        XCTAssertEqual(biggest?.entry.path, "/tmp/entry-0")
    }

    func testPercentagesSumToRoughlyOneHundred() {
        let entries = makeEntries([5_000, 3_000, 2_000])
        let rects = runner.computeTreemapRects(entries: entries)

        let total = rects.reduce(0.0) { $0 + $1.percentage }
        XCTAssertEqual(total, 100.0, accuracy: 0.01)
    }

    func testZeroSizedEntriesDoNotCrashOrProduceNaN() {
        let entries = makeEntries([0, 0, 0])
        let rects = runner.computeTreemapRects(entries: entries)

        XCTAssertEqual(rects.count, 3)
        let totalArea = rects.reduce(0.0) { $0 + Double($1.normalizedRect.width * $1.normalizedRect.height) }
        XCTAssertFalse(totalArea.isNaN)
        XCTAssertEqual(totalArea, 1.0, accuracy: 0.0001)
    }

    func testManyEntriesStillTileExactly() {
        let entries = makeEntries((1...40).map { Int64($0 * 137) })
        let rects = runner.computeTreemapRects(entries: entries)

        XCTAssertEqual(rects.count, 40)
        let totalArea = rects.reduce(0.0) { $0 + Double($1.normalizedRect.width * $1.normalizedRect.height) }
        XCTAssertEqual(totalArea, 1.0, accuracy: 0.0001)
    }
}
