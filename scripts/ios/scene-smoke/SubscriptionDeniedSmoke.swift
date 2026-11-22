import UIKit
import XCTest

final class SubscriptionDeniedSmoke: XCTestCase {
  private struct Enrollment: Decodable {
    let registration: String
    let tlsPin: String
    let presentationPin: String
    let saleRef: String
    let bookingRef: String?
    let travelRecipient: String?
  }

  @MainActor
  func testDeniedPermissionKeepsSameAuthenticatedInvoiceAcrossRestart() async throws {
    continueAfterFailure = false
    let environment = ProcessInfo.processInfo.environment
    let namespace = try XCTUnwrap(environment["NU54_SUBSCRIPTION_NAMESPACE"])
    let origin = try XCTUnwrap(environment["NU54_SUBSCRIPTION_ENROLLMENT"])
    let access = try XCTUnwrap(environment["NU54_SUBSCRIPTION_TEST_AUTH"])
    let roles = environment["NU54_SUBSCRIPTION_PAIR_ANDROID"] == "1" ? ["ios"] : ["ios", "ios-second"]
    var invoice: String?
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")

    try await registerKeys(namespace: namespace, origin: origin, access: access, roles: roles)

    for role in roles {
      let enrollment = try await receive(origin: origin, access: access, role: role)
      print("NU54_SUBSCRIPTION_CHECKPOINT=received")
      if let invoice { XCTAssertTrue(invoice == enrollment.saleRef, "The installations received different invoices") }
      invoice = enrollment.saleRef
      let app = XCUIApplication(bundleIdentifier: namespace + "-" + role)
      try enroll(enrollment, in: app)

      for attempt in 0..<2 {
        if attempt > 0 {
          app.launch()
          tap("내 결제", app)
        }
        tap("구독 관리", app)
        XCTAssertTrue(app.buttons["같은 회차 결제 검토"].firstMatch.waitForExistence(timeout: 12))
        if attempt == 0 {
          tap("회차 알림 허용", app)
          let deny = springboard.buttons.matching(NSPredicate(format: "label IN %@", ["허용 안 함", "Don't Allow", "Don’t Allow"])).firstMatch
          guard deny.waitForExistence(timeout: 8) else {
            XCTFail("Expected the fresh OS notification permission dialog")
            return
          }
          deny.tap()
          print("NU54_SUBSCRIPTION_CHECKPOINT=denied")
          XCTAssertTrue(app.staticTexts["알림을 허용하지 않았습니다. 미결 목록은 사용할 수 있습니다."].waitForExistence(timeout: 8))
        }
        XCTAssertEqual(app.buttons.matching(identifier: "같은 회차 결제 검토").count, 1)
        tap("같은 회차 결제 검토", app)
        guard app.staticTexts[enrollment.saleRef].waitForExistence(timeout: 12) else {
          XCTFail("The exact authenticated invoice did not open")
          return
        }
        XCTAssertTrue(app.staticTexts["회차가 연결되었습니다. 보드 승인 대기 중입니다."].exists)
        XCTAssertTrue(app.buttons["보드 요약 검토"].exists)
        XCTAssertFalse(app.staticTexts["서버 지급 확인"].exists)
        app.terminate()
      }
    }
  }

