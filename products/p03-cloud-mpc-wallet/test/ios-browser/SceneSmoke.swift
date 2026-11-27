import XCTest

final class SceneSmoke: XCTestCase {
    private struct Credential: Decodable {
        let username: String
        let password: String
    }

    private struct Account: Decodable {
        let login: Credential
        let approval: Credential
    }

    private struct Fixture: Decodable {
        let accounts: [Account]
        let recovery: Credential?
        let alternateLogin: Credential?
        let refusal: Bool?
        let signRefusal: Bool?
        let loginProvider: String?
        let originalReviewURL: URL?
        let partial: Bool?
        let resume: Bool?
        let originalCredential: String?
        let alternateCredential: String?
        let deployment: String?
        let venue: String?
        let expireMarketOrder: Bool?
    }

    private lazy var app = XCUIApplication(bundleIdentifier: ProcessInfo.processInfo.environment["NU54_P03_TEST_BUNDLE"] ?? "org.nu54.p03browser")
    private let safari = XCUIApplication(bundleIdentifier: "com.apple.mobilesafari")
    private var marketScenario = false

    private func tap(_ title: String) throws {
        let button = app.buttons[title].firstMatch
        XCTAssertTrue(button.waitForExistence(timeout: 12), "Missing app action: \(title)")
        let ready = XCTNSPredicateExpectation(predicate: NSPredicate(format: "enabled == true"), object: button)
        guard XCTWaiter.wait(for: [ready], timeout: 12) == .completed else {
            throw NSError(domain: "P03BrowserActionDisabled", code: 1, userInfo: [NSLocalizedDescriptionKey: title])
        }
        reveal(button)
        XCTAssertTrue(button.isHittable, "App action is not visible: \(title)")
        button.tap()
    }

    private func dismissPasswordSave() {
        for title in ["지금 안 함", "Not Now"] {
            let decline = safari.buttons[title].firstMatch
            if decline.exists && decline.isHittable { decline.tap() }
        }
    }

