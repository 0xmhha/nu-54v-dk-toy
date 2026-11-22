import XCTest
import UIKit
final class OwnerRoutesSmoke: XCTestCase {
  func testOwnerBackupAndRecoveryEntrances() {
    let app=XCUIApplication(bundleIdentifier:"com.nu54renter.w0406qa")
    app.launch()
    func tap(_ label:String) {
      let target=app.buttons[label]
      for _ in 0..<14 where !target.isHittable { app.swipeUp() }
      XCTAssertTrue(target.waitForExistence(timeout:10),"Missing \(label)");target.tap()
    }
    XCTAssertTrue(app.staticTexts.containing(NSPredicate(format:"label CONTAINS %@", "독립 설치 기록")).firstMatch.waitForExistence(timeout:15))
    XCTAssertTrue(app.buttons["내 결제"].exists)
    let w04=XCTAttachment(screenshot:app.screenshot());w04.name="W04 owner entry";w04.lifetime = .keepAlways;add(w04)
    tap("독립 백업 시작")
    XCTAssertTrue(app.staticTexts.containing(NSPredicate(format:"label CONTAINS %@", "복원 시험을 마치고")).firstMatch.waitForExistence(timeout:10))
    XCTAssertFalse(app.buttons["내 결제"].exists)
    XCTAssertFalse(app.buttons["테스트 시장 학습"].exists)
    XCTAssertFalse(app.buttons["여행지 찾기"].exists)
    XCTAssertTrue(app.buttons["개인 기기 찾기"].isHittable)
    let w05=XCTAttachment(screenshot:app.screenshot());w05.name="W05 backup entry";w05.lifetime = .keepAlways;add(w05)
    tap("등록 상태로 돌아가기")
    XCTAssertTrue(app.buttons["내 결제"].exists)
    tap("원래 보드에서 분실한 휴대전화 교체")
    XCTAssertTrue(app.staticTexts.containing(NSPredicate(format:"label CONTAINS %@", "독립 복구 장치")).firstMatch.waitForExistence(timeout:10))
    XCTAssertFalse(app.buttons["내 결제"].exists)
    XCTAssertTrue(app.buttons["개인 기기 찾기"].isHittable)
    let w06=XCTAttachment(screenshot:app.screenshot());w06.name="W06 replacement entry";w06.lifetime = .keepAlways;add(w06)
    tap("등록 상태로 돌아가기")
    tap("휴대전화 분실 및 지갑 복구")
    XCTAssertTrue(app.staticTexts.containing(NSPredicate(format:"label CONTAINS %@", "복구 기기가 기존 지갑 주소")).firstMatch.waitForExistence(timeout:10))
    XCTAssertFalse(app.buttons["내 결제"].exists)
    XCTAssertTrue(app.buttons["등록 상태로 돌아가기"].exists)
    let recovery=XCTAttachment(screenshot:app.screenshot());recovery.name="W06 board recovery entry";recovery.lifetime = .keepAlways;add(recovery)
  }
}

final class OwnerRecoveryChooserSmoke: XCTestCase {
  private let bundle = "com.nu54renter.w04qa"

  private func tap(_ app: XCUIApplication, _ label: String, file: StaticString = #filePath, line: UInt = #line) {
    let button = app.buttons[label].firstMatch
    XCTAssertTrue(button.waitForExistence(timeout:15), "missing button: \(label)", file:file, line:line)
    for _ in 0..<12 where !button.isHittable { app.swipeUp() }
    XCTAssertTrue(button.isHittable && button.isEnabled,"unavailable button: \(label)",file:file,line:line)
    button.tap()
  }

  private func text(_ app: XCUIApplication, _ label: String, file: StaticString = #filePath, line: UInt = #line) {
    let text = app.staticTexts.containing(NSPredicate(format:"label CONTAINS %@",label)).firstMatch
    XCTAssertTrue(text.waitForExistence(timeout:15),"missing text: \(label)",file:file,line:line)
  }

  func testFreshProfileShowsAllRecoveryChoices() {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: bundle)
    app.launch()
    tap(app,"w06-choose-recovery")
    text(app,"어떤 기기를 교체하나요?")
    tap(app,"w06-choice-phone")
    text(app,"원래 보드")
    XCTAssertTrue(app.textViews["w06-board-enrollment-document"].exists)
    XCTAssertFalse(app.textViews["w06-trust-document"].exists)
    XCTAssertFalse(app.buttons["개인 기기 찾기"].isEnabled)
    tap(app,"w06-recovery-back")
    tap(app,"w06-choice-board")
    text(app,"새 보드와 검증된 백업")
    for field in ["original", "new", "verifier"] {
      XCTAssertTrue(app.textFields["w06-\(field)-fingerprint"].exists)
    }
    XCTAssertTrue(app.textViews["w06-trust-document"].exists)
    XCTAssertFalse(app.buttons["새 기기 찾기"].isEnabled)
    tap(app,"w06-recovery-back")
    tap(app,"w06-choice-cloud")
    text(app,"이 설치에서는 클라우드 휴대전화 복구를 사용할 수 없습니다.")
    XCTAssertFalse(app.buttons["기존 클라우드 계정 인증"].exists)
    tap(app,"w06-recovery-back")
    tap(app,"w06-recovery-cancel")
    text(app,"지갑 시작하기")
  }

  func testSettingsReturnsWithOriginalWalletAndCloudEntry() {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: bundle)
    app.launch()
    tap(app,"설정")
    tap(app,"분실 복구")
    tap(app,"w06-choice-phone")
    text(app,"원래 보드")
    XCTAssertTrue(app.buttons["개인 기기 찾기"].isEnabled)
    XCTAssertFalse(app.textViews["w06-trust-document"].exists)
    tap(app,"w06-recovery-back")
    tap(app,"w06-choice-board")
    text(app,"새 보드와 검증된 백업")
    XCTAssertTrue(app.textViews["w06-board-enrollment-document"].exists)
    tap(app,"w06-recovery-back")
    tap(app,"w06-choice-cloud")
    text(app,"복구 접수는 인증 뒤 별도로 선택합니다.")
    tap(app,"기존 클라우드 계정 인증")
    text(app,"이미 연결한 계정")
    XCTAssertTrue(app.buttons["클라우드 서비스에 별도 로그인"].exists)
    XCTAssertFalse(app.buttons["클라우드 지갑 생성"].exists)
    XCTAssertFalse(app.buttons["분실 휴대전화 교체 접수 · 서명 동결"].exists)
    tap(app,"복구 안내로 돌아가기")
    tap(app,"w06-recovery-back")
    tap(app,"w06-recovery-cancel")
    text(app,"기기와 미결 요청")
    app.terminate()
    app.launch()
    text(app,"독립 설치 기록")
    tap(app,"원래 보드에서 분실한 휴대전화 교체")
    tap(app,"등록 상태로 돌아가기")
    tap(app,"휴대전화 분실 및 지갑 복구")
    XCTAssertTrue(app.textViews["w06-trust-document"].exists)
    tap(app,"등록 상태로 돌아가기")
  }
}

