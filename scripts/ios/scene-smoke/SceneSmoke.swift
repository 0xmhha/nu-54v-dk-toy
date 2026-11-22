import XCTest

final class SceneSmoke: XCTestCase {
  private let renter = XCUIApplication(bundleIdentifier: "com.nu54renter.custom")
  private let kiosk = XCUIApplication(bundleIdentifier: "com.nu54kiosk.custom")

  func testRenterLaunchShowsHome() {
    renter.launch()
    XCTAssertTrue(renter.buttons["여행지 찾기"].waitForExistence(timeout: 25), renter.debugDescription)
  }

  func testKioskLaunchShowsHome() {
    kiosk.launch()
    XCTAssertTrue(kiosk.staticTexts["매장 주문"].waitForExistence(timeout: 25), kiosk.debugDescription)
    XCTAssertTrue(kiosk.buttons["등록 공개키 확인"].waitForExistence(timeout: 10), kiosk.debugDescription)
  }

  func testRenterColdOrderURL() {
    renter.terminate()
    openOrderURL()
    XCTAssertTrue(renter.staticTexts["내 매장 주문"].waitForExistence(timeout: 25), renter.debugDescription)
  }

  func testRenterWarmOrderURLKeepsTravelState() {
    renter.launch()
    let travel = renter.buttons["여행지 찾기"]
    XCTAssertTrue(travel.waitForExistence(timeout: 25), renter.debugDescription)
    travel.tap()
    XCTAssertTrue(renter.staticTexts["여행지 찾기"].waitForExistence(timeout: 10), renter.debugDescription)
    openOrderURL()
    XCTAssertTrue(renter.staticTexts["내 매장 주문"].waitForExistence(timeout: 10), renter.debugDescription)
    renter.buttons["지갑으로 돌아가기"].tap()
    XCTAssertTrue(renter.staticTexts["여행지 찾기"].waitForExistence(timeout: 10), renter.debugDescription)
  }

  private func openOrderURL() {
    let expiry = Int(Date().timeIntervalSince1970) + 45
    let branch = "c830972d-ca86-4253-88aa-a3fe5f93bbe6"
    let order = "c9040f87-b820-4d93-83ac-58f1933e6617"
    renter.open(URL(string: "nu54://order?v=2&branch=\(branch)&order=\(order)&expires=\(expiry)")!)
  }
}

final class MarketSmoke: XCTestCase {
  private lazy var app = XCUIApplication(bundleIdentifier: ProcessInfo.processInfo.environment["NU54_MARKET_TEST_BUNDLE"] ?? "com.nu54renter.custom")

  func testMarketEntryRequiresInstalledDeployment() throws {
    app.launch()
    try tap("테스트 시장 학습")
    XCTAssertTrue(app.staticTexts["승인된 테스트 시장 배포 설정이 필요합니다."].waitForExistence(timeout: 15))
    XCTAssertFalse(app.buttons["격리 학습 지갑 만들기"].exists)
  }

  private let service = "http://127.0.0.1:8284/v1/profile"
  private let node = "http://127.0.0.1:8285"

  func testNativeWalletLiquidityAndOriginalHashAfterRestart() throws {
    try verifyLiquidityRestart()
  }

  private func verifyLiquidityRestart() throws {
    let (user, venue, base, quote) = try enrollFixture()
    try tap("유동성")
    try tap("유동성 공급")
    try fill("최대 지급 LAB", "100")
    try fill("최대 지급 LAQ", "120")
    try fill("최소 발행 지분", "100")
    try tap("견적 받기")
    XCTAssertTrue(app.staticTexts["예상 100 LP 지분"].waitForExistence(timeout: 12), app.debugDescription)

    for label in ["LAB 최대 100 사용 허용 승인", "LAQ 최대 120 사용 허용 승인"] {
      try tap(label)
      try finish()
    }
    try tap("견적 확인 및 학습 지갑에서 승인")
    let original = try pendingHash()
    app.terminate()
    app.launch()
    try tap("테스트 시장 학습")
    XCTAssertEqual(try pendingHash(), original)
    try finish()
    XCTAssertTrue(app.staticTexts["실제 100 LP 지분"].exists, app.debugDescription)
    XCTAssertTrue(app.staticTexts["LAB 900 · LAQ 880 · LP 100"].exists, app.debugDescription)
    fixture = (user, venue, base, quote)
  }

  func testIsolatedFXQuoteAndUnchangedTokenBalances() throws {
    let (user, _, _, _) = try enrollFixture()
    try tap("모의 환전")
    try tap("LAB → LAQ 모의 환전")
    try fill("가상 매도 LAB", "2")
    try fill("최소 가상 수령 LAQ", "2")
    try tap("견적 받기")
    XCTAssertTrue(app.staticTexts["예상 2.4 LAQ"].waitForExistence(timeout: 12), app.debugDescription)
    let beforeFunds = try XCTUnwrap(try json("http://127.0.0.1:8284/v1/wallet/" + user)["quoteBalance"] as? String)
    XCTAssertEqual(beforeFunds, "1000000000")
    let review = XCTAttachment(screenshot: app.screenshot())
    review.name = "L04-quote"
    review.lifetime = .keepAlways
    add(review)
    try tap("견적 확인 및 학습 지갑에서 승인")
    let original = try pendingHash()
    app.terminate()
    app.launch()
    try tap("테스트 시장 학습")
    XCTAssertEqual(try pendingHash(), original)
    try finish()
    XCTAssertTrue(app.staticTexts["실제 2.4 LAQ"].waitForExistence(timeout: 10), app.debugDescription)
    let afterFunds = try json("http://127.0.0.1:8284/v1/wallet/" + user)
    XCTAssertEqual(afterFunds["quoteBalance"] as? String, beforeFunds)
    XCTAssertEqual(afterFunds["baseBalance"] as? String, "100000000000")
    let result = XCTAttachment(screenshot: app.screenshot())
    result.name = "L04-done"
    result.lifetime = .keepAlways
    add(result)
    print("L04 original hash retained after relaunch, quoted and confirmed 2.4 LAQ, base/quote assets unchanged")

  }

  func testSwapRemoveAndPosition() throws {
    try verifyLiquidityRestart()
    let (user, venue, base, quote) = try XCTUnwrap(fixture)
    func balance(_ contract:String) throws -> UInt64 {
      let data = "0x70a08231" + String(repeating:"0",count:24) + String(user.dropFirst(2))
      let hex = try XCTUnwrap(try rpc("eth_call",[["to":contract,"data":data],"latest"]) as? String)
      return try XCTUnwrap(UInt64(hex.dropFirst(2),radix:16))
    }
    func publishLatest(_ value: UInt64 = 1_200_000) throws {
      let reporters = try XCTUnwrap(try rpc("eth_accounts") as? [String])
      let block=try XCTUnwrap(try rpc("eth_getBlockByNumber",["latest",false]) as? [String:Any])
      let raw=try XCTUnwrap(block["timestamp"] as? String)
      let at=try XCTUnwrap(UInt64(raw.dropFirst(2),radix:16))
      for reporter in reporters[2...3] {
        _=try rpc("eth_sendTransaction",[["from":reporter,"to":venue,"data":"0x07640c1f"+String(format:"%064llx%064llx",value,at)]])
      }
      _=try rpc("anvil_mine",["0x03"])
    }
    try publishLatest()
    try tap("교환"); try tap("LAB → LAQ 교환"); try fill("판매 LAB","2"); try fill("최소 수령 LAQ","2")
    try tap("견적 받기")
    XCTAssertTrue(app.staticTexts["예상 2.34602 LAQ"].waitForExistence(timeout:12),app.debugDescription)
    try tap("LAB 최대 2 사용 허용 승인"); try finish()
    try tap("견적 확인 및 학습 지갑에서 승인")
    let swap=try pendingHash(); app.terminate(); app.launch(); try tap("테스트 시장 학습")
    XCTAssertEqual(try pendingHash(),swap); try finish()
    XCTAssertTrue(app.staticTexts["실제 2.34602 LAQ"].exists,app.debugDescription)
    XCTAssertEqual(try balance(base),89_800_000_000)
    XCTAssertEqual(try balance(quote),882346020)
    try publishLatest()
    try tap("유동성"); try tap("유동성 회수"); try fill("소각 지분","10"); try fill("최소 수령 LAB","1"); try fill("최소 수령 LAQ","1")
    try tap("견적 받기"); try tap("견적 확인 및 학습 지갑에서 승인"); try finish()
    XCTAssertGreaterThan(try balance(base),89_800_000_000)
    XCTAssertGreaterThan(try balance(quote),882346020)
    let collateralBefore=try balance(quote)
    try publishLatest()
    try tap("포지션"); try tap("매수 포지션 열기"); try fill("격리 증거금 LAQ","10"); try fill("포지션 수량 LAB","1")
    try tap("견적 받기"); try tap("LAQ 최대 10 사용 허용 승인"); try finish()
    try tap("견적 확인 및 학습 지갑에서 승인"); let open=try pendingHash(); try finish()
    XCTAssertEqual(try balance(quote),collateralBefore-10_000_000)
    XCTAssertTrue(app.staticTexts["포지션 번호 1"].exists,app.debugDescription)
    try tap("포지션 1 확인")
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "포지션 개설 · 확정 블록")).firstMatch.exists,
      app.debugDescription)
    XCTAssertTrue(app.staticTexts[open].exists,app.debugDescription)

    // Move only the disposable chain to the funding window. Roll it back before
    // asking the native wallet to sign again against its trusted wall clock.
    let snapshot = try XCTUnwrap(try rpc("evm_snapshot") as? String)
    let position = try XCTUnwrap(try json("http://127.0.0.1:8284/v1/positions/1")["position"] as? [String: Any])
    let period = try XCTUnwrap(UInt64(try XCTUnwrap(position["nextPeriod"] as? String)))
    _ = try rpc("evm_setNextBlockTimestamp", [period * 300 + 1])
    _ = try rpc("evm_mine")
    try publishLatest()
    let version = try XCTUnwrap(UInt64(try XCTUnwrap(try json(service)["priceVersion"] as? String)))
    let operators = try XCTUnwrap(try rpc("eth_accounts") as? [String])
    let data = "0x325fab67" + String(format: "%064llx%064llx%064llx%064llx", UInt64(1), UInt64(1), period, version)
    let funding = try XCTUnwrap(try rpc("eth_sendTransaction", [["from":operators[4], "to":venue, "data":data]]) as? String)
    _ = try rpc("anvil_mine", ["0x04"])
    try tap("포지션 1 확인")
    XCTAssertTrue(app.staticTexts[funding].waitForExistence(timeout: 12), app.debugDescription)

    XCTAssertEqual(try rpc("evm_revert", [snapshot]) as? Bool, true)
    try tap("포지션 1 확인")
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format:"label ENDSWITH %@", "버전 1")).firstMatch.waitForExistence(timeout: 12), app.debugDescription)
    XCTAssertFalse(app.staticTexts[funding].exists)
    try publishLatest()
    try tap("이 포지션 종료 견적 준비")
    try tap("견적 받기")
    try tap("견적 확인 및 학습 지갑에서 승인")
    try finish()
    XCTAssertEqual(try balance(quote),collateralBefore)

    try publishLatest()
    try tap("모의 환전"); try tap("LAB → LAQ 모의 환전")
    try fill("가상 매도 LAB", "2"); try fill("최소 가상 수령 LAQ", "2")
    try tap("견적 받기")
    XCTAssertTrue(app.staticTexts["예상 2.4 LAQ"].waitForExistence(timeout: 12))
    try publishLatest(1_210_000)
    try tap("견적 확인 및 학습 지갑에서 승인")
    XCTAssertTrue(app.staticTexts["견적 조건이 달라졌거나 네트워크 응답을 확인할 수 없습니다."].waitForExistence(timeout: 10))
    XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "원래 거래 해시 0x")).firstMatch.exists)
    try tap("새 견적 받기")
    XCTAssertTrue(app.staticTexts["예상 2.42 LAQ"].waitForExistence(timeout: 12))

  }

  private var fixture: (user: String, venue: String, base: String, quote: String)?

  private func enrollFixture() throws -> (String, String, String, String) {
    continueAfterFailure = false
    let profile = try XCTUnwrap(try json(service)["profile"] as? [String: Any])
    let deploymentHash = try XCTUnwrap(profile["deploymentHash"] as? String)
    let venue = try XCTUnwrap((profile["venue"] as? [String: String])?["address"])
    let base = try XCTUnwrap((profile["base"] as? [String: String])?["address"])
    let quote = try XCTUnwrap((profile["quote"] as? [String: String])?["address"])
    app.launch()
    try tap("테스트 시장 학습")
    XCTAssertTrue(app.textFields["학습 배포 블록 해시"].waitForExistence(timeout: 5),
      "Run each scenario on a fresh simulator. Existing enrollments are preserved.")
    try fill("학습 배포 블록 해시", deploymentHash)
    try tap("격리 학습 지갑 만들기")
    let wallet = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "전용 학습 지갑 0x")).firstMatch
    XCTAssertTrue(wallet.waitForExistence(timeout: 15), app.debugDescription)
    let user = String(wallet.label.suffix(42))
    XCTAssertTrue(user.hasPrefix("0x"))
    try faucet(user, base, quote, venue)
    try tap("가격과 잔액 확인")
    return (user, venue, base, quote)
  }

  private func faucet(_ user: String, _ base: String, _ quote: String, _ venue: String) throws {
    let accounts = try XCTUnwrap(try rpc("eth_accounts") as? [String])
    _ = try rpc("anvil_setBalance", [user, "0x3635c9adc5dea00000"])
    let destination = String(user.dropFirst(2))
    for (token, amount) in [(base, 100_000_000_000 as UInt64), (quote, 1_000_000_000 as UInt64)] {
      let paddedAmount = String(format: "%064llx", amount)
      _ = try rpc("eth_sendTransaction", [["from": accounts[0], "to": token,
        "data": "0x40c10f19" + String(repeating: "0", count: 24) + destination + paddedAmount]])
    }
    let block = try XCTUnwrap(try rpc("eth_getBlockByNumber", ["latest", false]) as? [String: Any])
    let etime = try XCTUnwrap(block["timestamp"] as? String)
    let time = try XCTUnwrap(UInt64(etime.dropFirst(2), radix: 16))
    for source in accounts[2...3] {
      _ = try rpc("eth_sendTransaction", [["from": source, "to": venue,
        "data": "0x07640c1f" + String(format: "%064llx%064llx", 1_200_000 as UInt64, time)]])
    }
    _ = try rpc("anvil_mine", ["0x03"])
  }

  private func finish() throws {
    _ = try pendingHash()
    _ = try rpc("anvil_mine", ["0x04"])
    try tap("원래 거래 확정 확인")
    let confirmed = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "거래 확정 · 블록")).firstMatch
    XCTAssertTrue(confirmed.waitForExistence(timeout: 12), app.debugDescription)
  }

  private func pendingHash() throws -> String {
    let hashText = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "원래 거래 해시 0x")).firstMatch
    XCTAssertTrue(hashText.waitForExistence(timeout: 15), app.debugDescription)
    return String(hashText.label.suffix(66))
  }

  private func tap(_ label: String) throws {
    let button = app.buttons[label]
    XCTAssertTrue(button.waitForExistence(timeout: 15), app.debugDescription)
    for _ in 0..<6 {
      if button.isHittable { break }
      app.swipeUp()
    }
    for _ in 0..<9 where !button.isHittable { app.swipeDown() }
    XCTAssertTrue(button.isHittable && button.isEnabled, app.debugDescription)
    button.tap()
  }

  private func fill(_ label: String, _ text: String) throws {
    let field = app.textFields[label]
    XCTAssertTrue(field.waitForExistence(timeout: 12), app.debugDescription)
    for _ in 0..<4 {
      if field.frame.midY < 540 && field.isHittable { break }
      app.swipeUp()
    }
    XCTAssertTrue(field.isHittable, app.debugDescription)
    field.tap()
    field.typeText(text)
  }

  private func rpc(_ method: String, _ params: [Any] = []) throws -> Any {
    let reply = try json(node, ["jsonrpc": "2.0", "id": 1, "method": method, "params": params])
    XCTAssertNil(reply["error"], String(describing: reply))
    return try XCTUnwrap(reply["result"])
  }

  private func json(_ url: String, _ body: [String: Any]? = nil) throws -> [String: Any] {
    var request = URLRequest(url: URL(string: url)!, timeoutInterval: 8)
    if let body {
      request.httpMethod = "POST"
      request.setValue("application/json", forHTTPHeaderField: "Content-Type")
      request.httpBody = try JSONSerialization.data(withJSONObject: body)
    }
    let complete = expectation(description: url)
    var output: Data?
    var failure: Error?
    URLSession.shared.dataTask(with: request) { data, _, error in
      output = data
      failure = error
      complete.fulfill()
    }.resume()
    wait(for: [complete], timeout: 9)
    XCTAssertNil(failure)
    return try XCTUnwrap(JSONSerialization.jsonObject(with: try XCTUnwrap(output)) as? [String: Any])
  }
}

final class SubscriptionRouteSmoke: XCTestCase {
  private let renter = XCUIApplication(bundleIdentifier: "com.nu54renter.custom")

  func testSubscriptionURLRequiresAnEnrolledCustomer() throws {
    renter.terminate()
    let route = try XCTUnwrap(URL(string: "nu54://subscriptions/ab3cd900-3fb6-4821-ac19-48cb448f6633"))
    renter.open(route)
    XCTAssertTrue(renter.staticTexts["내 매장 주문"].waitForExistence(timeout: 25), renter.debugDescription)
    XCTAssertTrue(renter.buttons["주문 등록 공개키 보기"].exists, renter.debugDescription)
    XCTAssertFalse(renter.buttons["같은 회차 결제 검토"].exists, renter.debugDescription)
  }
}

final class SubscriptionPermissionSmoke: XCTestCase {
  func testAllowPendingSubscriptionPermissionPrompt() {
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    let allow = springboard.buttons.matching(NSPredicate(format: "label IN %@", ["허용", "Allow"])).firstMatch
    XCTAssertTrue(allow.waitForExistence(timeout: 15), springboard.debugDescription)
    allow.tap()
    XCTAssertFalse(allow.exists)
  }
}

final class SubscriptionNotificationTapSmoke: XCTestCase {
  func testDeliveredLocalReminderOpensPrivateEnrollmentRoute() {
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    let notification = springboard.staticTexts["확인할 구독 회차가 있습니다"].firstMatch
    if !notification.exists {
      springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.4, dy: 0.004))
        .press(forDuration: 0.1, thenDragTo: springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.4, dy: 0.75)))
    }
    XCTAssertTrue(notification.waitForExistence(timeout: 25), springboard.debugDescription)
    notification.tap()
    let renter = XCUIApplication(bundleIdentifier: "com.nu54renter.custom")
    XCTAssertTrue(renter.staticTexts["내 매장 주문"].waitForExistence(timeout: 25), renter.debugDescription)
    XCTAssertTrue(renter.buttons["주문 등록 공개키 보기"].exists, renter.debugDescription)
    XCTAssertFalse(renter.buttons["같은 회차 결제 검토"].exists)
  }
}

final class SubscriptionEnrolledSmoke: XCTestCase {
  private let app = XCUIApplication(bundleIdentifier: "com.nu54renter.custom")

  func testLocalNotificationOpensTheExactAuthenticatedUnpaidInvoice() throws {
    XCUIDevice.shared.press(.home)
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    let alert = springboard.staticTexts["확인할 구독 회차가 있습니다"].firstMatch
    // The hosted scheduler has just queued the alert; prefer the home-screen banner
    // before opening Notification Center, which can leave the simulator locked.
    if !alert.waitForExistence(timeout: 16) {
      springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.4, dy: 0.004))
        .press(forDuration: 0.1, thenDragTo: springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.4, dy: 0.75)))
    }
    guard alert.waitForExistence(timeout: 25) else {
      XCTFail("Local notification was not visible: " + springboard.debugDescription)
      return
    }
    alert.tap()
    if !app.wait(for: .runningForeground, timeout: 6), alert.exists {
      alert.tap()
    }
    try assertUnpaidOrder()
  }

  func testDirectAuthenticatedURLControl() throws {
    let invoice = try XCTUnwrap(ProcessInfo.processInfo.environment["NU54_SUBSCRIPTION_DIRECT_INVOICE"])
    let route = try XCTUnwrap(URL(string: "nu54://subscriptions/" + invoice))
    XCUIDevice.shared.press(.home)
    app.open(route)
    try assertUnpaidOrder()
  }

  private func assertUnpaidOrder() throws {
    let expected = try XCTUnwrap(ProcessInfo.processInfo.environment["NU54_SUBSCRIPTION_SALE"])
    let review = app.buttons["같은 회차 결제 검토"]
    guard review.waitForExistence(timeout: 20) else {
      let screenshot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
      screenshot.lifetime = .keepAlways
      add(screenshot)
      XCTFail("No authenticated invoice: " + app.debugDescription)
      return
    }
    XCTAssertEqual(app.buttons.matching(identifier: "같은 회차 결제 검토").count, 1)
    review.tap()
    let target = app.staticTexts[expected]
    XCTAssertTrue(target.waitForExistence(timeout: 15), app.debugDescription)
    XCTAssertTrue(app.staticTexts["회차가 연결되었습니다. 보드 승인 대기 중입니다."].exists, app.debugDescription)
    XCTAssertFalse(app.staticTexts["서버 지급 확인"].exists)
    XCTAssertTrue(app.buttons["보드 요약 검토"].exists, app.debugDescription)
  }
}

