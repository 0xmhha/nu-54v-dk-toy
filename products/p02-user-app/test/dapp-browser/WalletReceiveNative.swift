import CoreImage
import UIKit
import XCTest

final class WalletReceiveNativeJourney: XCTestCase {
  private let app = XCUIApplication(bundleIdentifier: "com.nu54renter.wallet.receiveqa")

  func testVerifyAndDecodeAllSixReceiveProfiles() throws {
    try verify(onlyXrpl:false)
  }

  func testVerifyXrplWithExplicitZeroTag() throws {
    try verify(onlyXrpl:true)
  }

  private func verify(onlyXrpl:Bool) throws {
    continueAfterFailure = false
    app.launch()
    let select = app.buttons["지갑 프로필 선택"]
    if select.waitForExistence(timeout: 4) { select.tap() }
    XCTAssertTrue(app.buttons["계정 및 자산 선택"].waitForExistence(timeout: 12))
    app.scrollViews.firstMatch.swipeDown()
    app.scrollViews.firstMatch.swipeDown()
    tap("계정 및 자산 선택")
    let profiles = [
      ("StableNet Testnet", "ethereum:", "@8283"),
      ("Ethereum Sepolia", "ethereum:", "@11155111"),
      ("Base Sepolia", "ethereum:", "@84532"),
      ("Bitcoin Signet", "bitcoin:tb1", "Bitcoin%20Signet"),
      ("Solana Devnet", "Solana Devnet", "Asset: SOL"),
      ("XRP Ledger Testnet", "XRP Ledger Testnet", "DestinationTag: 0"),
    ]
    for (index, profile) in profiles.enumerated() where !onlyXrpl || index == 5 {
      if index != 0 {
        goUp()
        tap(profile.0)
        tap("조회 네트워크 전환 동의")
      }
      if index == 5 {
        let tag = app.textFields["목적지 태그"]
        seek(tag)
        tag.tap()
        tag.typeText("0")
        let done = app.buttons["태그 입력 완료"]
        XCTAssertTrue(done.waitForExistence(timeout: 6), "Numeric tag keyboard has no dismiss action")
        done.tap()
        tap("수신 태그 저장")
      }
      let qr = app.images["수신 QR 코드"]
      XCTAssertFalse(qr.exists, "Previous address cannot enable the next profile's QR")
      let scan = app.buttons["개인 기기 찾기"]
      seek(scan)
      if !app.buttons["보드와 수신 주소 확인 · Loopback P01 demo board"].exists { scan.tap() }
      tap("보드와 수신 주소 확인 · Loopback P01 demo board")
      seek(qr)
      let payload = try decodeVisibleQR(qr)
      XCTAssertTrue(payload.hasPrefix(profile.1), "QR network/encoding mismatch")
      XCTAssertTrue(payload.contains(profile.2), "QR omitted required network or tag")
      if index == 5 {
        XCTAssertTrue(payload.contains("X-address: ") && payload.contains("Asset: XRP"),"XRPL receive details incomplete")
      }
      let address = displayAddress(from: payload)
      XCTAssertFalse(address.isEmpty, "QR has no receive address")
      XCTAssertTrue(app.staticTexts.allElementsBoundByIndex.contains { $0.label.contains("Address: " + address) },
        "Printed address disagrees with the native QR")
      tap("받기 정보 공유")
      let copy = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@ OR label == %@", "Copy", "복사")).firstMatch
      XCTAssertTrue(copy.waitForExistence(timeout: 12), "System share Copy action did not open")
      if index == 5 {
        let sheetClose=app.buttons.matching(NSPredicate(format:"label IN %@",["Close","닫기","Cancel","취소"])).firstMatch
        if sheetClose.exists && sheetClose.isHittable { sheetClose.tap() }
        else {
          app.coordinate(withNormalizedOffset:CGVector(dx:0.50,dy:0.10)).tap()
          if copy.exists {
            let start=app.coordinate(withNormalizedOffset:CGVector(dx:0.5,dy:0.47))
            let finish=app.coordinate(withNormalizedOffset:CGVector(dx:0.5,dy:0.94))
            start.press(forDuration:0.08,thenDragTo:finish)
          }
        }
        let dismissed=XCTNSPredicateExpectation(predicate:NSPredicate(format:"exists == false"),object:copy)
        XCTAssertEqual(XCTWaiter.wait(for:[dismissed],timeout:5),.completed)
        XCTAssertTrue(qr.waitForExistence(timeout:5))
        XCTAssertEqual(try decodeVisibleQR(qr),payload)
        tap("받기 정보 공유")
        XCTAssertTrue(copy.waitForExistence(timeout:5))
      }
      copy.tap()
      defer { UIPasteboard.general.string = nil }
      XCTAssertTrue(qr.waitForExistence(timeout: 6), "Receive screen did not return after local share action")
    }
  }