final class OwnerRegistrationSmoke: XCTestCase {
  func testRegistrationCommitWithoutReplyResumesAfterRestart() {
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    let ownerHelp = app.staticTexts.containing(NSPredicate(format:"label CONTAINS %@", "독립 설치 기록")).firstMatch
    continueAfterFailure = false
    let enabled = NSPredicate(format: "exists == true && enabled == true")
    func launchAndFindDevice() throws {
      app.launch()
      XCTAssertTrue(ownerHelp.waitForExistence(timeout: 20))
      try tap("개인 기기 찾기")
      for label in ["NU54 W04 QA · 검색 후보 1 · -other", "NU54 W04 QA · 검색 후보 2 · n-host"] {
        let candidate = app.staticTexts.matching(NSPredicate(format:"label == %@", label)).firstMatch
        XCTAssertTrue(candidate.waitForExistence(timeout: 10), "Missing \(label)")
      }
      XCTAssertEqual(app.buttons.matching(identifier: "기기 등록 시작").count, 2)
    }
    func tap(_ label: String, candidate: Int? = nil) throws {
      let target = candidate.map { app.buttons.matching(identifier: label).element(boundBy: $0) } ?? app.buttons[label]
      for _ in 0..<10 where !target.isHittable { app.swipeUp() }
      try XCTUnwrap(target.exists ? target : nil, "Missing \(label)")
      expectation(for: enabled, evaluatedWith: target)
      waitForExpectations(timeout: 10)
      target.tap()
    }
    func textContaining(_ label: String) -> XCUIElement {
      app.staticTexts.containing(NSPredicate(format:"label CONTAINS %@", label)).firstMatch
    }

    do {
      try launchAndFindDevice()
      try tap("기기 등록 시작", candidate: 0)
      XCTAssertTrue(textContaining("기기 확인 결과를 받지 못했습니다").waitForExistence(timeout: 20))
      XCTAssertFalse(textContaining("소유자 세대 1").exists)
      app.terminate()

      for _ in 0..<2 {
        try launchAndFindDevice()
        try tap("기기 등록 시작", candidate: 1)
        XCTAssertTrue(textContaining("기기 확인 결과를 받지 못했습니다").waitForExistence(timeout: 20))
        XCTAssertFalse(textContaining("소유자 세대 1").exists)
        app.terminate()
      }

      try launchAndFindDevice()
      try tap("등록 상태 확인", candidate: 1)
      XCTAssertTrue(textContaining("이 휴대전화의 소유자 등록을 기기에서 확인했습니다").waitForExistence(timeout: 15))
      XCTAssertTrue(textContaining("소유자 세대 1").exists)
      XCTAssertFalse(app.buttons["다른 체인 받기"].isEnabled)
      app.terminate()

      try launchAndFindDevice()
      try tap("기기 등록 시작", candidate: 1)
      XCTAssertTrue(textContaining("이 휴대전화의 소유자 등록을 기기에서 확인했습니다").waitForExistence(timeout: 15))
      XCTAssertTrue(textContaining("소유자 세대 1").exists)
      let resumed = XCTAttachment(screenshot: app.screenshot())
      resumed.name = "W04 registration resumed at generation 1"
      resumed.lifetime = .keepAlways
      add(resumed)
    } catch {
      XCTFail(String(describing: error))
    }
  }
}

final class OwnerBackupSmoke: XCTestCase {
  func testIndependentBackupUnlocksOnlyTheRestoredAddress() throws {
    try exerciseBackup(expectRefusal: false)
  }

  func testAuthenticatedBackupRefusalCanRetrySameOriginal() throws {
    try exerciseBackup(expectRefusal: true)
  }

