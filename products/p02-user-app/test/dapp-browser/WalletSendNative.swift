import UIKit
import XCTest

final class WalletSendNativeJourney: XCTestCase {
  private let app = XCUIApplication(bundleIdentifier: "com.nu54renter.wallet.sendqa")
  private var endpoint = URL(string: "https://127.0.0.1:18449/case")!

  func testOrdinaryEvmDeclineApproveAndResume() throws {
    continueAfterFailure = false
    let initial = try state()
    let chain = try XCTUnwrap(initial["chain"] as? Int)
    let recipient = try XCTUnwrap(initial["recipient"] as? String)
    start()
    input("수취인", recipient)
    input("전송 금액", "9")
    tap("전송 내용 검토")
    assertText { $0.contains("Chain ID \(chain) · Nonce") }
    if chain == 84532 { assertText { $0.contains("Base L1 + operator 1000") } }
    tap("거절하고 초안으로")
    assertText { $0.contains("서명 0건 · 제출 0건") }
    XCTAssertEqual(try state()["boardCalls"] as? Int, 0)
    tap("전송 내용 검토")
    tap("개인 기기 찾기")
    tap("기기에 전송 승인 요청")
    tap("이 네트워크에 전송")
    var reference: String?
    for _ in 0..<20 {
      reference = try state()["receipt"] as? String
      if reference != nil { break }
      Thread.sleep(forTimeInterval: 0.3)
    }
    let tx = try XCTUnwrap(reference)
    app.terminate()
    start()
    assertText { $0 == tx }
    tap("전송 상태 재조회")
    assertText { $0.hasPrefix("선택한 RPC가 확정 결과를 보고했습니다.") }
    XCTAssertEqual(try state()["boardCalls"] as? Int, 1)
  }

  func testLocalChainDeclineApproveAndResume() throws {
    continueAfterFailure = false
    let fixture = try state()
    let chain = try XCTUnwrap(fixture["chain"] as? Int)
    let name = try XCTUnwrap(fixture["name"] as? String)
    let commissioned = fixture["commissioned"] as? Bool == true
    start("다른 체인 보내기")
    if commissioned {
      tap("등록된 시험망 확인")
    } else {
      if chain != 1 { tap(name) }
      replace("로컬 HTTPS RPC 주소", "https://127.0.0.1:18449/chain-node", clear:true)
      replace("네트워크 식별자", try XCTUnwrap(fixture["anchor"] as? String))
      tap("로컬 네트워크 확인")
    }
    replace("수취인 주소", try XCTUnwrap(fixture["destination"] as? String))
    replace("보낼 금액", try XCTUnwrap(fixture["amount"] as? String), numeric:true)
    if chain == 3 && !commissioned { replace("목적지 태그","0",numeric:true) }
    tap("개인 기기 찾기")
    tap("거래 원문 검토 · Loopback P01 demo board")
    assertText { $0 == "검증된 거래 원문" }
    let expected = (["Input count", "Last valid block height", "DestinationTag · 0"])[chain-1]
    assertText { $0.contains(expected) }
    tap("거절하고 초안으로")
    XCTAssertEqual(try state()["signatures"] as? Int,0)
    XCTAssertEqual(try state()["submissions"] as? Int,0)
    if !app.buttons["거래 원문 검토 · Loopback P01 demo board"].exists { tap("개인 기기 찾기") }
    tap("거래 원문 검토 · Loopback P01 demo board")
    assertText { $0 == "검증된 거래 원문" }
    let approval = app.buttons["기기에 전송 승인 요청 · Loopback P01 demo board"]
    seek(approval)
    let scroll = app.scrollViews.firstMatch
    if approval.frame.midY > scroll.frame.minY + scroll.frame.height * 0.73 {
      scroll.coordinate(withNormalizedOffset:CGVector(dx:0.55,dy:0.82))
        .press(forDuration:0.1, thenDragTo:scroll.coordinate(withNormalizedOffset:CGVector(dx:0.55,dy:0.55)))
    }
    seek(approval)
    approval.tap()
    let submit=app.buttons["이 네트워크에 전송"]
    if !submit.waitForExistence(timeout:12) {
      let approvedText = ["보드에서 승인된 전송을 확인하지 못했습니다.","보드에서 전송을 거절했습니다.",
        "승인된 원문과 거래 ID를 저장했습니다.","검증된 거래 원문", "다시 검토하세요."]
      let displayed = app.staticTexts.allElementsBoundByIndex.map(\.label)
      XCTFail("Post-sign state: " + approvedText.filter { displayed.contains($0) }.joined(separator:","))
      return
    }
    tap("이 네트워크에 전송")
    let holdFinality = fixture["holdFinality"] as? Bool == true
    if holdFinality {
      var receipt: String?
      for _ in 0..<50 {
        receipt = try state()["receipt"] as? String
        if receipt != nil {break}
        Thread.sleep(forTimeInterval:0.2)
      }
      app.terminate()
      start("다른 체인 보내기")
      XCTAssertTrue(app.staticTexts[try XCTUnwrap(receipt)].waitForExistence(timeout:10))
      tap("전송 상태 재조회")
      let pending=app.staticTexts.matching(NSPredicate(format:"label IN %@",
        ["아직 네트워크에서 확인되지 않습니다.","블록에 포함되었습니다."])).firstMatch
      XCTAssertTrue(pending.waitForExistence(timeout:12),"Original transaction did not render its nonterminal state")
      XCTAssertEqual(try state()["pendingObserved"] as? Bool,true)
      XCTAssertFalse(app.buttons["이 네트워크에 전송"].exists)
    }
    var finalized: [String:Any] = [:]
    for _ in 0..<100 {
      finalized = try state()
      if finalized["finalized"] as? Bool == true { break }
      Thread.sleep(forTimeInterval:0.35)
    }
    XCTAssertEqual(finalized["finalized"] as? Bool,true)
    let id = try XCTUnwrap(finalized["receipt"] as? String)
    if !holdFinality {
      app.terminate()
      start("다른 체인 보내기")
    }
    assertText { $0 == id }
    tap("전송 상태 재조회")
    assertText { $0 == "선택한 네트워크에서 확정되었습니다." }
    let end = try state()
    XCTAssertEqual(end["signatures"] as? Int,1)
    XCTAssertEqual(end["submissions"] as? Int,1)
    XCTAssertEqual(end["requeries"] as? Int,holdFinality ? 2 : 1)
  }