  private func decodeVisibleQR(_ qr: XCUIElement) throws -> String {
    let detector = try XCTUnwrap(CIDetector(ofType: CIDetectorTypeQRCode,
      context: CIContext(options: [.useSoftwareRenderer: true]), options: [CIDetectorAccuracy: CIDetectorAccuracyHigh]))
    let scroll = app.scrollViews.firstMatch
    for attempt in 0..<8 {
      if let image = qr.screenshot().image.cgImage {
        let messages = detector.features(in: CIImage(cgImage:image)).compactMap { ($0 as? CIQRCodeFeature)?.messageString }
        if messages.count == 1, let message = messages.first { return message }
      }
      if attempt == 1 || attempt == 4 {
        let direction: CGFloat = qr.frame.midY > scroll.frame.midY ? -0.15 : 0.15
        let start = scroll.coordinate(withNormalizedOffset: CGVector(dx:0.83,dy:0.55))
        let end = scroll.coordinate(withNormalizedOffset: CGVector(dx:0.83,dy:0.55+direction))
        start.press(forDuration:0.05,thenDragTo:end)
      }
      Thread.sleep(forTimeInterval:0.3)
    }
    throw NSError(domain:"W07 QR pixels were unavailable",code:1)
  }

  private func displayAddress(from payload: String) -> String {
    if payload.hasPrefix("ethereum:") { return String(payload.dropFirst(9).prefix { $0 != "@" }) }
    if payload.hasPrefix("bitcoin:") { return String(payload.dropFirst(8).prefix { $0 != "?" }) }
    return payload.split(separator:"\n").first(where: { $0.hasPrefix("Address: ") })
      .map { String($0.dropFirst(9)) } ?? ""
  }

  private func goUp() {
    for _ in 0..<8 { app.scrollViews.firstMatch.swipeDown() }
  }

  private func seek(_ element: XCUIElement) {
    for _ in 0..<14 {
      if element.exists && element.isHittable { return }
      app.scrollViews.firstMatch.swipeUp()
    }
    if !(element.exists && element.isHittable) {
      let life=app.staticTexts.allElementsBoundByIndex.map { $0.label }.filter {
        $0.contains("못") || $0.contains("확인") || $0.contains("네트워크") || $0.contains("Devnet")
      }
      print("W07 visible selection evidence", life)
    }
    XCTAssertTrue(element.exists && element.isHittable, "Unavailable normal receive screen element")
  }

  private func tap(_ label: String) {
    let button = app.buttons[label]
    seek(button)
    XCTAssertTrue(button.isEnabled, "Receive action disabled")
    if label.hasPrefix("보드와 수신 주소 확인") {
      let scroll = app.scrollViews.firstMatch
      for _ in 0..<5 {
        let screen = app.frame
        let buttonY = button.frame.midY
        if buttonY > screen.minY + screen.height * 0.18 && buttonY < screen.minY + screen.height * 0.82 { break }
        let start=scroll.coordinate(withNormalizedOffset: CGVector(dx:0.72, dy:0.55))
        let to=scroll.coordinate(withNormalizedOffset: CGVector(dx:0.72, dy:buttonY > screen.midY ? 0.37 : 0.73))
        start.press(forDuration:0.08,thenDragTo:to)
      }
    }
    button.tap()
  }
}