  @MainActor
  func testTravelPickupWaitsForOriginalPaymentAndAcceptance() async throws {
    continueAfterFailure = false
    let environment = ProcessInfo.processInfo.environment
    let namespace = try XCTUnwrap(environment["NU54_SUBSCRIPTION_NAMESPACE"])
    let origin = try XCTUnwrap(environment["NU54_SUBSCRIPTION_ENROLLMENT"])
    let access = try XCTUnwrap(environment["NU54_SUBSCRIPTION_TEST_AUTH"])
    try await registerKeys(namespace: namespace, origin: origin, access: access, roles: ["ios"])
    let enrollment = try await receive(origin: origin, access: access, role: "ios")
    let app = XCUIApplication(bundleIdentifier: fixtureBundle(namespace, role: "ios"))
    try enroll(enrollment, in: app)
    app.terminate()
    app.launch()

    func stage(_ name: String) async throws {
      let name = "t06-" + name
      try await waitForTravelStage(name, origin: origin, access: access)
      print("NU54_SUBSCRIPTION_CHECKPOINT=" + name)
      let (_, status) = try await send(origin: origin, access: access, role: "travel-stage", key: name)
      XCTAssertEqual(status, 204)
    }
    func button(prefix: String, suffix: String) {
      let item = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@ AND label ENDSWITH %@", prefix, suffix)).firstMatch
      XCTAssertTrue(item.waitForExistence(timeout: 15))
      for _ in 0..<5 where !item.isHittable { app.swipeUp() }
      XCTAssertTrue(item.isHittable)
      item.tap()
    }
    func text(prefix: String) -> XCUIElement {
      app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", prefix)).firstMatch
    }
    func noPickupNumber() {
      XCTAssertFalse(text(prefix: "픽업 번호 ").exists)
    }

    try await waitForTravelStage("t06-reserved", origin: origin, access: access)
    tap("여행지 찾기", app)
    let region = app.textFields["지역 이름"]
    XCTAssertTrue(region.waitForExistence(timeout: 15))
    region.tap()
    region.typeText("seoul\n")
    tap("서울", app)
    tap("지역 맛집", app)
    tap("픽업 선주문", app)
    button(prefix: "픽업 ", suffix: " 선택")
    tap("지점 메뉴 보기", app)
    button(prefix: "시험 커피", suffix: "추가")
    XCTAssertTrue(app.staticTexts["합계 · 3.50 TestUSDC"].waitForExistence(timeout: 12))
    tap("픽업 시간과 결제 금액 확인 후 주문 접수", app)
    let order = app.staticTexts.matching(NSPredicate(format: "label MATCHES %@", "[0-9A-Fa-f-]{36}")).firstMatch
    XCTAssertTrue(order.waitForExistence(timeout: 15))
    let original = order.label
    XCTAssertNotNil(UUID(uuidString: original))
    noPickupNumber()
    try await stage("reserved")
    tap("이 주문의 결제 내용 검토", app)
    tap("원견적 상세", app)
    XCTAssertTrue(text(prefix: "체인 주문 키 ").waitForExistence(timeout: 15))
    XCTAssertTrue(app.staticTexts["원주문 " + original].exists)
    XCTAssertTrue(app.staticTexts["수령 주소 " + (try XCTUnwrap(enrollment.travelRecipient))].exists)
    XCTAssertTrue(app.staticTexts["네트워크 StableNet 테스트넷 · 8283"].exists)
    XCTAssertTrue(text(prefix: "검토한 주문 " + original + " · 금액 3.50").exists)
    XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
    tap("견적 확인으로", app)
    try await stage("quote")
    try await waitForTravelStage("t06-board-approve", origin: origin, access: access)
    tap("검증한 보드 찾기", app)
    let board = app.buttons.matching(identifier: "NU54 W04 QA에서 결제 검토").element(boundBy: 1)
    XCTAssertTrue(board.waitForExistence(timeout: 15))
    for _ in 0..<5 where !board.isHittable { app.swipeUp() }
    XCTAssertTrue(board.isHittable)
    board.tap()
    try await stage("board-approve")
    XCTAssertTrue(app.staticTexts["보드 서명 확인"].waitForExistence(timeout: 12))
    tap("지급 요청 제출", app)
    try await stage("submitted")

    func reopenOriginal() {
      app.terminate()
      app.launch()
      tap("내 결제", app)
      tap("매장 주문 " + original, app)
      XCTAssertTrue(app.staticTexts[original].waitForExistence(timeout: 12))
    }
    func refreshOriginal() {
      tap("같은 주문의 지급 결과 확인", app)
      XCTAssertTrue(app.staticTexts[original].waitForExistence(timeout: 12))
    }
    try await waitForTravelStage("t06-pending", origin: origin, access: access)
    reopenOriginal()
    refreshOriginal()
    let pendingRelay = text(prefix: "원 릴레이 거래 0x")
    XCTAssertTrue(pendingRelay.waitForExistence(timeout: 12))
    let originalRelay = String(pendingRelay.label.dropFirst("원 릴레이 거래 ".count))
    XCTAssertTrue(originalRelay.range(of: "^0x[0-9a-f]{64}$", options: .regularExpression) != nil)
    noPickupNumber()
    XCTAssertFalse(text(prefix: "영수증 번호 ").exists)
    XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
    try await stage("pending")

    try await waitForTravelStage("t06-paid", origin: origin, access: access)
    refreshOriginal()
    XCTAssertTrue(app.staticTexts["지급 확정 · 매장 접수 대기"].waitForExistence(timeout: 12))
    let receipt = text(prefix: "영수증 번호 ")
    XCTAssertTrue(receipt.waitForExistence(timeout: 12))
    let originalReceipt = receipt.label
    XCTAssertTrue(app.staticTexts["확정 거래 " + originalRelay].exists)
    noPickupNumber()
    try await stage("paid")

    try await waitForTravelStage("t06-preparing", origin: origin, access: access)
    refreshOriginal()
    XCTAssertTrue(app.staticTexts["픽업 준비 중"].waitForExistence(timeout: 12))
    XCTAssertFalse(app.staticTexts["픽업 준비 완료"].exists)
    let number = text(prefix: "픽업 번호 ")
    XCTAssertTrue(number.waitForExistence(timeout: 12))
    let originalNumber = number.label
    try await stage("preparing")

    try await waitForTravelStage("t06-ready", origin: origin, access: access)
    refreshOriginal()
    XCTAssertTrue(app.staticTexts["픽업 준비 완료"].waitForExistence(timeout: 12))
    XCTAssertTrue(app.staticTexts[originalNumber].exists)
    XCTAssertTrue(app.staticTexts[originalReceipt].exists)
    try await stage("ready")

    try await waitForTravelStage("t06-restarted", origin: origin, access: access)
    reopenOriginal()
    XCTAssertTrue(app.staticTexts["픽업 준비 완료"].waitForExistence(timeout: 12))
    XCTAssertTrue(app.staticTexts[originalNumber].exists)
    XCTAssertTrue(app.staticTexts[originalReceipt].exists)
    XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
    try await stage("restarted")
    app.terminate()
  }