  func testInstalledWalletRefusesUnsupportedStakeProgram() throws {
    continueAfterFailure=false
    let fixture=try state()
    XCTAssertEqual(fixture["chain"] as? Int,2)
    XCTAssertEqual(fixture["commissioned"] as? Bool,true)
    start("다른 체인 보내기")
    tap("등록된 시험망 확인")
    replace("수취인 주소",try XCTUnwrap(fixture["destination"] as? String))
    replace("보낼 금액",try XCTUnwrap(fixture["amount"] as? String),numeric:true)
    tap("개인 기기 찾기")
    tap("거래 원문 검토 · Loopback P01 demo board")
    assertText(stage:"W26 staking refusal") {$0 == "스테이킹 요청을 거절했습니다. 서명 0건 · 제출 0건"}
    let observed=try state()
    XCTAssertEqual(observed["stakingRefusal"] as? Bool,true)
    XCTAssertEqual(observed["signatures"] as? Int,0)
    XCTAssertEqual(observed["submissions"] as? Int,0)
    XCTAssertFalse(app.staticTexts["검증된 거래 원문"].exists)
    XCTAssertFalse(app.buttons["이 네트워크에 전송"].exists)
    app.terminate();start("다른 체인 보내기")
    XCTAssertFalse(app.buttons["전송 상태 재조회"].exists)
  }