  private func exerciseBackup(expectRefusal: Bool) throws {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    func text(_ label: String) -> XCUIElement {
      app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", label)).firstMatch
    }
    func tap(_ label: String, candidate: Int? = nil) throws {
      let target = candidate.map { app.buttons.matching(identifier: label).element(boundBy: $0) } ?? app.buttons[label]
      XCTAssertTrue(target.waitForExistence(timeout: 15), "Missing \(label)")
      for _ in 0..<12 where !target.isHittable {
        if target.frame.maxY < app.frame.minY { app.swipeDown() } else { app.swipeUp() }
      }
      expectation(for: NSPredicate(format: "enabled == true"), evaluatedWith: target)
      waitForExpectations(timeout: 10)
      target.tap()
    }

    app.launch()
    XCTAssertTrue(text("독립 설치 기록").waitForExistence(timeout: 20))
    try tap("개인 기기 찾기")
    try tap("기기 등록 시작", candidate: 1)
    XCTAssertTrue(text("이 휴대전화의 소유자 등록을 기기에서 확인했습니다").waitForExistence(timeout: 15))
    XCTAssertFalse(app.buttons["EVM 수신"].isEnabled)
    XCTAssertFalse(app.buttons["다른 체인 보내기"].isEnabled)

    if expectRefusal {
      try tap("독립 백업 시작")
      try tap("개인 기기 찾기")
      try tap("원본 기기에 백업 요청", candidate: 1)
      XCTAssertTrue(text("이 기기에서 독립 백업 요청을 열 수 없습니다").waitForExistence(timeout: 15))
      XCTAssertTrue(text("입금 안내가 잠겨 있습니다").exists)
      XCTAssertTrue(text("기존 활성 슬롯 — 보존").exists)
      XCTAssertTrue(text("대조 결과 — 미확인").exists)
      XCTAssertFalse(text("백업 요청 번호").exists)
      XCTAssertFalse(app.buttons["EVM 수신"].exists)
      XCTAssertFalse(app.buttons["개인 전송"].exists)
      XCTAssertEqual(app.textFields.count + app.secureTextFields.count + app.textViews.count, 0)
      try tap("다시 확인")
      XCTAssertFalse(text("이 기기에서 독립 백업 요청을 열 수 없습니다").exists)
      XCTAssertTrue(app.buttons.matching(identifier: "원본 기기에 백업 요청").element(boundBy: 1).exists)
      try tap("등록 상태로 돌아가기")
      app.terminate()
      app.launch()
      XCTAssertTrue(app.buttons["EVM 수신"].waitForExistence(timeout: 15))
      XCTAssertFalse(app.buttons["EVM 수신"].isEnabled)
      XCTAssertFalse(app.buttons["개인 전송"].isEnabled)
    }

    func startBackup() throws {
      try tap("독립 백업 시작")
      try tap("개인 기기 찾기")
      try tap("원본 기기에 백업 요청", candidate: 1)
      XCTAssertTrue(text("백업 요청 번호").waitForExistence(timeout: 15))
    }
    try startBackup()
    try tap("등록 상태로 돌아가기")
    XCTAssertFalse(app.buttons["EVM 수신"].isEnabled)
    try startBackup()
    try tap("원본 기기의 백업 상태 확인")
    XCTAssertTrue(text("독립 복구 확인을 기다리고 있습니다").waitForExistence(timeout: 15))
    XCTAssertTrue(text("입금 안내가 잠겨 있습니다").exists)
    XCTAssertTrue(text("기존 활성 슬롯 — 보존").exists)
    XCTAssertTrue(text("대조 결과 — 미확인").exists)
    XCTAssertTrue(text("보관한 기준 주소와 파생 경로").exists)
    XCTAssertFalse(app.buttons["EVM 수신"].exists)
    XCTAssertFalse(app.buttons["개인 전송"].exists)
    XCTAssertEqual(app.textFields.count + app.secureTextFields.count + app.textViews.count, 0)
    let ready = text("원본 기기가 독립 복원 시험 완료를 확인했습니다")
    for _ in 0..<6 {
      if ready.waitForExistence(timeout: 2) { break }
      try tap("원본 기기의 백업 상태 확인")
    }
    XCTAssertTrue(ready.waitForExistence(timeout: 3))

    let address = app.staticTexts.matching(NSPredicate(format:"label MATCHES %@", "^0x[0-9a-f]{40}$")).firstMatch
    XCTAssertTrue(address.waitForExistence(timeout: 10))
    let exactAddress = address.label
    XCTAssertTrue(app.buttons["EVM 수신"].isEnabled)
    let firstAddress = XCTAttachment(string: exactAddress)
    firstAddress.name = "First independently restored receiving address"
    firstAddress.lifetime = .keepAlways
    add(firstAddress)
    app.terminate()

    app.launch()
    try tap("개인 기기 찾기")
    try tap("등록 상태 확인", candidate: 1)
    XCTAssertTrue(address.waitForExistence(timeout: 15))
    XCTAssertEqual(address.label, exactAddress)
    XCTAssertTrue(app.buttons["EVM 수신"].isEnabled)
    let resumed = XCTAttachment(screenshot: app.screenshot())
    resumed.name = "W05 first receiving address after restart"
    resumed.lifetime = .keepAlways
    add(resumed)
  }
}

final class OwnerReplacementSmoke: XCTestCase {
  private let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")

  private func text(_ label: String) -> XCUIElement {
    app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", label)).firstMatch
  }

  private func tap(_ label: String, candidate: Int? = nil) {
    let target = candidate.map { app.buttons.matching(identifier: label).element(boundBy: $0) } ?? app.buttons[label]
    XCTAssertTrue(target.waitForExistence(timeout: 15), "Missing \(label)")
    for _ in 0..<15 where !target.isHittable {
      if target.frame.maxY < app.frame.minY { app.swipeDown() } else { app.swipeUp() }
    }
    expectation(for: NSPredicate(format: "enabled == true"), evaluatedWith: target)
    waitForExpectations(timeout: 10)
    target.tap()
  }

  func testPhoneReplacementResumesAtNewGenerationWithSameAddress() {
    continueAfterFailure = false
    app.launch()
    XCTAssertTrue(text("독립 설치 기록").waitForExistence(timeout: 20))
    XCTAssertFalse(app.buttons["EVM 수신"].isEnabled)
    tap("원래 보드에서 분실한 휴대전화 교체")

    let address = app.staticTexts.matching(NSPredicate(format:"label MATCHES %@", "^0x[0-9a-f]{40}$")).firstMatch
    func provePhone(expectedAddress: String? = nil) -> String {
      tap("개인 기기 찾기")
      tap("분실한 휴대전화 교체 요청", candidate: 1)
      XCTAssertTrue(text("독립 복구 장치에 전달할 요청").waitForExistence(timeout: 15))
      XCTAssertTrue(address.waitForExistence(timeout: 10))
      let original = address.label
      if let expectedAddress { XCTAssertEqual(original, expectedAddress) }
      tap("새 휴대전화 키 증명")
      XCTAssertTrue(text("결과가 불확실합니다").waitForExistence(timeout: 30))
      return original
    }

    let original = provePhone()
    app.terminate()
    app.launch()
    tap("개인 기기 찾기")
    tap("교체 후 보드 상태 확인", candidate: 1)
    XCTAssertTrue(text("보드에 기존 소유자가 남아 있습니다").waitForExistence(timeout: 15))
    _ = provePhone(expectedAddress: original)

    app.terminate()
    app.launch()
    tap("개인 기기 찾기")
    tap("교체 후 보드 상태 확인", candidate: 1)
    XCTAssertTrue(text("새 소유자 세대와 그대로 유지된 지갑 주소를 확인했습니다").waitForExistence(timeout: 15))
    XCTAssertTrue(text("소유자 세대 2").exists)
    XCTAssertTrue(address.waitForExistence(timeout: 10))
    XCTAssertEqual(address.label, original)
    XCTAssertTrue(app.buttons["EVM 수신"].isEnabled)
    let retained = XCTAttachment(string: original)
    retained.name = "W06 unchanged address at new generation"
    retained.lifetime = .keepAlways
    add(retained)
  }