final class SubscriptionBoardReviewSmoke: XCTestCase {
  func testEnableRealNotificationPermission() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    app.launch()
    let payments = app.buttons["내 결제"]
    XCTAssertTrue(payments.waitForExistence(timeout: 15))
    payments.tap()
    let subscriptions = app.buttons["구독 관리"]
    XCTAssertTrue(subscriptions.waitForExistence(timeout: 15))
    for _ in 0..<5 where !subscriptions.isHittable { app.swipeUp() }
    subscriptions.tap()
    let enable = app.buttons["회차 알림 허용"]
    let disable = app.buttons["회차 알림 끄기"]
    if disable.waitForExistence(timeout: 3) { return }
    XCTAssertTrue(enable.waitForExistence(timeout: 10), app.debugDescription)
    for _ in 0..<5 where !enable.isHittable { app.swipeUp() }
    enable.tap()
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    let allow = springboard.alerts.buttons.matching(NSPredicate(format: "label == %@ OR label == %@", "Allow", "허용")).firstMatch
    if allow.waitForExistence(timeout: 5) { allow.tap() }
    XCTAssertTrue(disable.waitForExistence(timeout: 12), app.debugDescription)
  }

  func testNotificationReviewRequiresBoardAndExplicitSubmission() throws {
    continueAfterFailure = false
    let environment = ProcessInfo.processInfo.environment
    let endpoint = try XCTUnwrap(environment["NU54_PAYMENT_CONTROL"])
    let secret = try XCTUnwrap(environment["NU54_PAYMENT_CONTROL_TOKEN"])
    let invoice = try XCTUnwrap(environment["NU54_SUBSCRIPTION_DIRECT_INVOICE"])
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    let alert = springboard.staticTexts["확인할 구독 회차가 있습니다"].firstMatch
    let deeplink = try XCTUnwrap(URL(string: "nu54://subscriptions/" + invoice))

    func stage(_ name: String) throws -> [String: Any] {
      var request = URLRequest(url: try XCTUnwrap(URL(string: endpoint + "/" + name)))
      request.httpMethod = "POST"
      request.setValue(secret, forHTTPHeaderField: "Authorization")
      let completed = expectation(description: name)
      var result = Data()
      URLSession.shared.dataTask(with: request) { body, response, error in
        XCTAssertNil(error)
        XCTAssertEqual((response as? HTTPURLResponse)?.statusCode, 200)
        result = body ?? Data()
        completed.fulfill()
      }.resume()
      wait(for: [completed], timeout: 40)
      return (try? JSONSerialization.jsonObject(with: result)) as? [String: Any] ?? [:]
    }

    func text(_ fragment: String) {
      let target = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", fragment)).firstMatch
      XCTAssertTrue(target.waitForExistence(timeout: 15), app.debugDescription)
    }

    func tap(_ button: XCUIElement) {
      XCTAssertTrue(button.waitForExistence(timeout: 15), app.debugDescription)
      for _ in 0..<5 where !button.isHittable { app.swipeUp(velocity: .fast) }
      for _ in 0..<5 where !button.isHittable { app.swipeDown(velocity: .fast) }
      XCTAssertTrue(button.isHittable && button.isEnabled, app.debugDescription)
      button.tap()
    }

    XCUIDevice.shared.press(.home)
    if !alert.waitForExistence(timeout: 16) {
      springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.4, dy: 0.004))
        .press(forDuration: 0.1, thenDragTo: springboard.coordinate(withNormalizedOffset: CGVector(dx: 0.4, dy: 0.75)))
    }
    XCTAssertTrue(alert.waitForExistence(timeout: 25))
    alert.tap()
    if !app.wait(for: .runningForeground, timeout: 6), alert.exists { alert.tap() }
    tap(app.buttons["같은 회차 결제 검토"])
    text("청구서 " + invoice)
    text("금액 4.00 TestUSDC")
    _ = try stage("notification-opened")

    tap(app.buttons["보드 요약 검토"])
    tap(app.buttons["원견적 상세"])
    text("청구서 " + invoice)
    text("네트워크 StableNet 테스트넷 · 8283")
    text("제출 대행 · 매장 릴레이")
    let quote = try stage("quote-reviewed")
    text(try XCTUnwrap(quote["quoteDigest"] as? String))
    tap(app.buttons["견적 확인으로"])
    tap(app.buttons["검증한 보드 찾기"])
    tap(app.buttons.matching(identifier: "NU54 W04 QA에서 결제 검토").element(boundBy: 1))
    _ = try stage("display-ready")
    XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
    _ = try stage("physical-approved")
    text("보드 서명 확인")
    _ = try stage("board-signed")
    tap(app.buttons["지급 요청 제출"])
    text("매장이 지급 요청을 받았습니다.")
    let result = try stage("explicitly-submitted")

    app.terminate()
    app.open(deeplink)
    let receipt = try XCTUnwrap(result["receipt"] as? String)
    let hash = try XCTUnwrap(result["hash"] as? String)
    let sale = try XCTUnwrap(result["sale"] as? String)
    tap(app.buttons["기존 회차 보기"])
    text(invoice)
    text(sale)
    text(receipt)
    text(hash)
    XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
    _ = try stage("original-reopened")
  }
}

final class PublicTravelSceneSmoke: XCTestCase {
  func testFreshPublicCatalogMapFallbackAndPrivateBookingGate() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.travel.acceptance")
    app.launch()
    func tap(_ label: String, timeout: TimeInterval = 12) {
      let button = app.buttons[label].firstMatch
      XCTAssertTrue(button.waitForExistence(timeout: timeout), app.debugDescription)
      button.tap()
    }
    tap("여행지 찾기", timeout: 25)
    tap("서울")
    XCTAssertTrue(app.staticTexts["시험용 장소 데이터"].exists, app.debugDescription)
    tap("공개 명소")
    XCTAssertFalse(app.buttons["예약 가능한 시간"].exists, app.debugDescription)
    tap("지도")
    tap("목록 보기")
    XCTAssertTrue(app.buttons["공개 명소"].exists, app.debugDescription)
    tap("지역 맛집")
    tap("예약 가능한 시간")
    XCTAssertTrue(app.staticTexts["등록된 고객 자격과 현재 예약 가능 여부를 확인해 주세요."].waitForExistence(timeout: 12),
                  app.debugDescription)
    XCTAssertFalse(app.buttons["이 조건으로 예약 보류"].exists, app.debugDescription)
    let screenshot = XCTAttachment(screenshot: app.screenshot())
    screenshot.name = "Public search and private checkout boundary"
    screenshot.lifetime = .keepAlways
    add(screenshot)
  }
}

final class PrivateTravelSceneSmoke: XCTestCase {
  func testSeparateNativeEnrollmentConsentAndWithdrawal() throws {
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.travel.itinerary-qa")
    app.launch()
    func tap(_ label: String, timeout: TimeInterval = 15) {
      let target = app.buttons[label].firstMatch
      XCTAssertTrue(target.waitForExistence(timeout: timeout))
      for _ in 0..<3 where !target.isHittable { app.swipeUp() }
      target.tap()
    }
    tap("여행지 찾기", timeout: 25)
    tap("여행 기록과 동의")
    if app.buttons["개인 기록 등록 공개키"].waitForExistence(timeout: 4) {
      tap("개인 기록 등록 공개키")
      let publicKey = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
      XCTAssertTrue(publicKey.waitForExistence(timeout: 12))
      var req = URLRequest(url: try XCTUnwrap(URL(string: "http://127.0.0.1:65142/enroll")))
      req.httpMethod = "POST"
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
      tap("개인 기록 등록 확인")
    }
    tap("음성 공유 선택")
    tap("이 목적과 수신 범위에 동의")
    tap("음성 공유 철회")
    XCTAssertTrue(app.buttons["음성 공유 선택"].waitForExistence(timeout: 10))
    XCTAssertFalse(app.buttons["음성 공유 철회"].exists)
    app.swipeDown()
    tap("장소 검색으로 돌아가기")
    tap("서울")
    tap("공개 명소")
    app.swipeDown()
    tap("여행 기록과 동의")
    tap("여행 추천과 일정 선택")
    tap("이 목적과 수신 범위에 동의")
    app.swipeDown()
    tap("내 일정과 추천")
    tap("날짜와 선호로 코스 추천")
    XCTAssertTrue(app.staticTexts["추천 일정 검토"].waitForExistence(timeout: 70))
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "게시된 영업시간")).firstMatch.exists)
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "출처 ")).firstMatch.exists)
    XCTAssertFalse(app.textFields["기존 일정 ID"].exists)
    let previewCapture = XCTAttachment(screenshot: app.screenshot())
    previewCapture.name = "Named itinerary proposal with hours and source"
    previewCapture.lifetime = .keepAlways
    add(previewCapture)
    tap("검토한 일정 적용")
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "버전 1")).firstMatch.waitForExistence(timeout: 10))
    let saved = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", " · ")).firstMatch
    XCTAssertTrue(saved.waitForExistence(timeout: 10))
    tap(saved.label)
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "버전 1")).firstMatch.exists)
    let screenshot = XCTAttachment(screenshot: app.screenshot())
    screenshot.name = "Reopened saved trip by place and date"
    screenshot.lifetime = .keepAlways
    add(screenshot)
  }
}

final class TravelMicrophoneSmoke: XCTestCase {
  func testSystemPermissionDenialKeepsAudioLocalAndEmpty() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.travel.itinerary-qa")
    app.launch()
    let discover = app.buttons["여행지 찾기"]
    XCTAssertTrue(discover.waitForExistence(timeout: 25))
    discover.tap()
    let records = app.buttons["여행 기록과 동의"]
    XCTAssertTrue(records.waitForExistence(timeout: 8))
    records.tap()
    let start = app.buttons["음성 녹음 시작"]
    XCTAssertTrue(start.waitForExistence(timeout: 8))
    start.tap()
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    let deny = springboard.alerts.buttons.matching(NSPredicate(format: "label IN %@",
      ["Don’t Allow", "Don't Allow", "허용 안 함", "허용하지 않음"])).firstMatch
    XCTAssertTrue(deny.waitForExistence(timeout: 8), springboard.debugDescription)
    deny.tap()
    XCTAssertTrue(app.staticTexts["마이크 권한 또는 녹음 상태를 확인해 주세요."].waitForExistence(timeout: 10))
    XCTAssertFalse(app.buttons["녹음 종료하고 이 기기에만 저장"].exists)
    XCTAssertFalse(app.buttons["선택한 녹음 한 건 공유"].exists)
  }
}

final class TravelErasureSceneSmoke: XCTestCase {
  func testSelectedReceiptRestartLeavesSecondRecord() throws {
    try checkReceipt(installation: "qa")
    try checkReceipt(installation: "observer")
  }

  private func checkReceipt(installation: String) throws {
    continueAfterFailure = false
    let namespace = try XCTUnwrap(ProcessInfo.processInfo.environment["NU54_T12_QA_NAMESPACE"])
    let app = XCUIApplication(bundleIdentifier: namespace + "-" + installation)
    app.launch()
    defer { app.terminate() }
    func tap(_ label: String, timeout: TimeInterval = 15) {
      let target = app.buttons[label].firstMatch
      XCTAssertTrue(target.waitForExistence(timeout: timeout))
      for _ in 0..<8 where !target.isHittable { app.swipeUp() }; for _ in 0..<14 where !target.isHittable { app.swipeDown() }
      target.tap()
    }
    tap("여행지 찾기", timeout: 25)
    tap("여행 기록과 동의")
    guard app.buttons["개인 기록 등록 공개키"].waitForExistence(timeout: 4) else {
      throw TravelPasteFailure.unavailable
    }
    do {
      tap("개인 기록 등록 공개키")
      let publicKey = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
      XCTAssertTrue(publicKey.waitForExistence(timeout: 12))
      var req = URLRequest(url: try XCTUnwrap(URL(string: "http://127.0.0.1:64142/"+(installation == "qa" ? "enroll" : "enroll-" + installation))))
      req.httpMethod = "POST"
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
      tap("개인 기록 등록 확인")
    }
    if installation == "observer" {
      tap("여행 기록 다시 읽기")
      XCTAssertFalse(app.buttons["삭제 영수증 보기"].exists)
      XCTAssertFalse(app.buttons["이 기록 삭제 검토"].exists)
      return
    }
    if installation == "peer" {
      tap("여행 기록 다시 읽기")
      tap("삭제 영수증 보기")
      XCTAssertTrue(app.staticTexts["요청한 기기의 복사본 삭제 확인 대기 중"].waitForExistence(timeout: 12))
      XCTAssertFalse(app.buttons["이 기기의 복사본 삭제와 확인"].exists)
      return
    }
    let review = app.buttons.matching(NSPredicate(format:"label == %@", "이 기록 삭제 검토"))
    XCTAssertTrue(review.firstMatch.waitForExistence(timeout: 12))
    XCTAssertEqual(review.count, 2)
    tap("이 기록 삭제 검토")
    tap("이 기록과 파생물 삭제 요청")
    XCTAssertTrue(app.buttons["삭제 상태 다시 조회"].waitForExistence(timeout:10))
    app.terminate()
    try checkReceipt(installation: "peer")
    app.launch()
    tap("여행지 찾기")
    tap("여행 기록과 동의")
    tap("선택한 기록의 삭제 상태 열기")
    tap("이 기기의 복사본 삭제와 확인")
    tap("삭제 상태 다시 조회")
    XCTAssertTrue(app.staticTexts["서버 접근 차단 확인"].waitForExistence(timeout:10))
    XCTAssertTrue(app.staticTexts["삭제를 요청한 설치본이 복사본 삭제를 보고했습니다."].waitForExistence(timeout:10))
    tap("완료한 삭제 상태 닫기")
    tap("여행 기록 다시 읽기")
    XCTAssertEqual(review.count, 1)
    tap("삭제 영수증 보기")
    XCTAssertTrue(app.staticTexts["서버 접근 차단 확인"].exists)
    XCTAssertTrue(app.staticTexts["삭제를 요청한 설치본이 복사본 삭제를 보고했습니다."].exists)
    XCTAssertFalse(app.buttons["이 기기의 복사본 삭제와 확인"].exists)
    tap("삭제 영수증 닫기")
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

final class PracticeSmoke: XCTestCase {
  private let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w02practice")
  private func tap(_ label: String) {
    let button = app.buttons[label]
    for _ in 0..<8 where !button.isHittable { app.swipeUp() }
    XCTAssertTrue(button.waitForExistence(timeout: 10), app.debugDescription)
    button.tap()
  }
  func testLargeTextPracticeComparison() {
    app.launchArguments += ["-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityXXXL"]
    app.launch()
    if app.buttons["결제 연습"].exists { tap("결제 연습") }
    else if app.buttons["연습 이어하기"].exists { tap("연습 이어하기") }
    if app.buttons["다른 연습 선택"].exists { tap("다른 연습 선택") }
    tap("틀린 금액 찾아보기")
    let comparison = app.otherElements.matching(NSPredicate(format: "label CONTAINS %@", "주문: 연습 카페")).firstMatch
    XCTAssertTrue(comparison.waitForExistence(timeout: 10), app.debugDescription)
    let capture = XCTAttachment(screenshot: app.screenshot())
    capture.name = "W02-practice-large-text"
    capture.lifetime = .keepAlways
    add(capture)
    tap("연습 거절")
    XCTAssertTrue(app.staticTexts["실제 지급 0건 · 서명 0건"].waitForExistence(timeout: 10))
  }
  func testWrongAmountResumeAndEnglishApproval() {
    app.launch()
    if app.buttons["연습 이어하기"].exists { tap("연습 이어하기") }
    else { tap("결제 연습") }
    if app.buttons["다른 연습 선택"].exists { tap("다른 연습 선택") }
    tap("틀린 금액 찾아보기")
    tap("연습 승인")
    XCTAssertTrue(app.staticTexts["금액이 다릅니다. 이 요청은 거절하세요."].waitForExistence(timeout: 10))
    tap("나중에 이어하기")
    app.terminate()
    app.launch()
    tap("연습 이어하기")
    tap("연습 거절")
    XCTAssertTrue(app.staticTexts["실제 지급 0건 · 서명 0건"].waitForExistence(timeout: 10))
    tap("다른 연습 선택")
    for _ in 0..<8 where !app.buttons["English"].isHittable { app.swipeDown() }
    tap("English")
    tap("Compare matching amounts")
    tap("Continue later")
    app.terminate()
    app.launch()
    tap("Resume practice")
    tap("Approve practice")
    XCTAssertTrue(app.staticTexts["Practice approved"].waitForExistence(timeout: 10))
    XCTAssertTrue(app.staticTexts["Real payments: 0 · Signatures: 0"].exists)
  }
}

final class PracticeUpgradeSmoke: XCTestCase {
  func testSupportedPreviousProfileOpensAfterUpgrade() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w02upgrade")
    app.launch()
    XCTAssertTrue(app.staticTexts["개인 지갑"].waitForExistence(timeout: 25), app.debugDescription)
    XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "보호된 지갑 기록을 열 수 없습니다")).firstMatch.exists)
  }
}

final class KioskExistingSmoke: XCTestCase {
  func testExistingTerminalRemainsAvailable() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54kiosk.custom")
    app.launch()
    XCTAssertTrue(app.staticTexts["매장 주문"].waitForExistence(timeout:25), app.debugDescription)
    XCTAssertTrue(app.buttons["새 고객 주문"].waitForExistence(timeout:10), app.debugDescription)
  }
}


final class KioskCatalogSmoke: XCTestCase {
  func testPublishedCatalogKeepsSelectionAcrossLanguages() {
    let kiosk = XCUIApplication(bundleIdentifier: "com.nu54kiosk.cataloglab")
    kiosk.launch()
    let start = kiosk.buttons["새 고객 주문"]
    XCTAssertTrue(start.waitForExistence(timeout: 25), kiosk.debugDescription)
    start.tap()
    let tea = kiosk.buttons["진한 차 뜨거움 추가"]
    XCTAssertTrue(tea.waitForExistence(timeout: 10), kiosk.debugDescription)
    tea.tap()
    XCTAssertTrue(kiosk.staticTexts["가능 수량 2 · 선택 1"].exists, kiosk.debugDescription)
    kiosk.buttons["메뉴 언어 English"].tap()
    XCTAssertTrue(kiosk.buttons["Strong tea Hot Add"].exists, kiosk.debugDescription)
    XCTAssertTrue(kiosk.staticTexts["가능 수량 2 · 선택 1"].exists, kiosk.debugDescription)
    XCTAssertTrue(kiosk.staticTexts.containing(NSPredicate(format: "label CONTAINS[c] %@", "milk")).firstMatch.exists)
  }

  func testReserveRemainingUnitAndClearPublicStateOnRestart() {
    let kiosk = XCUIApplication(bundleIdentifier: "com.nu54kiosk.cataloglab")
    kiosk.launch()
    kiosk.buttons["새 고객 주문"].tap()
    let item = kiosk.buttons["진한 차 뜨거움 추가"]
    XCTAssertTrue(item.waitForExistence(timeout: 10), kiosk.debugDescription)
    XCTAssertTrue(kiosk.staticTexts["가능 수량 1 · 선택 0"].exists)
    item.tap()
    kiosk.buttons["주문 접수 요청"].tap()
    XCTAssertTrue(kiosk.staticTexts["보드 승인 대기"].waitForExistence(timeout: 10), kiosk.debugDescription)
    kiosk.terminate()
    kiosk.launch()
    XCTAssertTrue(kiosk.buttons["새 고객 주문"].waitForExistence(timeout: 10))
    XCTAssertFalse(kiosk.otherElements["이 주문을 여는 일회용 QR"].exists)
  }

  func testSoldOutAfterSecondOrder() {
    let kiosk = XCUIApplication(bundleIdentifier: "com.nu54kiosk.cataloglab")
    kiosk.launch()
    kiosk.buttons["새 고객 주문"].tap()
    XCTAssertTrue(kiosk.staticTexts["품절"].waitForExistence(timeout: 10), kiosk.debugDescription)
    XCTAssertFalse(kiosk.buttons["진한 차 뜨거움 추가"].isEnabled)
  }
}


final class RenterCatalogSmoke: XCTestCase {
  func testPrivatePaymentListAfterRestartAndLinkDenial() {
    let renter = XCUIApplication(bundleIdentifier: "com.nu54renter.cataloglab")
    let branch = "5dc8d4be-c24d-4553-ab79-9c30f630e477"
    let owned = "e8ff8bac-1556-4ee1-9a0f-8b27338135e6"
    let foreign = "e8ff8bac-1556-4ee1-9a0f-8b27338135e7"
    let amount = renter.staticTexts["금액 6.50 TestUSDC"]
    let denied = renter.staticTexts["이 계정에서 열 수 없는 주문입니다."]
    let foreignRow = renter.buttons["매장 주문 \(foreign)"]
    let open = { (order: String, offset: Int) in
      let expiry = Int(Date().timeIntervalSince1970) + offset
      renter.open(URL(string: "nu54://order?v=2&branch=\(branch)&order=\(order)&expires=\(expiry)")!)
    }
    let openOwnedFromHome = {
      renter.launch()
      let payments = renter.buttons["내 결제"]
      XCTAssertTrue(payments.waitForExistence(timeout: 25), renter.debugDescription)
      payments.tap()
      let row = renter.buttons["매장 주문 \(owned)"]
      XCTAssertTrue(row.waitForExistence(timeout: 15), renter.debugDescription)
      XCTAssertFalse(foreignRow.exists)
      row.tap()
      XCTAssertTrue(amount.waitForExistence(timeout: 10), renter.debugDescription)
    }
    openOwnedFromHome()
    renter.terminate()
    openOwnedFromHome()
    open(foreign, 50)
    XCTAssertTrue(denied.waitForExistence(timeout: 12), renter.debugDescription)
    XCTAssertFalse(foreignRow.exists)
    XCTAssertFalse(amount.exists)
    open(owned, -1)
    XCTAssertTrue(denied.waitForExistence(timeout: 12), renter.debugDescription)
    XCTAssertFalse(renter.buttons["매장 QR 대조하기"].exists)
    XCTAssertFalse(amount.exists)
  }