  func testHistoricalSignetBranchesAndAccountNameRecovery() throws {
    continueAfterFailure=false
    endpoint=URL(string:"https://127.0.0.1:18525/case")!
    let fixture=try state()
    let initial=try XCTUnwrap(fixture["initial"] as? String)
    let next=try XCTUnwrap(fixture["next"] as? String)
    let head=try XCTUnwrap(fixture["head"] as? String)
    start("6개 체인 · 읽기 전용")
    XCTAssertFalse(app.buttons["숨긴 계정 선택 · Other generation · 4"].exists)
    tap("숨긴 계정 선택 · Previous archive · 4")
    assertText(stage:"W25 derivation account") {$0.contains("계정 4 · 파생 인덱스 3")}
    XCTAssertFalse(app.buttons["숨긴 계정 표시"].isEnabled)
    tap("개인 기기 찾기")
    tap("보드와 수신 주소 확인 · Loopback P01 demo board")
    tap("숨긴 계정 표시")
    assertText(stage:"W25 original receiving address") {$0.contains("Address: "+initial)}
    tap("사용한 Bitcoin 주소 재발견 · Loopback P01 demo board")
    let finished=app.staticTexts["사용한 주소와 파생경로 확인"]
    XCTAssertTrue(finished.waitForExistence(timeout:255),"W25 bounded discovery did not finish")
    assertText(stage:"W25 receive branch") {$0.contains("m/84'/1'/3'/0/*") && $0.contains("마지막 사용 인덱스 3")}
    assertText(stage:"W25 change branch") {$0.contains("m/84'/1'/3'/1/*") && $0.contains("마지막 사용 인덱스 5")}
    assertText(stage:"W25 common confirmed tip") {$0.contains(head)}
    assertText(stage:"W25 discovery did not change address") {$0.contains("Address: "+initial)}
    XCTAssertEqual(try state()["readCount"] as? Int,50)
    XCTAssertEqual(try state()["exports"] as? Int,1)
    tap("다음 수신 주소 선택 · 4")
    verifyDiscoveryBoard()
    assertText(stage:"W25 explicit next address") {$0.contains("Address: "+next)}
    replace("계정 이름","Restored archive")
    XCTAssertEqual(app.textFields["계정 이름"].value as? String,"Restored archive","W25 draft edit missing")
    if !app.buttons["검증한 계정 이름 저장"].isEnabled {
      verifyDiscoveryBoard()
    }
    tap("검증한 계정 이름 저장")
    assertText(stage:"W25 changed alias") {$0=="Restored archive"}
    tap("계정 이름 삭제")
    app.terminate();start("6개 체인 · 읽기 전용")
    XCTAssertFalse(app.buttons["계정 이름 삭제"].exists)
    XCTAssertFalse(app.staticTexts["Restored archive"].exists)
    assertText(stage:"W25 retained index after restart") {$0.contains("현재 수신 주소 인덱스 4")}
    verifyDiscoveryBoard()
    assertText(stage:"W25 same next address after restart") {$0.contains("Address: "+next)}
    let end=try state()
    XCTAssertEqual(end["signatures"] as? Int,0)
    XCTAssertEqual(end["boardCalls"] as? Int,0)
    XCTAssertEqual(end["submissions"] as? Int,0)
  }

  func testSignetWatchConsentDismissAndRestore() throws {
    continueAfterFailure = false
    start("다른 체인 받기")
    tap("개인 기기 찾기")
    let entry = "읽기 전용 Bitcoin 계정 내보내기 · Loopback P01 demo board"
    tap(entry)
    assertText { $0.contains("Bitcoin Signet · Native SegWit") }
    tap("취소")
    XCTAssertEqual(try state()["exports"] as? Int,0)
    tap(entry)
    tap("공개 범위 동의")
    assertText { $0.contains("서명 기능 없음") }
    assertText { $0.contains("wpkh(") && $0.contains("/0/*)") && $0.contains("/1/*)") }
    XCTAssertFalse(app.buttons["보내기"].exists)
    XCTAssertEqual(try state()["exports"] as? Int,1)
    XCTAssertEqual(try state()["publicReplies"] as? Int,2)
    tap("공개 디스크립터 공유")
    let copy=app.descendants(matching:.any).matching(NSPredicate(format:"label IN %@",["Copy","복사"])).firstMatch
    XCTAssertTrue(copy.waitForExistence(timeout:12),"System watch export share sheet unavailable")
    let close=app.buttons.matching(NSPredicate(format:"label IN %@",["Close","닫기","Cancel","취소"])).firstMatch
    if close.exists && close.isHittable {close.tap()}
    else {
      app.coordinate(withNormalizedOffset:CGVector(dx:0.50,dy:0.10)).tap()
      if copy.exists {
        app.coordinate(withNormalizedOffset:CGVector(dx:0.5,dy:0.47))
          .press(forDuration:0.08,thenDragTo:app.coordinate(withNormalizedOffset:CGVector(dx:0.5,dy:0.94)))
      }
    }
    let gone=XCTNSPredicateExpectation(predicate:NSPredicate(format:"exists == false"),object:copy)
    XCTAssertEqual(XCTWaiter.wait(for:[gone],timeout:6),.completed)
    app.terminate()
    start("다른 체인 받기")
    tap("저장된 읽기 전용 Bitcoin 계정")
    assertText { $0.contains("wpkh(") && $0.contains("/0/*)") && $0.contains("/1/*)") }
    XCTAssertFalse(app.buttons["보내기"].exists)
    XCTAssertEqual(try state()["exports"] as? Int,1)
  }