  func testRetiredPhoneCannotReadActiveOwnerStatus() {
    continueAfterFailure = false
    app.launch()
    XCTAssertTrue(text("독립 설치 기록").waitForExistence(timeout: 20))
    tap("개인 기기 찾기")
    tap("등록 상태 확인", candidate: 1)
    XCTAssertTrue(text("기기 확인 결과를 받지 못했습니다").waitForExistence(timeout: 15))
    XCTAssertFalse(app.buttons["EVM 수신"].isEnabled)
  }
}

#if NU54_BOARD_RESTORE_TICKET
final class OwnerBoardRestoreSmoke: XCTestCase {
  func testNewBoardRequiresBoundReceiptAndPreservesAReadyRoot() {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    func text(_ label: String) -> XCUIElement {
      app.staticTexts.matching(NSPredicate(format:"label CONTAINS %@", label)).firstMatch
    }
    func makeHittable(_ element: XCUIElement) {
      XCTAssertTrue(element.waitForExistence(timeout: 15))
      for _ in 0..<15 where !element.isHittable {
        if element.frame.maxY < app.frame.minY { app.swipeDown() } else { app.swipeUp() }
      }
    }
    func tap(_ label: String, candidate: Int? = nil) {
      let target = candidate.map { app.buttons.matching(identifier: label).element(boundBy: $0) } ?? app.buttons[label]
      makeHittable(target)
      expectation(for: NSPredicate(format:"enabled == true"), evaluatedWith: target)
      waitForExpectations(timeout: 10)
      target.tap()
    }
    func paste(_ field: XCUIElement, _ value: String) {
      makeHittable(field)
      field.tap()
      let clipboard = UIPasteboard.general
      clipboard.setItems([["public.utf8-plain-text": value]], options: [
        .localOnly: true, .expirationDate: Date().addingTimeInterval(30)
      ])
      let change = clipboard.changeCount
      defer { if clipboard.changeCount == change { clipboard.items = [] } }
      field.press(forDuration: 1.2)
      let pasteAction = app.descendants(matching: .any).matching(
        NSPredicate(format:"label IN %@", ["Paste", "붙여넣기"])).firstMatch
      XCTAssertTrue(pasteAction.waitForExistence(timeout: 5))
      pasteAction.tap()
      let allow = app.alerts.buttons.matching(
        NSPredicate(format:"label IN %@", ["Allow Paste", "붙여넣기 허용"])).firstMatch
      if allow.waitForExistence(timeout: 2) { allow.tap() }
    }

    app.launch()
    XCTAssertTrue(text("독립 설치 기록").waitForExistence(timeout: 20))
    tap("개인 기기 찾기")
    tap("기기 등록 시작", candidate: 0)
    XCTAssertTrue(text("이 휴대전화의 소유자 등록을 기기에서 확인했습니다").waitForExistence(timeout: 15))
    XCTAssertFalse(app.buttons["EVM 수신"].isEnabled)

    tap("휴대전화 분실 및 지갑 복구")
    for (id,pin) in zip(["w06-original-fingerprint","w06-new-fingerprint","w06-verifier-fingerprint"], w06TrustPins) {
      let field=app.textFields[id]
      makeHittable(field)
      field.tap()
      field.typeText(pin)
      XCTAssertTrue((field.value as? String)==pin,"The fingerprint field must retain all digits")
      tap("w06-dismiss-keyboard")
    }
    let document=app.textViews["w06-trust-document"]
    XCTAssertTrue(document.waitForExistence(timeout: 15))
    XCTAssertEqual(document.label,"독립 복구 검증자 등록서")
    paste(document,w06TrustDocument)
    XCTAssertTrue((document.value as? String)==w06TrustDocument,"The verifier document must be pasted completely")
    tap("w06-dismiss-keyboard")
    let hidden=XCTNSPredicateExpectation(predicate:NSPredicate(format:"exists == false"),object:app.keyboards.firstMatch)
    XCTAssertEqual(XCTWaiter.wait(for:[hidden],timeout:6),.completed)
    tap("새 기기 찾기")
    XCTAssertTrue(app.buttons["w06-recover-candidate-1"].exists)
    tap("w06-recover-candidate-2")
    XCTAssertTrue(text("선택한 후보가 등록된 복구 대상인지 확인하지 못했습니다").waitForExistence(timeout: 15))
    XCTAssertFalse(text("복구 요청 번호").exists)
    XCTAssertFalse(app.buttons["w06-authenticated-restore"].exists)
    print("W06_WRONG_CANDIDATE_BLOCKED")
    tap("w06-recover-candidate-1")
    XCTAssertTrue(text("인증된 복구 대상").waitForExistence(timeout: 10))
    XCTAssertFalse(text("복구 요청 번호").exists)
    tap("w06-authenticated-restore")
    XCTAssertTrue(text("복구 요청 번호").waitForExistence(timeout: 15))
    let request=app.staticTexts.matching(NSPredicate(format:"label MATCHES %@", "^[0-9a-f]{32}$")).firstMatch
    XCTAssertTrue(request.waitForExistence(timeout: 10))
    let originalRequest=request.label
    tap("복구 증거 확인")
    XCTAssertTrue(text("기기에서 아직 복구 증거를 받지 못했습니다").waitForExistence(timeout: 10))
    XCTAssertFalse(app.buttons["EVM 수신"].exists)

    let bad = text("복구 결과를 확인할 수 없습니다")
    for _ in 0..<6 {
      if bad.waitForExistence(timeout: 2) { break }
      tap("복구 증거 확인")
    }
    XCTAssertTrue(bad.exists)
    XCTAssertTrue(app.staticTexts[originalRequest].exists)
    XCTAssertFalse(app.buttons["EVM 수신"].exists)

    tap("복구 증거 확인")
    XCTAssertTrue(text("복구된 주소와 계정을 확인했습니다").waitForExistence(timeout: 15))
    XCTAssertTrue(text("소유자 세대 1").exists)
    let address = app.staticTexts.matching(NSPredicate(format:"label == %@", w06ExpectedAddress)).firstMatch
    XCTAssertTrue(address.waitForExistence(timeout: 10))
    XCTAssertTrue(app.buttons["EVM 수신"].isEnabled)

    tap("휴대전화 분실 및 지갑 복구")
    tap("새 기기 찾기")
    tap("w06-recover-candidate-1")
    XCTAssertTrue(text("인증된 복구 대상").waitForExistence(timeout: 10))
    tap("w06-authenticated-restore")
    XCTAssertTrue(text("이 기기는 독립 복구 요청을 제공하지 않습니다").waitForExistence(timeout: 15))
    tap("등록 상태로 돌아가기")
    XCTAssertTrue(app.buttons["EVM 수신"].isEnabled)
    let restored = XCTAttachment(string: w06ExpectedAddress)
    restored.name = "W06 new board address matches original"
    restored.lifetime = .keepAlways
    add(restored)
  }
}
#endif

