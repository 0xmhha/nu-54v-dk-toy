import UIKit
import XCTest

final class TravelItineraryConflictSmoke: XCTestCase {
  func testReviewedCandidateAfterConcurrentVersionChange() throws {
    continueAfterFailure = false
    let environment = ProcessInfo.processInfo.environment
    let bundle = try XCTUnwrap(environment["NU54_T13_TEST_BUNDLE"])
    let port = try XCTUnwrap(environment["NU54_T13_TEST_PORT"])
    let grant = try XCTUnwrap(environment["NU54_T13_TEST_GRANT"])
    let app = XCUIApplication(bundleIdentifier: bundle)
    defer { app.terminate() }
    app.launch()
    func tap(_ label: String, timeout: TimeInterval = 15) {
      let target = app.buttons[label].firstMatch
      XCTAssertTrue(target.waitForExistence(timeout: timeout))
      for _ in 0..<3 where !target.isHittable { app.swipeUp() }
      target.tap()
    }
    try openItineraryPlace(app, port: port, grant: grant, tap: { tap($0, timeout: $1) })
    tap("여행 추천과 일정 선택")
    tap("이 목적과 수신 범위에 동의")
    app.swipeDown()
    tap("내 일정과 추천")
    tap("영업시간과 일정 검토")
    XCTAssertTrue(app.staticTexts["방문 일정 검토"].waitForExistence(timeout: 70))
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "게시된 영업시간")).firstMatch.exists)
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "출처 ")).firstMatch.exists)
    XCTAssertFalse(app.textFields["기존 일정 ID"].exists)
    tap("검토한 일정 적용")
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "버전 1")).firstMatch.waitForExistence(timeout: 10))
    let saved = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", " · ")).firstMatch
    XCTAssertTrue(saved.waitForExistence(timeout: 10))
    tap(saved.label)
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "버전 1")).firstMatch.exists)
    var clock = Calendar(identifier: .gregorian)
    clock.timeZone = try XCTUnwrap(TimeZone(identifier: "Asia/Seoul"))
    if clock.component(.hour, from: Date()) < 9 {
      tap("이전 날짜")
    } else {
      for _ in 0..<5 { tap("종료 늦추기") }
      for _ in 0..<5 { tap("시작 늦추기") }
    }
    tap("영업시간과 일정 검토")
    XCTAssertTrue(app.staticTexts["방문 일정 검토"].waitForExistence(timeout: 15))
    var other = URLRequest(url: try XCTUnwrap(URL(string:"http://127.0.0.1:\(port)/advance")))
    other.httpMethod="POST"
    other.setValue("Bearer " + grant, forHTTPHeaderField: "Authorization")
    let changed=expectation(description:"Second enrolled client advanced revision")
    let value=TravelEnrollmentResult()
    URLSession.shared.dataTask(with:other) { data,response,_ in
      if (response as? HTTPURLResponse)?.statusCode==200 {value.raw=data}
      changed.fulfill()
    }.resume()
    wait(for:[changed],timeout:20)
    XCTAssertEqual(value.raw,Data("2".utf8))
    tap("검토한 일정 적용")
    let latest=app.buttons["변경된 최신 일정 열기"]
    XCTAssertTrue(latest.waitForExistence(timeout:15))
    XCTAssertFalse(app.buttons["검토한 일정 적용"].isEnabled)
    tap("변경된 최신 일정 열기")
    XCTAssertTrue(app.staticTexts["일정 변경 사항을 비교하세요"].waitForExistence(timeout: 15))
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format:"label CONTAINS %@", "최신 일정 · 버전 2")).firstMatch.exists)
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format:"label CONTAINS %@", "검토했던 후보 · 기준 버전 1")).firstMatch.exists)
    XCTAssertFalse(app.buttons["선택한 후보 검토"].isEnabled)
    tap("후보 1 공개 명소 선택")
    tap("선택한 후보 검토")
    XCTAssertTrue(app.staticTexts["방문 일정 검토"].waitForExistence(timeout: 15))
    tap("검토한 일정 적용")
    XCTAssertTrue(app.staticTexts["저장한 일정 · 버전 3"].waitForExistence(timeout: 15))
  }

  func testRegisteredMyTripDestinationsRemainReadOnly() throws {
    continueAfterFailure = false
    let environment = ProcessInfo.processInfo.environment
    let bundle = try XCTUnwrap(environment["NU54_T13_TEST_BUNDLE"])
    let port = try XCTUnwrap(environment["NU54_T13_TEST_PORT"])
    let grant = try XCTUnwrap(environment["NU54_T13_TEST_GRANT"])
    let app = XCUIApplication(bundleIdentifier: bundle)
    defer { app.terminate() }
    app.launch()
    func tap(_ label: String, _ timeout: TimeInterval = 15) {
      let target = app.buttons[label].firstMatch
      XCTAssertTrue(target.waitForExistence(timeout: timeout), "Missing action: \(label)")
      for _ in 0..<4 where !target.isHittable { app.swipeUp() }
      for _ in 0..<4 where !target.isHittable { app.swipeDown() }
      XCTAssertTrue(target.isHittable, "Action not visible: \(label)")
      target.tap()
    }
    try openItineraryPlace(app, port: port, grant: grant, tap: tap, exerciseConsent: false)
    XCTAssertTrue(app.buttons["음성 공유 선택"].waitForExistence(timeout: 15))
    tap("장소 검색으로 돌아가기")
    tap("서울")
    tap("공개 명소")
    tap("내 여행")
    XCTAssertTrue(app.staticTexts["나의 여행"].exists)
    for (entry, destination) in [("후기", "후기와 이용 자격"), ("스탬프", "스탬프와 혜택"),
                                  ("챌린지", "따라하기 챌린지")] {
      tap(entry)
      XCTAssertTrue(app.staticTexts[destination].waitForExistence(timeout: 15))
      XCTAssertTrue(app.staticTexts["발급이 확인된 이용 내역이 없습니다."].waitForExistence(timeout: 15))
      XCTAssertFalse(app.buttons["개인 기록 등록 확인"].exists)
      tap("내 여행으로 돌아가기")
    }
    tap("기록과 동의")
    XCTAssertTrue(app.buttons["음성 공유 선택"].waitForExistence(timeout: 15))
    XCTAssertFalse(app.buttons["음성 공유 철회"].exists)
    tap("내 여행으로 돌아가기")
    tap("AI 코스")
    XCTAssertTrue(app.staticTexts["내 일정과 추천"].waitForExistence(timeout: 15))
    XCTAssertTrue(app.buttons["추천 동의 확인"].waitForExistence(timeout: 15))
    XCTAssertFalse(app.buttons["날짜와 선호로 코스 추천"].exists)
    tap("내 여행으로 돌아가기")
    tap("탐색")
    XCTAssertTrue(app.buttons["공개 명소"].waitForExistence(timeout: 15))
    XCTAssertTrue(app.staticTexts["시험용 장소 데이터"].exists)
  }

  private func openItineraryPlace(_ app: XCUIApplication, port: String, grant: String, tap: (String, TimeInterval) -> Void, exerciseConsent: Bool = true) throws {
    tap("여행지 찾기", 25)
    tap("여행 기록과 동의", 15)
    if app.buttons["개인 기록 등록 공개키"].waitForExistence(timeout: 4) {
      tap("개인 기록 등록 공개키", 15)
      let publicKey = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
      XCTAssertTrue(publicKey.waitForExistence(timeout: 12))
      var req = URLRequest(url: try XCTUnwrap(URL(string: "http://127.0.0.1:\(port)/enroll")))
      req.httpMethod = "POST"
      req.setValue("Bearer " + grant, forHTTPHeaderField: "Authorization")
      req.httpBody = Data(publicKey.label.utf8)
      let enrollment = expectation(description: "Disposable P09 client certificate")
      let result = TravelEnrollmentResult()
      URLSession.shared.dataTask(with: req) { data, response, _ in
        if (response as? HTTPURLResponse)?.statusCode == 200 { result.raw = data }
        enrollment.fulfill()
      }.resume()
      wait(for: [enrollment], timeout: 20)
      let profile = try XCTUnwrap(result.raw.flatMap { String(data: $0, encoding: .utf8) })
      try pasteTravelEnrollment(profile, in: app)
      tap("개인 기록 등록 확인", 15)
    }
    guard exerciseConsent else { return }
    tap("음성 공유 선택", 15)
    tap("이 목적과 수신 범위에 동의", 15)
    tap("음성 공유 철회", 15)
    XCTAssertTrue(app.buttons["음성 공유 선택"].waitForExistence(timeout: 10))
    XCTAssertFalse(app.buttons["음성 공유 철회"].exists)
    app.swipeDown()
    tap("장소 검색으로 돌아가기", 15)
    tap("서울", 15)
    tap("공개 명소", 15)
    app.swipeDown()
    tap("여행 기록과 동의", 15)
  }

  func testPreConsentDraftCancelRetryAndSave() throws {
    continueAfterFailure = false
    let environment = ProcessInfo.processInfo.environment
    let app = XCUIApplication(bundleIdentifier: try XCTUnwrap(environment["NU54_T13_TEST_BUNDLE"]))
    let port = try XCTUnwrap(environment["NU54_T13_TEST_PORT"])
    let grant = try XCTUnwrap(environment["NU54_T13_TEST_GRANT"])
    defer { app.terminate() }
    app.launch()

    func press(_ node: XCUIElement, timeout: TimeInterval = 15) {
      XCTAssertTrue(node.waitForExistence(timeout: timeout))
      for _ in 0..<7 where !node.isHittable {
        if app.keyboards.firstMatch.exists {
          let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.8, dy: 0.56))
          let end = app.coordinate(withNormalizedOffset: CGVector(dx: 0.8, dy: 0.26))
          start.press(forDuration: 0.05, thenDragTo: end)
        } else { app.swipeUp(velocity: .fast) }
      }
      for _ in 0..<6 where !node.isHittable { app.swipeDown(velocity: .fast) }
      XCTAssertTrue(node.isHittable && node.isEnabled, node.label)
      node.tap()
    }
    func tap(_ label: String, timeout: TimeInterval = 15) { press(app.buttons[label].firstMatch, timeout: timeout) }
    func text(_ label: String) {
      XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", label)).firstMatch.waitForExistence(timeout: 15), label)
    }
    func control(_ path: String) throws -> [String: String] {
      var request = URLRequest(url: try XCTUnwrap(URL(string: "http://127.0.0.1:\(port)" + path)))
      request.httpMethod = "POST"
      request.setValue("Bearer " + grant, forHTTPHeaderField: "Authorization")
      let finished = expectation(description: path)
      let result = TravelEnrollmentResult()
      URLSession.shared.dataTask(with: request) { data, response, _ in
        if (response as? HTTPURLResponse)?.statusCode == 200 { result.raw = data }
        finished.fulfill()
      }.resume()
      wait(for: [finished], timeout: 15)
      return try JSONDecoder().decode([String: String].self, from: XCTUnwrap(result.raw))
    }

    try openItineraryPlace(app, port: port, grant: grant, tap: { tap($0, timeout: $1) })
    tap("내 일정과 추천")
    tap("다음 날짜")
    let budget = app.textFields["일정 예산 상한 (원)"]
    press(budget)
    budget.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 5) + "23000")
    if app.keyboards.firstMatch.exists { tap("예산 입력 완료") }
    tap("추천 동의 확인")
    text("추천 제공 동의를 확인하세요")
    text("23,000")
    XCTAssertEqual(try control("/draft-proof"), ["saved": "0", "ranked": "0"])
    tap("동의하지 않음")
    XCTAssertEqual(budget.value as? String, "23000")
    tap("추천 동의 확인")
    tap("동의하고 생성 요청")
    text("추천을 생성하고 있어요")
    XCTAssertEqual(try control("/ranking-started"), ["saved": "0", "ranked": "1"])
    tap("추천 생성 취소")
    text("추천 생성을 취소했어요")
    _ = try control("/ranking-release")
    XCTAssertFalse(app.buttons["검토한 일정 적용"].exists)
    XCTAssertEqual(budget.value as? String, "23000")

    tap("날짜와 선호로 코스 추천")
    text("추천 일정 검토")
    text("23,000")
    var date = Calendar(identifier: .gregorian)
    date.timeZone = try XCTUnwrap(TimeZone(identifier: "Asia/Seoul"))
    let selected = try XCTUnwrap(date.date(byAdding: .day, value: 2, to: Date()))
    let dateParts = date.dateComponents([.month,.day], from: selected)
    text("\(try XCTUnwrap(dateParts.month))월 \(try XCTUnwrap(dateParts.day))일")
    XCTAssertEqual(try control("/draft-proof"), ["saved": "0", "ranked": "2"])
    tap("검토한 일정 적용")
    text("저장한 일정 · 버전 1")
    XCTAssertEqual(try control("/draft-proof"), ["saved": "1", "ranked": "2"])
  }

}
private func pasteTravelEnrollment(_ profile: String, in app: XCUIApplication) throws {
  let field = app.secureTextFields["승인된 개인 기록 등록서"]
  guard field.waitForExistence(timeout: 5) else { throw TravelPasteFailure.unavailable }
  field.tap()
  let clipboard = UIPasteboard.general
  clipboard.setItems([["public.utf8-plain-text": profile]], options: [
    .localOnly: true, .expirationDate: Date().addingTimeInterval(15)
  ])
  let change = clipboard.changeCount
  defer { if clipboard.changeCount == change { clipboard.items = [] } }
  field.press(forDuration: 1.2)
  let paste = app.descendants(matching: .any).matching(
    NSPredicate(format: "label IN %@", ["Paste", "붙여넣기"])).firstMatch
  guard paste.waitForExistence(timeout: 4) else { throw TravelPasteFailure.unavailable }
  paste.tap()
  let allow = app.alerts.buttons.matching(
    NSPredicate(format: "label IN %@", ["Allow Paste", "붙여넣기 허용"])).firstMatch
  if allow.waitForExistence(timeout: 2) { allow.tap() }
  guard app.buttons["개인 기록 등록 확인"].isEnabled else { throw TravelPasteFailure.unavailable }
}

private enum TravelPasteFailure: Error { case unavailable }

private final class TravelEnrollmentResult: @unchecked Sendable {
  var raw: Data?
}