  func testInstalledQuoteCancelAndOriginalReceiptRecovery() throws {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.cataloglab")
    let fixture = ProcessInfo.processInfo.environment
    let control = try XCTUnwrap(fixture["NU54_PAYMENT_CONTROL"])
    let token = try XCTUnwrap(fixture["NU54_PAYMENT_CONTROL_TOKEN"])
    let branch = "5dc8d4be-c24d-4553-ab79-9c30f630e477"
    let reviewOrder = "e8ff8bac-1556-4ee1-9a0f-8b27338135e6"
    let receiptOrder = "e8ff8bac-1556-4ee1-9a0f-8b27338135e7"
    let originalHash = "0x" + String(repeating: "7", count: 64)
    func text(_ value: String) -> XCUIElement {
      app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", value)).firstMatch
    }
    func act(_ value: String) {
      let button = app.buttons[value]
      XCTAssertTrue(button.waitForExistence(timeout: 20), app.debugDescription)
      for _ in 0..<4 where !button.isHittable { app.swipeUp(velocity: .fast) }
      XCTAssertTrue(button.isHittable && button.isEnabled, app.debugDescription)
      button.tap()
    }
    func qr(_ order: String) {
      let expiry = Int(Date().timeIntervalSince1970) + 60
      app.open(URL(string: "nu54://order?v=2&branch=\(branch)&order=\(order)&expires=\(expiry)")!)
    }
    func change(_ command: String) throws {
      var request = URLRequest(url: try XCTUnwrap(URL(string: "\(control)/\(command)")))
      request.setValue(token, forHTTPHeaderField: "Authorization")
      request.httpMethod = "POST"
      let done = expectation(description: command)
      URLSession.shared.dataTask(with: request) { _, response, error in
        XCTAssertNil(error)
        XCTAssertEqual((response as? HTTPURLResponse)?.statusCode, 200)
        done.fulfill()
      }.resume()
      wait(for: [done], timeout: 8)
    }
    qr(reviewOrder)
    XCTAssertTrue(text("6.50 TestUSDC").waitForExistence(timeout: 15), app.debugDescription)
    act("원견적 상세")
    XCTAssertTrue(text("가맹점 식별자").waitForExistence(timeout: 10), app.debugDescription)
    XCTAssertTrue(text("0x" + String(repeating: "3", count: 40)).exists)
    act("견적 확인으로")
    act("검증한 보드 찾기")
    act("승인 종료")
    XCTAssertTrue(text("승인 대기가 끝났습니다").waitForExistence(timeout: 12), app.debugDescription)
    app.terminate()
    qr(reviewOrder)
    XCTAssertTrue(text("승인 대기가 끝났습니다").waitForExistence(timeout: 12), app.debugDescription)
    XCTAssertFalse(app.buttons["검증한 보드 찾기"].exists)
    XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
    qr(receiptOrder)
    XCTAssertTrue(text(originalHash).waitForExistence(timeout: 12), app.debugDescription)
    XCTAssertFalse(text("영수증 번호").exists)
    XCTAssertFalse(text("픽업 번호").exists)
    try change("temporary-unavailable")
    act("같은 주문의 지급 결과 확인")
    XCTAssertTrue(text("네트워크 응답을 기다립니다").waitForExistence(timeout: 12), app.debugDescription)
    XCTAssertTrue(text(receiptOrder).exists)
    try change("confirm-original")
    act("같은 주문의 지급 결과 확인")
    let receipt = text("영수증 번호")
    XCTAssertTrue(receipt.waitForExistence(timeout: 12), app.debugDescription)
    let originalReceipt = receipt.label
    XCTAssertTrue(text(originalHash).exists)
    XCTAssertTrue(text("픽업 번호").exists)
    app.terminate()
    app.launch()
    act("내 결제")
    act("매장 주문 \(receiptOrder)")
    XCTAssertTrue(text(originalReceipt).waitForExistence(timeout: 12), app.debugDescription)
    XCTAssertTrue(text(originalHash).exists)
    XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
  }

  func testSimulatedBleBoundaryUsesP01ApprovalOnlyAfterExplicitReview() throws {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    let fixture = ProcessInfo.processInfo.environment
    let control = try XCTUnwrap(fixture["NU54_PAYMENT_CONTROL"])
    let token = try XCTUnwrap(fixture["NU54_PAYMENT_CONTROL_TOKEN"])
    let branch = "5dc8d4be-c24d-4553-ab79-9c30f630e477"
    let sale = "e8ff8bac-1556-4ee1-9a0f-8b27338135e6"
    func evidence(_ label: String) -> XCUIElement {
      app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", label)).firstMatch
    }
    func tap(_ node: XCUIElement) {
      XCTAssertTrue(node.waitForExistence(timeout: 15), app.debugDescription)
      for _ in 0..<4 where !node.isHittable { app.swipeUp(velocity: .fast) }
      for _ in 0..<4 where !node.isHittable { app.swipeDown(velocity: .fast) }
      XCTAssertTrue(node.isHittable && node.isEnabled, app.debugDescription)
      node.tap()
    }
    func qr() {
      let expiry = Int(Date().timeIntervalSince1970) + 60
      app.open(URL(string: "nu54://order?v=2&branch=\(branch)&order=\(sale)&expires=\(expiry)")!)
    }
    func board() {
      tap(app.buttons["검증한 보드 찾기"])
      let candidates = app.buttons.matching(identifier: "NU54 W04 QA에서 결제 검토")
      tap(candidates.element(boundBy: 1))
    }
    qr()
    XCTAssertTrue(evidence("6.50 TestUSDC").waitForExistence(timeout: 15), app.debugDescription)
    board()
    XCTAssertTrue(evidence("보드에 표시된 주문과 금액").waitForExistence(timeout: 10), app.debugDescription)
    tap(app.buttons["승인 종료"])
    XCTAssertTrue(evidence("승인 대기가 끝났습니다").waitForExistence(timeout: 10), app.debugDescription)
    XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
    tap(app.buttons["새 견적 확인"])
    XCTAssertTrue(app.buttons["검증한 보드 찾기"].waitForExistence(timeout: 12), app.debugDescription)
    XCTAssertFalse(app.buttons["지급 요청 제출"].exists)

    var request = URLRequest(url: try XCTUnwrap(URL(string: "\(control)/allow-board")))
    request.setValue(token, forHTTPHeaderField: "Authorization")
    request.httpMethod = "POST"
    let allowed = expectation(description: "isolated simulated physical approval")
    URLSession.shared.dataTask(with: request) { _, response, error in
      XCTAssertNil(error)
      XCTAssertEqual((response as? HTTPURLResponse)?.statusCode, 200)
      allowed.fulfill()
    }.resume()
    wait(for: [allowed], timeout: 8)

    board()
    XCTAssertTrue(evidence("보드 서명 확인").waitForExistence(timeout: 18), app.debugDescription)
    XCTAssertTrue(app.buttons["지급 요청 제출"].exists)
    app.terminate()
    qr()
    XCTAssertTrue(evidence("보드 서명 확인").waitForExistence(timeout: 18), app.debugDescription)
    XCTAssertTrue(app.buttons["지급 요청 제출"].exists)
    XCTAssertFalse(evidence("픽업 번호").exists)
  }

  func testRepricedCartSettlesAndReopensTheSameReceipt() throws {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    let consumer = XCUIApplication(bundleIdentifier: "com.nu54renter.cataloglab")
    let fixture = ProcessInfo.processInfo.environment
    let control = try XCTUnwrap(fixture["NU54_PAYMENT_CONTROL"])
    let token = try XCTUnwrap(fixture["NU54_PAYMENT_CONTROL_TOKEN"])
    func change(_ action: String) throws -> [String: Any] {
      var request = URLRequest(url: try XCTUnwrap(URL(string: "\(control)/\(action)")))
      request.setValue(token, forHTTPHeaderField: "Authorization")
      request.httpMethod = "POST"
      let done = expectation(description: action)
      var result = Data()
      URLSession.shared.dataTask(with: request) { data, response, error in
        XCTAssertNil(error)
        XCTAssertEqual((response as? HTTPURLResponse)?.statusCode, 200)
        result = data ?? Data()
        done.fulfill()
      }.resume()
      wait(for: [done], timeout: 40)
      return (try? JSONSerialization.jsonObject(with: result)) as? [String: Any] ?? [:]
    }
    func has(_ app: XCUIApplication, _ text: String, timeout: TimeInterval = 15) {
      XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", text))
        .firstMatch.waitForExistence(timeout: timeout), app.debugDescription)
    }
    func tap(_ app: XCUIApplication, _ button: XCUIElement) {
      XCTAssertTrue(button.waitForExistence(timeout: 15), app.debugDescription)
      for _ in 0..<5 where !button.isHittable { app.swipeUp(velocity: .fast) }
      for _ in 0..<5 where !button.isHittable { app.swipeDown(velocity: .fast) }
      XCTAssertTrue(button.isHittable && button.isEnabled, app.debugDescription)
      button.tap()
    }
    func act(_ label: String) { tap(app, app.buttons[label]) }
    app.launch()
    act("가까운 매장에서 주문")
    confirmNativeShop(app, expected: "Yeoul Coffee Seongsu")
    for item in ["Iced coffee", "Croissant"] {
      tap(app, app.buttons.containing(NSPredicate(format: "label BEGINSWITH %@ AND label ENDSWITH %@", item, "추가")).firstMatch)
    }
    has(app, "합계 · 6.50 TestUSDC")
    _ = try change("shown-original")
    act("주문 접수 요청")
    has(app, "기존 합계 · 6.50 TestUSDC")
    has(app, "새 합계 · 7.00 TestUSDC")
    _ = try change("review-shown")
    act("바뀐 장바구니 확인 후 주문 접수")
    has(app, "금액 7.00 TestUSDC")
    act("이 주문의 결제 내용 검토")
    act("원견적 상세")
    has(app, "수령 주소")
    has(app, "제출 대행 · 매장 릴레이")
    let quoteBinding = try change("quote-rechecked")
    has(app, try XCTUnwrap(quoteBinding["beneficiary"] as? String))
    has(app, try XCTUnwrap(quoteBinding["sale"] as? String))
    act("견적 확인으로")
    act("검증한 보드 찾기")
    tap(app, app.buttons.matching(identifier: "NU54 W04 QA에서 결제 검토").element(boundBy: 1))
    has(app, "보드 서명 확인")
    act("지급 요청 제출")
    has(app, "매장이 지급 요청을 받았습니다.")
    let final = try change("explicitly-submitted")
    let sale = try XCTUnwrap(final["sale"] as? String)
    let hash = try XCTUnwrap(final["hash"] as? String)
    let receipt = try XCTUnwrap(final["receipt"] as? String)
    app.terminate()
    consumer.terminate()
    consumer.launch()
    tap(consumer, consumer.buttons["내 결제"])
    tap(consumer, consumer.buttons["매장 주문 \(sale)"])
    has(consumer, "금액 7.00 TestUSDC")
    has(consumer, hash)
    has(consumer, receipt)
    has(consumer, "픽업 수령 완료")
    _ = try change("original-reopened")
  }

  func testPublishedCatalogKeepsSelectionAcrossLanguages() {
    let renter = XCUIApplication(bundleIdentifier: "com.nu54renter.cataloglab")
    renter.open(URL(string: "nu54://order")!)
    confirmNativeShop(renter)
    let tea = renter.buttons["진한 차 뜨거움 추가"]
    XCTAssertTrue(tea.waitForExistence(timeout: 10), renter.debugDescription)
    tea.tap()
    XCTAssertTrue(renter.staticTexts["선택 1 · 가능 2"].exists, renter.debugDescription)
    renter.buttons["메뉴 언어 English"].tap()
    XCTAssertTrue(renter.buttons["Strong tea Hot Add"].exists, renter.debugDescription)
    XCTAssertTrue(renter.staticTexts["선택 1 · 가능 2"].exists, renter.debugDescription)
    XCTAssertTrue(renter.staticTexts.containing(NSPredicate(format: "label CONTAINS[c] %@", "milk")).firstMatch.exists)
  }

  func testReserveFirstUnitAndResume() {
    let renter = XCUIApplication(bundleIdentifier: "com.nu54renter.cataloglab")
    renter.open(URL(string: "nu54://order")!)
    confirmNativeShop(renter)
    let item = renter.buttons["진한 차 뜨거움 추가"]
    XCTAssertTrue(item.waitForExistence(timeout: 10), renter.debugDescription)
    XCTAssertTrue(renter.staticTexts["선택 0 · 가능 2"].exists)
    item.tap()
    renter.buttons["주문 접수 요청"].tap()
    let recorded = renter.staticTexts["주문이 연결되었습니다. 보드 승인 대기 중입니다."]
    XCTAssertTrue(recorded.waitForExistence(timeout: 10), renter.debugDescription)
    renter.terminate()
    renter.open(URL(string: "nu54://order")!)
    let resume = renter.buttons["저장한 주문 다시 확인"]
    XCTAssertTrue(resume.waitForExistence(timeout: 10), renter.debugDescription)
    resume.tap()
    XCTAssertTrue(recorded.waitForExistence(timeout: 10), renter.debugDescription)
    XCTAssertTrue(renter.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "7.00 TestUSDC")).firstMatch.exists, renter.debugDescription)
  }

  func testResumeOriginalAmountAfterPriceChange() {
    let renter = XCUIApplication(bundleIdentifier: "com.nu54renter.cataloglab")
    renter.open(URL(string: "nu54://order")!)
    let resume = renter.buttons["저장한 주문 다시 확인"]
    XCTAssertTrue(resume.waitForExistence(timeout: 10), renter.debugDescription)
    resume.tap()
    XCTAssertTrue(renter.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "7.00 TestUSDC")).firstMatch.waitForExistence(timeout: 10), renter.debugDescription)
  }

}

final class ShopDiscoverySmoke: XCTestCase {
  private struct Fixture: Decodable {
    let bundle: String
    let branch: UUID
    let otherBranch: UUID
    let name: String
    let menuItem: String
    let originalSale: UUID?
    let resumeURL: URL?

    func entrance(_ branch: UUID) -> URL {
      URL(string: "nu54://shop/\(branch.uuidString.lowercased())")!
    }
  }

  private func fixture() throws -> Fixture {
    guard let raw = ProcessInfo.processInfo.environment["NU54_SHOP_SMOKE_FIXTURE"] else {
      throw XCTSkip("Run the isolated signed-app shop discovery fixture.")
    }
    let fixture = try JSONDecoder().decode(Fixture.self, from: Data(raw.utf8))
    XCTAssertTrue(fixture.bundle.hasPrefix("com.nu54renter."))
    XCTAssertNotEqual(fixture.branch, fixture.otherBranch)
    return fixture
  }

  func testVerifiedEntranceAndPrivateCheckout() throws {
    continueAfterFailure = false
    let fixture = try fixture()
    let app = XCUIApplication(bundleIdentifier: fixture.bundle)
    app.launch()
    tap("가까운 매장에서 주문", in: app)
    XCTAssertFalse(app.buttons["이 매장 메뉴 열기"].exists)
    tap("근처 단말 찾기", in: app)
    shown("매장 입구의 QR을 스캔하세요", in: app)
    XCTAssertTrue(app.buttons["카메라로 매장 QR 확인"].exists)

    app.terminate()
    app.open(fixture.entrance(fixture.otherBranch))
    shown("등록 지점의 매장 입구 QR을 확인하세요.", in: app)
    XCTAssertFalse(app.buttons["이 매장 메뉴 열기"].exists)

    app.terminate()
    app.launch()
    tap("가까운 매장에서 주문", in: app)
    let sign = app.textFields["간판의 지점명"]
    XCTAssertTrue(sign.waitForExistence(timeout: 10), app.debugDescription)
    sign.tap()
    sign.typeText("Other branch " + UUID().uuidString)
    tap("온라인으로 매장 확인", in: app)
    shown("입력한 지점명이 확인된 지점명과 다릅니다.", in: app)
    XCTAssertFalse(app.buttons["이 매장 메뉴 열기"].isEnabled)

    app.terminate()
    app.open(fixture.entrance(fixture.branch))
    shown("\(fixture.name)에서 주문", in: app)
    XCTAssertFalse(app.textFields["간판의 지점명"].exists)
    tap("이 매장 메뉴 열기", in: app)
    shown("\(fixture.name) 메뉴", in: app)
    tap(fixture.menuItem, in: app)
    tap("주문 접수 요청", in: app)
    shown("주문이 연결되었습니다. 보드 승인 대기 중입니다.", in: app)
  }

  func testOfflineReopensOnlyOriginal() throws {
    continueAfterFailure = false
    let fixture = try fixture()
    let original = try XCTUnwrap(fixture.originalSale).uuidString.lowercased()
    let resume = try XCTUnwrap(fixture.resumeURL)
    XCTAssertTrue(resume.scheme == "http" && resume.host == "127.0.0.1")
    let app = XCUIApplication(bundleIdentifier: fixture.bundle)
    app.terminate()
    app.open(fixture.entrance(fixture.branch))
    shown("새 주문을 시작할 수 없습니다", in: app)
    XCTAssertFalse(app.buttons["이 매장 메뉴 열기"].exists)
    XCTAssertFalse(app.buttons["주문 접수 요청"].exists)
    XCTAssertTrue(app.buttons["원래 주문 상태 확인"].exists)
    XCTAssertEqual(try String(contentsOf: resume, encoding: .utf8).trimmingCharacters(in: .whitespacesAndNewlines), original)
    tap("원래 주문 상태 확인", in: app)
    shown(original, in: app)
  }

  private func tap(_ label: String, in app: XCUIApplication) {
    let button = app.buttons[label]
    XCTAssertTrue(button.waitForExistence(timeout: 10), app.debugDescription)
    if !button.isHittable { app.swipeUp() }
    XCTAssertTrue(button.isEnabled, app.debugDescription)
    button.tap()
  }

  private func shown(_ text: String, in app: XCUIApplication) {
    XCTAssertTrue(app.staticTexts[text].waitForExistence(timeout: 25), app.debugDescription)
  }
}

private func confirmNativeShop(_ app: XCUIApplication, expected: String? = nil) {
  let check = app.buttons["온라인으로 매장 확인"]
  XCTAssertTrue(check.waitForExistence(timeout: 25), app.debugDescription)
  check.tap()
  let titleSuffix = "에서 주문"
  let title = app.staticTexts.matching(NSPredicate(format: "label ENDSWITH %@", titleSuffix)).firstMatch
  XCTAssertTrue(title.waitForExistence(timeout: 15), app.debugDescription)
  let name = String(title.label.dropLast(titleSuffix.count))
  if let expected { XCTAssertEqual(name, expected) }
  let open = app.buttons["이 매장 메뉴 열기"]
  XCTAssertTrue(open.exists && open.isEnabled, app.debugDescription)
  open.tap()
}

final class SetupRoutesSmoke: XCTestCase {
  func testNewAndInstalledBoardRoutes() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w02practice")
    app.launch()
    if app.buttons["한국어"].waitForExistence(timeout: 10) { app.buttons["한국어"].tap() }
    func tap(_ label:String) {
      let target=app.buttons[label]
      for _ in 0..<8 where !target.isHittable { app.swipeUp() }
      XCTAssertTrue(target.waitForExistence(timeout:10),app.debugDescription)
      target.tap()
    }
    tap("새 보드 설치")
    XCTAssertTrue(app.staticTexts["설치 검증을 기다리고 있습니다"].waitForExistence(timeout:10),app.debugDescription)
    let image=XCTAttachment(screenshot:app.screenshot()); image.name="W03-start"; image.lifetime = .keepAlways; add(image)
    tap("지갑 선택으로")
    tap("설치된 보드 연결")
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format:"label CONTAINS %@", "독립 설치 기록")).firstMatch.waitForExistence(timeout:10),app.debugDescription)
    let connected=XCTAttachment(screenshot:app.screenshot()); connected.name="W04-start"; connected.lifetime = .keepAlways; add(connected)
  }
}

final class SetupReceiptSmoke: XCTestCase {
  private let app=XCUIApplication(bundleIdentifier:"com.nu54renter.w02practice")
  private func fetch(_ path:String) throws -> [String:Any] {
    let data=try Data(contentsOf: XCTUnwrap(URL(string:"http://127.0.0.1:18941"+path)))
    return try XCTUnwrap(JSONSerialization.jsonObject(with:data) as? [String:Any])
  }
  private func tap(_ name:String) {
    let item=app.buttons[name]
    for _ in 0..<12 where !item.isHittable { app.swipeUp() }
    XCTAssertTrue(item.waitForExistence(timeout:10),app.debugDescription); item.tap()
  }
  private func selectFile(_ action:String,_ name:String) {
    tap(action)
    XCTAssertTrue(app.buttons["Cancel"].waitForExistence(timeout:20),app.debugDescription)
    if !app.staticTexts[name].exists {
      let browse=app.buttons["둘러보기"]
      if browse.exists {browse.tap()}
      let device=app.cells["나의 iPhone"]
      if device.waitForExistence(timeout:5) {device.tap()}
      let folder=app.staticTexts["NU54Fixture"]
      if folder.waitForExistence(timeout:5) {folder.tap()}
    }
    let file=app.cells[name+", json"]
    XCTAssertTrue(file.waitForExistence(timeout:15),app.debugDescription); file.tap()
  }
  func testSignedReceiptSurvivesRestartAndRejectsWrongDevice() throws {
    _ = try fetch("/setup")
    app.launch()
    if app.buttons["한국어"].waitForExistence(timeout:5) {app.buttons["한국어"].tap()}
    tap("새 보드 설치")
    if app.buttons["설치 요청 취소"].exists {tap("설치 요청 취소")}
    selectFile("검증된 설치 파일 선택","oversize.json")
    XCTAssertTrue(app.staticTexts["설치 파일을 선택하지 못했습니다."].waitForExistence(timeout:15),app.debugDescription)
    selectFile("검증된 설치 파일 선택","setup.json")
    selectFile("독립 신뢰 파일 선택","trust.json")
    tap("독립 기록과 일치함")
    tap("설치 확인 요청 만들기")
    let nonce=app.staticTexts.matching(NSPredicate(format:"label MATCHES %@","[0-9a-f]{64}")).firstMatch
    XCTAssertTrue(nonce.waitForExistence(timeout:10),app.debugDescription); let original=nonce.label
    app.terminate(); app.launch(); tap("새 보드 설치")
    XCTAssertTrue(nonce.waitForExistence(timeout:10),app.debugDescription); XCTAssertEqual(nonce.label,original)
    _ = try fetch("/receipt?challenge="+original+"&wrong=1")
    selectFile("서명된 확인서 파일 선택","wrong.json")
    tap("설치 확인서 검증")
    XCTAssertTrue(app.staticTexts["확인서가 유효하지 않거나 요청이 만료됐습니다."].waitForExistence(timeout:10),app.debugDescription)
    _ = try fetch("/receipt?challenge="+original)
    selectFile("서명된 확인서 파일 선택","receipt.json")
    tap("설치 확인서 검증")
    XCTAssertTrue(app.buttons["같은 보드 등록 계속"].waitForExistence(timeout:10),app.debugDescription)
    tap("같은 보드 등록 계속")
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format:"label CONTAINS %@","독립 설치 기록")).firstMatch.waitForExistence(timeout:10),app.debugDescription)
  }
}