final class OwnerToolsSmoke: XCTestCase {
  func testToolsUseAuthenticatedBoardAndExplicitCancel() {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    app.launch()

    func tap(_ button: XCUIElement) {
      XCTAssertTrue(button.waitForExistence(timeout: 15))
      for _ in 0..<10 where !button.isHittable {
        if button.frame.maxY < app.frame.minY { app.swipeDown() } else { app.swipeUp() }
      }
      XCTAssertTrue(button.isEnabled && button.isHittable)
      button.tap()
    }

    tap(app.buttons["설정"])
    tap(app.buttons["기기 상태 확인"])
    tap(app.buttons["기기 부가기능"])
    XCTAssertFalse(app.buttons["찾기 신호 요청"].isEnabled)
    tap(app.buttons["등록 기기 찾기"])
    let candidates = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "인증된 기기 상태 조회"))
    tap(candidates.element(boundBy: 1))
    XCTAssertTrue(app.staticTexts["찾기 · 보드 LED"].waitForExistence(timeout: 15))
    XCTAssertTrue(app.staticTexts["보드 오디오 전송 미지원"].exists)
    tap(app.buttons["찾기 신호 요청"])
    let cancel = app.buttons["찾기 신호 취소"]
    expectation(for: NSPredicate(format: "enabled == true"), evaluatedWith: cancel)
    waitForExpectations(timeout: 4)
    tap(cancel)
    expectation(for: NSPredicate(format: "enabled == false"), evaluatedWith: cancel)
    waitForExpectations(timeout: 10)
    let phoneSpeech = app.buttons["휴대전화로 상태 듣기"]
    tap(phoneSpeech)
    // A rejected native voice request clears the authenticated board and
    // disables this control. A successful submission keeps it available.
    expectation(for: NSPredicate(format: "enabled == true"), evaluatedWith: phoneSpeech)
    waitForExpectations(timeout: 10)
    XCTAssertTrue(app.staticTexts["보드 오디오 전송 미지원"].exists)
    XCTAssertTrue(app.buttons["찾기 신호 요청"].isEnabled)

  }
}

private struct InstalledPolicyControls {
  let app: XCUIApplication
  let test: XCTestCase

  func reveal(_ element: XCUIElement) {
    XCTAssertTrue(element.waitForExistence(timeout: 15), app.debugDescription)
    for _ in 0..<12 {
      let viewport = app.scrollViews.firstMatch.frame
      let keyboard = app.keyboards.firstMatch
      let bottom = keyboard.exists ? min(viewport.maxY, keyboard.frame.minY) : viewport.maxY
      let top = viewport.minY
      let center = element.frame.midY
      if center > top + 20 && center < bottom - 20 && element.isHittable { return }
      let middle = (top + bottom) / 2
      let distance = max(-0.60 * (bottom - top), min(0.60 * (bottom - top), middle - center))
      let startY = middle - distance / 2
      let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: startY / app.frame.height))
      let end = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: (startY + distance) / app.frame.height))
      start.press(forDuration: 0.05, thenDragTo: end, withVelocity: .slow, thenHoldForDuration: 0.15)
    }
    XCTFail("Control is outside the visible scroll area: " + element.debugDescription)
  }
  func tap(_ name: String) {
    let button = app.buttons[name].firstMatch
    reveal(button)
    let enabled = NSPredicate(format: "enabled == true")
    test.expectation(for: enabled, evaluatedWith: button)
    test.waitForExpectations(timeout: 10)
    button.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
  }
  func text(_ name: String) {
    let label = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", name)).firstMatch
    XCTAssertTrue(label.waitForExistence(timeout: 15), app.debugDescription)
  }
  func handoffIndependentDisplay(_ operation: String) throws {
    let code = app.staticTexts["owner-display-request-code"]
    XCTAssertFalse(code.exists)
    tap("독립 표시 요청 정보 보기")
    XCTAssertTrue(code.waitForExistence(timeout: 8))
    XCTAssertTrue(app.images["독립 표시용 일회용 QR"].exists)
    let binding = code.label
    XCTAssertEqual(binding.split(separator: "/").first.map(String.init), operation)
    let path = try XCTUnwrap(ProcessInfo.processInfo.environment["NU54_W27_DISPLAY_TEST_INPUT"])
    try binding.write(toFile: path, atomically: true, encoding: .utf8)
  }
  func readFromSettings() {
    tap("설정")
    tap("한도와 잠금")
    text("등록 기기 선택")
    let candidates = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "등록 기기 인증"))
    let board = candidates.element(boundBy: 1)
    reveal(board)
    board.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
  }
}