  @MainActor
  func testBookingReopensDepositAndPartialRefund() async throws {
    continueAfterFailure = false
    let environment = ProcessInfo.processInfo.environment
    let namespace = try XCTUnwrap(environment["NU54_SUBSCRIPTION_NAMESPACE"])
    let origin = try XCTUnwrap(environment["NU54_SUBSCRIPTION_ENROLLMENT"])
    let access = try XCTUnwrap(environment["NU54_SUBSCRIPTION_TEST_AUTH"])
    let roles = ["ios", "ios-second"]
    try await registerKeys(namespace: namespace, origin: origin, access: access, roles: roles)
    let enrollment = try await receive(origin: origin, access: access, role: "ios")
    let uc1 = environment["NU54_TRAVEL_NATIVE_UC1"] == "1"
    let t07 = environment["NU54_TRAVEL_NATIVE_T07"] == "1"
    let t07Full = environment["NU54_TRAVEL_NATIVE_T07_FULL"] == "1"
    var booking = enrollment.bookingRef ?? ""
    var depositOrder = enrollment.saleRef
    if !uc1 { XCTAssertFalse(booking.isEmpty) }
    let app = XCUIApplication(bundleIdentifier: fixtureBundle(namespace, role: "ios"))
    try enroll(enrollment, in: app)
    app.terminate()
    func releaseStage(_ stage: String) async throws {
      print("NU54_SUBSCRIPTION_CHECKPOINT=" + stage)
      let (_, status) = try await send(origin: origin, access: access, role: "travel-stage", key: stage)
      XCTAssertEqual(status, 204)
    }
    if uc1 {
      try await waitForTravelStage("booking", origin: origin, access: access)
      app.launch()
      tap("여행지 찾기", app)
      tap("여행 기록과 동의", app)
      tap("동의하지 않음", app)
      XCTAssertFalse(app.buttons["개인 기록 등록 확인"].exists)
      XCTAssertFalse(app.alerts.firstMatch.exists)
      let region = app.textFields["지역 이름"]
      XCTAssertTrue(region.waitForExistence(timeout: 15))
      region.tap()
      region.typeText("seoul\n")
      tap("서울", app)
      tap("지역 맛집", app)
      tap("예약 가능한 시간", app)
      XCTAssertTrue(app.staticTexts["1명 합계 30.00 TestUSDC"].waitForExistence(timeout: 12))
      let options = app.buttons.matching(identifier: "이 조건으로 예약 보류")
      XCTAssertTrue(options.firstMatch.waitForExistence(timeout: 12))
      for _ in 0..<6 where !options.firstMatch.isHittable { app.swipeUp() }
      options.firstMatch.tap()
      let reference = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "예약 참조 ")).firstMatch
      XCTAssertTrue(reference.waitForExistence(timeout: 15))
      booking = String(reference.label.dropFirst("예약 참조 ".count))
      XCTAssertNotNil(UUID(uuidString: booking))
      tap("예약 지급 및 영수증 조회", app)
      tap("예약금 결제 검토", app)
      XCTAssertTrue(app.staticTexts["이번 예약금 10.00 TestUSDC"].waitForExistence(timeout: 12))
      XCTAssertTrue(app.staticTexts["별도 잔금 20.00 TestUSDC"].exists)
      tap("이 조건으로 지급 주문 열기", app)
      let referenceOrder = app.staticTexts.matching(NSPredicate(format: "label MATCHES %@", "[0-9A-Fa-f-]{36}")).firstMatch
      XCTAssertTrue(referenceOrder.waitForExistence(timeout: 12))
      depositOrder = referenceOrder.label
      XCTAssertNotNil(UUID(uuidString: depositOrder))
      try await releaseStage("booking")
      app.terminate()
    }
    func openDeposit() {
      app.launch()
      tap("내 결제", app)
      tap("여행 예약", app)
      XCTAssertTrue(app.staticTexts["예약 참조 " + booking].waitForExistence(timeout: 12))
      tap("예약 지급 및 영수증 조회", app)
      tap("예약금 결제 검토", app)
      XCTAssertTrue(app.staticTexts["이번 예약금 10.00 TestUSDC"].waitForExistence(timeout: 12))
      XCTAssertTrue(app.staticTexts["별도 잔금 20.00 TestUSDC"].exists)
      tap("이 조건으로 지급 주문 열기", app)
      XCTAssertTrue(app.staticTexts[depositOrder].waitForExistence(timeout: 12))
    }
    func unpaid() {
      XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
      XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "영수증 번호")).firstMatch.exists)
    }
    func board() {
      tap("검증한 보드 찾기", app)
      let board = app.buttons.matching(identifier: "NU54 W04 QA에서 결제 검토").element(boundBy: 1)
      XCTAssertTrue(board.waitForExistence(timeout: 15))
      for _ in 0..<4 where !board.isHittable { app.swipeUp() }
      XCTAssertTrue(board.isHittable)
      board.tap()
    }
    func approve(_ leg: String, recipient: String, order: String) async throws {
      func quoteDetails() {
        let chainOrder = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "체인 주문 키 ")).firstMatch
        XCTAssertTrue(chainOrder.waitForExistence(timeout: 12))
        let originalKey = chainOrder.label
        tap("주소 전체 보기", app)
        XCTAssertTrue(app.staticTexts["수취 주소 확인"].waitForExistence(timeout: 12))
        XCTAssertTrue(app.staticTexts["서명 견적 수령 주소 " + recipient].exists)
        XCTAssertTrue(app.staticTexts[leg == "deposit" ? "이번 지급 10.00 TestUSDC" : "이번 지급 20.00 TestUSDC"].exists)
        XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
        XCTAssertFalse(app.buttons["검증한 보드 찾기"].exists)
        tap("돌아가기", app)
        XCTAssertTrue(app.staticTexts[originalKey].waitForExistence(timeout: 12))
        XCTAssertTrue(app.staticTexts[order].exists)
        print("NU54_TRAVEL_ADDRESS_RETURN=" + leg)
        XCTAssertTrue(app.staticTexts["네트워크 StableNet 테스트넷 · 8283"].exists)
        XCTAssertTrue(app.staticTexts["네트워크 수수료 · 매장 릴레이 부담"].exists)
        XCTAssertTrue(app.staticTexts[leg == "deposit" ? "이번 예약금 10.00 TestUSDC" : "이번 잔금 20.00 TestUSDC"].exists)
      }
      tap("이 원주문의 서명 견적 요청", app)
      quoteDetails()
      XCTAssertTrue(app.buttons["검증한 보드 찾기"].exists)
      unpaid()
      try await releaseStage(leg + "-quote")
      if !t07 {
        try await waitForTravelStage(leg + "-board-refuse", origin: origin, access: access)
        board()
        unpaid()
        try await releaseStage(leg + "-board-refuse")
        try await waitForTravelStage(leg + "-refused", origin: origin, access: access)
        XCTAssertTrue(app.staticTexts["승인 대기가 끝났습니다. 보드에서 결제를 거절했습니다."].waitForExistence(timeout: 12))
        tap("같은 주문의 지급 결과 확인", app)
        XCTAssertTrue(app.staticTexts[order].exists)
        unpaid()
        try await releaseStage(leg + "-refused")
      }
      try await waitForTravelStage(leg + "-board-approve", origin: origin, access: access)
      if !t07 {
        tap("새 견적 확인", app)
        quoteDetails()
      }
      board()
      unpaid()
      try await releaseStage(leg + "-board-approve")
      try await waitForTravelStage(leg + "-approved", origin: origin, access: access)
      XCTAssertTrue(app.staticTexts["보드 서명 확인"].waitForExistence(timeout: 12))
      tap("지급 요청 제출", app)
      if leg == "deposit" && environment["NU54_TRAVEL_LOSE_DEPOSIT_ACK"] == "1" {
        XCTAssertTrue(app.staticTexts["네트워크 응답을 기다립니다. 같은 원주문을 조회하세요."].waitForExistence(timeout: 12))
        XCTAssertFalse(app.staticTexts["매장이 지급 요청을 받았습니다."].exists)
        print("NU54_TRAVEL_COMMITTED_REPLY_UNAVAILABLE")
        XCTAssertTrue(app.staticTexts[order].exists)
      } else {
        XCTAssertTrue(app.staticTexts["매장이 지급 요청을 받았습니다."].waitForExistence(timeout: 12))
      }
      try await releaseStage(leg + "-approved")
      app.terminate()
    }
    func reopenBooking() {
      app.launch()
      tap("내 결제", app)
      tap("여행 예약", app)
      XCTAssertTrue(app.staticTexts["예약 참조 " + booking].waitForExistence(timeout: 12))
      tap("예약 지급 및 영수증 조회", app)
    }
    func assertCancellationPolicy() async throws {
      let (body, status) = try await send(origin: origin, access: access, role: "travel-cancellation-terms")
      XCTAssertEqual(status, 200)
      let policy = try JSONDecoder().decode([String: String].self, from: body)
      XCTAssertEqual(policy["booking"], booking)
      let cutoff = try XCTUnwrap(ISO8601DateFormatter().date(from: XCTUnwrap(policy["cutoff"])))
      let label = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "취소 기한 · ")).firstMatch
      XCTAssertTrue(label.waitForExistence(timeout: 8))
      let visible = String(label.label.dropFirst("취소 기한 · ".count)).trimmingCharacters(in: .whitespacesAndNewlines)
      let zonePattern = try NSRegularExpression(pattern: #"\s+(GMT|UTC)(?:([+−-])(\d{1,2})(?::?(\d{2}))?)?$"#)
      let zoneMatch = zonePattern.firstMatch(in: visible, range: NSRange(visible.startIndex..., in: visible))
      func zonePart(_ index: Int) -> String? {
        guard let zoneMatch, let range = Range(zoneMatch.range(at: index), in: visible) else { return nil }
        return String(visible[range])
      }
      let wall: String
      let zone: TimeZone
      if let zoneMatch, let range = Range(zoneMatch.range, in: visible) {
        wall = String(visible[..<range.lowerBound])
        let hours = Int(zonePart(3) ?? "0") ?? 0
        let minutes = Int(zonePart(4) ?? "0") ?? 0
        let sign = zonePart(2) == "+" ? 1 : -1
        XCTAssertLessThan(minutes, 60)
        zone = try XCTUnwrap(TimeZone(secondsFromGMT: sign * (hours * 3600 + minutes * 60)))
      } else {
        let abbreviation = try XCTUnwrap(TimeZone.current.abbreviation(for: cutoff))
        XCTAssertTrue(visible.hasSuffix(abbreviation), "T07 cutoff missing an interpretable timezone")
        wall = String(visible.dropLast(abbreviation.count))
        zone = TimeZone.current
      }
      func components(_ value: String) -> [Int] {
        value.split(whereSeparator: { !$0.isNumber }).compactMap { Int($0) }
      }
      func meridiem(_ value: String) -> String? {
        let letters = value.unicodeScalars.filter { CharacterSet.letters.contains($0) }.map(String.init).joined().lowercased()
        if letters.contains("오전") || letters.contains("am") { return "am" }
        if letters.contains("오후") || letters.contains("pm") { return "pm" }
        return nil
      }
      let matches = [Locale.current.identifier, "en_US", "en_GB", "ko_KR"].contains { identifier in
        let localized = DateFormatter()
        localized.locale = Locale(identifier: identifier)
        localized.timeZone = zone
        return ["yMdjmmss", "yMdHms", "yMdhms"].contains { pattern in
          localized.setLocalizedDateFormatFromTemplate(pattern)
          let expected = localized.string(from: cutoff)
          return components(wall) == components(expected) && meridiem(wall) == meridiem(expected)
        }
      }
      if !matches {
        let stamp = ISO8601DateFormatter().string(from: cutoff)
        print("NU54_T07_CUTOFF_DIAGNOSTIC=utc_" + stamp + "_wall_" + components(wall).prefix(8).map(String.init).joined(separator: "_") + "_zone_" + String(zone.secondsFromGMT(for: cutoff)))
        XCTFail("T07 cutoff wall-clock components differ from the authenticated UTC cutoff")
      }
      XCTAssertTrue(app.staticTexts["취소 조건 · " + (try XCTUnwrap(policy["policy"]))].exists)
      XCTAssertTrue(app.staticTexts["노쇼 · " + (try XCTUnwrap(policy["noShow"]))].exists)
      XCTAssertTrue(app.staticTexts["예약 정책 v" + (try XCTUnwrap(policy["terms"])) + " · 가격 v" + (try XCTUnwrap(policy["price"]))].exists)
    }
    let t05 = environment["NU54_TRAVEL_NATIVE_T05"] == "1" || t07Full
    var originalBalance: String?
    if let recipient = enrollment.travelRecipient {
      try await waitForTravelStage("deposit-quote", origin: origin, access: access)
      openDeposit()
      try await approve("deposit", recipient: recipient, order: depositOrder)
      if t05 {
        try await waitForTravelStage("deposit-checking", origin: origin, access: access)
        reopenBooking()
        XCTAssertTrue(app.staticTexts["원주문 " + depositOrder].waitForExistence(timeout: 12))
        XCTAssertTrue(app.staticTexts["지급 확인 중"].exists)
        XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "원 지급 영수증")).firstMatch.exists)
        XCTAssertFalse(app.buttons["잔금 결제 검토"].exists)
        tap("같은 예약금 주문 조회", app)
        XCTAssertTrue(app.staticTexts[depositOrder].waitForExistence(timeout: 12))
        tap("같은 주문의 지급 결과 확인", app)
        XCTAssertTrue(app.staticTexts[depositOrder].exists)
        XCTAssertFalse(app.buttons["이 원주문의 서명 견적 요청"].exists)
        XCTAssertFalse(app.buttons["같은 지급 요청 다시 제출"].exists)
        unpaid()
        try await releaseStage("deposit-checking")
        app.terminate()
      }
    }
    var paidReceiptIDs: Set<UUID> = []
    let stages = t07Full ? ["deposit", "balance-quote", "balance-checking", "cancel-preview", "partial", "refunded"] :
      uc1 ? ["deposit"] : t05 ? ["deposit", "balance-quote", "balance-checking", "both-paid", "partial", "refunded"]
                      : ["deposit", "balance-checking", "partial", "refunded"]
    for stage in stages {
      if stage == "balance-quote" {
        try await waitForTravelStage(stage, origin: origin, access: access)
        reopenBooking()
        let orders = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "원주문 "))
        XCTAssertTrue(orders.element(boundBy: 1).waitForExistence(timeout: 12))
        let references = orders.allElementsBoundByIndex.compactMap { $0.label.split(separator: " ").last.map(String.init) }
        let balance = try XCTUnwrap(references.first { $0 != depositOrder })
        originalBalance = balance
        tap("잔금 결제 검토", app)
        XCTAssertTrue(app.staticTexts["기확정 예약금 10.00 TestUSDC"].exists)
        tap("이 조건으로 지급 주문 열기", app)
        XCTAssertTrue(app.staticTexts[balance].waitForExistence(timeout: 12))
        try await approve("balance", recipient: try XCTUnwrap(enrollment.travelRecipient), order: balance)
        continue
      }
      try await waitForTravelStage(stage, origin: origin, access: access)
      app.launch()
      tap("여행지 찾기", app)
      if t07 && stage == "deposit" {
        tap("내 여행", app)
        XCTAssertTrue(app.staticTexts["나의 여행"].waitForExistence(timeout: 12))
        XCTAssertTrue(app.staticTexts["여행 기록은 선택 사항입니다."].exists)
        XCTAssertTrue(app.staticTexts["선택형 기록과 추천 동의 없이도 장소 탐색은 가능합니다."].exists)
        for label in ["예약", "후기", "스탬프", "기록과 동의", "AI 코스", "챌린지", "탐색"] {
          XCTAssertTrue(app.buttons[label].exists, "T10 missing action: \(label)")
        }
        XCTAssertFalse(app.staticTexts["예약 참조 " + booking].exists)
        for (entry, destination) in [("후기", "후기와 이용 자격"), ("스탬프", "스탬프와 혜택"),
          ("기록과 동의", "음성 기록"), ("AI 코스", "내 일정과 추천"), ("챌린지", "따라하기 챌린지")] {
          tap(entry, app)
          XCTAssertTrue(app.staticTexts[destination].waitForExistence(timeout: 10), "T10 could not open: \(entry)")
          tap("내 여행으로 돌아가기", app)
          XCTAssertTrue(app.staticTexts["나의 여행"].waitForExistence(timeout: 10))
        }
        tap("탐색", app)
        tap("서울", app)
        tap("내 여행", app)
        tap("예약", app)
        XCTAssertTrue(app.staticTexts["내 여행 예약"].waitForExistence(timeout: 12))
        XCTAssertTrue(app.staticTexts["예약 참조 " + booking].waitForExistence(timeout: 12))
        tap("내 여행으로 돌아가기", app)
        tap("탐색", app)
        XCTAssertTrue(app.buttons["지역 맛집"].exists)
        tap("내 여행", app)
        tap("예약", app)
      } else {
        tap("서울", app)
        tap("지역 맛집", app)
        tap("내 예약 다시 열기", app)
      }
      XCTAssertTrue(app.staticTexts["예약 참조 " + booking].waitForExistence(timeout: 12))
      tap("예약 지급 및 영수증 조회", app)
      XCTAssertTrue(app.staticTexts["총액 30.00 TestUSDC"].exists)
      let receipts = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "원 지급 영수증"))
      XCTAssertTrue(receipts.firstMatch.waitForExistence(timeout: 12))
      let receiptIDs = Set(receipts.allElementsBoundByIndex.compactMap { element in
        element.label.split(separator: " ").last.flatMap { UUID(uuidString: String($0)) }
      })
      if stage == "deposit" {
        XCTAssertEqual(receiptIDs.count, 1)
        if uc1 && !t07Full {
          XCTAssertTrue(app.staticTexts["원주문 " + depositOrder].exists)
          XCTAssertTrue(app.buttons["잔금 결제 검토"].exists)
          let receipt = try XCTUnwrap(receiptIDs.first).uuidString.lowercased()
          let (body, status) = try await send(origin: origin, access: access, role: "travel-proof")
          XCTAssertEqual(status, 200)
          let paid = try JSONDecoder().decode([String: String].self, from: body)
          XCTAssertEqual(paid, ["booking":booking,"order":depositOrder,"receipt":receipt,"amount":"10000000"])
          try await releaseStage("deposit")
          if t07 {
            try await waitForTravelStage("cancel-preview", origin: origin, access: access)
            tap("예약 취소 검토", app)
            try await assertCancellationPolicy()
            let estimate = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@ AND label ENDSWITH %@",
              "예약금", "예상 환불 10.00 TestUSDC")).firstMatch
            XCTAssertTrue(estimate.waitForExistence(timeout: 12))
            XCTAssertTrue(app.staticTexts["원주문 " + depositOrder].exists)
            XCTAssertFalse(app.staticTexts["확정 환불 10.00 TestUSDC"].exists)
            try await releaseStage("cancel-preview")
            // The host only exposes this stage after checking that preview made no writes.
            try await waitForTravelStage("cancel-requested", origin: origin, access: access)
            tap("예약 취소 요청 확인", app)
            XCTAssertTrue(app.staticTexts["예약금 환불 확인 중"].waitForExistence(timeout: 12))
            XCTAssertTrue(app.staticTexts["원 지급 영수증 " + receipt].exists)
            XCTAssertFalse(app.staticTexts["확정 환불 10.00 TestUSDC"].exists)
            try await releaseStage("cancel-requested")
            app.terminate()
            try await waitForTravelStage("deposit-refunded", origin: origin, access: access)
            reopenBooking()
            XCTAssertTrue(app.staticTexts["예약 참조 " + booking].exists)
            XCTAssertTrue(app.staticTexts["확정 환불 10.00 TestUSDC"].waitForExistence(timeout: 12))
            XCTAssertTrue(app.staticTexts["원주문 " + depositOrder].exists)
            XCTAssertTrue(app.staticTexts["원 지급 영수증 " + receipt].exists)
            XCTAssertFalse(app.buttons["예약 취소 요청 확인"].exists)
            XCTAssertFalse(app.buttons["잔금 결제 검토"].exists)
            try await releaseStage("deposit-refunded")
          }
          return
        }
        XCTAssertTrue(app.buttons["잔금 결제 검토"].exists)
        XCTAssertFalse(app.buttons["예약금 결제 검토"].exists)
        tap("잔금 결제 검토", app)
        for text in ["예약 총액 30.00 TestUSDC", "기확정 예약금 10.00 TestUSDC", "이번 잔금 20.00 TestUSDC",
                     "예약 정책 v3 · 가격 v2", "예약 네트워크 · StableNet 테스트넷 8283"] {
          XCTAssertTrue(app.staticTexts[text].exists, "Missing balance review line " + text)
        }
        tap("원예약으로", app)
      } else if stage == "balance-checking" {
        XCTAssertEqual(receiptIDs.count, 1)
        let orderLabels = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "원주문 ")).allElementsBoundByIndex
        let balanceOrder = try XCTUnwrap(orderLabels.compactMap {
          $0.label.split(separator: " ").last.map(String.init)
        }.first { $0 != depositOrder })
        if t05 { XCTAssertEqual(balanceOrder, originalBalance) }
        tap("같은 잔금 주문 조회", app)
        XCTAssertTrue(app.staticTexts[balanceOrder].waitForExistence(timeout: 12))
        XCTAssertFalse(app.buttons["이 원주문의 서명 견적 요청"].exists)
        tap("같은 주문의 지급 결과 확인", app)
        XCTAssertTrue(app.staticTexts[balanceOrder].exists)
        XCTAssertFalse(app.buttons["이 원주문의 서명 견적 요청"].exists)
      } else if stage == "cancel-preview" {
        XCTAssertEqual(receiptIDs.count, 2)
        let balanceOrder = try XCTUnwrap(originalBalance)
        paidReceiptIDs = receiptIDs
        XCTAssertTrue(app.staticTexts["원주문 " + depositOrder].exists)
        XCTAssertTrue(app.staticTexts["원주문 " + balanceOrder].exists)
        tap("예약 취소 검토", app)
        try await assertCancellationPolicy()
        for (leg, amount) in [("예약금", "10.00"), ("잔금", "20.00")] {
          let estimate = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@ AND label ENDSWITH %@",
            leg, "예상 환불 " + amount + " TestUSDC")).firstMatch
          XCTAssertTrue(estimate.waitForExistence(timeout: 8), "Expected exact per-leg estimate")
        }
        try await releaseStage("cancel-preview")
        try await waitForTravelStage("cancel-requested", origin: origin, access: access)
        tap("예약 취소 요청 확인", app)
        XCTAssertTrue(app.staticTexts["예약금 환불 확인 중"].waitForExistence(timeout: 8))
        XCTAssertTrue(app.staticTexts["잔금 환불 확인 중"].exists)
        XCTAssertTrue(app.staticTexts["원주문 " + depositOrder].exists && app.staticTexts["원주문 " + balanceOrder].exists)
        for receipt in paidReceiptIDs {
          XCTAssertTrue(app.staticTexts["원 지급 영수증 " + receipt.uuidString.lowercased()].exists)
        }
        XCTAssertFalse(app.staticTexts["확정 환불 10.00 TestUSDC"].exists)
        XCTAssertFalse(app.staticTexts["확정 환불 20.00 TestUSDC"].exists)
        try await releaseStage("cancel-requested")
        app.terminate()
        continue
      } else if stage == "both-paid" {
        XCTAssertEqual(receiptIDs.count, 2)
        XCTAssertTrue(app.staticTexts["예약금과 잔금이 확인됐어요"].exists)
        XCTAssertTrue(app.staticTexts["원주문 " + depositOrder].exists)
        XCTAssertTrue(app.staticTexts["원주문 " + (try XCTUnwrap(originalBalance))].exists)
      } else {
        XCTAssertEqual(receiptIDs.count, 2)
        XCTAssertTrue(app.staticTexts[t07Full ? "확정 환불 10.00 TestUSDC" : "확정 환불 3.00 TestUSDC"].exists)
        if t07Full {
          XCTAssertEqual(receiptIDs, paidReceiptIDs)
          XCTAssertTrue(app.staticTexts["원주문 " + depositOrder].exists)
          XCTAssertTrue(app.staticTexts["원주문 " + (try XCTUnwrap(originalBalance))].exists)
        }
        XCTAssertFalse(app.buttons["잔금 결제 검토"].exists)
        if stage == "partial" {
          XCTAssertTrue(app.staticTexts["부분 환불"].exists)
          XCTAssertTrue(app.staticTexts["잔금 환불 확인 중"].exists)
          if t07Full { XCTAssertFalse(app.staticTexts["확정 환불 20.00 TestUSDC"].exists) }
        } else {
          XCTAssertTrue(app.staticTexts["확정 환불 20.00 TestUSDC"].exists)
          XCTAssertFalse(app.staticTexts["잔금 환불 확인 중"].exists)
        }
      }
      print("NU54_SUBSCRIPTION_CHECKPOINT=" + stage)
      app.terminate()
      let (_, status) = try await send(origin: origin, access: access, role: "travel-stage", key: stage)
      XCTAssertEqual(status, 204)
    }
  }

  private func waitForTravelStage(_ stage: String, origin: String, access: String) async throws {
    for _ in 0..<120 {
      let (body, status) = try await send(origin: origin, access: access, role: "travel-stage")
      if status == 200 && String(data: body, encoding: .utf8) == stage { return }
      guard status == 202 || status == 200 else { break }
      try await Task.sleep(nanoseconds: 1_000_000_000)
    }
    throw EnrollmentFailure.unavailable
  }

  private func fixtureBundle(_ namespace: String, role: String) -> String {
    if role == "ios" && ProcessInfo.processInfo.environment["NU54_TRAVEL_NATIVE_BOARD"] == "1" {
      return "com.nu54renter.w04qa"
    }
    return namespace + "-" + role
  }

  @MainActor
  private func registerKeys(namespace: String, origin: String, access: String, roles: [String]) async throws {
    for role in roles {
      let app = XCUIApplication(bundleIdentifier: fixtureBundle(namespace, role: role))
      app.launch()
      tap("내 결제", app)
      tap("주문 등록 공개키 보기", app)
      let key = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
      guard key.waitForExistence(timeout: 12) else { throw EnrollmentFailure.unavailable }
      let (_, response) = try await send(origin: origin, access: access, role: role, key: key.label)
      XCTAssertEqual(response, 204, "The fixture did not accept a fresh registration key")
      print("NU54_SUBSCRIPTION_CHECKPOINT=key")
      app.terminate()
    }

  }

  @MainActor
  private func enroll(_ enrollment: Enrollment, in app: XCUIApplication) throws {
      app.launch()
      tap("내 결제", app)
      XCTAssertTrue(app.buttons["주문 등록 공개키 보기"].waitForExistence(timeout: 10))
      try paste(enrollment.registration, field: "customer-commerce-registration", in: app)
      let registration = app.descendants(matching: .any).matching(identifier: "customer-commerce-registration").firstMatch
      XCTAssertTrue((registration.value as? String) == enrollment.registration, "The approved registration did not reach its input")
      print("NU54_SUBSCRIPTION_CHECKPOINT=pasted")
      advanceRegistrationFocus(app)
      enterFingerprint(enrollment.tlsPin, field: "customer-commerce-tls-pin", in: app)
      advanceRegistrationFocus(app)
      enterFingerprint(enrollment.presentationPin, field: "customer-commerce-presentation-pin", in: app)
      let done = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Done", "완료"])).firstMatch
      if done.waitForExistence(timeout: 2) { done.tap() }
      else if app.keyboards.firstMatch.exists { app.typeText("\n") }
      let ready = app.buttons["구독 관리"]
      let confirm = app.buttons["주문 등록 확인"]
      if !ready.waitForExistence(timeout: 4) && confirm.exists && confirm.isEnabled {
        tap("주문 등록 확인", app)
      }
      print("NU54_SUBSCRIPTION_CHECKPOINT=submit")
      if !ready.waitForExistence(timeout: 5) && confirm.exists {
        let phase = confirm.isEnabled ? "registration-idle" : "registration-pending"
        print("NU54_SUBSCRIPTION_CHECKPOINT=" + phase)
      }
      if app.staticTexts["등록서를 확인할 수 없습니다. 발급 기록을 대조하세요."].exists {
        print("NU54_SUBSCRIPTION_CHECKPOINT=registration-rejected")
      }
      if app.staticTexts["보호된 주문 등록을 열 수 없습니다. 발급 담당자에게 문의하세요."].exists {
        print("NU54_SUBSCRIPTION_CHECKPOINT=registration-locked")
      }
      if !ready.exists && app.state == .runningForeground {
        app.terminate()
        app.launch()
        tap("내 결제", app)
      }
      XCTAssertTrue(ready.waitForExistence(timeout: 12), "The shipping order profile did not activate")
  }

  private func send(origin: String, access: String, role: String, key: String? = nil) async throws -> (Data, Int) {
    var request = URLRequest(url: try XCTUnwrap(URL(string: origin + "/" + role)), timeoutInterval: 5)
    request.setValue("Bearer " + access, forHTTPHeaderField: "Authorization")
    request.cachePolicy = .reloadIgnoringLocalCacheData
    if let key {
      request.httpMethod = "POST"
      request.httpBody = Data(key.utf8)
    }
    let (body, response) = try await URLSession.shared.data(for: request)
    return (body, (response as? HTTPURLResponse)?.statusCode ?? 0)
  }

  private func receive(origin: String, access: String, role: String) async throws -> Enrollment {
    for _ in 0..<120 {
      let (body, status) = try await send(origin: origin, access: access, role: role)
      if status == 200 { return try JSONDecoder().decode(Enrollment.self, from: body) }
      guard status == 202 else { break }
      try await Task.sleep(nanoseconds: 1_000_000_000)
    }
    throw EnrollmentFailure.unavailable
  }

  @MainActor
  private func paste(_ raw: String, field: String, in app: XCUIApplication) throws {
    let input = app.descendants(matching: .any).matching(identifier: field).firstMatch
    for _ in 0..<4 where !input.isHittable { app.swipeUp() }
    guard input.waitForExistence(timeout: 6) else { throw EnrollmentFailure.unavailable }
    input.tap()
    let clipboard = UIPasteboard.general
    clipboard.setItems([["public.utf8-plain-text": raw]], options: [
      .localOnly: true, .expirationDate: Date().addingTimeInterval(15),
    ])
    let change = clipboard.changeCount
    defer { if clipboard.changeCount == change { clipboard.items = [] } }
    input.press(forDuration: 1.2)
    let paste = app.descendants(matching: .any).matching(
      NSPredicate(format: "label IN %@", ["Paste", "붙여넣기"])).firstMatch
    guard paste.waitForExistence(timeout: 5) else { throw EnrollmentFailure.unavailable }
    paste.tap()
    let allow = app.alerts.buttons.matching(
      NSPredicate(format: "label IN %@", ["Allow Paste", "붙여넣기 허용"])).firstMatch
    if allow.waitForExistence(timeout: 2) { allow.tap() }
  }

  private func advanceRegistrationFocus(_ app: XCUIApplication) {
    let next = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Next", "다음"])).firstMatch
    if next.waitForExistence(timeout: 2) { next.tap() }
    else { app.typeText("\n") }
  }

  private func enterFingerprint(_ value: String, field: String, in app: XCUIApplication) {
    app.typeText(value)
    let input = app.textFields[field]
    XCTAssertTrue((input.value as? String) == value, "The trust fingerprint did not reach its input")
  }

  private func tap(_ label: String, _ app: XCUIApplication) {
    let button = app.buttons[label].firstMatch
    guard button.waitForExistence(timeout: 15) else {
      XCTFail("Missing button " + label)
      return
    }
    for _ in 0..<3 where !button.isHittable { app.swipeUp() }
    button.tap()
  }

  private enum EnrollmentFailure: Error { case unavailable }
}
