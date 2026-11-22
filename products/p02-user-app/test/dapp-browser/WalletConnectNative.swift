import XCTest
import UIKit

final class WalletConnectNativeJourney: XCTestCase {
  private var app = XCUIApplication(bundleIdentifier: "com.nu54renter.walletconnect.journey")
  private var endpoint = URL(string: "https://127.0.0.1:18449/case")!

  func testInstalledWalletApprovesAndRestoresTheSameExternalTransfer() throws {
    let fixture = try state()
    let first = try XCTUnwrap(fixture["first"] as? String)
    let second = try XCTUnwrap(fixture["second"] as? String)
    continueAfterFailure = false
    app.launch()
    wallet()
    tap("외부 DApp 연결")
    pair(first, spend: false)
    decline("외부 DApp 연결")
    let rejected = app.staticTexts["연결 제안을 거절했습니다. 새 연결 주소를 사용할 수 있습니다."]
    XCTAssertTrue(rejected.waitForExistence(timeout: 18), "Declined proposal notice missing")
    pair(second, spend: true)
    approve("외부 DApp 연결")
    decline("외부 DApp 송금")
    approve("외부 DApp 송금")
    approve("서명한 거래 전송")
    let hash = try XCTUnwrap(try until("receipt") as? String)
    XCTAssertEqual(hash.count, 66)
    app.terminate()
    app.launch()
    wallet()
    tap("외부 DApp 연결")
    tap("현재 보드 찾기")
    tap("외부 DApp 다시 연결")
    XCTAssertEqual(try until("done") as? Bool, true)
    tap("지갑으로 돌아가기")
    tap("개인 전송")
    assertVisibleHash(hash)
    tap("전송 상태 재조회")
    XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "선택한 RPC가 확정 결과를 보고했습니다.")).firstMatch.waitForExistence(timeout: 12), "Finalized receipt not visible")
  }

  func testInstalledWalletWithPinnedReferencePeer() throws {
    app = XCUIApplication(bundleIdentifier: "com.nu54renter.walletconnect.reference")
    let fixture = try state()
    continueAfterFailure = false
    app.launch()
    wallet()
    tap("외부 DApp 연결")
    pair(try XCTUnwrap(fixture["first"] as? String), spend: false)
    let unknown = app.alerts["외부 DApp 연결"]
    XCTAssertTrue(unknown.waitForExistence(timeout: 16), "Unverified review missing")
    XCTAssertTrue(unknown.staticTexts.allElementsBoundByIndex.contains { $0.label.contains("출처 미확인 · 계정 조회만") })
    approve("외부 DApp 연결")
    XCTAssertEqual(try until("readPassed") as? Bool, true)
    XCTAssertEqual(try until("readClosed") as? Bool, true)
    pair(try XCTUnwrap(try until("second") as? String), spend: true)
    let verified = app.alerts["외부 DApp 연결"]
    XCTAssertTrue(verified.waitForExistence(timeout: 16), "Registered review missing")
    XCTAssertTrue(verified.staticTexts.allElementsBoundByIndex.contains { $0.label.contains("등록된 출처") })
    approve("외부 DApp 연결")
    decline("외부 DApp 송금")
    approve("외부 DApp 송금")
    approve("서명한 거래 전송")
    let hash = try XCTUnwrap(try until("receipt") as? String)
    app.terminate()
    app.launch()
    wallet()
    tap("외부 DApp 연결")
    tap("현재 보드 찾기")
    tap("외부 DApp 다시 연결")
    XCTAssertEqual(try until("done") as? Bool, true)
    tap("지갑으로 돌아가기")
    tap("개인 전송")
    assertVisibleHash(hash)
    tap("전송 상태 재조회")
    XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "선택한 RPC가 확정 결과를 보고했습니다.")).firstMatch.waitForExistence(timeout: 12), "Finalized receipt not visible")
  }

  func testInstalledWalletRefusesUnsupportedExternalMethod() throws {
    app = XCUIApplication(bundleIdentifier: "com.nu54renter.walletconnect.reference")
    continueAfterFailure=false
    let fixture=try state()
    let kind=try XCTUnwrap(fixture["refusalKind"] as? String)
    XCTAssertTrue(["nft","batch"].contains(kind))
    app.launch();wallet();tap("외부 DApp 연결")
    pair(try XCTUnwrap(fixture["first"] as? String),spend:true)
    approve("외부 DApp 연결")
    checkPrefix(kind=="nft" ? "NFT 전송 요청을 거절했습니다." : "복합 호출 요청을 거절했습니다.")
    XCTAssertEqual(try until("refused") as? Bool,true)
    XCTAssertEqual(try state()["boardCalls"] as? Int,0)
    XCTAssertEqual(try state()["submissions"] as? Int,0)
    XCTAssertFalse(app.alerts["외부 DApp 송금"].exists)
    tap("지갑으로 돌아가기");tap("개인 전송")
    XCTAssertFalse(app.buttons["전송 상태 재조회"].exists)
  }

  func testDisconnectAndRevokeExistingAllowance() throws {
    app = XCUIApplication(bundleIdentifier: "com.nu54renter.wallet.w15qa")
    endpoint = URL(string: "https://127.0.0.1:18483/case")!
    continueAfterFailure = false
    let fixture=try state()
    app.launch(); wallet();tap("외부 DApp 연결")
    pair(try XCTUnwrap(fixture["first"] as? String),spend:false);approve("외부 DApp 연결")
    XCTAssertEqual(try until("readPassed", attempts: 100) as? Bool,true)
    let endSession=app.buttons["외부 DApp 연결 해제"]
    if endSession.exists && endSession.frame.midY>app.frame.height*0.78 {
      app.scrollViews.firstMatch.swipeUp()
    }
    tap("외부 DApp 연결 해제")
    let ended=app.staticTexts["외부 DApp 연결이 종료되었습니다."]
    XCTAssertTrue(ended.waitForExistence(timeout: 14), "Wallet-initiated disconnect did not complete")
    XCTAssertEqual(try until("disconnected", attempts: 100) as? Bool,true)
    tap("지갑으로 돌아가기");tap("온체인 토큰 권한")
    let spender=app.textFields["권한 대상 주소"]
    XCTAssertTrue(spender.waitForExistence(timeout:12))
    spender.tap();spender.typeText(try XCTUnwrap(fixture["spender"] as? String));spender.typeText("\n")
    tap("확정 한도 조회")
    checkPrefix("현재 확정 한도 42")
    tap("토큰 사용 권한 철회 검토");checkPrefix("변경 후 사용 한도 42 → 0")
    tap("거절하고 초안으로");XCTAssertEqual(try state()["boardCalls"] as? Int,0)
    tap("토큰 사용 권한 철회 검토");tap("개인 기기 찾기");tap("기기에 토큰 권한 철회 승인 요청")
    tap("이 네트워크에 전송")
    let hash=try XCTUnwrap(try until("receipt", attempts: 100) as? String)
    checkPrefix("철회 거래 확정 대기")
    app.terminate();app.launch();wallet();tap("온체인 토큰 권한")
    assertVisibleHash(hash);tap("같은 철회 거래 조회");checkPrefix("현재 확정 한도 0")
    XCTAssertEqual(try state()["boardCalls"] as? Int,1)
    XCTAssertEqual(try state()["submitted"] as? Int,1)
  }

  func testInstalledWalletGrantsAndRestoresBoundedAllowance() throws {
    app = XCUIApplication(bundleIdentifier: "com.nu54renter.wallet.w15qa")
    endpoint = URL(string: "https://127.0.0.1:18483/case")!
    continueAfterFailure = false
    let fixture = try state()
    app.launch(); wallet(); tap("외부 DApp 연결")
    pair(try XCTUnwrap(fixture["first"] as? String), spend: true)
    approve("외부 DApp 연결")
    let review = app.alerts["외부 DApp 토큰 권한 부여"]
    XCTAssertTrue(review.waitForExistence(timeout: 40))
    for field in ["token", "spender"] {
      let address = try XCTUnwrap(fixture[field] as? String)
      XCTAssertTrue(review.staticTexts.allElementsBoundByIndex.contains { $0.label.contains(address) })
    }
    for term in ["5.000000 DEMO", "Ethereum Sepolia", "최대 수수료"] {
      XCTAssertTrue(review.staticTexts.allElementsBoundByIndex.contains { $0.label.contains(term) })
    }
    decline("외부 DApp 토큰 권한 부여")
    XCTAssertEqual(try state()["boardCalls"] as? Int, 0)
    decision("외부 DApp 토큰 권한 부여", "연결 보기")
    XCTAssertEqual(try until("viewed") as? Bool, true)
    checkPrefix("승인 검토를 종료했습니다.")
    checkPrefix("https://independent.example")
    XCTAssertEqual(try state()["boardCalls"] as? Int, 0)
    XCTAssertEqual(try state()["submitted"] as? Int, 0)
    tap("외부 DApp 연결 관리"); tap("현재 보드 찾기"); tap("외부 DApp 다시 연결")
    approve("외부 DApp 토큰 권한 부여")
    approve("서명한 거래 전송")
    let hash = try XCTUnwrap(try until("receipt", attempts: 100) as? String)
    XCTAssertEqual(try until("peerReturnedOriginal", attempts: 100) as? Bool, true)
    checkPrefix("권한 부여 거래 확정 대기")
    assertVisibleHash(hash)
    app.terminate(); app.launch(); wallet(); tap("온체인 토큰 권한")
    assertVisibleHash(hash)
    tap("같은 권한 부여 거래 조회")
    checkPrefix("아직 선택한 네트워크에서 확인되지 않습니다.")
    XCTAssertFalse(app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "현재 확정 한도")).firstMatch.exists)
    tap("같은 권한 부여 거래 조회")
    checkPrefix("현재 확정 한도 5000000")
    XCTAssertEqual(try state()["boardCalls"] as? Int, 1)
    XCTAssertEqual(try state()["submitted"] as? Int, 1)
  }

  func testInstalledWalletManagesTwoApprovedSessionsWithoutChangingAllowance() throws {
    app = XCUIApplication(bundleIdentifier: "com.nu54renter.wallet.w15qa")
    endpoint = URL(string: "https://127.0.0.1:18483/case")!
    continueAfterFailure = false
    let fixture=try state()
    app.launch(); wallet(); tap("외부 DApp 연결")
    pair(try XCTUnwrap(fixture["first"] as? String), spend:false)
    approve("외부 DApp 연결")
    XCTAssertEqual(try until("firstRead", attempts:100) as? Bool,true)
    tap("온체인 토큰 권한"); tap("새 외부 DApp 연결")
    pair(try XCTUnwrap(fixture["second"] as? String), spend:false)
    approve("외부 DApp 연결")
    XCTAssertEqual(try until("twoAuthorized", attempts:100) as? Bool,true)
    tap("온체인 토큰 권한")
    checkPrefix("허용된 연결 2")
    checkPrefix("https://first.example"); checkPrefix("https://second.example")
    tap("확정 한도 조회"); checkPrefix("현재 확정 한도 5000000")
    tap("세션 해제 · https://first.example")
    XCTAssertEqual(try until("selectedClosed", attempts:100) as? Bool,true)
    checkPrefix("허용된 연결 1")
    checkPrefix("현재 확정 한도 5000000")
    tap("외부 DApp 연결 관리"); tap("현재 보드 찾기"); tap("외부 DApp 다시 연결")
    XCTAssertEqual(try until("unselectedActive", attempts:100) as? Bool,true)
    tap("온체인 토큰 권한"); tap("전체 연결 해제")
    decision("전체 연결 해제","취소")
    checkPrefix("허용된 연결 1")
    tap("전체 연결 해제")
    decision("전체 연결 해제","전체 연결 해제")
    XCTAssertEqual(try until("allClosed", attempts:100) as? Bool,true)
    checkPrefix("허용된 연결 0")
    tap("확정 한도 조회"); checkPrefix("현재 확정 한도 5000000")
    XCTAssertEqual(try state()["signingAttempts"] as? Int,0)
    XCTAssertEqual(try state()["submissions"] as? Int,0)
  }

  func testInstalledBrowserReturnsOriginalLoginAndRejectsReplay() throws {
    app = XCUIApplication(bundleIdentifier: "com.nu54renter.wallet.w15qa")
    endpoint = URL(string: "https://127.0.0.1:18483/case")!
    continueAfterFailure = false
    let fixture = try state()
    let site = try XCTUnwrap(fixture["site"] as? String)
    let origin = "https://127.0.0.1:18483"
    app.launch(); wallet(); tap("DApp 둘러보기")
    let url = app.textFields["HTTPS 사이트 주소"]
    XCTAssertTrue(url.waitForExistence(timeout: 16))
    url.tap(); url.typeText(site)
    let done = app.keyboards.buttons["Done"]
    if done.exists { done.tap() }
    tap("사이트 범위 검토")
    checkPrefix("브라우저 출처 · " + origin)
    tap("현재 보드 찾기"); tap("계정 공개 동의 및 사이트 열기")
    let web = app.webViews.firstMatch
    XCTAssertTrue(web.staticTexts["Account scope ready"].waitForExistence(timeout: 16))
    let sign = web.buttons["Sign in"]
    XCTAssertTrue(sign.isEnabled)
    let title = "사이트 로그인 · " + origin
    sign.tap()
    let review = app.alerts[title]
    XCTAssertTrue(review.waitForExistence(timeout: 16))
    XCTAssertTrue(review.staticTexts.allElementsBoundByIndex.contains { $0.label.contains("Nonce 8abc0022") })
    decision(title, "거절")
    XCTAssertTrue(web.staticTexts["Login declined"].waitForExistence(timeout: 16))
    XCTAssertEqual(try state()["boardCalls"] as? Int, 0)
    sign.tap()
    XCTAssertTrue(review.waitForExistence(timeout: 16))
    let terms = review.staticTexts.allElementsBoundByIndex.map(\.label).joined(separator: "\n")
    for term in ["Nonce 8abc0023", "Chain 11155111", "127.0.0.1:18483 wants you to sign in", "Expiration Time:", "기한", "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"] {
      XCTAssertTrue(terms.contains(term), "Missing login review field")
    }
    decision(title, "보드 요청")
    XCTAssertTrue(web.staticTexts["Verified original login. Replay refused. Account scope unchanged."].waitForExistence(timeout: 16))
    XCTAssertEqual(try state()["finished"] as? Bool, true)
    XCTAssertEqual(try state()["boardCalls"] as? Int, 1)
    XCTAssertEqual(try state()["submissions"] as? Int, 0)
    tap("Close · " + origin)
    checkPrefix("사이트 로그인 서명을 반환했습니다")
    checkPrefix("메시지 · 8abc0023")
    checkPrefix("온체인 지출 · 0건")
    tap("연결 목록"); checkPrefix("허용된 연결 0")
  }

  func testInstalledWalletReviewsAndReturnsLogin() throws {
    app = XCUIApplication(bundleIdentifier: "com.nu54renter.wallet.w15qa")
    endpoint = URL(string: "https://127.0.0.1:18483/case")!
    continueAfterFailure = false
    let fixture = try state()
    app.launch(); wallet(); tap("외부 DApp 연결")
    pair(try XCTUnwrap(fixture["first"] as? String), spend: false)
    approve("외부 DApp 연결")
    let review = app.alerts["외부 DApp 로그인"]
    XCTAssertTrue(review.waitForExistence(timeout: 16))
    XCTAssertTrue(review.buttons["세션 보기"].exists)
    XCTAssertTrue(review.staticTexts.allElementsBoundByIndex.contains { $0.label.contains("Nonce 8abc0022") })
    decline("외부 DApp 로그인")
    XCTAssertEqual(try until("refused", attempts: 35) as? Bool,true)
    XCTAssertEqual(try state()["boardCalls"] as? Int,0)
    XCTAssertTrue(review.waitForExistence(timeout: 16))
    for term in ["https://independent.example", "Nonce 8abc0023", "Ethereum Sepolia", "기한", "Expiration Time:"] {
      XCTAssertTrue(review.staticTexts.allElementsBoundByIndex.contains { $0.label.contains(term) })
    }
    approve("외부 DApp 로그인")
    XCTAssertEqual(try until("verified", attempts: 60) as? Bool,true)
    checkPrefix("사이트 로그인 서명을 반환했습니다")
    checkPrefix("메시지 · 8abc0023")
    checkPrefix("온체인 지출 · 0건")
    XCTAssertEqual(try state()["boardCalls"] as? Int,1)
    XCTAssertEqual(try state()["submissions"] as? Int,0)
    tap("연결 목록"); checkPrefix("허용된 연결 1")
  }

  func testInstalledExploreReviewsBrowserAndExternalConnection() throws {
    app = XCUIApplication(bundleIdentifier: "com.nu54renter.wallet.w15qa")
    endpoint = URL(string: "https://127.0.0.1:18483/case")!
    continueAfterFailure = false
    let fixture = try state()
    app.launch(); wallet(); tap("DApp 둘러보기")
    let origin = "https://127.0.0.1:18483"
    let field = app.textFields["HTTPS 사이트 주소"]
    XCTAssertTrue(field.waitForExistence(timeout: 16))
    field.tap()
    field.typeText(origin + "/site")
    let done = app.keyboards.buttons["Done"]
    if done.exists { done.tap() }
    tap("사이트 범위 검토")
    checkPrefix("브라우저 출처 · " + origin)
    XCTAssertFalse(app.buttons["Close · " + origin].exists)
    tap("거절")
    tap("연결 관리"); checkPrefix("허용된 연결 0"); tap("등록 상태로 돌아가기")
    tap("여행 지도"); checkPrefix("여행지 찾기"); tap("지갑으로 돌아가기")
    XCTAssertEqual(field.value as? String, origin + "/site")
    tap("학습 시장"); checkPrefix("격리 테스트 체인"); tap("지갑으로 돌아가기")
    XCTAssertEqual(field.value as? String, origin + "/site")
    tap("사이트 범위 검토"); tap("현재 보드 찾기"); tap("계정 공개 동의 및 사이트 열기")
    XCTAssertTrue(app.buttons["Close · " + origin].waitForExistence(timeout: 16))
    XCTAssertTrue(app.webViews.staticTexts["Local DApp preview"].waitForExistence(timeout: 16))
    tap("Close · " + origin); tap("새 연결 검토"); tap("거절")
    tap("WalletConnect")
    XCTAssertTrue(app.buttons["WalletConnect QR 읽기"].exists)
    pair(try XCTUnwrap(fixture["first"] as? String), spend: false)
    let consent = app.alerts["외부 DApp 연결"]
    XCTAssertTrue(consent.waitForExistence(timeout: 16))
    XCTAssertTrue(consent.staticTexts.allElementsBoundByIndex.contains { $0.label.contains("https://reference.example") })
    XCTAssertEqual(try state()["signingAttempts"] as? Int, 0)
    XCTAssertEqual(try state()["submissions"] as? Int, 0)
    approve("외부 DApp 연결")
    XCTAssertEqual(try until("externalApproved", attempts: 60) as? Bool, true)
  }

  func testInstalledWalletRequiresFreshApprovalAfterExpiryAndRestart() throws {
    app = XCUIApplication(bundleIdentifier: "com.nu54renter.wallet.w15qa")
    endpoint = URL(string: "https://127.0.0.1:18483/case")!
    continueAfterFailure = false
    let fixture = try state()
    app.launch(); wallet(); tap("외부 DApp 연결")
    pair(try XCTUnwrap(fixture["first"] as? String), spend: true)
    approve("외부 DApp 연결")
    XCTAssertEqual(try until("firstRead") as? Bool, true)
    tap("온체인 토큰 권한")
    checkPrefix("허용된 연결 1")
    checkPrefix("https://independent.example")
    tap("외부 DApp 연결 관리")
    tap("현재 보드 찾기"); tap("외부 DApp 다시 연결")
    let pending = app.alerts["외부 DApp 송금"]
    XCTAssertTrue(pending.waitForExistence(timeout: 16))
    let expiry = try XCTUnwrap(try until("expires") as? Int)
    Thread.sleep(forTimeInterval: max(0, Double(expiry) - Date().timeIntervalSince1970) + 1)
    if pending.exists { approve("외부 DApp 송금") }
    checkPrefix("기존 연결로 요청할 수 없습니다")
    XCTAssertEqual(try until("oldSessionDenied", attempts: 60) as? Bool, true)
    XCTAssertEqual(try state()["signingAttempts"] as? Int, 0)
    XCTAssertEqual(try state()["submissions"] as? Int, 0)
    tap("새 연결 검토")
    XCTAssertTrue(app.secureTextFields["WalletConnect 연결 주소"].exists)
    XCTAssertFalse(app.buttons["외부 DApp 다시 연결"].exists)

    pair(try XCTUnwrap(fixture["second"] as? String), spend: false)
    approve("외부 DApp 연결")
    XCTAssertEqual(try until("freshScopeApproved", attempts: 60) as? Bool, true)
    tap("온체인 토큰 권한")
    checkPrefix("허용된 연결 1")
    checkPrefix("https://reference.example")
    XCTAssertFalse(app.staticTexts["https://independent.example"].exists)
    app.terminate(); app.launch(); wallet(); tap("온체인 토큰 권한")
    checkPrefix("허용된 연결 1")
    checkPrefix("https://reference.example")
    XCTAssertFalse(app.staticTexts["https://independent.example"].exists)
    tap("외부 DApp 연결 관리")
    tap("현재 보드 찾기"); tap("외부 DApp 다시 연결")
    XCTAssertEqual(try until("freshResumedAfterRestart", attempts: 60) as? Bool, true)
    XCTAssertEqual(try state()["signingAttempts"] as? Int, 0)
    XCTAssertEqual(try state()["submissions"] as? Int, 0)
  }

  private func checkPrefix(_ prefix:String) {
    let text=app.staticTexts.containing(NSPredicate(format:"label BEGINSWITH %@",prefix)).firstMatch
    XCTAssertTrue(text.waitForExistence(timeout: 15), "Missing wallet result: \(prefix)")
  }
  private func assertVisibleHash(_ hash: String) {
    for _ in 0..<20 {
      if app.staticTexts.allElementsBoundByIndex.contains(where: { $0.label == hash }) { return }
      Thread.sleep(forTimeInterval: 0.3)
    }
    XCTFail("Original hash not visible")
  }

  private func wallet() {
    let rental = app.buttons["지갑 프로필 선택"]
    if rental.exists { rental.tap() }
  }

  private func tap(_ label: String) {
    let button = app.buttons[label]
    let scroll = app.scrollViews.firstMatch
    for attempt in 0..<15 {
      if button.exists && button.isHittable && button.isEnabled { break }
      guard scroll.exists else { break }
      if button.exists && !button.frame.isEmpty {
        let viewport = scroll.frame.intersection(app.frame)
        guard viewport.height > 0 else { break }
        let delta = button.frame.midY - viewport.midY
        let distance = min(0.30, max(0.08, abs(delta) / viewport.height))
        let from = scroll.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
        let to = scroll.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5 + (delta < 0 ? distance : -distance)))
        from.press(forDuration: 0.1, thenDragTo: to, withVelocity: .slow, thenHoldForDuration: 0.2)
      } else if (5..<9).contains(attempt) {
        scroll.swipeDown()
      } else {
        scroll.swipeUp()
      }
    }
    guard button.waitForExistence(timeout: 16), button.isHittable, button.isEnabled else {
      XCTFail("Unavailable action: \(label) (exists: \(button.exists), visible: \(button.isHittable), enabled: \(button.isEnabled))")
      return
    }
    button.tap()
  }

  private func pair(_ uri: String, spend: Bool) {
    let field = app.secureTextFields["WalletConnect 연결 주소"]
    XCTAssertTrue(field.waitForExistence(timeout: 16), "Secure pairing field missing")
    field.tap()
    UIPasteboard.general.string = uri
    defer { UIPasteboard.general.string = nil }
    field.press(forDuration: 1)
    let paste = app.menuItems["Paste"].firstMatch
    guard paste.waitForExistence(timeout: 3) else {
      XCTFail("Secure pairing input did not offer Paste")
      return
    }
    paste.tap()
    if app.alerts.buttons["Allow Paste"].exists { app.alerts.buttons["Allow Paste"].tap() }
    let done=app.keyboards.buttons["Done"]
    if done.exists && done.isHittable { done.tap() }
    else if app.keyboards.firstMatch.exists { field.typeText("\n") }
    if app.keyboards.firstMatch.exists { tap("주소 입력 완료") }
    XCTAssertFalse(app.keyboards.firstMatch.exists, "The keyboard must be closed before board selection")
    if spend { tap("등록된 출처의 송금 요청 허용") }
    tap("현재 보드 찾기")
    tap("외부 DApp 연결 검토")
  }

  private func decline(_ title: String) { decision(title, "거절") }
  private func approve(_ title: String) { decision(title, "승인") }

  private func decision(_ title: String, _ action: String) {
    let alert = app.alerts[title]
    guard alert.waitForExistence(timeout: 16), alert.buttons[action].isHittable else {
      XCTFail("Missing consent: \(title)")
      return
    }
    alert.buttons[action].tap()
  }

  private func state() throws -> [String: Any] {
    let ready = expectation(description: "local QA evidence")
    var answer = Data()
    URLSession.shared.dataTask(with: endpoint) { data, _, _ in
      answer = data ?? Data()
      ready.fulfill()
    }.resume()
    wait(for: [ready], timeout: 5)
    return try XCTUnwrap(try JSONSerialization.jsonObject(with: answer) as? [String: Any])
  }

  private func until(_ key: String, attempts: Int = 35) throws -> Any? {
    for _ in 0..<attempts {
      if let value = try state()[key] { return value }
      Thread.sleep(forTimeInterval: 0.3)
    }
    return nil
  }
}