final class OwnerPolicySmoke: XCTestCase {
  func testPolicyUsesIndependentApprovalAndDeviceTime() throws {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    app.launch()

    let policy = InstalledPolicyControls(app: app, test: self)
    let tap = policy.tap
    let text = policy.text
    let reveal = policy.reveal
    let readFromSettings = policy.readFromSettings
    func review12() {
      let input = app.textFields["새 건별 한도"]
      reveal(input)
      XCTAssertEqual(input.value as? String, "10")
      input.tap()
      input.typeText(XCUIKeyboardKey.delete.rawValue + "2")
      if app.keyboards.firstMatch.exists {
        let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.55))
        let end = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.26))
        start.press(forDuration: 0.05, thenDragTo: end)
      }
      XCTAssertEqual(input.value as? String, "12")
      tap("변경 내용 검토")
      text("12 TestUSDC / 30 TestUSDC")
    }

    readFromSettings()
    text("자산: TestUSDC")
    text("건별 한도: 10 TestUSDC · 일별 한도: 30 TestUSDC")
    XCTAssertFalse(app.buttons["허용 수취인 편집"].exists)
    XCTAssertFalse(app.buttons["한도만 편집"].exists)
    text("자동 잠금 규칙 미확인")
    tap("기기 잠금 규칙 확인")
    text("기기가 확인한 자동 잠금 · 유휴 2분 또는 분리 즉시")
    review12()
    tap("변경 검토 취소")
    XCTAssertFalse(app.buttons["독립 표시로 변경 요청"].exists)
    review12()
    tap("독립 표시로 변경 요청")
    try policy.handoffIndependentDisplay("24")
    text("변경 결과를 확인하지 못했습니다.")
    tap("보호 정책 읽기")
    text("건별 한도: 10 TestUSDC · 일별 한도: 30 TestUSDC")
    XCTAssertFalse(app.staticTexts["보호 정책 변경 대기"].exists)

    review12()
    tap("독립 표시로 변경 요청")
    try policy.handoffIndependentDisplay("24")
    text("기기 기록을 다시 읽어 대기 상태를 확인하세요.")
    tap("보호 정책 읽기")
    text("건별 한도: 10 TestUSDC · 일별 한도: 30 TestUSDC")
    text("보호 정책 변경 대기")
    text("변경 대기: 12 TestUSDC / 30 TestUSDC")
    reveal(app.staticTexts["보호 정책 변경 대기"].firstMatch)
    let waiting = XCTAttachment(screenshot: app.screenshot())
    waiting.name = "W27 device pending 10 to 12"
    waiting.lifetime = .keepAlways
    add(waiting)
    tap("기기 상태 확인")
    tap("등록 기기 찾기")
    let status = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "인증된 기기 상태 조회")).element(boundBy: 1)
    reveal(status)
    status.tap()
    text("인증된 소유 세대")
    tap("보호 정책으로 돌아가기")
    text("보호 정책 변경 대기")
    text("건별 한도: 10 TestUSDC · 일별 한도: 30 TestUSDC")
    tap("대기 완료 확인")
    text("변경 적용을 확인하지 못했습니다.")
    text("건별 한도: 10 TestUSDC · 일별 한도: 30 TestUSDC")

    app.terminate()
    app.launch()
    readFromSettings()
    text("보호 정책 변경 대기")
    tap("대기 완료 확인")
    text("변경 적용을 확인하지 못했습니다.")
    text("건별 한도: 10 TestUSDC · 일별 한도: 30 TestUSDC")
    tap("대기 완료 확인")
    text("기기에서 새 활성 정책을 확인했습니다.")
    text("건별 한도: 12 TestUSDC · 일별 한도: 30 TestUSDC")
    XCTAssertFalse(app.staticTexts["보호 정책 변경 대기"].exists)
    let applied = XCTAttachment(screenshot: app.screenshot())
    applied.name = "W27 device active 12 and 30"
    applied.lifetime = .keepAlways
    add(applied)
    tap("설정으로 돌아가기")
    text("기기와 미결 요청")
    text("인증된 활성 보드 끝자리")
    text("기기가 확인한 자동 잠금 · 유휴 2분 또는 분리 즉시")
    let settledSettings = XCTAttachment(screenshot: app.screenshot())
    settledSettings.name = "W27 Settings with authenticated lock rule"
    settledSettings.lifetime = .keepAlways
    add(settledSettings)
    tap("기기 잠금 규칙 확인")
    text("기기가 확인한 자동 잠금 · 유휴 2분 또는 분리 즉시")
    XCTAssertFalse(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "기기에서 새 활성 정책을 확인했습니다")).firstMatch.exists)

    app.terminate()
    app.launch()
    readFromSettings()
    text("건별 한도: 12 TestUSDC · 일별 한도: 30 TestUSDC")
  }

  func testRecipientSetPersistsAcrossLostReply() throws {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    let policy = InstalledPolicyControls(app: app, test: self)
    app.launch()
    policy.readFromSettings()
    policy.text("활성 수취인")
    policy.text("정책 버전 2")
    let address: (Int) -> String = { "0x" + String(repeating: String(format: "%02x", $0), count: 20) }
    policy.text(address(5))
    policy.text(address(6))

    func reviewTarget(checkLimit: Bool) {
      policy.tap("허용 수취인 편집")
      for number in [1, 2, 3, 4, 7, 8] {
        policy.tap(String(format: "주소록에서 추가 QA-%02d", number))
      }
      if checkLimit {
        policy.tap("주소록에서 추가 QA-09")
        policy.text("서로 다른 유효 주소를 최대 8개 입력하세요.")
      }
      policy.tap("제거 " + address(6))
      policy.tap("주소록에서 추가 QA-09")
      policy.tap("수취인 변경 검토")
    }

    policy.tap("허용 수취인 편집")
    policy.tap("제거 " + address(6))
    policy.tap("수취인 변경 검토")
    policy.tap("편집 취소")
    XCTAssertFalse(app.buttons["수취인 변경 독립 표시 요청"].exists)
    policy.text(address(6))

    reviewTarget(checkLimit: true)
    policy.tap("수취인 변경 독립 표시 요청")
    try policy.handoffIndependentDisplay("33")
    policy.text("변경 결과를 확인하지 못했습니다.")
    policy.tap("보호 정책 읽기")
    policy.text(address(6))
    XCTAssertFalse(app.staticTexts["보호 정책 변경 대기"].exists)

    reviewTarget(checkLimit: false)
    policy.tap("수취인 변경 독립 표시 요청")
    try policy.handoffIndependentDisplay("33")
    policy.text("변경 결과를 확인하지 못했습니다.")
    app.terminate()
    app.launch()
    policy.readFromSettings()
    policy.text("보호 정책 변경 대기")
    for number in [1, 2, 3, 4, 5, 7, 8, 9] { policy.text(address(number)) }
    policy.text(address(6))
    verifyActivation(policy, lostReply: false)
    for number in [1, 2, 3, 4, 5, 7, 8, 9] { policy.text(address(number)) }
    XCTAssertFalse(app.staticTexts[address(6)].exists)
    policy.text("건별 한도: 10 TestUSDC · 일별 한도: 30 TestUSDC")
  }

  func testCapEditKeepsEightRecipientsAndReservation() throws {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    let policy = InstalledPolicyControls(app: app, test: self)
    app.launch()
    policy.readFromSettings()
    let addresses = (1...8).map { "0x" + String(repeating: String(format: "%02x", $0), count: 20) }
    addresses.forEach(policy.text)
    policy.text("예약된 금액: 5 TestUSDC")
    policy.tap("한도만 편집")
    let limit = app.textFields["새 건별 한도"]
    policy.reveal(limit)
    XCTAssertEqual(limit.value as? String, "10")
    limit.tap()
    limit.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 2) + "8")
    if app.keyboards.firstMatch.exists {
      let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.55))
      let end = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.26))
      start.press(forDuration: 0.05, thenDragTo: end)
    }
    XCTAssertEqual(limit.value as? String, "8")
    policy.tap("한도 변경 검토")
    policy.tap("한도 변경 독립 표시 요청")
    try policy.handoffIndependentDisplay("34")
    policy.text("기기 기록을 다시 읽어 대기 상태를 확인하세요.")
    policy.tap("보호 정책 읽기")
    policy.text("변경 대기: 8 TestUSDC / 30 TestUSDC")
    policy.text("예약된 금액: 5 TestUSDC")
    verifyActivation(policy, lostReply: true)
    addresses.forEach(policy.text)
    policy.text("건별 한도: 8 TestUSDC · 일별 한도: 30 TestUSDC")
    app.terminate()
    app.launch()
    policy.readFromSettings()
    addresses.forEach(policy.text)
    policy.text("예약된 금액: 5 TestUSDC")
    policy.text("건별 한도: 8 TestUSDC · 일별 한도: 30 TestUSDC")
  }

  private func verifyActivation(_ policy: InstalledPolicyControls, lostReply: Bool) {
    for _ in 0..<2 {
      policy.tap("대기 완료 확인")
      policy.text("변경 적용을 확인하지 못했습니다.")
      policy.text("보호 정책 변경 대기")
    }
    policy.tap("대기 완료 확인")
    if lostReply {
      policy.text("변경 적용을 확인하지 못했습니다.")
      policy.tap("보호 정책 읽기")
    }
    policy.text("기기에서 새 활성 정책을 확인했습니다")
    policy.text("정책 버전 3")
    XCTAssertFalse(policy.app.staticTexts["보호 정책 변경 대기"].exists)
  }

}