final class SetupPickerSmoke: XCTestCase {
  func testPickerOpens() {
    let app=XCUIApplication(bundleIdentifier:"com.nu54renter.w02practice")
    app.launch()
    if app.buttons["한국어"].waitForExistence(timeout:5) {app.buttons["한국어"].tap()}
    let start=app.buttons["새 보드 설치"]
    for _ in 0..<10 where !start.isHittable {app.swipeUp()}
    XCTAssertTrue(start.waitForExistence(timeout:10)); start.tap()
    let choose=app.buttons["검증된 설치 파일 선택"]
    for _ in 0..<10 where !choose.isHittable {app.swipeUp()}
    XCTAssertTrue(choose.waitForExistence(timeout:10)); choose.tap()
    XCTAssertTrue(app.buttons["Cancel"].waitForExistence(timeout:20) || app.buttons["취소"].exists)
    app.buttons["둘러보기"].tap()
    XCTAssertTrue(app.cells["나의 iPhone"].waitForExistence(timeout:10))
    app.cells["나의 iPhone"].tap()
    XCTAssertTrue(app.staticTexts["NU54Fixture"].waitForExistence(timeout:10))
    app.staticTexts["NU54Fixture"].tap()
    XCTAssertTrue(app.cells["setup.json, json"].waitForExistence(timeout:10))
  }
}

final class WalletHomePrivacySmoke: XCTestCase {
  func testSyntheticHomeVisibilityAndResume() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w01qa")
    app.launch()
    if app.buttons["한국어"].waitForExistence(timeout: 10) { app.buttons["한국어"].tap() }
    if app.buttons["잔액 표시"].waitForExistence(timeout: 2) { app.buttons["잔액 표시"].tap() }
    XCTAssertTrue(app.staticTexts["내 자산"].waitForExistence(timeout: 25), app.debugDescription)
    XCTAssertTrue(app.staticTexts["1.25 TEST"].exists, app.debugDescription)
    XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "현재 연결 미확인")).firstMatch.exists)
    let shown = XCTAttachment(screenshot: app.screenshot()); shown.name = "W01 Korean last observation"; shown.lifetime = .keepAlways; add(shown)
    app.buttons["잔액 숨기기"].tap()
    XCTAssertTrue(app.staticTexts["잔액 숨김"].waitForExistence(timeout: 5))
    XCTAssertFalse(app.staticTexts["1.25 TEST"].exists)
    let hidden = XCTAttachment(screenshot: app.screenshot()); hidden.name = "W01 hidden"; hidden.lifetime = .keepAlways; add(hidden)
    app.buttons["English"].tap()
    XCTAssertTrue(app.staticTexts["Balance hidden"].waitForExistence(timeout: 5))
    XCUIDevice.shared.press(.home)
    app.activate()
    XCTAssertTrue(app.staticTexts["Balance hidden"].waitForExistence(timeout: 10))
    XCTAssertFalse(app.staticTexts["1.25 TEST"].exists)
    let resumed = XCTAttachment(screenshot: app.screenshot()); resumed.name = "W01 English hidden resumed"; resumed.lifetime = .keepAlways; add(resumed)
    app.buttons["Show balance"].tap()
    XCTAssertTrue(app.staticTexts["1.25 TEST"].waitForExistence(timeout: 5))
    let bottom = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.995))
    let middle = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
    bottom.press(forDuration: 0.1, thenDragTo: middle, withVelocity: .slow, thenHoldForDuration: 1.5)
    let switcher = XCTAttachment(screenshot: XCUIScreen.main.screenshot()); switcher.name = "W01 app switcher after visible balance"; switcher.lifetime = .keepAlways; add(switcher)
    app.activate()
    XCTAssertTrue(app.staticTexts["1.25 TEST"].waitForExistence(timeout: 10))
  }
}

final class WalletSettingsSmoke: XCTestCase {
  func testAccessibilityTutorialAndPrivateNotificationPreview() {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w01qa")
    app.launch()
    if app.buttons["한국어"].waitForExistence(timeout: 8) { app.buttons["한국어"].tap() }
    func tap(_ label: String, beforeTap: (() -> Void)? = nil) {
      let target = app.buttons[label].firstMatch
      XCTAssertTrue(target.waitForExistence(timeout: 10), label)
      let scroll = app.scrollViews.firstMatch
      func fullyVisible() -> Bool {
        target.isHittable && (!scroll.exists || scroll.frame.insetBy(dx: -1, dy: -1).contains(target.frame))
      }
      for _ in 0..<12 where !fullyVisible() {
        let viewport = scroll.frame
        let height = viewport.height
        let middle = viewport.midY
        let distance = max(-0.60 * height, min(0.60 * height, middle - target.frame.midY))
        let startY = middle - distance / 2
        let from = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: startY / app.frame.height))
        let to = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: (startY + distance) / app.frame.height))
        from.press(forDuration: 0.05, thenDragTo: to, withVelocity: .slow, thenHoldForDuration: 0.15)
      }
      XCTAssertTrue(fullyVisible(), label)
      beforeTap?()
      target.tap()
    }
    tap("설정")
    tap("접근성")
    tap("글자와 움직임")
    let amount = app.staticTexts["w27-practice-amount"].firstMatch
    XCTAssertTrue(amount.waitForExistence(timeout: 10))
    let defaultHeight = amount.frame.height
    tap("200%와 동작 줄이기 미리보기")
    XCTAssertTrue(amount.exists)
    XCTAssertEqual(amount.label, "2 연습 코인")
    let largeHeight = amount.frame.height
    XCTAssertGreaterThan(largeHeight, defaultHeight * 1.6)
    XCTAssertTrue(app.staticTexts["수취인: 연습 카페"].exists)
    XCTAssertTrue(app.staticTexts["체인: 연습 모드"].exists)
    XCTAssertTrue(app.staticTexts["동작 줄이기 · 보드 버튼 정적 안내"].exists)
    let previewShot = XCTAttachment(screenshot: app.screenshot())
    previewShot.name = "W27 scoped large practice preview"
    previewShot.lifetime = .keepAlways
    add(previewShot)
    tap("연습 결제 취소") {
      let cancelShot = XCTAttachment(screenshot: app.screenshot())
      cancelShot.name = "W27 cancellation fully visible"
      cancelShot.lifetime = .keepAlways
      self.add(cancelShot)
    }
    XCTAssertTrue(app.staticTexts["실제 지급 0건 · 서명 0건"].waitForExistence(timeout: 10))
    XCTAssertTrue(app.buttons["실물 보드 등록"].exists)
    app.terminate()
    app.launch()
    tap("결제 연습")
    XCTAssertTrue(app.staticTexts["연습 주문을 거절했습니다"].waitForExistence(timeout: 10))
    tap("같은 연습 다시 보기")
    let repeated = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@",
      "주문: 연습 카페, 2 연습 코인. 비교 화면: 연습 카페, 2 연습 코인.")).firstMatch
    XCTAssertTrue(repeated.waitForExistence(timeout: 10))
    tap("연습 거절")
    tap("나중에 이어하기")
    tap("설정")
    tap("접근성")
    tap("English 선택")
    tap("Screen reader order")
    let readerRows = [
      ("source", "Request source: Practice without a board"),
      ("amount", "Amount: 2 practice coins"),
      ("recipient", "Recipient: Practice Café"),
      ("chain", "Network: Practice mode"),
      ("fee", "Practice fee: 0"),
      ("method", "Review method: Virtual button practice"),
      ("cancel", "Last action: Cancel request")
    ]
    let actualRows = app.staticTexts.matching(NSPredicate(format: "identifier BEGINSWITH %@", "w27-read-"))
      .allElementsBoundByIndex.map { $0.identifier }
    XCTAssertEqual(actualRows, readerRows.map { "w27-read-" + $0.0 })
    for (id, label) in readerRows {
      let item = app.staticTexts["w27-read-" + id].firstMatch
      XCTAssertTrue(item.exists, id)
      XCTAssertEqual(item.label, label)
    }
    tap("Hear again")
    tap("Real approval screen example")
    XCTAssertTrue(app.staticTexts["Amount: 6.50 TestUSDC"].exists)
    tap("Back to screen reader order")
    tap("Decline practice")
    XCTAssertTrue(app.staticTexts["Real payments: 0 · Signatures: 0"].exists)
    tap("Continue later")
    tap("Settings")
    tap("Accessibility")
    tap("Notification preview")
    let preview = app.staticTexts["w27-lock-notification"].firstMatch
    XCTAssertTrue(preview.exists)
    XCTAssertEqual(preview.label, "A payment needs review")
    XCTAssertTrue(app.buttons["Manage notifications in device settings"].exists)
    let permission = app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "Device notification permission")).firstMatch
    XCTAssertTrue(permission.exists)
    let permissionLabel = permission.label
    let system = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    XCTAssertFalse(system.alerts.firstMatch.exists)
    tap("Request notification permission")
    let systemAlert = system.alerts.firstMatch
    let alert = systemAlert.waitForExistence(timeout: 6) ? systemAlert : app.alerts.firstMatch
    XCTAssertTrue(alert.exists)
    let refuse = alert.buttons.matching(NSPredicate(format: "label IN %@", ["Don’t Allow", "Don't Allow", "허용 안 함"])).firstMatch
    XCTAssertTrue(refuse.exists)
    refuse.tap()
    XCTAssertTrue(app.staticTexts["Device notification permission: Scheduling unavailable"].waitForExistence(timeout: 10))
    tap("Pending payment list")
    XCTAssertFalse(app.staticTexts["w27-lock-notification"].exists)
    XCTAssertTrue(app.staticTexts["내 결제와 미결 청구"].waitForExistence(timeout: 10))
    print("W27_METRICS defaultHeight=\(defaultHeight) largeHeight=\(largeHeight) order=\(actualRows.joined(separator: ",")) permission=\(permissionLabel)")
  }

  func testSettingsAndMinimalDiagnosticPreview() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w01qa")
    app.launch()
    if app.buttons["한국어"].waitForExistence(timeout: 10) { app.buttons["한국어"].tap() }
    let settings = app.buttons["설정"]
    XCTAssertTrue(settings.waitForExistence(timeout: 25))
    for _ in 0..<8 where !settings.isHittable { app.swipeUp() }
    settings.tap()
    XCTAssertTrue(app.staticTexts["기기와 미결 요청"].waitForExistence(timeout: 10))
    XCTAssertTrue(app.staticTexts.containing(NSPredicate(format:"label CONTAINS %@", "정책 변경은 현재 이 화면에서 지원하지 않습니다")).firstMatch.exists)
    let preview = app.buttons["진단 항목 검토"]
    for _ in 0..<12 where !preview.isHittable { app.swipeUp() }
    XCTAssertTrue(preview.waitForExistence(timeout: 10)); preview.tap()
    XCTAssertTrue(app.staticTexts["공유할 진단 미리보기"].waitForExistence(timeout: 10))
    XCTAssertTrue(app.staticTexts["주소, 잔액, PIN과 키는 포함하지 않습니다."].exists)
    let shot = XCTAttachment(screenshot:app.screenshot()); shot.name="W27 diagnostics preview"; shot.lifetime = .keepAlways; add(shot)
    let cancel = app.buttons["공유 취소"]
    for _ in 0..<6 where !cancel.isHittable { app.swipeUp() }
    cancel.tap()
    XCTAssertFalse(app.staticTexts["공유할 진단 미리보기"].exists)
  }
}


final class TravelVisitSmoke: XCTestCase {
  func testSelectPublishResumeChallengeAndAdjustment() throws {
    continueAfterFailure = false
    let environment = ProcessInfo.processInfo.environment
    let bundle = try XCTUnwrap(environment["NU54_VISIT_QA_BUNDLE"])
    let fixture = try XCTUnwrap(environment["NU54_VISIT_QA_FIXTURE"])
    let app = XCUIApplication(bundleIdentifier: bundle)
    app.launch()
    func button(_ label: String, timeout: TimeInterval = 15) -> XCUIElement {
      let target = app.buttons[label].firstMatch
      XCTAssertTrue(target.waitForExistence(timeout: timeout), label)
      for _ in 0..<6 where !target.isHittable { app.swipeUp(velocity: .fast) }
      for _ in 0..<8 where !target.isHittable { app.swipeDown(velocity: .fast) }
      XCTAssertTrue(target.isHittable, label)
      return target
    }
    func tap(_ label: String, timeout: TimeInterval = 15) { button(label, timeout:timeout).tap() }
    func label(_ text: String) {
      XCTAssertTrue(app.staticTexts[text].firstMatch.waitForExistence(timeout: 12), text + app.debugDescription)
    }
    func request(_ path: String, body: Data? = nil) throws -> Data {
      var req = URLRequest(url: try XCTUnwrap(URL(string: fixture + "/" + path)))
      req.httpMethod = "POST"
      req.httpBody = body
      let finished = expectation(description: path)
      let result = TravelEnrollmentResult()
      URLSession.shared.dataTask(with:req) { data,response,_ in
        if (response as? HTTPURLResponse)?.statusCode == 200 { result.raw = data ?? Data() }
        finished.fulfill()
      }.resume()
      wait(for:[finished], timeout:15)
      return try XCTUnwrap(result.raw)
    }
    tap("여행지 찾기", timeout:25)
    tap("여행 기록과 동의")
    tap("개인 기록 등록 공개키")
    let key = app.staticTexts.matching(NSPredicate(format:"label BEGINSWITH %@", "MFkwEwYH")).firstMatch
    XCTAssertTrue(key.waitForExistence(timeout:10))
    let profile = try XCTUnwrap(String(data:request("enroll",body:Data(key.label.utf8)),encoding:.utf8))
    try pasteTravelEnrollment(profile,in:app)
    tap("개인 기록 등록 확인")
    tap("후기와 이용 자격")
    let choices = app.buttons.matching(NSPredicate(format:"label BEGINSWITH %@ AND label ENDSWITH %@", "이용 장소 ·", " 선택"))
    XCTAssertTrue(choices.firstMatch.waitForExistence(timeout:12))
    let rows = choices.allElementsBoundByIndex.map(\.label)
    XCTAssertEqual(rows.count,5)
    tap(rows[0])
    tap("후기 공개 동의 내용 확인")
    tap("후기 공개 동의")
    let draft = app.textViews["후기 본문"].firstMatch
    XCTAssertTrue(draft.waitForExistence(timeout:10))
    draft.tap()
    draft.typeText("Native fare completed")
    tap("후기 공개 게시")
    label("후기가 게시됐어요.")
    app.terminate()
    app.launch()
    tap("여행지 찾기")
    tap("여행 기록과 동의")
    tap("후기와 이용 자격")
    tap(rows[0])
    tap("저장된 후기 보기")
    label("저장된 후기 · Native fare completed")
    let edit = app.textViews["후기 본문"].firstMatch
    edit.tap()
    edit.typeText("Conflicting body")
    tap("후기 공개 게시")
    label("이미 저장된 후기가 있어요. 기존 내용과 지금 입력한 내용을 비교해 주세요.")
    let capture = XCTAttachment(screenshot: app.screenshot())
    capture.lifetime = .keepAlways
    add(capture)
    tap("따라하기 챌린지")
    label("검증된 방문 0 / 3")
    tap(rows[0])
    tap("챌린지 방문 확인 동의 내용 확인")
    tap("챌린지 방문 확인 동의")
    tap("선택한 방문 근거 반영")
    label("검증된 방문 1 / 3")
    tap("선택한 방문 근거 반영")
    label("이미 반영한 이용 근거예요.")
    label("검증된 방문 1 / 3")
    tap("스탬프와 혜택")
    for stamp in ["스탬프 · 검증", "혜택 원장 · 확정 1건"] {
      label(stamp)
      let visible = app.staticTexts.matching(NSPredicate(format: "label == %@", stamp)).allElementsBoundByIndex.filter { $0.isHittable }
      XCTAssertEqual(Set(visible.map { String(describing: $0.frame) }).count, 1)
    }
    label("시험 근거 · 실제 혜택 없음")
    tap("이용과 혜택 상태 다시 조회")
    label("혜택 원장 · 확정 1건")
    _ = try request("verify-one")
    tap("따라하기 챌린지")
    tap(rows[1])
    tap("선택한 방문 근거 반영")
    label("이미 반영한 장소예요. 진척은 그대로입니다.")
    tap(rows[4])
    tap("선택한 방문 근거 반영")
    label("시험 방문을 확인했어요. 진척과 실제 혜택은 그대로입니다.")
    tap(rows[2])
    tap("선택한 방문 근거 반영")
    label("검증된 방문 2 / 3")
    tap(rows[3])
    tap("선택한 방문 근거 반영")
    label("검증된 방문 3 / 3 · 완료")
    let completed = XCTAttachment(screenshot: app.screenshot())
    completed.lifetime = .keepAlways
    add(completed)
    _ = try request("adjust")
    tap("이용과 혜택 상태 다시 조회")
    label("검증된 방문 2 / 3")
    tap("스탬프와 혜택")
    label("혜택 원장 · 회수 또는 조정 1건")
    tap("따라하기 챌린지")
    tap("챌린지 방문 확인 동의 철회")
    label("검증된 방문 0 / 3")
    _ = try request("populate")
    tap("이용과 혜택 상태 다시 조회")
    label("이용 내역 1쪽")
    tap("지난 이용 내역 더 보기")
    label("이용 내역 2쪽")
    tap("이용과 혜택 상태 다시 조회")
    label("이용 내역 2쪽")
    tap("이전 내역 페이지")
    label("이용 내역 1쪽")
    label("검증된 방문 0 / 3")
    let pageCapture=XCTAttachment(screenshot:app.screenshot())
    pageCapture.name="Native high-history travel page"
    pageCapture.lifetime = .keepAlways
    add(pageCapture)

  }
}

final class TravelPickupSmoke: XCTestCase {
  func testPaidAcceptanceKitchenHandoverAndOriginalRefund() throws {
    try exercisePickup(changed: false)
  }

  func testChangedPickupRequiresNewConsent() throws {
    try exercisePickup(changed: true)
  }