    private func consent(_ credential: Credential, market: Bool = false, refusing: Bool = false) throws {
        XCTAssertTrue(safari.wait(for: .runningForeground, timeout: 20), "Verification URL must open Safari")
        dismissPasswordSave()
        let web = safari.webViews.firstMatch
        if market {
            let terms = web.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Isolated StableNet lab 8283 · fx-base")).firstMatch
            XCTAssertTrue(terms.waitForExistence(timeout: 10), "Independent approval must show the pinned market and direction")
            XCTAssertTrue(web.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "minimum A 2000000")).firstMatch.exists)
            print("P03_STAGE market_consent_reviewed")
        }
        let identity = web.textFields.firstMatch
        if !identity.waitForExistence(timeout: 5) {
            for title in ["Continue", "Not Now", "Done"] {
                let onboarding = safari.buttons[title].firstMatch
                if onboarding.exists && onboarding.isHittable { onboarding.tap() }
            }
        }
        guard identity.waitForExistence(timeout: 15) else {
            XCTFail("The trusted issuer's sign-in form was not displayed")
            return
        }
        identity.tap()
        identity.typeText(credential.username)
        let password = web.secureTextFields.firstMatch
        XCTAssertTrue(password.exists)
        if marketScenario { liftAboveKeyboard(password, on: web) }
        password.tap()
        password.typeText(credential.password)
        let keyboard = safari.keyboards.firstMatch
        if keyboard.exists {
            let dismiss = ["Done", "완료", "Hide keyboard", "키보드 가리기"]
                .map { safari.buttons[$0].firstMatch }
                .first { $0.exists && $0.isHittable }
            try XCTUnwrap(dismiss, "Safari did not expose its keyboard dismissal control").tap()
        }
        let confirm = web.buttons[refusing ? "Deny" : "Confirm"]
        let visible = {
            let y = confirm.frame.midY
            return confirm.isHittable && y > web.frame.minY + 35 && y < web.frame.maxY - 65
        }
        for _ in 0..<6 where !visible() {
            let y = confirm.frame.midY
            if y > 0 && y < web.frame.minY + 35 { web.swipeDown() }
            else { web.swipeUp() }
        }
        guard visible() else { XCTFail("Safari did not expose the Confirm button"); return }
        confirm.tap()
        dismissPasswordSave()
        let result = web.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Confirmation recorded.")).firstMatch
        XCTAssertTrue(result.waitForExistence(timeout: 12), "The independent authority did not accept the form")
        app.activate()
    }

    private func liftAboveKeyboard(_ element: XCUIElement, on web: XCUIElement) {
        let keyboard = safari.keyboards.firstMatch
        for _ in 0..<3 where keyboard.exists && element.frame.midY >= keyboard.frame.minY - 100 {
            let from = web.coordinate(withNormalizedOffset: CGVector(dx: 0.75, dy: 0.43))
            let to = web.coordinate(withNormalizedOffset: CGVector(dx: 0.75, dy: 0.23))
            from.press(forDuration: 0.05, thenDragTo: to)
        }
    }

    private func reveal(_ element: XCUIElement) {
        for attempt in 0..<8 where !element.isHittable {
            let frame = app.frame
            let keyboard = app.keyboards.firstMatch
            let bottom = keyboard.exists ? min(keyboard.frame.minY, frame.maxY) : frame.maxY
            let visibleHeight = (bottom - frame.minY) / frame.height
            let topAnchor = 0.19
            let bottomAnchor = max(topAnchor + 0.12, min(0.72, visibleHeight - 0.09))
            let swipeUp = element.frame.isEmpty ? attempt < 4 : element.frame.midY > frame.minY + (bottom - frame.minY) / 2
            let from = app.coordinate(withNormalizedOffset: CGVector(dx: 0.65, dy: swipeUp ? bottomAnchor : topAnchor))
            let to = app.coordinate(withNormalizedOffset: CGVector(dx: 0.65, dy: swipeUp ? topAnchor : bottomAnchor))
            from.press(forDuration: 0.05, thenDragTo: to)
        }
    }

    private func enter(_ label: String, _ value: String, stage: Int = 0) throws {
        if stage != 0 { print("P03_TRANSFER_FIELD \(stage)") }
        let fields = app.textFields
        let field = fields.matching(NSPredicate(format: "label == %@ OR placeholderValue == %@", label, label)).firstMatch
        guard field.waitForExistence(timeout: 10) else {
            throw NSError(domain: "P03TransferFieldMissing", code: stage)
        }
        reveal(field)
        guard field.isHittable else {
            throw NSError(domain: "P03TransferFieldOutOfView", code: stage)
        }
        field.tap()
        field.typeText(value)
        if stage != 0 && app.keyboards.firstMatch.exists {
            let dismiss = app.buttons["키보드 닫기"].firstMatch
            XCTAssertTrue(dismiss.waitForExistence(timeout: 5))
            reveal(dismiss)
            // The scroll view's on-drag mode may already have dismissed it.
            if app.keyboards.firstMatch.exists {
                XCTAssertTrue(dismiss.isHittable)
                dismiss.tap()
            }
            let hidden = XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == false"), object: app.keyboards.firstMatch)
            XCTAssertEqual(XCTWaiter.wait(for: [hidden], timeout: 5), .completed)
        }
    }

    private func waitForReceipt(_ receipt: XCUIElement) throws {
        let deadline = Date().addingTimeInterval(110)
        while !receipt.exists && Date() < deadline {
            try tap("같은 요청 재조회")
            _ = receipt.waitForExistence(timeout: 2)
        }
        XCTAssertTrue(receipt.exists)
    }

    private func prepareSign(amount: String = "11") throws {
        try tap("Sepolia 전송 요청")
        try enter("받는 주소", "0x1111111111111111111111111111111111111111", stage: 1)
        try enter("전송액 (wei)", amount, stage: 2)
        try enter("거래 nonce", "0", stage: 3)
        try enter("최대 가스 단가 (wei)", "1000000000", stage: 4)
        try enter("우선 가스 단가 (wei)", "1", stage: 5)
        try tap("원문 검토")
        XCTAssertTrue(app.staticTexts["받는 주소 · 0x1111111111111111111111111111111111111111"].waitForExistence(timeout: 12))
        XCTAssertTrue(app.staticTexts["전송액 · \(amount) wei"].exists)
        XCTAssertTrue(app.staticTexts["최대 가스비 · 21000000000000 wei"].exists)
        XCTAssertFalse(app.staticTexts["검증된 서명 거래 · 체인 전송과 확정은 별도"].exists)
    }

    private func sign(_ credential: Credential, amount: String = "11") throws {
        try prepareSign(amount: amount)
        try tap("요청 원문 독립 승인")
        try consent(credential)
        try tap("독립 확인 결과 가져오기")
        try waitForReceipt(app.staticTexts["검증된 서명 거래 · 체인 전송과 확정은 별도"])
        try tap("요청 확인 마침")
    }

    private func login(_ credential: Credential) throws {
        if marketScenario { safari.terminate() }
        try tap("클라우드 서비스에 별도 로그인")
        try consent(credential)
        try tap("브라우저 로그인 확인")
    }

    private func create(_ account: Account) throws -> String {
        try login(account.login)
        try tap("이 계정을 클라우드 지갑에 연계")
        try tap("클라우드 지갑 생성")
        if marketScenario { safari.terminate() }
        try tap("요청 원문 독립 승인")
        try consent(account.approval)
        try tap("독립 확인 결과 가져오기")
        let address = app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "수신 주소 · 0x")).firstMatch
        try waitForReceipt(address)
        let label = address.label
        try tap("요청 확인 마침")
        return label
    }

    private func enroll(_ account: Account) throws -> String {
        let label = try create(account)
        try sign(account.approval)
        try tap("클라우드 로그아웃")
        XCTAssertFalse(app.staticTexts[label].exists)
        return label
    }

    private func loadFixture() throws -> Fixture {
        let encoded = try XCTUnwrap(ProcessInfo.processInfo.environment["NU54_P03_TEST_BROWSER"])
        let fixture = try JSONDecoder().decode(Fixture.self, from: XCTUnwrap(Data(base64Encoded: encoded)))
        XCTAssertEqual(fixture.accounts.count, 2)
        return fixture
    }

    func testSelectedMarketWalletUsesIndependentConsentAndKeepsOriginalHash() throws {
        continueAfterFailure = false
        let fixture = try loadFixture()
        let deployment = try XCTUnwrap(fixture.deployment)
        let venue = try XCTUnwrap(fixture.venue)
        marketScenario = true
        app.launch()
        try market(fixture.accounts[0], deployment: deployment, venue: venue,
            expireBeforeApproval: fixture.expireMarketOrder == true)
    }

    private func refuseSign(_ account: Account, originalReviewURL: URL) throws {
        _ = try create(account)
        try prepareSign()
        let request = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "요청 번호 · ")).firstMatch
        XCTAssertTrue(request.exists)
        let original = request.label
        XCTAssertTrue(app.staticTexts["이 원문의 세대 · 1"].exists)
        XCTAssertTrue(app.staticTexts["서명 참여 조합 · A + B"].exists)
        let loaded = expectation(description: "read original review from fixture ledger")
        var payload: Data?
        URLSession.shared.dataTask(with: originalReviewURL) { data, _, _ in payload = data; loaded.fulfill() }.resume()
        wait(for: [loaded], timeout: 8)
        let row = try JSONDecoder().decode([String: String].self, from: XCTUnwrap(payload))
        XCTAssertEqual(original, "요청 번호 · " + (try XCTUnwrap(row["request"])))
        XCTAssertTrue(app.staticTexts["이 원문의 지갑 · " + (try XCTUnwrap(row["wallet"]))].exists)
        XCTAssertEqual(row["chain"], "11155111")
        XCTAssertTrue(app.staticTexts["거래 체인 ID · " + (try XCTUnwrap(row["chain"]))].exists)
        XCTAssertTrue(app.staticTexts["만료 · " + (try XCTUnwrap(row["expires"]))].exists)
        print("P03_SIGN_REFUSAL_ORIGINAL_UI")

        try tap("요청 원문 독립 승인")
        try consent(account.approval, refusing: true)
        try tap("독립 확인 결과 가져오기")
        let terminal = app.staticTexts["이 원문을 거절했습니다. 서명 반환 · 0건"]
        XCTAssertTrue(terminal.waitForExistence(timeout: 12))
        app.terminate()
        app.launch()
        try tap("클라우드 지갑")
        try login(account.login)
        try tap("같은 요청 재조회")
        XCTAssertTrue(terminal.waitForExistence(timeout: 12))
        XCTAssertTrue(app.staticTexts[original].exists)
        XCTAssertFalse(app.buttons["요청 원문 독립 승인"].exists)
        XCTAssertFalse(app.staticTexts["검증된 서명 거래 · 체인 전송과 확정은 별도"].exists)
        print("P03_SIGN_REFUSAL_TERMINAL_UI")
        try tap("요청 확인 마침")
        try sign(account.approval, amount: "12")
        print("P03_SIGN_REFUSAL_FRESH_UI")
    }

    private func loginRegistered(_ provider: String) throws {
        try tap("\(provider)로 별도 로그인")
        XCTAssertTrue(safari.wait(for: .runningForeground, timeout: 20))
        let web = safari.webViews.firstMatch
        let approve = web.buttons["Continue with QA account"]
        if !approve.waitForExistence(timeout: 5) {
            for title in ["Continue", "Not Now", "Done"] {
                let onboarding = safari.buttons[title].firstMatch
                if onboarding.exists && onboarding.isHittable { onboarding.tap() }
            }
        }
        XCTAssertTrue(approve.waitForExistence(timeout: 15))
        approve.tap()
        let result = web.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Confirmation recorded.")).firstMatch
        XCTAssertTrue(result.waitForExistence(timeout: 15))
        app.activate()
        try tap("브라우저 로그인 확인")
        XCTAssertTrue(app.buttons["클라우드 로그아웃"].waitForExistence(timeout: 12))
    }

    func testRegisteredOIDCOnboardingAndReopen() throws {
        continueAfterFailure = false
        let fixture = try loadFixture()
        let provider = try XCTUnwrap(fixture.loginProvider)
        app.launch()
        try tap("클라우드 지갑")
        XCTAssertTrue(app.staticTexts["별도 클라우드 MPC 지갑"].exists)
        XCTAssertTrue(app.buttons["\(provider)로 별도 로그인"].exists)
        XCTAssertFalse(app.buttons["클라우드 서비스에 별도 로그인"].exists)
        XCTAssertFalse(app.buttons["이 계정을 클라우드 지갑에 연계"].exists)
        let address = app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "수신 주소 · 0x")).firstMatch
        XCTAssertFalse(address.exists)
        try tap("지갑 선택")
        XCTAssertTrue(app.staticTexts["개인 보드"].exists)
        try tap("클라우드 지갑")
        print("P03_STAGE w20_entry_and_wallet_choice")
        try loginRegistered(provider)
        XCTAssertTrue(app.buttons["이 계정을 클라우드 지갑에 연계"].exists)
        XCTAssertFalse(address.exists)
        print("P03_STAGE w20_registered_provider_login")
        try tap("이 계정을 클라우드 지갑에 연계")
        try tap("클라우드 지갑 생성")
        XCTAssertTrue(app.buttons["요청 원문 독립 승인"].exists)
        XCTAssertFalse(address.exists)
        print("P03_STAGE w20_registered_provider_pending")
        try tap("요청 원문 독립 승인")
        try consent(fixture.accounts[0].approval)
        try tap("독립 확인 결과 가져오기")
        try waitForReceipt(address)
        let original = address.label
        try tap("요청 확인 마침")
        print("P03_STAGE w20_registered_provider_active")
        app.terminate()
        app.launch()
        try tap("클라우드 지갑")
        try loginRegistered(provider)
        XCTAssertTrue(app.staticTexts[original].waitForExistence(timeout: 12))
        XCTAssertFalse(app.buttons["이 계정을 클라우드 지갑에 연계"].exists)
        print("P03_STAGE w20_reopened_enrolled_wallet")
    }

    func testTwoWalletOwnersUseExternalS256BrowserConsent() throws {
        continueAfterFailure = false
        let fixture = try loadFixture()
        XCTAssertNil(fixture.deployment)
        app.launch()
        try tap("클라우드 지갑")
        if fixture.recovery != nil {
            try recoverAndClose(fixture)
            return
        }
        if fixture.signRefusal == true {
            try refuseSign(fixture.accounts[0], originalReviewURL: XCTUnwrap(fixture.originalReviewURL))
            return
        }
        XCTAssertTrue(app.staticTexts["별도 클라우드 MPC 지갑"].exists)
        XCTAssertTrue(app.staticTexts["Ethereum Sepolia · 시험 프로필"].exists)
        XCTAssertTrue(app.staticTexts["참여자 · 모바일 A · 서비스 B · 독립 복구 C"].exists)
        XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "활성 주소 0개")).firstMatch.exists)
        XCTAssertFalse(app.buttons["이 계정을 클라우드 지갑에 연계"].exists)
        XCTAssertFalse(app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "수신 주소 · ")).firstMatch.exists)
        try tap("지갑 선택")
        XCTAssertTrue(app.staticTexts["개인 보드"].exists)
        try tap("클라우드 지갑")
        print("P03_STAGE w20_entry_and_wallet_choice")
        let first = try enroll(fixture.accounts[0])
        let second = try enroll(fixture.accounts[1])
        XCTAssertNotEqual(first, second)
        app.terminate()
        app.launch()
        try tap("클라우드 지갑")
        try login(fixture.accounts[0].login)
        XCTAssertTrue(app.staticTexts[first].waitForExistence(timeout: 12))
        XCTAssertFalse(app.staticTexts[second].exists)
        XCTAssertFalse(app.buttons["이 계정을 클라우드 지갑에 연계"].exists)
        print("P03_STAGE w20_reopened_enrolled_wallet")
        try tap("클라우드 로그아웃")
    }

    private func chain(_ method: String, _ parameters: [Any] = []) throws -> Any {
        var request = URLRequest(url: URL(string: "http://127.0.0.1:8285")!)
        request.timeoutInterval = 5
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONSerialization.data(withJSONObject: ["jsonrpc": "2.0", "id": 1,
            "method": method, "params": parameters])
        let ready = expectation(description: "isolated fixture RPC " + method)
        var payload: Data?
        URLSession.shared.dataTask(with: request) { data, _, _ in payload = data; ready.fulfill() }.resume()
        wait(for: [ready], timeout: 6)
        let document = try XCTUnwrap(try JSONSerialization.jsonObject(with: XCTUnwrap(payload)) as? [String: Any])
        return try XCTUnwrap(document["result"], String(describing: document["error"]))
    }

    private func market(_ account: Account, deployment: String, venue: String, expireBeforeApproval: Bool) throws {
        try tap("테스트 시장 학습")
        try enter("학습 배포 블록 해시", deployment)
        try tap("P03 격리 학습 지갑 선택")
        let label = try create(account)
        let user = String(label.suffix(42))
        try tap("이 클라우드 지갑을 격리 시장에 연결")
        XCTAssertTrue(app.staticTexts["P03 두 참여자 공동 서명 · 독립 승인 필요"].waitForExistence(timeout: 15))
        print("P03_STAGE market_wallet_selected")
        _ = try chain("anvil_setBalance", [user, "0x56bc75e2d63100000"])
        let actors = try XCTUnwrap(try chain("eth_accounts") as? [String])
        let head = try XCTUnwrap(try chain("eth_getBlockByNumber", ["latest", false]) as? [String: Any])
        let timestamp = try XCTUnwrap(UInt64(try XCTUnwrap(head["timestamp"] as? String).dropFirst(2), radix: 16))
        for publisher in actors[2...3] {
            let data = "0x07640c1f" + String(format: "%064llx%064llx", UInt64(1_200_000), timestamp)
            _ = try chain("eth_sendTransaction", [["from": publisher, "to": venue, "data": data]])
        }
        _ = try chain("anvil_mine", ["0x03"])
        try tap("가격과 잔액 확인")
        try tap("모의 환전")
        try tap("LAB → LAQ 모의 환전")
        try enter("가상 매도 LAB", "2")
        try enter("최소 가상 수령 LAQ", "2")
        try tap("견적 받기")
        XCTAssertTrue(app.staticTexts["예상 2.4 LAQ"].waitForExistence(timeout: 12))
        print("P03_STAGE market_quote_visible")
        try tap("견적 확인 및 학습 지갑에서 승인")
        if expireBeforeApproval {
            // Use actual wall time on both the simulator and the local chain.
            Thread.sleep(forTimeInterval: 72)
            try tap("독립 승인 열기")
            XCTAssertTrue(app.staticTexts["이 견적의 승인 기한이 지났습니다. 새 견적을 받으세요."].waitForExistence(timeout: 10))
            XCTAssertFalse(app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "원래 거래 해시 0x")).firstMatch.exists)
            XCTAssertFalse(app.buttons["견적 확인 및 학습 지갑에서 승인"].exists)
            print("P03_STAGE market_quote_expired")
            try tap("견적 받기")
            XCTAssertTrue(app.staticTexts["예상 2.4 LAQ"].waitForExistence(timeout: 12))
            print("P03_STAGE market_quote_replaced")
            return
        }
        safari.terminate()
        try tap("독립 승인 열기")
        try consent(account.approval, market: true)
        try tap("독립 승인 결과 가져오기")
        let original = app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "원래 거래 해시 0x")).firstMatch
        let deadline = Date().addingTimeInterval(37)
        while !original.exists && Date() < deadline {
            try tap("P03 원래 요청 조회")
            _ = original.waitForExistence(timeout: 1)
        }
        XCTAssertTrue(original.exists)
        let originalLabel = original.label
        let hash = String(originalLabel.suffix(66))
        app.terminate()
        app.launch()
        try tap("테스트 시장 학습")
        XCTAssertTrue(app.staticTexts[originalLabel].waitForExistence(timeout: 15))
        print("P03_STAGE market_original_hash_retained")
        try tap("원래 거래 확정 확인")
        XCTAssertTrue(app.staticTexts["실제 2.4 LAQ"].waitForExistence(timeout: 15))
        XCTAssertTrue(app.staticTexts[hash].exists)
        XCTAssertTrue(app.staticTexts["LAB 0 · LAQ 0 · LP 0"].exists)
        print("P03_STAGE market_receipt_confirmed")
    }

    private func waitForClosure() throws {
        let deadline = Date().addingTimeInterval(100)
        let complete = app.staticTexts["종료 완료 · 참여자 확인 완료"]
        while !complete.exists && Date() < deadline {
            try tap("같은 종료 요청 조회")
            _ = complete.waitForExistence(timeout:2)
        }
        XCTAssertTrue(complete.exists)
    }

    private func recoverAndClose(_ fixture: Fixture) throws {
        let owner = fixture.accounts[0]
        let secondary = try XCTUnwrap(fixture.alternateLogin)
        let recovery = try XCTUnwrap(fixture.recovery)
        let oldCredential = try XCTUnwrap(fixture.originalCredential)
        let secondaryCredential = try XCTUnwrap(fixture.alternateCredential)
        print("P03_RECOVERY 1")
        let address = try create(owner)

        try tap("다른 인증수단 연계 시작")
        try consent(secondary)
        try tap("브라우저 로그인 확인")
        XCTAssertTrue(app.staticTexts["연계 수단 · " + secondaryCredential].waitForExistence(timeout: 12))

        print("P03_RECOVERY 2")
        try tap("분실 휴대전화 교체 접수 · 서명 동결")
        if fixture.resume == true {
            XCTAssertTrue(app.staticTexts["동결 접수 · 서버 확인 대기"].waitForExistence(timeout: 12))
            XCTAssertTrue(app.staticTexts["요청을 확인할 수 없습니다. 저장한 요청 번호로 다시 조회하세요."].waitForExistence(timeout: 12))
            XCTAssertFalse(app.buttons["독립 복구 확인"].exists)
        } else {
            XCTAssertTrue(app.staticTexts["계정 상태 · 서명 동결"].waitForExistence(timeout: 12))
            let confirmation=app.buttons["독립 복구 확인"]
            XCTAssertFalse(confirmation.exists && confirmation.isEnabled)
        }
        XCTAssertFalse(app.buttons["Sepolia 전송 요청"].exists)
        let request = app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "요청 번호 · ")).firstMatch
        let requestLabel = request.label

        try tap("클라우드 로그아웃")
        app.terminate()
        app.launch()
        try tap("클라우드 지갑")
        XCTAssertFalse(request.exists)
        try login(secondary)
        XCTAssertTrue(app.staticTexts[requestLabel].waitForExistence(timeout: 12))
        if fixture.resume == true {
            XCTAssertTrue(app.staticTexts["계정 상태 · 서명 동결"].exists)
            XCTAssertFalse(app.buttons["같은 번호로 제출"].exists)
            print("P03_RESUME_RECOVERY_UI")
        }
        let recoveryButton = app.buttons["독립 복구 확인"]
        let latest = Date().addingTimeInterval(78)
        while !recoveryButton.exists || !recoveryButton.isEnabled {
            XCTAssertLessThan(Date(), latest)
            try tap("같은 요청 재조회")
            _ = recoveryButton.waitForExistence(timeout: 2)
        }
        print("P03_RECOVERY 3")
        // Each realm URL should open a new browser view. Leave Safari history alone.
        safari.terminate()
        try tap("독립 복구 확인")
        let firstTapOpenedSafari = safari.wait(for: .runningForeground, timeout: 12)
        print("P03_RECOVERY_BROWSER_FIRST_TAP \(firstTapOpenedSafari ? 1 : 0)")
        if !firstTapOpenedSafari && recoveryButton.exists && recoveryButton.isEnabled {
            try tap("독립 복구 확인")
        }
        if fixture.refusal == true {
            try consent(recovery, refusing: true)
            app.terminate()
            app.launch()
            try tap("클라우드 지갑")
            try login(secondary)
            XCTAssertTrue(app.staticTexts[requestLabel].waitForExistence(timeout: 12))
            XCTAssertTrue(app.staticTexts["요청 상태 · 독립 복구 승인 거절"].waitForExistence(timeout: 12))
            XCTAssertTrue(app.staticTexts["계정 상태 · 서명 동결"].exists)
            XCTAssertFalse(app.buttons["Sepolia 전송 요청"].exists)
            XCTAssertFalse(app.buttons["독립 복구 확인"].exists)
            XCTAssertFalse(app.buttons["허가된 복구 재심사 시작"].exists)
            try tap("복구 거절 기록 조회")
            try tap("거절 영수증 · " + requestLabel.suffix(8))
            XCTAssertTrue(app.staticTexts["원래 복구 요청 · " + requestLabel.dropFirst("요청 번호 · ".count)].exists)
            print("P03_DENIED_UI: same request and refusal after app relaunch")
            return
        }
        try consent(recovery)
        try tap("독립 확인 결과 가져오기")
        print("P03_RECOVERY 4")
        if fixture.partial == true {
            try waitForReceipt(app.staticTexts["응답 · 4 / 5"])
            XCTAssertTrue(app.staticTexts[requestLabel].exists)
            XCTAssertTrue(app.staticTexts["계정 상태 · 서명 동결"].exists)
            XCTAssertFalse(app.buttons["Sepolia 전송 요청"].exists)
            XCTAssertFalse(app.staticTexts["세대 · 2"].exists)
            try tap("같은 요청 재조회")
            XCTAssertTrue(app.staticTexts["응답 · 4 / 5"].exists)
            print("P03_PARTIAL_UI 4 of 5; frozen; no signing action")
        }
        try waitForReceipt(app.staticTexts["세대 · 2"])
        XCTAssertTrue(app.staticTexts[address].exists)
        XCTAssertTrue(app.staticTexts["응답 · 5 / 5"].exists)
        if fixture.partial == true {
            XCTAssertTrue(app.staticTexts[requestLabel].exists)
            return
        }
        try tap("요청 확인 마침")
        print("P03_RECOVERY 5")
        try sign(owner.approval)

        print("P03_RECOVERY 6")
        try tap("연결 해제 · " + oldCredential.suffix(8))
        try login(owner.login)
        XCTAssertFalse(app.staticTexts[address].exists)
        XCTAssertFalse(app.buttons["Sepolia 전송 요청"].exists)
        try tap("클라우드 로그아웃")
        try login(secondary)
        XCTAssertTrue(app.staticTexts[address].waitForExistence(timeout: 12))
        print("P03_RECOVERY 7")
        let finalLink = app.buttons["연결 해제 · " + secondaryCredential.suffix(8)]
        XCTAssertTrue(finalLink.waitForExistence(timeout: 12))
        XCTAssertFalse(finalLink.isEnabled)

        print("P03_RECOVERY 8")
        try tap("클라우드 계정 종료 검토")
        try tap("서명 이용 종료 확인 및 독립 인증")
        try consent(owner.approval)
        try tap("독립 확인 결과 가져오기")
        if fixture.resume == true {
            XCTAssertTrue(app.staticTexts["종료 접수 · 서버 확인 대기"].waitForExistence(timeout: 12))
            XCTAssertTrue(app.staticTexts["요청을 확인할 수 없습니다. 저장한 요청 번호로 다시 조회하세요."].waitForExistence(timeout: 12))
            let receipt=app.staticTexts.containing(NSPredicate(format:"label BEGINSWITH %@", "종료 요청 · ")).firstMatch
            let original=receipt.label
            app.terminate()
            app.launch()
            try tap("클라우드 지갑")
            XCTAssertFalse(app.staticTexts[original].exists)
            try login(secondary)
            XCTAssertTrue(app.staticTexts[original].waitForExistence(timeout: 12))
            XCTAssertTrue(app.staticTexts["종료 요청을 접수했습니다 · 참여자 확인 대기"].waitForExistence(timeout: 12))
            XCTAssertFalse(app.staticTexts["종료 완료 · 참여자 확인 완료"].exists)
            XCTAssertTrue(app.staticTexts["자산 이동 · 0건"].exists)
            try waitForClosure()
            XCTAssertTrue(app.staticTexts[original].exists)
            print("P03_RESUME_CLOSURE_UI")
        } else {
            try waitForClosure()
        }
        print("P03_RECOVERY 9")
        XCTAssertTrue(app.staticTexts["계정 상태 · 계정 종료"].waitForExistence(timeout: 12))
        XCTAssertFalse(app.staticTexts[address].exists)
        XCTAssertFalse(app.buttons["Sepolia 전송 요청"].exists)
        try tap("클라우드 로그아웃")
    }
}