final class OwnerSettingsRoutesSmoke: XCTestCase {
  func testSettingsOpensRecoverySignInNetworksAndTravelConsent() {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    app.launch()

    func reveal(_ element: XCUIElement) {
      XCTAssertTrue(element.waitForExistence(timeout: 15), app.debugDescription)
      for _ in 0..<12 {
        let viewport = app.scrollViews.firstMatch.frame
        let keyboard = app.keyboards.firstMatch
        let bottom = keyboard.exists ? min(viewport.maxY, keyboard.frame.minY) : viewport.maxY
        let top = viewport.minY
        let center = element.frame.midY
        if center > top + 20 && center < bottom - 20 && element.isHittable { return }
        let middle = (top + bottom) / 2
        let distance = max(-0.60 * (bottom - top), min(0.60 * (bottom - top), middle - center))
        let startY = middle - distance / 2
        let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: startY / app.frame.height))
        let end = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: (startY + distance) / app.frame.height))
        start.press(forDuration: 0.05, thenDragTo: end, withVelocity: .slow, thenHoldForDuration: 0.15)
      }
      XCTFail("Control is outside the visible scroll area: " + element.debugDescription)
    }
    func tap(_ name: String) {
      let button = app.buttons[name].firstMatch
      reveal(button)
      let enabled = NSPredicate(format: "enabled == true")
      expectation(for: enabled, evaluatedWith: button)
      waitForExpectations(timeout: 10)
      button.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
    }
    func text(_ name: String) {
      let label = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", name)).firstMatch
      XCTAssertTrue(label.waitForExistence(timeout: 15), app.debugDescription)
    }
    tap("설정")
    text("등록 기기 상태 미확인")
    text("자동 잠금 규칙 미확인")
    tap("접근성")
    text("표시와 알림을 설정하세요")
    text("화면 읽기")
    let access = XCTAttachment(screenshot: app.screenshot())
    access.name = "W27 Accessibility"
    access.lifetime = .keepAlways
    add(access)
    tap("설정으로 돌아가기")
    tap("도움말·진단")
    tap("진단 항목 검토")
    text("공유할 진단 미리보기")
    let preview = XCTAttachment(screenshot: app.screenshot())
    preview.name = "W27 Diagnostic preview"
    preview.lifetime = .keepAlways
    add(preview)
    tap("공유 취소")
    tap("설정으로 돌아가기")
    tap("기기·업데이트")
    text("개인 보드 유지관리")
    tap("설정으로 돌아가기")
    tap("분실 복구")
    tap("분실한 보드 복원")
    XCTAssertTrue(app.textViews["독립 복구 검증자 등록서"].waitForExistence(timeout: 15))
    tap("복구 방법으로 돌아가기")
    tap("취소")

    tap("인증수단")
    text("내 인증수단")
    text("개인 보드는 소셜 가입 없이 사용할 수 있습니다.")
    tap("설정으로 돌아가기")

    tap("네트워크")
    text("6개 체인 · 읽기 전용")
    tap("등록 상태로 돌아가기")
    tap("설정")

    tap("개인정보와 여행 동의")
    text("여행 기록과 동의")
    tap("설정으로 돌아가기")
    text("기기와 미결 요청")

    tap("연습 다시 보기")
    text("이 연습은 실제 돈을 보내거나 서명하지 않습니다.")
    tap("나중에 이어하기")
    tap("설정")
    tap("미결 결제")
    text("내 결제와 미결 청구")
  }
}