  private func exercisePickup(changed: Bool) throws {
    continueAfterFailure = false
    let values = ProcessInfo.processInfo.environment
    let phone = XCUIApplication(bundleIdentifier: try XCTUnwrap(values["NU54_PICKUP_P02"]))
    let kitchen = XCUIApplication(bundleIdentifier: try XCTUnwrap(values["NU54_PICKUP_P04"]))
    let bridge = try XCTUnwrap(values["NU54_PICKUP_CONTROL"])
    let token = try XCTUnwrap(values["NU54_PICKUP_TOKEN"])

    var phase = "launch"
    func mark(_ value: String) {
      phase = value
      print("NU54_PICKUP_STEP=\(value)")
    }
    func tap(_ item: XCUIElement, in app: XCUIApplication, file: StaticString = #filePath, line: UInt = #line) {
      XCTAssertTrue(item.waitForExistence(timeout: 15), "Missing pickup control at \(phase)", file: file, line: line)
      for _ in 0..<5 where !item.isHittable { app.swipeUp(velocity: .fast) }
      for _ in 0..<6 where !item.isHittable { app.swipeDown(velocity: .fast) }
      XCTAssertTrue(item.isHittable, "Pickup control outside the viewport at \(phase)", file: file, line: line)
      item.tap()
    }
    func action(_ title: String, _ app: XCUIApplication, file: StaticString = #filePath, line: UInt = #line) {
      tap(app.buttons[title].firstMatch, in: app, file: file, line: line)
    }
    func button(_ predicate: NSPredicate, _ app: XCUIApplication, file: StaticString = #filePath, line: UInt = #line) {
      tap(app.buttons.matching(predicate).firstMatch, in: app, file: file, line: line)
    }
    func text(_ title: String, _ app: XCUIApplication, file: StaticString = #filePath, line: UInt = #line) {
      XCTAssertTrue(app.staticTexts[title].firstMatch.waitForExistence(timeout: 12),
        "Missing pickup display at \(phase)", file: file, line: line)
    }
    func control(_ path: String, _ key: String? = nil) throws -> [String: String] {
      var req = URLRequest(url: try XCTUnwrap(URL(string: bridge + path)))
      req.httpMethod = "POST"
      req.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      if let key { req.httpBody = Data(key.utf8) }
      let finished = expectation(description: path)
      let result = TravelEnrollmentResult()
      URLSession.shared.dataTask(with: req) { data, response, _ in
        if (response as? HTTPURLResponse)?.statusCode == 200 { result.raw = data }
        finished.fulfill()
      }.resume()
      wait(for: [finished], timeout: 12)
      let raw = try XCTUnwrap(result.raw, path)
      return try XCTUnwrap(JSONSerialization.jsonObject(with: raw) as? [String: String])
    }
    func registrationKey(_ app: XCUIApplication) throws -> String {
      let item = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
      XCTAssertTrue(item.waitForExistence(timeout: 12))
      return item.label
    }
    func fill(_ field: String, _ value: String, _ app: XCUIApplication, paste: Bool = false) throws {
      let identifier = app.textViews[field].firstMatch
      let labeled = app.textViews.matching(NSPredicate(format: "label BEGINSWITH %@", field)).firstMatch
      let input = identifier.exists ? identifier : labeled.exists ? labeled : app.textFields[field].firstMatch
      tap(input, in: app)
      if !paste { input.typeText(value); return }
      let clipboard = UIPasteboard.general
      clipboard.setItems([["public.utf8-plain-text": value]], options: [.localOnly: true, .expirationDate: Date().addingTimeInterval(15)])
      let sequence = clipboard.changeCount
      defer { if clipboard.changeCount == sequence { clipboard.items = [] } }
      input.press(forDuration: 1.2)
      let affordance = app.descendants(matching: .any).matching(NSPredicate(format: "label IN %@", ["Paste", "붙여넣기"])).firstMatch
      XCTAssertTrue(affordance.waitForExistence(timeout: 5), "Registration paste did not open")
      affordance.tap()
      let allow = app.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Allow Paste", "붙여넣기 허용"])).firstMatch
      if allow.waitForExistence(timeout: 2) { allow.tap() }
      XCTAssertTrue(input.value as? String == value, "Pickup registration text did not reach its input")
    }
    func attach(_ name: String, _ app: XCUIApplication) {
      let screenshot = XCTAttachment(screenshot: app.screenshot())
      screenshot.name = name
      screenshot.lifetime = .keepAlways
      add(screenshot)
    }

    var staff: [String: String] = [:]
    if !changed {
      kitchen.launch()
      action("등록 공개키 확인", kitchen)
      staff = try control("/enroll/terminal", registrationKey(kitchen))
    }
    mark("customer-registration")
    phone.launch()
    action("내 결제", phone)
    action("주문 등록 공개키 보기", phone)
    let buyer = try control("/enroll/customer", registrationKey(phone))
    try fill("customer-commerce-registration", XCTUnwrap(buyer["registration"]), phone, paste: true)
    func nextInput(_ app: XCUIApplication) {
      let next = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Next", "다음"])).firstMatch
      if next.waitForExistence(timeout: 2) { next.tap() }
      else { app.typeText("\n") }
    }
    nextInput(phone)
    phone.typeText(try XCTUnwrap(buyer["pin"]))
    nextInput(phone)
    phone.typeText(try XCTUnwrap(buyer["presentationPin"]))
    let done = phone.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Done", "완료"])).firstMatch
    if done.waitForExistence(timeout: 2) { done.tap() }
    else if phone.keyboards.firstMatch.exists { phone.typeText("\n") }
    if !phone.buttons["구독 관리"].waitForExistence(timeout: 4) { action("주문 등록 확인", phone) }
    XCTAssertTrue(phone.buttons["구독 관리"].waitForExistence(timeout: 10))
    action("지갑으로 돌아가기", phone)

    func openKitchenQueue() {
      kitchen.activate()
      XCTAssertFalse(kitchen.buttons["지점 픽업 접수 조회"].exists, "Pickup staff scope survived an app switch")
      action("직원 픽업 접수", kitchen)
      action("직원 권한 확인", kitchen)
      action("지점 픽업 대기열", kitchen)
      action("지점 픽업 접수 조회", kitchen)
    }

    if !changed {
      mark("terminal-registration")
      kitchen.activate()
      try fill("매장 발급서", XCTUnwrap(staff["registration"]), kitchen, paste: true)
      let fingerprint = try XCTUnwrap(staff["pin"])
      let pinField = kitchen.textFields["독립 확인한 서버 공개키 지문"].firstMatch
      tap(pinField, in: kitchen)
      pinField.typeText(fingerprint)
      XCTAssertTrue(pinField.value as? String == fingerprint, "The terminal server fingerprint did not reach its input")
      action("등록서 확인", kitchen)
      mark("terminal-authority")
      openKitchenQueue()
      text("접수된 픽업이 없습니다.", kitchen)
    }

    mark("travel-entry")
    phone.activate()
    action("여행지 찾기", phone)
    action("서울", phone)
    action("지역 맛집", phone)
    if changed {
      let before = ["version":"1", "orders":"0", "amount":"0", "bookings":"0", "quotes":"0", "payments":"0", "relays":"0"]
      XCTAssertEqual(try control("/pickup-change-proof"), before)
      action("픽업 선주문", phone)
      button(NSPredicate(format: "label BEGINSWITH %@ AND label ENDSWITH %@", "픽업 ", " 선택"), phone)
      action("지점 메뉴 보기", phone)
      button(NSPredicate(format: "label BEGINSWITH %@ AND label ENDSWITH %@", "시험 커피", "추가"), phone)
      text("합계 · 3.50 TestUSDC", phone)
      XCTAssertEqual(try control("/publish-pickup-change"), ["version":"2", "price":"4500000"])
      action("픽업 시간과 결제 금액 확인 후 주문 접수", phone)
      text("재고 또는 가격이 변경됐어요", phone)
      text("시험 커피 · plain → 귀리 음료", phone)
      text("기존 합계 · 3.50 TestUSDC", phone)
      text("새 합계 · 4.50 TestUSDC", phone)
      XCTAssertFalse(phone.buttons["이 주문의 결제 내용 검토"].exists)
      XCTAssertFalse(phone.buttons["이 원주문의 서명 견적 요청"].exists)
      var expected = before
      expected["version"] = "2"
      XCTAssertEqual(try control("/pickup-change-proof"), expected)
      action("변경된 옵션과 금액 확인 후 주문 접수", phone)
      XCTAssertTrue(phone.buttons["이 주문의 결제 내용 검토"].waitForExistence(timeout: 12))
      expected["orders"] = "1"
      expected["amount"] = "4500000"
      XCTAssertEqual(try control("/pickup-change-proof"), expected)
      return
    }
    func reserve() throws -> String {
      action("픽업 선주문", phone)
      button(NSPredicate(format: "label BEGINSWITH %@ AND label ENDSWITH %@", "픽업 ", " 선택"), phone)
      action("지점 메뉴 보기", phone)
      button(NSPredicate(format: "label BEGINSWITH %@ AND label ENDSWITH %@", "시험 커피", "추가"), phone)
      text("합계 · 3.50 TestUSDC", phone)
      action("픽업 시간과 결제 금액 확인 후 주문 접수", phone)
      XCTAssertTrue(phone.buttons["이 주문의 결제 내용 검토"].waitForExistence(timeout: 12), "Pickup order did not reach review")
      let sale = try XCTUnwrap(control("/paid")["sale"])
      text(sale, phone)
      return sale
    }
    func advanceKitchen() throws {
      _ = try control("/accepted")
      openKitchenQueue()
      button(NSPredicate(format: "label MATCHES %@", "픽업 [0-9]+ · .*"), kitchen)
      action("준비 시작", kitchen)
      action("준비 완료 확인", kitchen)
    }
    func refreshPhone() {
      phone.activate()
      action("이 주문의 픽업과 환불 상태 다시 조회", phone)
    }
    mark("first-order")
    let first = try reserve()
    action("이 주문의 픽업과 환불 상태 다시 조회", phone)
    text("지급 확정 · 매장 접수 대기", phone)
    XCTAssertFalse(phone.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "픽업 번호 ")).firstMatch.exists)
    mark("first-kitchen-acceptance")
    try advanceKitchen()
    refreshPhone()
    text("픽업 준비 완료", phone)
    text(first, phone)
    attach("Pickup ready on the original order", phone)
    mark("first-kitchen-handover")
    openKitchenQueue()
    button(NSPredicate(format: "label MATCHES %@", "픽업 [0-9]+ · .*"), kitchen)
    action("수령 완료 확인", kitchen)
    refreshPhone()
    text("수령 완료", phone)
    action("저장한 주문 다시 확인", phone)
    action("완료한 주문 화면 닫기", phone)
    action("지갑으로 돌아가기", phone)

    mark("second-order")
    let second = try reserve()
    XCTAssertNotEqual(first, second)
    mark("second-kitchen-acceptance")
    try advanceKitchen()
    action("이행 불가 검토", kitchen)
    action("이행 불가 확정", kitchen)
    refreshPhone()
    text("매장 이행 불가", phone)
    text("재무 검토 대기", phone)
    mark("refund-request")
    _ = try control("/refund-request")
    refreshPhone()
    text("환불 지급 확인 중", phone)
    XCTAssertFalse(phone.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "환불 지급 증거 ")).firstMatch.exists)
    mark("refund-finality")
    _ = try control("/refund-final")
    refreshPhone()
    text("환불 지급 확정", phone)
    text("환불 금액 3.50 TestUSDC", phone)
    text(second, phone)
    let proof = phone.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "환불 지급 증거 ")).firstMatch
    XCTAssertTrue(proof.waitForExistence(timeout: 8))
    attach("Separate finalized refund of the same unfulfillable order", phone)
  }
}


final class TravelDiscoveryT01Smoke: XCTestCase {
  func testSignedPublicDiscoveryOffersTheSameFilteredPlaceInMapAndList() throws {
    continueAfterFailure = false
    let bundle = try XCTUnwrap(ProcessInfo.processInfo.environment["NU54_DISCOVERY_QA_BUNDLE"])
    let app = XCUIApplication(bundleIdentifier: bundle)
    app.launch()
    func tap(_ title: String) {
      let button = app.buttons[title].firstMatch
      XCTAssertTrue(button.waitForExistence(timeout: 12), "Missing action " + title)
      for _ in 0..<5 where !button.isHittable { app.swipeUp(velocity: .fast) }
      for _ in 0..<6 where !button.isHittable { app.swipeDown(velocity: .fast) }
      button.tap()
    }
    func capture(_ name: String) {
      let snapshot = XCTAttachment(screenshot: app.screenshot())
      snapshot.name = name
      snapshot.lifetime = .keepAlways
      add(snapshot)
    }
    tap("여행지 찾기")
    tap("서울")
    tap("필터")
    tap("맛집 유형")
    tap("영업 중")
    tap("예약 가능")
    tap("취소")
    XCTAssertTrue(app.buttons["공개 명소"].firstMatch.exists)
    tap("필터")
    tap("맛집 유형")
    tap("예약 가능")
    tap("필터 적용")
    XCTAssertTrue(app.buttons["지역 맛집"].firstMatch.exists)
    XCTAssertFalse(app.buttons["공개 명소"].firstMatch.exists)
    XCTAssertTrue(app.staticTexts["예약 기능 있음 · 좌석 미확인"].firstMatch.exists)
    capture("Published booking capability without a seat claim")
    tap("지도")
    tap("지역 맛집")
    XCTAssertTrue(app.staticTexts["Fixture curator · 2026-11-20T00:00:00Z"].firstMatch.exists)
    XCTAssertTrue(app.buttons["예약 가능한 시간"].firstMatch.exists)
    capture("Published map place and source")
    tap("목록")
    XCTAssertTrue(app.buttons["지역 맛집"].firstMatch.exists)
    XCTAssertFalse(app.buttons["공개 명소"].firstMatch.exists)
    tap("필터 해제")
    XCTAssertTrue(app.buttons["공개 명소"].firstMatch.exists)
    XCTAssertFalse(app.alerts.firstMatch.exists)
  }
}

final class KioskModeReviewSmoke: XCTestCase {
  private let terminalID = "3ce37c55-4f32-433a-9807-2bef754496b5"
  private let original = "80000000-0000-4000-8000-000000000001"
  private let lastOriginal = "80000000-0000-4000-8000-000000000021"

  private func ledger() throws -> [String] {
    let url = try XCTUnwrap(URL(string: "http://127.0.0.1:18966/state"))
    return String(decoding: try Data(contentsOf: url), as: UTF8.self).split(separator: "|").map(String.init)
  }

  private func tap(_ label: String, in app: XCUIApplication) {
    let button = app.buttons[label]
    XCTAssertTrue(button.waitForExistence(timeout: 15), app.debugDescription)
    for _ in 0..<5 where !button.isHittable { app.swipeUp() }
    XCTAssertTrue(button.isHittable, app.debugDescription)
    button.tap()
  }

  private func visible(_ label: String, in app: XCUIApplication) {
    XCTAssertTrue(app.staticTexts[label].waitForExistence(timeout: 15), app.debugDescription)
  }

  func testOperatorReviewResumeAndHandoff() throws {
    let staff = XCUIApplication(bundleIdentifier: "com.nu54kiosk.k06operator")
    staff.launch()
    XCTAssertFalse(staff.buttons["현재 단말 모드 조회"].exists)
    tap("직원 권한 확인", in: staff)
    let field = staff.textFields["단말 식별값"]
    XCTAssertTrue(field.waitForExistence(timeout: 15), staff.debugDescription)
    field.tap()
    field.typeText(terminalID)
    tap("현재 단말 모드 조회", in: staff)
    tap("결제 전용 모드로 전환", in: staff)
    visible("전환 영향 검토", in: staff)
    visible(original, in: staff)
    tap("다음 인계 주문", in: staff)
    visible(lastOriginal, in: staff)
    XCTAssertEqual(try ledger(), ["menu","1","0","0","0","0","21"])
    tap("전환 요청 저장", in: staff)
    visible("신규 주문 · 중지", in: staff)
    XCTAssertEqual(try ledger(), ["menu","1","1","0","0","21","21"])

    staff.terminate()
    staff.launch()
    tap("직원 권한 확인", in: staff)
    tap("기존 요청 재조회", in: staff)
    visible("결제 전용 v2 · 전환 대기", in: staff)
    tap("다음 인계 주문", in: staff)
    visible(lastOriginal, in: staff)
    XCTAssertEqual(try ledger(), ["menu","1","1","0","0","21","21"])
    tap("지점 인계 확인", in: staff)
    visible("지점 인계 · 확인 · 단말 적용 · 대기", in: staff)
    XCTAssertEqual(try ledger(), ["menu","1","1","1","0","21","21"])
  }

  func testTerminalAppliesExactVersion() throws {
    let terminal = XCUIApplication(bundleIdentifier: "com.nu54kiosk.k06terminal")
    terminal.launch()
    tap("새 고객 주문", in: terminal)
    let state = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in
      (try? self.ledger()) == ["pos","2","1","1","1","21","21"]
    }, object: nil)
    XCTAssertEqual(XCTWaiter.wait(for: [state],timeout: 15), .completed)
    tap("새 고객 주문", in: terminal)
    visible("POS 결제 전용", in: terminal)
    XCTAssertTrue(terminal.textFields["POS 인계 코드"].exists)
  }

  func testOperatorRecoversActiveOriginalRequest() throws {
    let staff = XCUIApplication(bundleIdentifier: "com.nu54kiosk.k06operator")
    staff.launch()
    tap("직원 권한 확인", in: staff)
    tap("기존 요청 재조회", in: staff)
    visible("결제 전용 v2 · 적용 완료", in: staff)
    visible(original, in: staff)
    XCTAssertEqual(try ledger(), ["pos","2","1","1","1","21","21"])
  }
}


final class KioskClerkAuthorizationSmoke: XCTestCase {
  func testClerkSeesOnlyItsPickupQueueAndClearsItOnExit() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54kiosk.k06operator")
    app.launch()
    XCTAssertTrue(app.buttons["직원 권한 확인"].waitForExistence(timeout: 15), app.debugDescription)
    XCTAssertFalse(app.buttons["지점 픽업 대기열"].exists)
    app.buttons["직원 권한 확인"].tap()
    let queue = app.buttons["지점 픽업 대기열"]
    XCTAssertTrue(queue.waitForExistence(timeout: 10), app.debugDescription)
    XCTAssertFalse(app.buttons["현재 단말 모드 조회"].exists)
    XCTAssertFalse(app.buttons["P05 환불 운영 화면"].exists)
    queue.tap()
    app.buttons["지점 픽업 접수 조회"].tap()
    XCTAssertTrue(app.staticTexts["접수된 픽업이 없습니다."].waitForExistence(timeout: 10), app.debugDescription)
    app.buttons["고객 화면"].tap()
    XCTAssertFalse(app.staticTexts["접수된 픽업이 없습니다."].exists)
    XCTAssertFalse(app.buttons["지점 픽업 대기열"].exists)
  }
}

final class TravelBookingSeatsSmoke: XCTestCase {
  func testLastSeatChangedTermsAndOriginalHoldAfterRestart() throws {
    continueAfterFailure = false
    let values = ProcessInfo.processInfo.environment
    let app = XCUIApplication(bundleIdentifier: try XCTUnwrap(values["NU54_BOOKING_P02"]))
    let bridge = try XCTUnwrap(values["NU54_BOOKING_CONTROL"])
    let token = try XCTUnwrap(values["NU54_BOOKING_TOKEN"])
    defer { app.terminate() }

    func tap(_ element: XCUIElement) {
      XCTAssertTrue(element.waitForExistence(timeout: 20), app.debugDescription)
      for _ in 0..<5 where !element.isHittable { app.swipeUp(velocity: .fast) }
      for _ in 0..<6 where !element.isHittable { app.swipeDown(velocity: .fast) }
      XCTAssertTrue(element.isHittable && element.isEnabled, app.debugDescription)
      element.tap()
    }
    func action(_ label: String) { tap(app.buttons[label].firstMatch) }
    func text(_ label: String) {
      XCTAssertTrue(app.staticTexts[label].firstMatch.waitForExistence(timeout: 15), app.debugDescription)
    }
    func control(_ route: String, _ key: String? = nil) throws -> [String: String] {
      var request = URLRequest(url: try XCTUnwrap(URL(string: bridge + route)))
      request.httpMethod = "POST"
      request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      if let key { request.httpBody = Data(key.utf8) }
      let completed = expectation(description: route)
      let result = TravelEnrollmentResult()
      URLSession.shared.dataTask(with: request) { data, response, _ in
        if (response as? HTTPURLResponse)?.statusCode == 200 { result.raw = data }
        completed.fulfill()
      }.resume()
      wait(for: [completed], timeout: 15)
      return try JSONDecoder().decode([String: String].self, from: XCTUnwrap(result.raw, route))
    }
    func nextField() {
      let next = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Next", "다음"])).firstMatch
      if next.waitForExistence(timeout: 2) { next.tap() }
      else { app.typeText("\n") }
    }
    func openPlace() {
      action("여행지 찾기")
      action("서울")
      action("지역 맛집")
      action("예약 가능한 시간")
    }

    app.launch()
    action("내 결제")
    action("주문 등록 공개키 보기")
    let key = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
    XCTAssertTrue(key.waitForExistence(timeout: 12))
    let registration = try control("/enroll/customer", key.label)
    let clipboard = UIPasteboard.general
    clipboard.setItems([["public.utf8-plain-text": try XCTUnwrap(registration["registration"])]],
      options: [.localOnly: true, .expirationDate: Date().addingTimeInterval(15)])
    let clipboardVersion = clipboard.changeCount
    defer { if clipboard.changeCount == clipboardVersion { clipboard.items = [] } }
    let input = app.textViews["customer-commerce-registration"].firstMatch
    tap(input)
    input.press(forDuration: 1.2)
    tap(app.descendants(matching: .any).matching(NSPredicate(format: "label IN %@", ["Paste", "붙여넣기"])).firstMatch)
    let allow = app.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Allow Paste", "붙여넣기 허용"])).firstMatch
    if allow.waitForExistence(timeout: 2) { allow.tap() }
    nextField()
    app.typeText(try XCTUnwrap(registration["pin"]))
    nextField()
    app.typeText(try XCTUnwrap(registration["presentationPin"]))
    let done = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Done", "완료"])).firstMatch
    if done.waitForExistence(timeout: 2) { done.tap() }
    else if app.keyboards.firstMatch.exists { app.typeText("\n") }
    if !app.buttons["구독 관리"].waitForExistence(timeout: 4) { action("주문 등록 확인") }
    XCTAssertTrue(app.buttons["구독 관리"].waitForExistence(timeout: 10))
    action("지갑으로 돌아가기")

    openPlace()
    let party = app.textFields["인원"]
    tap(party)
    party.typeText(XCUIKeyboardKey.delete.rawValue + "2")
    action("인원 입력 완료")
    text("2명 합계 60.00 TestUSDC")
    let review = app.buttons.matching(identifier: "이 조건으로 예약 보류")
    _ = try control("/last-seat")
    tap(review.firstMatch)
    text("선택한 시간이 마감됐어요. 다른 시간을 선택해 주세요.")
    XCTAssertFalse(app.buttons["같은 예약 요청 다시 시도"].exists)
    XCTAssertEqual(try control("/booking-proof")["count"], "0")

    action("다른 시간 선택")
    text("취소 조건 · 둘째 시간 이전 조건")
    _ = try control("/change-terms")
    tap(review.element(boundBy: 1))
    text("예약 조건이 바뀌었어요. 최신 가격과 취소 조건을 확인해 주세요.")
    XCTAssertEqual(try control("/booking-proof")["count"], "0")
    action("최신 조건 확인")
    text("취소 조건 · 둘째 시간 최신 조건")
    text("2명 합계 30.00 TestUSDC")

    _ = try control("/drop-booking-response")
    tap(review.element(boundBy: 1))
    XCTAssertTrue(app.buttons["같은 예약 요청 다시 시도"].waitForExistence(timeout: 15))
    let committed = try control("/booking-proof")
    XCTAssertEqual(committed, ["count":"1", "party":"2", "terms":"3", "slot":"6978011b-2888-4340-a04e-2587e13a1d55"])
    app.terminate()
    app.launch()
    openPlace()
    XCTAssertFalse(review.firstMatch.exists)
    action("같은 예약 요청 다시 시도")
    text("예약 보류 · 2명")
    text("총액 30.00 TestUSDC")
    XCTAssertEqual(try control("/booking-proof"), committed)
    XCTAssertFalse(app.buttons["같은 예약 요청 다시 시도"].exists)

    let bookingRef = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "예약 참조 ")).firstMatch
    XCTAssertTrue(bookingRef.waitForExistence(timeout: 12), app.debugDescription)
    let originalBooking = bookingRef.label
    app.terminate()
    app.launch()
    action("내 결제")
    action("여행 예약")
    text("내 여행 예약")
    text(originalBooking)
    text("총액 30.00 TestUSDC")
    XCTAssertFalse(app.buttons["서울"].exists)
    action("예약 지급 및 영수증 조회")
    text("예약 보류 · 2명")
    XCTAssertTrue(app.buttons["예약금 결제 검토"].exists)
    XCTAssertFalse(app.buttons["잔금 결제 검토"].exists)
    action("예약금 결제 검토")
    text("예약 총액 30.00 TestUSDC")
    text("이번 예약금 10.00 TestUSDC")
    text("별도 잔금 20.00 TestUSDC")
    text("예약 정책 v3 · 가격 v2")
    text("예약 네트워크 · StableNet 테스트넷 8283")
    text("노쇼 · 미방문 시 예약금 환불 불가")
    let cutoff = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "취소 기한 · ")).firstMatch
    XCTAssertTrue(cutoff.exists)
    XCTAssertFalse(cutoff.label.contains("기록 없음"))
    XCTAssertEqual(try control("/booking-payment-proof"), ["count":"0", "order":"", "quotes":"0"])
    action("원예약으로")
    action("예약금 결제 검토")
    action("이 조건으로 지급 주문 열기")
    text("원주문 연결 · 견적 요청 전")
    XCTAssertTrue(app.buttons["이 원주문의 서명 견적 요청"].exists)
    let order = try control("/booking-payment-proof")
    XCTAssertEqual(order["count"], "1")
    XCTAssertEqual(order["quotes"], "0")
    let originalOrder = try XCTUnwrap(order["order"])
    XCTAssertFalse(originalOrder.isEmpty)
    text(originalOrder)
    action("같은 주문의 지급 결과 확인")
    text(originalOrder)
    XCTAssertEqual(try control("/booking-payment-proof"), order)
    action("지갑으로 돌아가기")
    text("내 여행 예약")
    action("예약 취소 검토")
    text("위 취소 조건으로 이 예약의 취소를 요청합니다.")
    action("예약 유지")
    action("내 결제로")
    XCTAssertTrue(app.buttons["구독 관리"].waitForExistence(timeout: 12), app.debugDescription)
    XCTAssertEqual(try control("/booking-proof"), committed)
  }
}