  func testSignetMultisigReviewExportAndRestore() throws {
    continueAfterFailure = false
    let stateBefore=try state()
    start("다른 체인 보내기")
    tap("Bitcoin 공동서명")
    tap("개인 기기 찾기")
    tap("공동서명 지갑 확인 · Loopback P01 demo board")
    let policy="보드에 공동서명 정책 등록"
    seek(app.buttons[policy])
    XCTAssertFalse(app.buttons["보드에 부분 서명 요청"].exists)
    XCTAssertEqual(try state()["signatures"] as? Int,0)
    tap(policy)
    if let document=stateBefore["document"] as? String {
      tap("공동서명 PSBT 가져오기")
      tap("PSBT 파일 또는 QR 이미지 선택")
      selectPsbtDocument(document)
      XCTAssertTrue(app.staticTexts["PSBT 파일을 불러왔습니다."].waitForExistence(timeout:8),
                     "W17 document decode state missing")
    } else {
      replace("수취인 주소",try XCTUnwrap(stateBefore["destination"] as? String))
      replace("보낼 금액",try XCTUnwrap(stateBefore["amount"] as? String),numeric:true)
    }
    tap("PSBT 원문 검토")
    if stateBefore["wrongChange"] as? Bool == true {
      assertText(stage:"W17 invalid change refusal") {$0 == "공동서명 정책이나 거래를 확인하지 못했습니다."}
      XCTAssertFalse(app.buttons["보드에 부분 서명 요청"].exists)
      XCTAssertEqual(try state()["signatures"] as? Int,0)
      return
    }
    assertText(stage:"W17 authenticated review") {$0.contains("거래 원문 검토 · ")}
    assertText {$0.contains("자기 거스름돈")}
    if stateBefore["bridge"] as? Bool == true {
      let details=try XCTUnwrap(stateBefore["reviewDetails"] as? [String:Any])
      let inputs=try XCTUnwrap(details["inputs"] as? [[String:Any]])
      let outputs=try XCTUnwrap(details["allOutputs"] as? [[String:Any]])
      XCTAssertEqual(inputs.count,stateBefore["inputs"] as? Int)
      for input in inputs {
        let outpoint=try XCTUnwrap(input["outpoint"] as? String)
        let amount=try XCTUnwrap(input["satoshis"] as? Int)
        let script=try XCTUnwrap(input["scriptPubKey"] as? String)
        assertText {$0.contains(outpoint) && $0.contains("\(amount) sat") && $0.contains(script)}
      }
      let change=try XCTUnwrap(outputs.first { $0["changePathClaim"] as? Bool == true })
      let changeScript=try XCTUnwrap(change["scriptPubKey"] as? String)
      let changeAmount=try XCTUnwrap(change["satoshis"] as? Int)
      assertText {$0.contains("자기 거스름돈") && $0.contains(changeScript) &&
        $0.contains("\(changeAmount) sat") && $0.contains("m/48'/1'/0'/2'/1/0")}
      for output in outputs where output["changePathClaim"] as? Bool == false {
        let script=try XCTUnwrap(output["scriptPubKey"] as? String)
        let amount=try XCTUnwrap(output["satoshis"] as? Int)
        let destination=try XCTUnwrap(output["address"] as? String)
        assertText {$0.contains("외부 출력") && $0.contains(script) &&
          $0.contains("\(amount) sat") && $0.contains(destination)}
      }
      let inputTotal=try XCTUnwrap(details["inputTotal"] as? Int)
      let outputTotal=try XCTUnwrap(details["outputTotal"] as? Int)
      let fee=try XCTUnwrap(details["feeSatoshis"] as? Int)
      let rate=try XCTUnwrap(details["minimumFeeRateSatPerVbyte"] as? String)
      let vsize=try XCTUnwrap(details["maximumSignedVbytes"] as? Int)
      assertText {$0.contains("입력 합계 \(inputTotal) sat") && $0.contains("출력 합계 \(outputTotal) sat")}
      assertText {$0.contains("수수료 \(fee) sat") && $0.contains("\(rate) sat/vB")}
      assertText {$0.contains("최대 서명 거래 크기 \(vsize) vB")}
    }
    if stateBefore["inputs"] as? Int == 2 {
      let inputs=app.staticTexts.matching(NSPredicate(format:"label BEGINSWITH %@","입력 UTXO"))
      XCTAssertEqual(Set(inputs.allElementsBoundByIndex.map(\.label)).count,2)
    }
    tap("거절하고 초안으로")
    XCTAssertEqual(try state()["signatures"] as? Int,0)
    if stateBefore["reviewOnly"] as? Bool == true { return }
    tap("PSBT 원문 검토")
    assertText {$0.contains("거래 원문 검토 · ")}
    tap("보드에 부분 서명 요청")
    let partial=app.staticTexts.matching(NSPredicate(format:"label BEGINSWITH %@","서명 1개.")).firstMatch
    XCTAssertTrue(partial.waitForExistence(timeout:12),"Protected partial receipt did not appear")
    XCTAssertEqual(try state()["register"] as? Int,1)
    XCTAssertEqual(try state()["signatures"] as? Int,1)
    let expected=stateBefore["original"] as? String
    let identity=app.staticTexts.matching(expected.map {NSPredicate(format:"label == %@",$0)} ??
      NSPredicate(format:"label MATCHES %@","^[a-f0-9]{64}$")).firstMatch
    XCTAssertTrue(identity.waitForExistence(timeout:8),"Original transaction ID missing")
    let original=identity.label
    tap("서명된 PSBT 공유")
    let copy=app.descendants(matching:.any).matching(NSPredicate(format:"label IN %@",["Copy","복사"])).firstMatch
    XCTAssertTrue(copy.waitForExistence(timeout:12),"System PSBT share sheet unavailable")
    app.coordinate(withNormalizedOffset:CGVector(dx:0.50,dy:0.10)).tap()
    if copy.exists {
      app.coordinate(withNormalizedOffset:CGVector(dx:0.5,dy:0.47))
        .press(forDuration:0.08,thenDragTo:app.coordinate(withNormalizedOffset:CGVector(dx:0.5,dy:0.94)))
    }
    app.terminate()
    start("다른 체인 보내기")
    tap("Bitcoin 공동서명")
    XCTAssertTrue(app.staticTexts[original].waitForExistence(timeout:8))
    XCTAssertFalse(app.buttons["보드에 부분 서명 요청"].exists)
    XCTAssertEqual(try state()["signatures"] as? Int,1)
    if stateBefore["bridge"] as? Bool == true {
      tap("원 거래 상태 재조회")
      assertText {$0 == "원 거래가 확정되었습니다."}
      XCTAssertEqual(try state()["requeried"] as? Bool,true)
    }

  }