final class OwnerFirmwareSmoke: XCTestCase {
  func testRunningVersionIsReadFromSignedImage() throws {
    verifyRunningAndCandidate(checkCandidate:false)
  }

  #if NU54_CANDIDATE_FIXTURE
  func testCandidateIsVerifiedForTheCurrentBoard() throws {
    verifyRunningAndCandidate(checkCandidate:true)
  }
  #endif

  private func verifyRunningAndCandidate(checkCandidate: Bool) {
    continueAfterFailure = false
    let app = XCUIApplication(bundleIdentifier: "com.nu54renter.w04qa")
    func text(_ part: String) -> XCUIElement {
      app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", part)).firstMatch
    }
    func tap(_ label: String, candidate: Int? = nil, line: UInt = #line) {
      let target = candidate.map { app.buttons.matching(identifier: label).element(boundBy: $0) } ?? app.buttons[label]
      XCTAssertTrue(target.waitForExistence(timeout: 15), "Missing action", line: line)
      for _ in 0..<12 where !target.isHittable {
        let start = target.frame.maxY < app.frame.minY ? 0.42 : 0.8
        let end = target.frame.maxY < app.frame.minY ? 0.8 : 0.42
        app.coordinate(withNormalizedOffset: CGVector(dx:0.5,dy:start)).press(forDuration:0.03,
          thenDragTo:app.coordinate(withNormalizedOffset: CGVector(dx:0.5,dy:end)))
      }
      XCTAssertTrue(target.isHittable && target.isEnabled, "Action unavailable", line: line)
      target.tap()
    }

    app.launch()
    XCTAssertTrue(text("독립 설치 기록").waitForExistence(timeout: 20))
    tap("개인 기기 찾기")
    tap("기기 등록 시작",candidate:1)
    XCTAssertTrue(text("이 휴대전화의 소유자 등록을 기기에서 확인했습니다").waitForExistence(timeout: 15))
    tap("설정")
    tap("기기·업데이트")
    tap("펌웨어 업데이트 준비")
    XCTAssertTrue(text("현재 실행 버전 · 확인 불가").exists)
    tap("등록 기기 찾기")
    tap("인증된 기기 상태 조회 · NU54 W04 QA",candidate:1)
    XCTAssertTrue(text("현재 실행 버전 · 2.3.258+65538").waitForExistence(timeout:20))
    XCTAssertTrue(text("실행 이미지 보안 카운터 · 512").exists)
    XCTAssertTrue(text("서명 후보 · 확인 불가").exists)
    XCTAssertTrue(text("기기 전원 · 확인 불가").exists)
    XCTAssertTrue(text("후보와 기기 전원을 확인하기 전까지 업데이트 준비를 보류합니다.").exists)
    XCTAssertTrue(text("설치 후 캠페인 증거").exists)
    XCTAssertFalse(app.buttons["이 기기 적용 증거 읽기"].isEnabled)
    guard checkCandidate else { return }

    func chooseCandidate(_ name: String, line: UInt = #line) {
      tap("서명된 후보 확인서 선택",line:line)
      func button(_ english: String, _ korean: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format:"label IN %@",[english,korean])).firstMatch
      }
      XCTAssertTrue(button("Cancel","취소").waitForExistence(timeout:12),"Document picker did not open",line:line)
      let file = app.cells.matching(NSPredicate(format:"label BEGINSWITH %@",name)).firstMatch
      if !file.waitForExistence(timeout:3) {
        let browse = button("Browse","둘러보기")
        if browse.exists { browse.tap() }
        let storage = app.cells.matching(NSPredicate(format:"label IN %@",["On My iPhone","나의 iPhone"])).firstMatch
        if storage.waitForExistence(timeout:5) { storage.tap() }
        let folder = app.staticTexts["NU54 Wallet"]
        if folder.waitForExistence(timeout:5) { folder.tap() }
      }
      XCTAssertTrue(file.waitForExistence(timeout:10),"Candidate file not visible",line:line)
      XCTAssertTrue(file.isHittable,"Candidate file not selectable",line:line)
      file.tap()
    }

    chooseCandidate("w12-valid-candidate")
    XCTAssertTrue(text("서명 후보 · 2.4.0 · custom · 보안 카운터 513").waitForExistence(timeout:20))
    XCTAssertTrue(text("현재 실행 버전 · 2.3.258+65538").exists)
    XCTAssertTrue(text("기기 전원 · 확인 불가").exists)
    XCTAssertTrue(text("업데이트 적용 권한 · 보류").exists)
    tap("후보 선택 취소")
    XCTAssertTrue(text("서명 후보 · 확인 불가").exists)
    chooseCandidate("w12-invalid-candidate")
    XCTAssertTrue(text("서명 후보가 현재 기기 및 배포 정책과 일치하지 않습니다.").waitForExistence(timeout:20))
    XCTAssertTrue(text("서명 후보 · 확인 불가").exists)
    XCTAssertFalse(app.buttons["이 기기 적용 증거 읽기"].isEnabled)

  }
}