final class KioskPickupShiftSmoke: XCTestCase {
  private let original = "80000000-0000-4000-8000-000000000001"
  private let replacement = "80000000-0000-4000-8000-000000000002"
  private let retry = "같은 픽업 상태 요청 다시 확인"

  private func tap(_ label: String, in app: XCUIApplication) {
    let button = app.buttons[label]
    XCTAssertTrue(button.waitForExistence(timeout: 15), app.debugDescription)
    button.tap()
  }

  private func openQueue() -> XCUIApplication {
    let app = XCUIApplication(bundleIdentifier: "com.nu54kiosk.k06operator")
    app.launch()
    tap("직원 권한 확인", in: app)
    tap("지점 픽업 대기열", in: app)
    tap("지점 픽업 접수 조회", in: app)
    return app
  }

  private func openOrder(_ app: XCUIApplication) {
    let pickup = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "픽업 1 ·")).firstMatch
    XCTAssertTrue(pickup.waitForExistence(timeout: 10), app.debugDescription)
    pickup.tap()
  }

  func testOriginalClerkLosesACommittedPickupReply() {
    let app = openQueue()
    openOrder(app)
    XCTAssertTrue(app.staticTexts["원주문 " + original].waitForExistence(timeout: 10), app.debugDescription)
    tap("준비 시작", in: app)
    XCTAssertTrue(app.buttons[retry].waitForExistence(timeout: 15), app.debugDescription)
  }

  func testReplacementClerkCanActAndCannotSeeOriginalClerksRequest() {
    let app = openQueue()
    openOrder(app)
    XCTAssertTrue(app.staticTexts["원주문 " + replacement].waitForExistence(timeout: 10), app.debugDescription)
    XCTAssertFalse(app.staticTexts["원주문 " + original].exists)
    XCTAssertFalse(app.buttons[retry].exists)
    tap("준비 시작", in: app)
    XCTAssertTrue(app.staticTexts["픽업 번호 1 · 준비 중"].waitForExistence(timeout: 15), app.debugDescription)
    XCTAssertFalse(app.buttons[retry].exists)
  }

  func testOriginalClerkRecoversTheSameRequestAfterReenrollment() {
    let app = openQueue()
    tap(retry, in: app)
    XCTAssertTrue(app.staticTexts["원주문 " + original].waitForExistence(timeout: 10), app.debugDescription)
    XCTAssertTrue(app.staticTexts["픽업 번호 1 · 준비 중"].waitForExistence(timeout: 10), app.debugDescription)
    XCTAssertFalse(app.staticTexts["원주문 " + replacement].exists)
    XCTAssertFalse(app.buttons[retry].exists)
  }
}

final class KioskOutcomeSmoke: XCTestCase {
  @MainActor
  func testOriginalPaidAndExpiredMenuOrders() async throws {
    continueAfterFailure = false
    let env = ProcessInfo.processInfo.environment
    let base = try XCTUnwrap(env["NU54_KIOSK_OUTCOME_CONTROL"])
    let token = try XCTUnwrap(env["NU54_KIOSK_OUTCOME_TOKEN"])
    let app = XCUIApplication(bundleIdentifier: "com.nu54kiosk.outcomeqa")
    defer { app.terminate() }

    func exchange(_ stage: String? = nil) async throws -> (Data, Int) {
      var request = URLRequest(url: try XCTUnwrap(URL(string: base + "/stage")), timeoutInterval: 5)
      request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      if let stage {
        request.httpMethod = "POST"
        request.httpBody = Data(stage.utf8)
      }
      let (body, response) = try await URLSession.shared.data(for: request)
      return (body, (response as? HTTPURLResponse)?.statusCode ?? 0)
    }
    func wait(_ name: String) async throws -> [String: String] {
      for _ in 0..<110 {
        let (body, status) = try await exchange()
        if status == 200, let stage = try? JSONDecoder().decode([String: String].self, from: body),
           stage["stage"] == name { return stage }
        try await Task.sleep(nanoseconds: 500_000_000)
      }
      XCTFail("Missing stage " + name)
      return [:]
    }
    func done(_ name: String) async throws {
      let (_, status) = try await exchange(name)
      XCTAssertEqual(status, 204)
    }
    func tap(_ label: String) {
      let button = app.buttons[label].firstMatch
      XCTAssertTrue(button.waitForExistence(timeout: 15), label)
      for _ in 0..<5 where !button.isHittable { app.swipeUp() }
      for _ in 0..<5 where !button.isHittable { app.swipeDown() }
      XCTAssertTrue(button.isHittable && button.isEnabled, label)
      button.tap()
    }
    func text(_ label: String) {
      XCTAssertTrue(app.staticTexts[label].firstMatch.waitForExistence(timeout: 12), label)
    }
    func bind(_ name: String) async throws -> (String, String) {
      let pair = try await wait(name + "-pair")
      let code = try XCTUnwrap(pair["code"])
      let sale = try XCTUnwrap(pair["sale"])
      tap("휴대전화 확인 번호 조회")
      tap("확인 번호 \(code)와 일치")
      text("연결한 휴대전화 · " + code)
      try await done(name + "-pair")
      return (sale, code)
    }
    func excludesPrior(_ sale: String, _ marker: String) {
      XCTAssertFalse(app.staticTexts["원주문 · " + sale].exists)
      if !marker.isEmpty {
        XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", marker)).firstMatch.exists)
      }
      XCTAssertFalse(app.staticTexts["금액 7.00 TestUSDC"].exists)
      XCTAssertFalse(app.staticTexts["결제 완료"].exists)
      XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "픽업 번호 ")).firstMatch.exists)
      XCTAssertFalse(app.descendants(matching: .any).matching(identifier: "이 주문을 여는 일회용 QR").firstMatch.exists)
      XCTAssertFalse(app.staticTexts["원견적 만료 · 지급 없음 확정"].exists)
      XCTAssertFalse(app.staticTexts["네트워크 수수료 · 가맹점 릴레이 부담"].exists)
    }
    func fresh(_ sale: String, _ marker: String) {
      tap("화면을 비우고 대기")
      app.terminate()
      app.launch()
      XCTAssertTrue(app.buttons["새 고객 주문"].waitForExistence(timeout: 12))
      excludesPrior(sale, marker)
      tap("새 고객 주문")
      excludesPrior(sale, marker)
    }

    _ = try await wait("old-price")
    app.launch()
    tap("새 고객 주문")
    tap("시험 크루아상 plain 추가")
    text("합계 · 6.50 TestUSDC")
    try await done("old-price")
    _ = try await wait("changed-price")
    tap("주문 접수 요청")
    text("새 합계 · 7.00 TestUSDC")
    tap("바뀐 장바구니 확인 후 주문 접수")
    text("금액 7.00 TestUSDC")
    try await done("changed-price")
    if env["NU54_KIOSK_OUTCOME_SCENARIO"] == "binding-recovery" {
      let chosen = try await wait("reprice-recovery-pair")
      let sale = try XCTUnwrap(chosen["sale"])
      let code = try XCTUnwrap(chosen["code"])
      let other = try XCTUnwrap(chosen["other"])
      tap("휴대전화 확인 번호 조회")
      XCTAssertTrue(app.buttons["확인 번호 \(code)와 일치"].exists)
      XCTAssertTrue(app.buttons["확인 번호 \(other)와 일치"].exists)
      tap("확인 번호 \(code)와 일치")
      text("기존 결제 연결을 확인하고 있습니다")
      text("원주문 · " + sale)
      text("금액 7.00 TestUSDC")
      XCTAssertFalse(app.buttons["확인 번호 \(other)와 일치"].exists)
      XCTAssertFalse(app.buttons["확인 번호 \(code)와 일치"].exists)
      XCTAssertFalse(app.descendants(matching: .any).matching(identifier: "이 주문을 여는 일회용 QR").firstMatch.exists)
      try await done("reprice-recovery-pair")
      _ = try await wait("reprice-recovery-check")
      tap("기존 연결 조회")
      text("원주문 · " + sale)
      text("금액 7.00 TestUSDC")
      text("연결한 휴대전화 · " + code)
      XCTAssertFalse(app.buttons["기존 연결 조회"].exists)
      XCTAssertFalse(app.buttons["확인 번호 \(other)와 일치"].exists)
      XCTAssertFalse(app.descendants(matching: .any).matching(identifier: "이 주문을 여는 일회용 QR").firstMatch.exists)
      try await done("reprice-recovery-check")
      _ = try await wait("reprice-recovery-issued")
      tap("이 주문 다시 확인")
      text("기존 견적 · 연결한 폰에서 결제를 계속하세요")
      text("원주문 · " + sale)
      XCTAssertFalse(app.staticTexts["결제 완료"].exists)
      tap("화면을 비우고 대기")
      try await done("reprice-recovery-issued")

      _ = try await wait("baseline-order")
      app.terminate()
      app.launch()
      tap("새 고객 주문")
      excludesPrior(sale, code)
      tap("시험 크루아상 plain 추가")
      text("합계 · 6.50 TestUSDC")
      tap("주문 접수 요청")
      text("금액 6.50 TestUSDC")
      XCTAssertFalse(app.buttons["바뀐 장바구니 확인 후 주문 접수"].exists)
      try await done("baseline-order")
      let (paid, paidMarker) = try await bind("baseline")
      XCTAssertNotEqual(paid, sale)
      let confirmed = try await wait("baseline-paid")
      let pickup = try XCTUnwrap(confirmed["pickup"])
      tap("이 주문 다시 확인")
      text("원주문 · " + paid)
      text("결제 완료")
      text("금액 6.50 TestUSDC")
      text("네트워크 수수료 · 가맹점 릴레이 부담")
      text("픽업 번호 " + pickup)
      XCTAssertFalse(app.staticTexts["연결한 휴대전화 · " + paidMarker].exists)
      XCTAssertFalse(app.descendants(matching: .any).matching(identifier: "이 주문을 여는 일회용 QR").firstMatch.exists)
      fresh(paid, paidMarker)
      XCTAssertFalse(app.staticTexts["금액 6.50 TestUSDC"].exists)
      excludesPrior(sale, code)
      try await done("baseline-paid")
      return
    }
    let (paid, paidMarker) = try await bind("paid")
    _ = try await wait("paid-read")
    tap("이 주문 다시 확인")
    text("원주문 · " + paid)
    text("결제 완료")
    text("금액 7.00 TestUSDC")
    text("네트워크 수수료 · 가맹점 릴레이 부담")
    XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "픽업 번호 ")).firstMatch.exists)
    XCTAssertFalse(app.staticTexts["연결한 휴대전화 · " + paidMarker].exists)
    XCTAssertFalse(app.descendants(matching: .any).matching(identifier: "이 주문을 여는 일회용 QR").firstMatch.exists)
    try await done("paid-read")
    _ = try await wait("restart-paid")
    fresh(paid, paidMarker)
    try await done("restart-paid")

    _ = try await wait("unpaid-order")
    tap("시험 크루아상 plain 추가")
    tap("주문 접수 요청")
    text("금액 7.00 TestUSDC")
    try await done("unpaid-order")
    let (unpaid, unpaidMarker) = try await bind("unpaid")
    XCTAssertNotEqual(unpaid, paid)
    _ = try await wait("unpaid-issued")
    tap("이 주문 다시 확인")
    text("기존 견적 · 연결한 폰에서 결제를 계속하세요")
    XCTAssertFalse(app.staticTexts["원견적 만료 · 지급 없음 확정"].exists)
    try await done("unpaid-issued")
    _ = try await wait("unpaid-read")
    tap("이 주문 다시 확인")
    text("원주문 · " + unpaid)
    text("원견적 만료 · 지급 없음 확정")
    XCTAssertFalse(app.staticTexts["결제 완료"].exists)
    XCTAssertFalse(app.staticTexts["연결한 휴대전화 · " + unpaidMarker].exists)
    XCTAssertFalse(app.descendants(matching: .any).matching(identifier: "이 주문을 여는 일회용 QR").firstMatch.exists)
    XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "픽업 번호 ")).firstMatch.exists)
    try await done("unpaid-read")
    _ = try await wait("restart-unpaid")
    fresh(unpaid, unpaidMarker)
    excludesPrior(paid, paidMarker)
    try await done("restart-unpaid")

    _ = try await wait("unbound-order")
    tap("시험 크루아상 plain 추가")
    tap("주문 접수 요청")
    XCTAssertTrue(app.descendants(matching: .any).matching(identifier: "이 주문을 여는 일회용 QR").firstMatch.waitForExistence(timeout: 12))
    try await done("unbound-order")
    let unboundState = try await wait("unbound-idle")
    let unbound = try XCTUnwrap(unboundState["sale"])
    text("원주문 · " + unbound)
    XCTAssertTrue(app.staticTexts["연결 시간이 지났습니다"].waitForExistence(timeout: 50))
    text("공용 화면 · 자동 초기화 대기")
    text("견적을 받은 경우 폰에서 원주문을 확인할 수 있습니다.")
    XCTAssertTrue(app.buttons["공용 화면 닫기"].exists)
    excludesPrior(unbound, "")
    XCTAssertFalse(app.buttons["이 주문 다시 확인"].exists)
    XCTAssertTrue(app.buttons["새 고객 주문"].waitForExistence(timeout: 30))
    excludesPrior(unbound, "")
    XCTAssertFalse(app.staticTexts["연결 시간이 지났습니다"].exists)
    try await done("unbound-idle")

    _ = try await wait("signed-order")
    tap("새 고객 주문")
    excludesPrior(unbound, "")
    tap("시험 크루아상 plain 추가")
    tap("주문 접수 요청")
    text("금액 7.00 TestUSDC")
    try await done("signed-order")
    let (signed, signedMarker) = try await bind("signed")
    _ = try await wait("signed-idle")
    tap("이 주문 다시 확인")
    text("원주문 · " + signed)
    text("기존 견적 · 연결한 폰에서 결제를 계속하세요")
    XCTAssertTrue(app.buttons["새 고객 주문"].waitForExistence(timeout: 70))
    excludesPrior(signed, signedMarker)
    tap("새 고객 주문")
    excludesPrior(signed, signedMarker)
    try await done("signed-idle")
  }
}

final class KioskK07Smoke: XCTestCase {
  private struct Proof: Decodable, Equatable {
    struct Line: Decodable, Equatable {
      let sku: String
      let option: String
      let quantity: Int
      let unitPrice: String
    }
    let orders: Int
    let orderRef: String
    let lines: [Line]
    let stock: [String: Int]
    let optionStock: [String: Int]
  }

  @MainActor
  func testPublishedOptionsAndExactOriginalOrder() async throws {
    continueAfterFailure = false
    let env = ProcessInfo.processInfo.environment
    let app = XCUIApplication(bundleIdentifier: try XCTUnwrap(env["NU54_K07_APP"]))
    let origin = try XCTUnwrap(env["NU54_K07_CONTROL"])
    let token = try XCTUnwrap(env["NU54_K07_TOKEN"])

    func control(_ path: String, body: String = "") async throws -> Data {
      var req = URLRequest(url: try XCTUnwrap(URL(string: origin + path)), timeoutInterval: 10)
      req.httpMethod = "POST"
      req.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      req.httpBody = Data(body.utf8)
      let (data, response) = try await URLSession.shared.data(for: req)
      XCTAssertEqual((response as? HTTPURLResponse)?.statusCode, 200, path)
      return data
    }
    func proof() async throws -> Proof {
      try JSONDecoder().decode(Proof.self, from: await control("/k07-proof"))
    }
    func tap(_ element: XCUIElement) {
      XCTAssertTrue(element.waitForExistence(timeout: 15), element.debugDescription)
      for _ in 0..<5 where !element.isHittable { app.swipeUp(velocity: .fast) }
      for _ in 0..<6 where !element.isHittable { app.swipeDown(velocity: .fast) }
      XCTAssertTrue(element.isHittable && element.isEnabled, element.debugDescription)
      element.tap()
    }
    func action(_ title: String) { tap(app.buttons[title].firstMatch) }
    func text(_ title: String) {
      XCTAssertTrue(app.staticTexts[title].firstMatch.waitForExistence(timeout: 12), title)
    }
    func attach(_ title: String) {
      let image = XCTAttachment(screenshot: app.screenshot())
      image.name = title
      image.lifetime = .keepAlways
      add(image)
    }

    app.launch()
    defer { app.terminate() }
    action("등록 공개키 확인")
    let publicKey = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
    XCTAssertTrue(publicKey.waitForExistence(timeout: 12))
    let staff = try JSONDecoder().decode([String: String].self, from: await control("/enroll/terminal", body: publicKey.label))
    let registration = try XCTUnwrap(staff["registration"])
    let field = app.textViews.matching(NSPredicate(format: "label BEGINSWITH %@", "매장 발급서")).firstMatch
    tap(field)
    let clipboard = UIPasteboard.general
    clipboard.setItems([["public.utf8-plain-text": registration]],
                       options: [.localOnly: true, .expirationDate: Date().addingTimeInterval(15)])
    let sequence = clipboard.changeCount
    defer { if clipboard.changeCount == sequence { clipboard.items = [] } }
    field.press(forDuration: 1.2)
    let paste = app.descendants(matching: .any).matching(NSPredicate(format: "label IN %@", ["Paste", "붙여넣기"])).firstMatch
    tap(paste)
    let allow = app.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Allow Paste", "붙여넣기 허용"])).firstMatch
    if allow.waitForExistence(timeout: 2) { allow.tap() }
    XCTAssertEqual(field.value as? String, registration)
    let next = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Next", "다음"])).firstMatch
    if next.waitForExistence(timeout: 2) { next.tap() } else { app.typeText("\n") }
    app.typeText(try XCTUnwrap(staff["pin"]))
    if next.waitForExistence(timeout: 2) { next.tap() } else { app.typeText("\n") }
    app.typeText(try XCTUnwrap(staff["presentationPin"]))
    let done = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Done", "완료"])).firstMatch
    if done.waitForExistence(timeout: 2) { done.tap() }
    else if app.keyboards.firstMatch.exists { app.typeText("\n") }
    if !app.buttons["새 고객 주문"].waitForExistence(timeout: 4) { action("등록서 확인") }

    let initial = try await proof()
    XCTAssertEqual(initial.orders, 0)
    XCTAssertEqual(initial.lines, [])
    XCTAssertEqual(initial.stock, ["baked-09": 5, "drink-12": 5])
    XCTAssertEqual(initial.optionStock, ["baked-09/standard": 5, "drink-12/iced": 5, "drink-12/hot": 5])
    action("새 고객 주문")
    action("크루아상 상세")
    text("가격 · 3.00 TestUSDC")
    text("점주 신고 성분 밀, 우유, 달걀 · 공용 제빵 설비에서 교차접촉 가능 · 견과류")
    text("알레르기가 있으면 주문 전에 직원에게 제조 환경을 확인하세요.")
    action("메뉴 언어 English")
    text("Merchant-declared ingredients wheat, milk, egg · Cross-contact possible on shared baking equipment · tree nuts")
    text("If you have an allergy, ask staff about preparation before ordering.")
    action("Menu language 한국어")
    action("수량 늘리기")
    text("수량 · 2")
    text("소계 · 6.00 TestUSDC")
    attach("K07 published bakery details")
    action("장바구니에 담기")

    action("아메리카노 옵션 선택")
    action("수량 늘리기")
    action("장바구니에 담기")
    text("온도 (필수)")
    text("온도를 선택해 주세요")
    text("수량 · 2")
    action("메뉴 언어 English")
    text("Temperature (required)")
    text("Choose an option for Temperature")
    text("Quantity · 2")
    action("Menu language 한국어")
    attach("K07 required option missed with quantity retained")
    let invalid = try await proof()
    XCTAssertEqual(invalid, initial)
    action("아이스 선택")
    text("소계 · 7.00 TestUSDC")
    action("장바구니에 담기")
    text("아메리카노 · 아이스 ×2")

    action("아메리카노 옵션 선택")
    action("따뜻하게 선택")
    text("소계 · 4.00 TestUSDC")
    action("장바구니에 담기")
    text("아메리카노 · 아이스 ×2")
    text("아메리카노 · 따뜻하게 ×1")
    text("합계 · 17.00 TestUSDC")
    action("아메리카노 따뜻하게 빼기")
    text("아메리카노 · 아이스 ×2")
    XCTAssertFalse(app.staticTexts["아메리카노 · 따뜻하게 ×1"].exists)
    let edited = try await proof()
    XCTAssertEqual(edited, initial)
    action("아메리카노 옵션 선택")
    action("따뜻하게 선택")
    action("장바구니에 담기")
    text("아메리카노 · 따뜻하게 ×1")
    text("아메리카노 · 아이스 ×2")
    text("크루아상 · 기본 ×2")
    text("합계 · 17.00 TestUSDC")
    action("주문 접수 요청")
    let original = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "원주문 · ")).firstMatch
    XCTAssertTrue(original.waitForExistence(timeout: 15))
    let accepted = try await proof()
    XCTAssertEqual(accepted.orders, 1)
    XCTAssertEqual(original.label, "원주문 · " + accepted.orderRef)
    XCTAssertEqual(accepted.lines, [
      .init(sku: "baked-09", option: "standard", quantity: 2, unitPrice: "3000000"),
      .init(sku: "drink-12", option: "hot", quantity: 1, unitPrice: "4000000"),
      .init(sku: "drink-12", option: "iced", quantity: 2, unitPrice: "3500000"),
    ])
    XCTAssertEqual(accepted.stock, ["baked-09": 3, "drink-12": 2])
    XCTAssertEqual(accepted.optionStock, ["baked-09/standard": 3, "drink-12/iced": 3, "drink-12/hot": 4])
    action("이 주문 다시 확인")
    text("원주문 · " + accepted.orderRef)
    let replayed = try await proof()
    XCTAssertEqual(replayed, accepted)
    attach("K07 exact original order")
    let ledger = XCTAttachment(data: try await control("/k07-proof"), uniformTypeIdentifier: "public.json")
    ledger.name = "K07 original order and inventory evidence"
    ledger.lifetime = .keepAlways
    add(ledger)
  }
}

