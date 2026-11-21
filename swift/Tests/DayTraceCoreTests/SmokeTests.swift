import XCTest
@testable import DayTraceCore

final class SmokeTests: XCTestCase {
    func testPathsAreStable() {
        XCTAssertEqual(DayTracePaths.beijing.identifier, "Asia/Shanghai")
        XCTAssertEqual(DayTracePaths.dayString(Date(timeIntervalSince1970: 0)), "1970-01-01")
    }
}
