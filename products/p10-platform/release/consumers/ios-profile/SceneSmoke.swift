import XCTest

final class SceneSmoke: XCTestCase {
    private let app = XCUIApplication(bundleIdentifier: "__NU54_QA_PACKAGE__")
    private func reveal(_ element: XCUIElement) {
        if element.waitForExistence(timeout: 3), element.isHittable { return }
        for _ in 0..<6 {
            if element.exists, element.isHittable { return }
            app.swipeUp()
        }
        for _ in 0..<12 {
            if element.exists, element.isHittable { return }
            app.swipeDown()
        }
        XCTAssertTrue(element.exists && element.isHittable, "Missing visible product control or result")
    }
    private func tap(_ label: String) throws {
        let button = app.buttons[label].firstMatch
        reveal(button)
        button.tap()
    }
    func testSavePractice() throws {
        continueAfterFailure = false
        app.launch()
        try tap("결제 연습")
        try tap("틀린 금액 찾아보기")
        try tap("나중에 이어하기")
        reveal(app.buttons["연습 이어하기"].firstMatch)
        app.terminate()
        app.launch()
        reveal(app.buttons["연습 이어하기"].firstMatch)
    }
    func testReadAndUpdate() throws {
        continueAfterFailure = false
        app.launch()
        try tap("연습 이어하기")
        try tap("연습 승인")
        reveal(app.staticTexts["금액이 다릅니다. 이 요청은 거절하세요."].firstMatch)
        try tap("연습 거절")
        reveal(app.staticTexts["실제 지급 0건 · 서명 0건"].firstMatch)
        try tap("다른 연습 선택")
        try tap("나중에 이어하기")
        try tap("English")
        try tap("Payment practice")
        try tap("Compare matching amounts")
        try tap("Continue later")
        reveal(app.buttons["Resume practice"].firstMatch)
    }
    func testReadAfterRollback() throws {
        continueAfterFailure = false
        app.launch()
        try tap("Resume practice")
        try tap("Approve practice")
        reveal(app.staticTexts["Practice approved"].firstMatch)
        reveal(app.staticTexts["Real payments: 0 · Signatures: 0"].firstMatch)
    }
}