final class KioskOfflineSmoke: XCTestCase {
  @MainActor
  func testColdOfflineMenuAndFreshOrderAfterServiceRestart() async throws {
    continueAfterFailure = false
    let env = ProcessInfo.processInfo.environment
    let origin = try XCTUnwrap(env["NU54_K01_CONTROL"])
    let token = try XCTUnwrap(env["NU54_K01_TOKEN"])
    let app = XCUIApplication(bundleIdentifier: "com.nu54kiosk.outcomeqa")
    defer { app.terminate() }

    struct Ledger: Decodable {
      let orders: Int
      let available: Int
      let categoryCount: Int
      let publicRows: Int
      let service: String
      let original: String
    }
    func request(_ path: String, method: String = "GET") async throws -> (Data, Int) {
      var request = URLRequest(url: try XCTUnwrap(URL(string: origin + path)), timeoutInterval: 20)
      request.httpMethod = method
      request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      let (body, response) = try await URLSession.shared.data(for: request)
      return (body, try XCTUnwrap((response as? HTTPURLResponse)?.statusCode))
    }
    func state() async throws -> Ledger {
      let (body, status) = try await request("/state")
      XCTAssertEqual(status, 200)
      return try JSONDecoder().decode(Ledger.self, from: body)
    }
    func tap(_ label: String) {
      let button = app.buttons[label].firstMatch
      XCTAssertTrue(button.waitForExistence(timeout: 15), label)
      for _ in 0..<5 where !button.isHittable { app.swipeUp() }
      XCTAssertTrue(button.isHittable && button.isEnabled, label)
      button.tap()
    }
    func textContaining(_ label: String) {
      let text = app.staticTexts.matching(NSPredicate(format:"label CONTAINS %@",label)).firstMatch
      XCTAssertTrue(text.waitForExistence(timeout: 12), label)
    }
    app.launch()
    tap("새 고객 주문")
    textContaining("여울 커피 성수점")
    textContaining("3.50 TestUSDC")
    let before = try await state()
    XCTAssertEqual(before.orders, 0)
    XCTAssertEqual(before.available, 2)
    XCTAssertEqual(before.categoryCount, 2)
    XCTAssertEqual(before.publicRows, 52)
    XCTAssertTrue(app.buttons["내 장바구니"].firstMatch.isHittable)
    tap("내 장바구니")
    textContaining("담은 메뉴가 없습니다.")
    let viewedEmpty = try await state()
    XCTAssertEqual(viewedEmpty.orders, 0)
    tap("메뉴로 돌아가기")
    tap("베이커리")
    textContaining("소금빵")
    XCTAssertFalse(app.staticTexts.matching(NSPredicate(format:"label CONTAINS %@", "아이스 아메리카노")).firstMatch.exists)
    tap("전체 메뉴")
    app.terminate()

    let stop = try await request("/stop", method: "POST")
    XCTAssertEqual(stop.1, 204)
    app.launch()
    tap("새 고객 주문")
    textContaining("최근 메뉴를 보고 있습니다")
    textContaining("여울 커피 성수점")
    textContaining("마지막 조회")
    textContaining("새 견적 발급")
    textContaining("3.50 TestUSDC")
    textContaining("품절")
    XCTAssertFalse(app.buttons["새 고객 주문"].exists)
    XCTAssertFalse(app.buttons["주문 접수 요청"].exists)
    XCTAssertFalse(app.buttons["내 장바구니"].exists)
    XCTAssertFalse(app.buttons["매장 입구 QR 보기"].exists)
    tap("베이커리")
    textContaining("소금빵")
    XCTAssertFalse(app.staticTexts.matching(NSPredicate(format:"label CONTAINS %@", "한정 메뉴")).firstMatch.exists)
    tap("메뉴 언어 English")
    textContaining("Salt bread")
    tap("All items")
    textContaining("Iced americano")
    textContaining("Sold out")
    tap("Check store connection")
    textContaining("Viewing the last synced menu")
    let disconnected = try await state()
    XCTAssertEqual(disconnected.service, "stopped")
    XCTAssertEqual(disconnected.orders, 0)
    XCTAssertEqual(disconnected.available, before.available)

    let restart = try await request("/restart", method: "POST")
    XCTAssertEqual(restart.1, 204)
    tap("Check store connection")
    textContaining("현재 메뉴 동기화 완료")
    let recovered = try await state()
    XCTAssertEqual(recovered.service, "restarted")
    XCTAssertEqual(recovered.orders, 0)
    tap("새 고객 주문")
    textContaining("여울 커피 성수점")
    textContaining("4.20 TestUSDC")
    tap("전체 메뉴")
    tap("아이스 아메리카노 plain 추가")
    let entry = app.buttons["내 장바구니"].firstMatch
    XCTAssertTrue(entry.isHittable)
    let drink = app.buttons["아이스 아메리카노 plain 추가"].firstMatch
    for _ in 0..<3 where drink.isHittable { app.swipeUp() }
    XCTAssertFalse(drink.isHittable)
    XCTAssertTrue(entry.isHittable)
    tap("내 장바구니")
    textContaining("아이스 아메리카노 · plain ×1")
    textContaining("합계 · 4.20 TestUSDC")
    let viewedBasket = try await state()
    XCTAssertEqual(viewedBasket.orders, 0)
    XCTAssertEqual(viewedBasket.available, 2)
    tap("메뉴로 돌아가기")
    textContaining("담은 수량 1")
    tap("내 장바구니")
    tap("주문 접수 요청")
    textContaining("원주문")
    let ordered = try await state()
    XCTAssertEqual(ordered.orders, 1)
    XCTAssertEqual(ordered.available, 1)
    XCTAssertNotNil(UUID(uuidString: ordered.original))
    textContaining(ordered.original)
    tap("화면을 비우고 대기")
    tap("닫고 화면 지우기")
    XCTAssertFalse(app.buttons["내 장바구니"].exists)
    tap("새 고객 주문")
    tap("내 장바구니")
    textContaining("담은 메뉴가 없습니다.")
  }
}

final class CustomerEnrollmentSmoke: XCTestCase {
  func testFirstRegistrationAndSameStoreOrder() throws {
    continueAfterFailure = false
    let values = ProcessInfo.processInfo.environment
    let app = XCUIApplication(bundleIdentifier: try XCTUnwrap(values["NU54_K07_APP"]))
    let bridge = try XCTUnwrap(values["NU54_K07_CONTROL"])
    let token = try XCTUnwrap(values["NU54_K07_TOKEN"])
    defer { app.terminate() }

    func tap(_ element: XCUIElement) {
      XCTAssertTrue(element.waitForExistence(timeout: 20), app.debugDescription)
      for _ in 0..<5 where !element.isHittable { app.swipeUp(velocity: .fast) }
      for _ in 0..<6 where !element.isHittable { app.swipeDown(velocity: .fast) }
      XCTAssertTrue(element.isHittable && element.isEnabled, app.debugDescription)
      element.tap()
    }
    func action(_ label: String) { tap(app.buttons[label].firstMatch) }
    func text(_ label: String) {
      XCTAssertTrue(app.staticTexts[label].firstMatch.waitForExistence(timeout: 15), app.debugDescription)
    }
    func control(_ route: String, _ key: String? = nil) throws -> [String: String] {
      var request = URLRequest(url: try XCTUnwrap(URL(string: bridge + route)))
      request.httpMethod = "POST"
      request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      if let key { request.httpBody = Data(key.utf8) }
      let completed = expectation(description: route)
      let result = TravelEnrollmentResult()
      URLSession.shared.dataTask(with: request) { data, response, _ in
        if (response as? HTTPURLResponse)?.statusCode == 200 { result.raw = data }
        completed.fulfill()
      }.resume()
      wait(for: [completed], timeout: 15)
      return try JSONDecoder().decode([String: String].self, from: XCTUnwrap(result.raw, route))
    }
    func nextField() {
      let next = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Next", "다음"])).firstMatch
      if next.waitForExistence(timeout: 2) { next.tap() }
      else { app.typeText("\n") }
    }
    app.launch()
    action("내 결제")
    action("주문 등록 공개키 보기")
    let key = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
    XCTAssertTrue(key.waitForExistence(timeout: 12))
    let registration = try control("/enroll/customer", key.label)
    let clipboard = UIPasteboard.general
    clipboard.setItems([["public.utf8-plain-text": try XCTUnwrap(registration["registration"])]],
      options: [.localOnly: true, .expirationDate: Date().addingTimeInterval(15)])
    let clipboardVersion = clipboard.changeCount
    defer { if clipboard.changeCount == clipboardVersion { clipboard.items = [] } }
    let input = app.textViews["customer-commerce-registration"].firstMatch
    tap(input)
    input.press(forDuration: 1.2)
    tap(app.descendants(matching: .any).matching(NSPredicate(format: "label IN %@", ["Paste", "붙여넣기"])).firstMatch)
    let allow = app.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Allow Paste", "붙여넣기 허용"])).firstMatch
    if allow.waitForExistence(timeout: 2) { allow.tap() }
    XCTAssertTrue(input.value as? String == registration["registration"], "UI registration differs from issued record")
    XCTAssertLessThan(input.frame.height, app.frame.height / 3, "Pasted registration hides the submit and retry controls")
    nextField()
    app.typeText(try XCTUnwrap(registration["pin"]))
    let firstPin = app.textFields["customer-commerce-tls-pin"].firstMatch
    XCTAssertTrue(firstPin.value as? String == registration["pin"],"UI server pin differs from issued reference")
    nextField()
    app.typeText(try XCTUnwrap(registration["presentationPin"]))
    let finalPin = app.textFields["customer-commerce-presentation-pin"].firstMatch
    XCTAssertTrue(finalPin.value as? String == registration["presentationPin"],"UI presentation pin differs from issued reference")
    let done = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Done", "완료"])).firstMatch
    if done.waitForExistence(timeout: 2) { done.tap() }
    else if app.keyboards.firstMatch.exists { app.typeText("\n") }
    let confirm = app.buttons["주문 등록 확인"].firstMatch
    if confirm.exists && confirm.isEnabled && confirm.isHittable { confirm.tap() }
    let committed = NSPredicate(format: "exists == false")
    func didSave() -> Bool {
      XCTWaiter.wait(for: [XCTNSPredicateExpectation(predicate: committed, object: confirm)], timeout: 12) == .completed
    }
    if !didSave() {
      let cleanupRetry = app.staticTexts["이전 구독 알림을 정리할 수 없습니다. 등록을 다시 시도하세요."].firstMatch
      XCTAssertTrue(cleanupRetry.exists, "Registration did not complete or explain how to retry")
      tap(confirm)
      XCTAssertTrue(didSave(), "The explicit retry did not save the registration")
    }
    let raw = Data(try XCTUnwrap(registration["registration"]).utf8)
    let profile = try XCTUnwrap(JSONSerialization.jsonObject(with: raw) as? [String:Any])
    let branch = try XCTUnwrap((profile["scope"] as? [String:Any])?["branchRef"] as? String)
    app.terminate()
    app.open(try XCTUnwrap(URL(string:"nu54://shop/"+branch)))
    action("이 매장 메뉴 열기")
    let hot="아메리카노 따뜻하게 추가",iced="아메리카노 아이스 추가"
    action(hot)
    action(iced)
    action(iced)
    text("합계 · 11.00 TestUSDC")
    action("아메리카노 아이스 빼기")
    text("합계 · 7.50 TestUSDC")
    action(iced)
    action(hot)
    action(hot)
    text("합계 · 19.00 TestUSDC")
    XCTAssertFalse(app.buttons[hot].isEnabled)
    XCTAssertFalse(app.buttons[iced].isEnabled)
    action("주문 접수 요청")
    text("주문이 연결되었습니다. 보드 승인 대기 중입니다.")
    text("금액 19.00 TestUSDC")
    struct Ledger: Codable {
      struct Line: Codable {
        let option: String
        let quantity: Int
      }
      let orders: Int
      let orderRef: String
      let lines: [Line]
      let stock: [String: Int]
    }
    func ledger() throws -> Ledger {
      var req = URLRequest(url: try XCTUnwrap(URL(string: bridge + "/k07-proof")))
      req.httpMethod = "POST"
      req.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      let complete = expectation(description: "Original P05 order")
      let reply = TravelEnrollmentResult()
      URLSession.shared.dataTask(with: req) { data, response, _ in
        if (response as? HTTPURLResponse)?.statusCode == 200 { reply.raw = data }
        complete.fulfill()
      }.resume()
      wait(for: [complete], timeout: 15)
      return try JSONDecoder().decode(Ledger.self, from: XCTUnwrap(reply.raw))
    }
    let original = try ledger()
    XCTAssertEqual(original.orders, 1)
    XCTAssertEqual(original.lines.map(\.quantity), [3, 2])
    XCTAssertEqual(original.lines.map(\.option), ["hot", "iced"])
    XCTAssertEqual(original.stock["drink-12"], 0)

    app.terminate()
    let expiry = Int(Date().timeIntervalSince1970) + 45
    let link = "nu54://order?v=2&branch=\(branch)&order=\(original.orderRef)&expires=\(expiry)"
    app.open(try XCTUnwrap(URL(string: link)))
    text("금액 19.00 TestUSDC")
    let resumed = try ledger()
    XCTAssertEqual(resumed.orderRef, original.orderRef)
    XCTAssertEqual(resumed.orders, 1)
    let proof = XCTAttachment(data: try JSONEncoder().encode(resumed), uniformTypeIdentifier: "public.json")
    proof.name = "Persisted customer order after first setup and restart"
    proof.lifetime = .keepAlways
    add(proof)
  }
}

final class W12RenterStatusSmoke: XCTestCase {
  func testEnrolledRenterRestoresOriginalHandoverAndHidesRevokedStatus() throws {
    let env = ProcessInfo.processInfo.environment
    let origin = try XCTUnwrap(env["NU54_W12_CONTROL"])
    let token = try XCTUnwrap(env["NU54_W12_TOKEN"])
    let app = XCUIApplication(bundleIdentifier: try XCTUnwrap(env["NU54_W12_APP"]))
    continueAfterFailure = false

    func control(_ route: String, _ input: String? = nil) throws -> [String: String] {
      var request = URLRequest(url: try XCTUnwrap(URL(string: origin + route)))
      request.httpMethod = "POST"
      request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      if let input { request.httpBody = Data(input.utf8) }
      let replied = expectation(description: route)
      let result = TravelEnrollmentResult()
      URLSession.shared.dataTask(with: request) { data, response, _ in
        if (response as? HTTPURLResponse)?.statusCode == 200 { result.raw = data }
        replied.fulfill()
      }.resume()
      wait(for: [replied], timeout: 10)
      return try JSONDecoder().decode([String: String].self, from: XCTUnwrap(result.raw, route))
    }
    func tap(_ element: XCUIElement) {
      XCTAssertTrue(element.waitForExistence(timeout: 8), element.debugDescription)
      for _ in 0..<4 where !element.isHittable {
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.77)).press(forDuration: 0.05,
          thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.42)))
      }
      XCTAssertTrue(element.isHittable && element.isEnabled, element.debugDescription)
      element.tap()
    }
    func action(_ label: String) { tap(app.buttons[label].firstMatch) }
    func fill(_ label: String, _ value: String) {
      let field = app.textFields[label].firstMatch
      tap(field)
      field.typeText(value)
      if label != "customer-commerce-tls-pin" { field.typeText("\n") }
    }
    func openRental() {
      if app.buttons["지갑 프로필 선택"].exists { action("지갑 프로필 선택") }
      action("대여 보드 반납 상태")
    }
    func pasteRegistration(_ value: String) {
      let field = app.textViews["customer-commerce-registration"].firstMatch
      tap(field)
      let clipboard = UIPasteboard.general
      clipboard.setItems([["public.utf8-plain-text": value]],
        options: [.localOnly: true, .expirationDate: Date().addingTimeInterval(15)])
      let version = clipboard.changeCount
      defer { if clipboard.changeCount == version { clipboard.items = [] } }
      field.press(forDuration: 1.2)
      tap(app.descendants(matching: .any).matching(NSPredicate(format: "label IN %@", ["Paste", "붙여넣기"])).firstMatch)
      let allow = app.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Allow Paste", "붙여넣기 허용"])).firstMatch
      if allow.waitForExistence(timeout: 1) { allow.tap() }
    }

    app.launch()
    defer { app.terminate() }
    action("내 결제")
    action("주문 등록 공개키 보기")
    let key = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
    XCTAssertTrue(key.waitForExistence(timeout: 8))
    let issue = try control("/enroll", key.label)
    let assignment = try XCTUnwrap(issue["assignment"])
    let account = try XCTUnwrap(issue["account"])
    let generation = try XCTUnwrap(issue["generation"])
    pasteRegistration(try XCTUnwrap(issue["registration"]))
    let next = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Next", "다음"])).firstMatch
    if next.waitForExistence(timeout: 2) { next.tap() } else { app.typeText("\n") }
    let pin = try XCTUnwrap(issue["pin"])
    app.typeText(pin)
    XCTAssertEqual(app.textFields["customer-commerce-tls-pin"].firstMatch.value as? String, pin)
    if next.waitForExistence(timeout: 2) { next.tap() } else { app.typeText("\n") }
    let done = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Done", "완료"])).firstMatch
    if done.waitForExistence(timeout: 2) { done.tap() }
    else if app.keyboards.firstMatch.exists { app.typeText("\n") }
    if !app.buttons["구독 관리"].waitForExistence(timeout: 3) { action("주문 등록 확인") }
    XCTAssertTrue(app.buttons["구독 관리"].waitForExistence(timeout: 8))
    action("지갑으로 돌아가기")
    openRental()
    fill("대여 인도 참조", assignment)
    fill("인도받은 대여 계정", account)
    action("이 계정의 대여 인도 수락")
    let original = app.staticTexts["원래 대여 계정 · " + account]
    XCTAssertTrue(original.waitForExistence(timeout: 8))

    let checksCredential = env["NU54_W12_CREDENTIAL"] == "1"
    let checksSlot = env["NU54_W12_SLOT_OPEN"] == "1"
    let slotUnknown = app.staticTexts["대여 슬롯 · 확인 불가"].firstMatch
    let slotOpen = app.staticTexts["대여 슬롯 · 열림"].firstMatch
    if checksSlot { XCTAssertTrue(slotUnknown.exists) }
    let unconfirmedCredential = app.staticTexts["대여 패스키 사용기한 · 확인 불가"].firstMatch
    if checksCredential {
      XCTAssertTrue(unconfirmedCredential.exists, "The POST accept must only confirm the handover.")
      action("같은 대여 상태 조회")
      let liveTerm = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@ AND label != %@",
        "대여 패스키 사용기한 · ", "대여 패스키 사용기한 · 확인 불가")).firstMatch
      let evidenceEnd = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "패스키 근거 유효기한 · ")).firstMatch
      XCTAssertTrue(liveTerm.waitForExistence(timeout: 8))
      XCTAssertTrue(evidenceEnd.exists)
      let ready = try control("/credential-observed")
      XCTAssertEqual(ready["proof"], "C")
      let wire = ISO8601DateFormatter()
      let termTime = try XCTUnwrap(wire.date(from: XCTUnwrap(ready["credentialExpiresAt"])))
      let evidenceTime = try XCTUnwrap(wire.date(from: XCTUnwrap(ready["credentialEvidenceUntil"])))
      XCTAssertGreaterThan(termTime, evidenceTime)
      XCTAssertLessThanOrEqual(evidenceTime.timeIntervalSinceNow, 30)
      let local = DateFormatter()
      local.locale = Locale(identifier: "ko-KR")
      local.calendar = Calendar(identifier: .gregorian)
      local.dateFormat = "yyyy. M. d. a h:mm:ss"
      func compact(_ label: String) -> String { label.filter { !$0.isWhitespace } }
      XCTAssertEqual(compact(liveTerm.label), compact("대여 패스키 사용기한 · " + local.string(from: termTime)))
      XCTAssertEqual(compact(evidenceEnd.label), compact("패스키 근거 유효기한 · " + local.string(from: evidenceTime)))
      if checksSlot {
        XCTAssertTrue(slotOpen.exists)
        let checked = try XCTUnwrap(wire.date(from: XCTUnwrap(ready["slotCheckedAt"])))
        let slotUntil = try XCTUnwrap(wire.date(from: XCTUnwrap(ready["slotEvidenceUntil"])))
        let slotCheckedLabel = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "슬롯 확인 시각 · ")).firstMatch
        let slotUntilLabel = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "슬롯 확인 유효기한 · ")).firstMatch
        XCTAssertLessThanOrEqual(checked, Date())
        XCTAssertGreaterThan(slotUntil, checked)
        XCTAssertLessThanOrEqual(slotUntil.timeIntervalSince(checked), 30)
        XCTAssertEqual(compact(slotCheckedLabel.label), compact("슬롯 확인 시각 · " + local.string(from: checked)))
        XCTAssertEqual(compact(slotUntilLabel.label), compact("슬롯 확인 유효기한 · " + local.string(from: slotUntil)))
        XCTAssertGreaterThan(slotUntil.timeIntervalSinceNow, 4)
        Thread.sleep(forTimeInterval: max(0, slotUntil.timeIntervalSinceNow - 4))
        XCTAssertLessThan(Date(), slotUntil)
        XCTAssertTrue(slotOpen.exists && !slotUnknown.exists)
        XCTAssertTrue(slotUnknown.waitForExistence(timeout: max(1, slotUntil.timeIntervalSinceNow + 3)))
        XCTAssertGreaterThanOrEqual(Date(), slotUntil.addingTimeInterval(-0.5))
        XCTAssertFalse(slotOpen.exists || slotCheckedLabel.exists || slotUntilLabel.exists)
        // A new read of the same still-valid credential must preserve the original stated term.
        XCTAssertGreaterThan(termTime, Date())
        XCTAssertTrue(original.exists)
        action("같은 대여 상태 조회")
        XCTAssertTrue(liveTerm.waitForExistence(timeout: 8))
        XCTAssertEqual(compact(liveTerm.label), compact("대여 패스키 사용기한 · " + local.string(from: termTime)))
      } else {
        // Read the view shortly before the actual server deadline without tapping.
        XCTAssertGreaterThan(evidenceTime.timeIntervalSinceNow, 4)
        Thread.sleep(forTimeInterval: max(0, evidenceTime.timeIntervalSinceNow - 4))
        XCTAssertLessThan(Date(), evidenceTime)
        XCTAssertTrue(liveTerm.exists && evidenceEnd.exists)
        XCTAssertFalse(unconfirmedCredential.exists)
        XCTAssertTrue(unconfirmedCredential.waitForExistence(timeout: max(1, evidenceTime.timeIntervalSinceNow + 3)),
          "Evidence must leave the screen without another user action.")
        XCTAssertGreaterThanOrEqual(Date(), evidenceTime.addingTimeInterval(-0.5))
        XCTAssertFalse(liveTerm.exists)
        XCTAssertFalse(evidenceEnd.exists)
        XCTAssertTrue(original.exists, "The handover remains active when credential evidence expires.")
      }
    }
    var linkedOrigin: String?
    if env["NU54_W12_LINKED_PAYMENTS"] == "1" {
      let linked = try control("/observe")
      let originRef = try XCTUnwrap(linked["origin"])
      linkedOrigin = originRef
      action("같은 대여 상태 조회")
      let pending = app.staticTexts["미결 지급 · 1"].firstMatch
      XCTAssertTrue(pending.waitForExistence(timeout: 8))
      XCTAssertTrue(app.staticTexts["미결 한도 변경 · 1"].exists)
      if checksCredential { XCTAssertTrue(unconfirmedCredential.exists) }
      if checksSlot { XCTAssertTrue(slotUnknown.exists && !slotOpen.exists) }
      let button = "미결 원거래 " + originRef
      XCTAssertEqual(app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "미결 원거래 ")).count, 1)
      action(button)
      XCTAssertTrue(app.staticTexts["원거래 참조 · " + originRef].firstMatch.waitForExistence(timeout: 8))
      XCTAssertTrue(app.staticTexts["금액 · 0.000020 tUSDC"].exists)
      XCTAssertFalse(app.buttons["결제 요청"].exists)
      XCTAssertFalse(app.buttons["같은 대여 상태 조회"].exists)
      action("대여 반납 상태로 돌아가기")
      XCTAssertTrue(app.buttons["같은 대여 상태 조회"].exists)
      if checksSlot {
        action("프로필 선택")
        XCTAssertTrue(app.staticTexts["어떤 지갑을 사용할까요?"].waitForExistence(timeout: 8))
        XCTAssertFalse(original.exists)
        XCTAssertFalse(app.buttons["같은 대여 상태 조회"].exists)
        let rentalEntry = app.buttons["대여 보드 반납 상태"].firstMatch
        if !rentalEntry.isHittable {
          app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.35)).press(forDuration: 0.05,
            thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.73)))
        }
        tap(rentalEntry)
        fill("대여 인도 참조", assignment)
        action("같은 대여 상태 조회")
        XCTAssertTrue(original.waitForExistence(timeout: 8))
        XCTAssertTrue(app.staticTexts["원래 대여 세대 · " + generation].exists)
        XCTAssertTrue(app.buttons[button].exists)
      }
    }
    app.terminate()
    _ = try control("/restart")
    app.launch()
    openRental()
    fill("대여 인도 참조", assignment)
    action("같은 대여 상태 조회")
    XCTAssertTrue(original.waitForExistence(timeout: 8))
    XCTAssertTrue(app.staticTexts["원래 대여 세대 · " + generation].exists)
    if checksCredential, let linkedOrigin {
      XCTAssertTrue(app.buttons["미결 원거래 " + linkedOrigin].firstMatch.waitForExistence(timeout: 8))
      XCTAssertTrue(unconfirmedCredential.exists)
    }
    _ = try control("/revoke")
    action("같은 대여 상태 조회")
    XCTAssertTrue(app.staticTexts["이 대여의 상태를 확인할 수 없습니다."].waitForExistence(timeout: 8))
    XCTAssertFalse(original.exists)
    if checksCredential { XCTAssertTrue(unconfirmedCredential.exists) }
  }
}