  func testSixRegisteredReviewRoutesAndAbsentAuthority() throws {
    continueAfterFailure = false
    let missing = try state()["missingXrp"] as? Bool == true
    start("6개 체인 · 읽기 전용")
    tap("체인별 기능표")
    let networks=[
      "StableNet Testnet", "Ethereum Sepolia", "Base Sepolia",
      "Bitcoin Signet", "Solana Devnet", "XRP Ledger Testnet"
    ]
    for name in networks {
      let entry=app.buttons["체인별 거래 원문 확인 · "+name]
      if missing && name=="XRP Ledger Testnet" {
        XCTAssertTrue(entry.exists)
        XCTAssertFalse(entry.isEnabled)
      } else {
        XCTAssertTrue(entry.exists)
        XCTAssertTrue(entry.isEnabled)
      }
    }
    tap("체인별 거래 원문 확인 · Ethereum Sepolia")
    seek(app.textFields["수취인"])
    tap("등록 상태로 돌아가기")
    tap("6개 체인 · 읽기 전용")
    tap("체인별 기능표")
    tap("체인별 거래 원문 확인 · Bitcoin Signet")
    seek(app.buttons["등록된 시험망 확인"])
    app.terminate()
    start("다른 체인 보내기")
    seek(app.buttons["등록된 시험망 확인"])
    assertText {$0.contains("bitcoin-signet")}
  }

  private func selectPsbtDocument(_ name:String) {
    let provider=app.descendants(matching:.any)
    let base=(name as NSString).deletingPathExtension
    let leaf=provider.matching(NSPredicate(format:"label BEGINSWITH %@",base)).firstMatch
    if !leaf.waitForExistence(timeout:3) {
      let browse=provider.matching(NSPredicate(format:"label IN %@",["Browse","둘러보기"])).firstMatch
      if browse.exists && browse.isHittable {browse.tap()}
      else {app.coordinate(withNormalizedOffset:CGVector(dx:0.72,dy:0.95)).tap()}
      let local=provider.matching(NSPredicate(format:"label IN %@",["On My iPhone","나의 iPhone"])).firstMatch
      if local.waitForExistence(timeout:3) && local.isHittable {local.tap()}
      let folder=app.cells.matching(NSPredicate(format:"label == %@ OR label BEGINSWITH %@","NU54 Wallet","NU54 Wallet,")).firstMatch
      if !leaf.exists && folder.waitForExistence(timeout:3) && folder.isHittable {folder.tap()}
    }
    if leaf.waitForExistence(timeout:8) {leaf.tap()}
    else if app.navigationBars["NU54 Wallet"].exists {
      app.coordinate(withNormalizedOffset:CGVector(dx:0.22,dy:0.28)).tap()
    } else {
      XCTFail("Local synthetic PSBT document missing")
    }
  }