final class K10NearbySmoke: XCTestCase {
  private enum BoardDecision {
    case refuse
    case approve
  }

  func testOriginalDenialSurvivesRelaunch() throws {
    try recover(signedReview: false)
  }

  func testLostSignedQuoteReopensOriginalReview() throws {
    try recover(signedReview: true)
  }

  func testSponsoredReviewAndBoardRefusal() throws {
    try recover(signedReview: true, decision: .refuse)
  }

  func testSponsoredBoardApprovalMaintenanceHold() throws {
    try recover(signedReview: true, decision: .refuse, busyMaintenance: true)
  }

  func testSponsoredBoardApprovalAndSettlement() throws {
    try recover(signedReview: true, decision: .approve)
  }

  func testSponsoredPickupReceipt() throws {
    try recover(signedReview: true, decision: .approve, pickupReceipt: true)
  }

  func testSponsoredPendingReceipt() throws {
    try recover(signedReview: true, decision: .approve, pendingReceipt: true)
  }

  func testSponsoredUnpaidReceipt() throws {
    try recover(signedReview: true, decision: .approve, unpaidReceipt: true)
  }

  private func recover(signedReview: Bool, decision: BoardDecision? = nil,
                       pickupReceipt: Bool = false, pendingReceipt: Bool = false, unpaidReceipt: Bool = false,
                       busyMaintenance: Bool = false) throws {
    continueAfterFailure = false
    let env = ProcessInfo.processInfo.environment
    guard let origin = env["NU54_K10_CONTROL"], let token = env["NU54_K10_TOKEN"],
          let bundle = env["NU54_K10_APP"] else { throw XCTSkip("Use the isolated K10 virtual nearby fixture.") }
    let app = XCUIApplication(bundleIdentifier: bundle)
    func control(_ route: String, _ input: String? = nil) throws -> [String: String] {
      var request = URLRequest(url: try XCTUnwrap(URL(string: origin + route)))
      request.httpMethod = "POST"
      request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
      if let input { request.httpBody = Data(input.utf8) }
      let reply = expectation(description: route)
      let result = TravelEnrollmentResult()
      URLSession.shared.dataTask(with: request) { data, response, _ in
        if (response as? HTTPURLResponse)?.statusCode == 200 { result.raw = data }
        reply.fulfill()
      }.resume()
      wait(for: [reply], timeout: 10)
      return try JSONDecoder().decode([String: String].self, from: XCTUnwrap(result.raw, route))
    }
    func tap(_ element: XCUIElement) {
      XCTAssertTrue(element.waitForExistence(timeout: 12), element.debugDescription)
      for _ in 0..<4 where !element.isHittable {
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.78)).press(forDuration: 0.05,
          thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.36)))
      }
      XCTAssertTrue(element.isHittable && element.isEnabled, element.debugDescription)
      element.tap()
    }
    func action(_ label: String) { tap(app.buttons[label].firstMatch) }
    func shown(_ label: String) {
      XCTAssertTrue(app.staticTexts[label].firstMatch.waitForExistence(timeout: 12), app.debugDescription)
    }
    func reopenShop() {
      action("가까운 매장에서 주문")
      action("근처 단말 찾기")
      action("근거리 단말 후보 1 인증")
      action("이 매장 메뉴 열기")
      shown("여울 커피 성수점 메뉴")
      shown("온라인 단말 중계")
    }
    app.launch()
    defer { app.terminate() }
    action("내 결제")
    action("주문 등록 공개키 보기")
    let key = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "MFkwEwYH")).firstMatch
    XCTAssertTrue(key.waitForExistence(timeout: 8))
    let issue = try control("/enroll", key.label)
    let registration = try XCTUnwrap(issue["registration"])
    let pin = try XCTUnwrap(issue["pin"])
    let profile = app.textViews["customer-commerce-registration"].firstMatch
    tap(profile)
    let clipboard = UIPasteboard.general
    clipboard.setItems([["public.utf8-plain-text": registration]],
      options: [.localOnly: true, .expirationDate: Date().addingTimeInterval(15)])
    let version = clipboard.changeCount
    defer { if clipboard.changeCount == version { clipboard.items = [] } }
    profile.press(forDuration: 1.2)
    tap(app.descendants(matching: .any).matching(NSPredicate(format: "label IN %@", ["Paste", "붙여넣기"])).firstMatch)
    let allow = app.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Allow Paste", "붙여넣기 허용"])).firstMatch
    if allow.waitForExistence(timeout: 1) { allow.tap() }
    let next = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Next", "다음"])).firstMatch
    if next.waitForExistence(timeout: 2) { next.tap() } else { app.typeText("\n") }
    app.typeText(pin)
    XCTAssertEqual(app.textFields["customer-commerce-tls-pin"].firstMatch.value as? String, pin)
    if next.waitForExistence(timeout: 2) { next.tap() } else { app.typeText("\n") }
    if signedReview {
      let proof = try XCTUnwrap(issue["presentationPin"])
      app.typeText(proof)
      XCTAssertEqual(app.textFields["customer-commerce-presentation-pin"].firstMatch.value as? String, proof)
    }
    let done = app.keyboards.buttons.matching(NSPredicate(format: "label IN %@", ["Done", "완료"])).firstMatch
    if done.waitForExistence(timeout: 2) { done.tap() }
    else if app.keyboards.firstMatch.exists { app.typeText("\n") }
    let registered = app.buttons["구독 관리"]
    if !signedReview && !registered.waitForExistence(timeout: 12) {
      let register = app.buttons["주문 등록 확인"].firstMatch
      if register.exists && register.isEnabled { tap(register) }
    }
    if signedReview {
      let failure = app.staticTexts.matching(NSPredicate(format: "label ENDSWITH %@", "등록을 다시 시도하세요.")).firstMatch
      func waitForRegistration() {
        let result = XCTNSPredicateExpectation(predicate: NSPredicate { _, _ in registered.exists || failure.exists }, object: nil)
        XCTAssertEqual(XCTWaiter.wait(for: [result], timeout: 20), .completed, "Registration did not finish")
      }
      waitForRegistration()
      if (pickupReceipt || pendingReceipt || unpaidReceipt) && failure.exists &&
          failure.label == "시스템 알림을 확인하는 시간이 초과됐습니다. 등록을 다시 시도하세요." {
        action("주문 등록 확인")
        waitForRegistration()
      }
      XCTAssertTrue(registered.exists, failure.exists ? failure.label : "Registration did not finish")
    } else {
      XCTAssertTrue(registered.waitForExistence(timeout: 15), app.debugDescription)
    }
    action("지갑으로 돌아가기")

    reopenShop()
    action("Iced tea cold 추가")
    action("차 따뜻함 추가")
    _ = try control("/arm")
    action("주문 접수 요청")
    shown("결제 견적을 확인하고 있습니다")
    shown("발급 여부 미확인")

    app.terminate()
    if signedReview {
      app.launch()
      reopenShop()
      action("원래 요청 재조회")
      shown("Iced tea · cold ×1 · 8.00 TestUSDC")
      shown("차 · 따뜻함 ×1 · 7.00 TestUSDC")
      let orderAmount = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@ AND label ENDSWITH %@",
        "검토한 주문 ", "15.00 TestUSDC")).firstMatch
      XCTAssertTrue(orderAmount.waitForExistence(timeout: 12), app.debugDescription)
      action("원견적 상세")
      let merchant = try XCTUnwrap(issue["merchant"])
      shown("가맹점 식별자 " + merchant)
      shown("네트워크 StableNet 테스트넷 · 8283")
      shown("수령 주소 " + (try XCTUnwrap(issue["recipient"])))
      XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
      if let decision {
        let original = String(try XCTUnwrap(orderAmount.label.split(separator: " ").dropFirst(2).first))
        XCTAssertNotNil(UUID(uuidString: original))
        action("견적 확인으로")
        shown("인증한 가맹점 견적 · " + merchant)
        shown("가스 비용 · 매장 부담")
        shown("등록된 릴레이 " + (try XCTUnwrap(issue["relay"])))
        action("검증한 보드 찾기")
        action(busyMaintenance ? "NU54 W04 QA에서 결제 검토 및 기기 상태 확인" : "NU54 W04 QA에서 결제 검토")
        shown("보드가 원견적을 검증했습니다")
        if busyMaintenance {
          action("원승인 기기 상태")
          shown("개인 보드 유지관리")
          shown("기기 승인 진행 · 업데이트 적용 보류")
          XCTAssertFalse(app.buttons["이 기기 적용 증거 읽기"].exists)
          XCTAssertFalse(app.buttons["인증된 기기 상태 조회 · 원승인 기기"].exists)
          action("원승인으로 돌아가기")
          shown("보드가 원견적을 검증했습니다")
          XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
          action("원승인 기기 상태")
          shown("기기 승인 진행 · 업데이트 적용 보류")
          _ = try control("/owner/sw2")
          action("인증된 기기 상태 조회 · 원승인 기기")
          shown("인증된 소유 세대 · 1")
          XCTAssertFalse(app.staticTexts["기기 승인 진행 · 업데이트 적용 보류"].exists)
          action("원승인으로 돌아가기")
        }
        switch decision {
        case .refuse:
          if !busyMaintenance { _ = try control("/owner/sw2") }
          shown("승인 대기가 끝났습니다. 보드에서 결제를 거절했습니다.")
          XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
          action("같은 주문의 지급 결과 확인")
          shown(original)
          shown("금액 15.00 TestUSDC")
          XCTAssertFalse(app.staticTexts["네트워크 응답을 기다립니다. 같은 원주문을 조회하세요."].exists)
          XCTAssertFalse(app.buttons["보드 서명 확인"].exists)
        case .approve:
          _ = try control("/owner/sw1")
          shown("보드 서명 확인")
          _ = try control("/arm-submit-loss", original)
          action("지급 요청 제출")
          shown("네트워크 응답을 기다립니다. 같은 원주문을 조회하세요.")
          if pickupReceipt {
            XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "픽업 번호 ")).firstMatch.exists)
          }
          if pendingReceipt || unpaidReceipt {
            func checkPending() {
              shown(original + " 지급 결과 확인 중")
              shown("금액 15.00 TestUSDC")
              shown("지점 여울 커피 성수점")
              shown("이 원주문의 결합 견적과 네트워크 결과를 조회합니다. 중복 제출은 차단됩니다.")
              XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
              XCTAssertFalse(app.buttons["같은 지급 요청 다시 제출"].exists)
              XCTAssertFalse(app.buttons["검증한 보드 찾기"].exists)
              XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "픽업 번호 ")).firstMatch.exists)
            }
            action("같은 주문의 지급 결과 확인")
            checkPending()
            action("내 결제")
            action("매장 주문 " + original)
            checkPending()
            app.terminate()
            app.launch()
            action("내 결제")
            action("매장 주문 " + original)
            checkPending()
            if unpaidReceipt {
              let limit = Date().addingTimeInterval(150)
              var resolved = false
              while Date() < limit && !resolved {
                resolved = try control("/expire-original", original)["closed"] == "true"
                if !resolved { Thread.sleep(forTimeInterval: 2) }
              }
              XCTAssertTrue(resolved, "Original signed quote should close only after finalized absence")
              action("같은 주문의 지급 결과 확인")
              func checkUnpaid() {
                shown(original + " 미지급 확인")
                shown("원 견적 미소비와 유효 범위를 확인했습니다.")
                shown("지급 0건")
                shown("금액 15.00 TestUSDC")
                XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
                XCTAssertFalse(app.buttons["같은 지급 요청 다시 제출"].exists)
                XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "픽업 번호 ")).firstMatch.exists)
                XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "확정 거래 ")).firstMatch.exists)
              }
              checkUnpaid()
              action("내 결제")
              action("매장 주문 " + original)
              checkUnpaid()
              app.terminate()
              app.launch()
              action("내 결제")
              action("매장 주문 " + original)
              checkUnpaid()
              return
            }
            let payment = try control("/finalize-original", original)
            let hash = try XCTUnwrap(payment["hash"])
            XCTAssertNotNil(hash.range(of: "^0x[0-9a-f]{64}$", options: .regularExpression))
            action("같은 주문의 지급 결과 확인")
            shown(original)
            shown("금액 15.00 TestUSDC")
            shown("확정 거래 " + hash)
            XCTAssertFalse(app.staticTexts[original + " 지급 결과 확인 중"].exists)
            return
          }
          let payment = try control("/finalize-original", original)
          let hash = try XCTUnwrap(payment["hash"])
          XCTAssertNotNil(hash.range(of: "^0x[0-9a-f]{64}$", options: .regularExpression))
          app.terminate()
          app.launch()
          reopenShop()
          action("원래 요청 재조회")
          XCTAssertTrue(app.staticTexts[original].firstMatch.waitForExistence(timeout: 12),
            "W10 restart terminal view must reopen the original sale")
          XCTAssertTrue(app.staticTexts["금액 15.00 TestUSDC"].firstMatch.waitForExistence(timeout: 12),
            "W10 restart terminal view must retain the original amount")
          XCTAssertFalse(app.staticTexts["확정 거래 " + hash].exists,
            "W10 terminal view must withhold the customer's transaction")
          action("같은 주문의 지급 결과 확인")
          XCTAssertTrue(app.staticTexts["주문 원장과 체인 지급 결과가 일치합니다."].firstMatch.waitForExistence(timeout: 12),
            "W10 authenticated owner view must show the confirmed payment")
          XCTAssertTrue(app.staticTexts["확정 거래 " + hash].firstMatch.waitForExistence(timeout: 12),
            "W10 authenticated owner view must retain the original transaction")
          if pickupReceipt {
            let readyLine = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "픽업 가능 시각 ")).firstMatch
            XCTAssertFalse(readyLine.exists)
            let references = try control("/ready-original", original)
            let pickupNo = try XCTUnwrap(references["pickupNo"])
            let wire = ISO8601DateFormatter()
            wire.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            func instant(_ key: String) throws -> Date {
              let value = try XCTUnwrap(references[key])
              return try XCTUnwrap(wire.date(from: value) ?? ISO8601DateFormatter().date(from: value))
            }
            let finalized = try instant("finalizedAt")
            let ready = try instant("pickupReadyAt")
            XCTAssertLessThan(finalized, ready)
            // Foundation and Hermes can choose different default locales on the same simulator.
            // Both labels must match one exact format for the independently observed instants.
            let formats = [("en-US", "M/d/yyyy, h:mm:ss a"), ("ko-KR", "yyyy. M. d. a h:mm:ss")].map { locale, pattern in
              let formatter = DateFormatter()
              formatter.locale = Locale(identifier: locale)
              formatter.calendar = Calendar(identifier: .gregorian)
              formatter.timeZone = .current
              formatter.dateFormat = pattern
              return formatter
            }
            var matchingFormats = formats
            func checkedTime(_ title: String, _ date: Date) {
              let line = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", title + " ")).firstMatch
              XCTAssertTrue(line.waitForExistence(timeout: 12), title)
              let actual = line.label.filter { !$0.isWhitespace }
              matchingFormats.removeAll { formatter in
                actual != (title + formatter.string(from: date)).filter { !$0.isWhitespace }
              }
              XCTAssertFalse(matchingFormats.isEmpty, "\(title) differs from the original ledger instant")
            }
            func checkReady() throws {
              shown(original)
              shown("금액 15.00 TestUSDC")
              shown("확정 거래 " + hash)
              shown("픽업 번호 " + pickupNo)
              shown("픽업 준비 완료")
              checkedTime("지급 확정 시각", finalized)
              checkedTime("픽업 가능 시각", ready)
              XCTAssertFalse(app.buttons["지급 요청 제출"].exists)
            }
            action("같은 주문의 지급 결과 확인")
            try checkReady()
            action("결제 목록")
            let orderButton = app.buttons["매장 주문 " + original].firstMatch
            tap(orderButton)
            try checkReady()
            app.terminate()
            app.launch()
            action("내 결제")
            tap(orderButton)
            try checkReady()
          }
        }
      }
      return
    }
    _ = try control("/restock")
    app.launch()
    reopenShop()
    action("원래 요청 재조회")
    shown("선택한 상품이나 옵션의 재고가 부족합니다")
    shown("확보 못한 품목 · 차 · 따뜻함 ×1")
    shown("선택 유지 · Iced tea · cold ×1")
    action("수량·옵션 수정")
    action("차 따뜻함 빼기")
    action("바뀐 장바구니 확인 후 주문 접수")
    shown("주문이 연결되었습니다. 보드 승인 대기 중입니다.")
  }
}