  private func replace(_ label:String, _ text:String, clear:Bool=false, numeric:Bool=false) {
    let field=app.textFields[label]
    seek(field)
    field.tap()
    if clear, let previous=field.value as? String {
      field.typeText(String(repeating:XCUIKeyboardKey.delete.rawValue,count:previous.count))
    }
    field.typeText(text)
    if !numeric {
      field.typeText("\n")
    } else if app.keyboards.firstMatch.exists {
      let done=app.buttons["전송 입력 완료"]
      if done.exists && done.isHittable { done.tap() }
      else { app.coordinate(withNormalizedOffset:CGVector(dx:0.51,dy:0.33)).tap() }
    }
  }

  private func start(_ action:String="개인 전송") {
    app.launch()
    let select = app.buttons["지갑 프로필 선택"]
    if select.exists { select.tap() }
    tap(action)
  }

  private func assertText(stage:String="Expected ordinary-transfer review evidence", where predicate: (String) -> Bool) {
    for _ in 0..<10 {
      if app.staticTexts.allElementsBoundByIndex.contains(where: {predicate($0.label)}) { return }
      app.scrollViews.firstMatch.swipeUp()
    }
    XCTFail(stage)
  }

  private func input(_ label: String, _ text: String) {
    let field = app.textFields[label]
    seek(field)
    field.tap()
    field.typeText(text)
    if label == "수취인" {
      field.typeText("\n")
    } else if app.keyboards.firstMatch.exists {
      let header = app.staticTexts.matching(NSPredicate(format:"label BEGINSWITH %@", "0x")).firstMatch
      if header.exists && header.isHittable { header.tap() }
    }
  }

  private func verifyDiscoveryBoard() {
    let board = app.buttons["보드와 수신 주소 확인 · Loopback P01 demo board"]
    if !board.exists { tap("개인 기기 찾기") }
    let scroll = app.scrollViews.firstMatch
    for _ in 0..<36 {
      guard board.exists else { break }
      if board.isEnabled && board.isHittable {
        board.tap()
        return
      }
      let viewport = scroll.frame.intersection(app.frame)
      guard viewport.height > 0 else { break }
      let delta = board.frame.midY - viewport.midY
      print("W25_BOARD_VIEW centerDelta=\(Int(delta)) height=\(Int(viewport.height))")
      let distance = min(0.30, max(0.08, abs(delta) / viewport.height))
      let start = scroll.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
      let finish = scroll.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5 + (delta < 0 ? distance : -distance)))
      start.press(forDuration: 0.10, thenDragTo: finish, withVelocity: .slow, thenHoldForDuration: 0.20)
    }
    XCTFail("W25 board confirmation button did not become hittable")
  }

  private func seek(_ control: XCUIElement) {
    let scroll=app.scrollViews.firstMatch
    for attempt in 0..<18 {
      if control.exists && control.isHittable {
        if control.isEnabled { return }
        Thread.sleep(forTimeInterval:0.3)
        continue
      }
      if control.exists && !control.frame.isEmpty {
        if control.frame.midY < scroll.frame.minY { scroll.swipeDown() }
        else { scroll.swipeUp() }
      } else if (5..<11).contains(attempt) {
        scroll.swipeDown()
      } else { scroll.swipeUp() }
    }
    XCTAssertTrue(control.exists && control.isHittable && control.isEnabled, "Ordinary transfer control unavailable")
  }
  private func tap(_ label: String) {
    let button = app.buttons[label]
    seek(button)
    button.tap()
  }

  private func state() throws -> [String: Any] {
    let ready = expectation(description: "loopback evidence")
    var data = Data()
    URLSession.shared.dataTask(with: endpoint) { response, _, _ in
      data = response ?? Data()
      ready.fulfill()
    }.resume()
    wait(for:[ready], timeout:5)
    return try XCTUnwrap(try JSONSerialization.jsonObject(with:data) as? [String:Any])
  }
}
