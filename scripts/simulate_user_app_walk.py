"""Taps through the user app on an emulator. Run by simulate_user_app.py --walk."""

import html
import json
import re
import time

PACKAGE = "com.nu54renter"
TABS = ("홈", "결제", "지갑", "둘러보기", "설정")


class Walk:
    def __init__(self, adb, step, evidence, token="usd"):
        self.adb, self.step, self.evidence = adb, step, evidence
        size = re.search(rb"(\d+)x(\d+)", self.shell("wm", "size"))
        self.width, self.height = int(size[1]), int(size[2])
        self.failed = 0
        self.won = token != "usd"
        self.unit = 10 ** 16 if token == "krwc" else 1  # base units in one unit of the board
        self.label = {"usd": "TestUSDC", "won": "tKRW", "krwc": "KRWC"}[token]
        self.wallet = (lambda dollars: f"{self.label} {dollars * 10000:,}원") if self.won else (lambda dollars: f"TestUSDC {dollars}.00")
        self.money = (lambda dollars: f"{dollars * 10000:,}원") if self.won else (lambda dollars: f"{dollars}.00 TestUSDC")

    # ---- the screen ----
    def shell(self, *args):
        return self.adb("shell", *args).stdout

    def nodes(self):
        for _ in range(10):
            self.shell("rm", "-f", "/sdcard/nu54-ui.xml")
            self.shell("uiautomator", "dump", "/sdcard/nu54-ui.xml")
            text = self.adb("exec-out", "cat", "/sdcard/nu54-ui.xml").stdout.decode(errors="replace")
            found = []
            for node in re.findall(r"<node [^>]*>", text):
                def value(name):
                    match = re.search(rf' {name}="([^"]*)"', node)
                    return html.unescape(match[1]) if match else ""
                found.append({"words": (value("text"), value("content-desc")), "tap": value("clickable") == "true",
                              "enabled": value("enabled") == "true", "box": [int(n) for n in re.findall(r"\d+", value("bounds"))]})
            if found:
                return found
            time.sleep(1)
        raise AssertionError("the screen cannot be read")

    def words(self):
        return [word for node in self.nodes() for word in node["words"] if word]

    def swipe(self, up=True):
        low, high = self.height * 65 // 100, self.height * 30 // 100
        start, end = (low, high) if up else (high, low)
        self.shell("input", "swipe", str(self.width // 2), str(start), str(self.width // 2), str(end), "400")
        time.sleep(0.7)

    def find(self, label):
        """The button or field with this label, scrolled into view."""
        last, turned = None, False
        for _ in range(40):
            nodes = self.nodes()
            for node in sorted(nodes, key=lambda item: not item["tap"]):
                left, upper, right, lower = node["box"]
                if label in node["words"] and lower - upper > 25 and upper > 60 and (label in TABS or (upper + lower) // 2 < self.height * 88 // 100):
                    return node
            seen = [(node["words"], node["box"]) for node in nodes]
            if seen == last:
                if turned:
                    return None
                turned, last = True, None
                for _ in range(8):
                    self.swipe(False)
                continue
            last = seen
            self.swipe()
        return None

    def touch(self, node):
        left, upper, right, lower = node["box"]
        self.shell("input", "tap", str((left + right) // 2), str((upper + lower) // 2))
        time.sleep(1.2)

    def tap(self, label, seconds=20):
        deadline = time.monotonic() + seconds
        while True:
            node = self.find(label)
            if node and node["enabled"]:
                return self.touch(node)
            if time.monotonic() > deadline:
                raise AssertionError(f"no button {label}: {' | '.join(self.words())[:700]}")
            time.sleep(1)

    def sees(self, text, seconds=40):
        deadline = time.monotonic() + seconds
        while True:
            seen = self.words()
            if any(text in word for word in seen):
                return
            if time.monotonic() > deadline:
                raise AssertionError(f"not on the screen: {text}: {' | '.join(seen)[:700]}")
            time.sleep(1)

    def type(self, label, value):
        node = self.find(label)
        assert node, f"no field {label}"
        self.touch(node)
        time.sleep(1)
        for at in range(0, len(value), 50):
            self.shell(f"input text '{value[at:at + 50]}'")
            time.sleep(0.2)
        if b"mInputShown=true" in self.shell("dumpsys", "input_method"):
            self.back()

    def back(self):
        self.shell("input", "keyevent", "4")
        time.sleep(1.2)

    def shot(self, name):
        (self.evidence / f"{name}.png").write_bytes(self.adb("exec-out", "screencap", "-p").stdout)

    def restart(self):
        self.shell("am", "force-stop", PACKAGE)
        self.shell("am", "start", "-n", PACKAGE + "/.MainActivity")
        time.sleep(5)
        self.wait_for_app()

    def wait_for_app(self, words=TABS):
        deadline = time.monotonic() + 150
        while not set(words) <= set(self.words()):
            if time.monotonic() > deadline:
                raise AssertionError("the app did not start")
            time.sleep(2)

    def pressed_late(self, label, name):
        """The owner takes a moment on the board, so the app has to say what to do."""
        self.step("board", change={"answer": "ignore"})
        node = self.find(label)
        assert node, f"no button {label}"
        self.touch(node)
        time.sleep(1.5)
        self.shot(name)
        self.step("press")
        self.step("board", change={"answer": "press"})

    def labelled(self, start):
        """The whole label of the first button that starts with these words."""
        for _ in range(12):
            found = next((word for node in self.nodes() for word in node["words"] if word.startswith(start)), None)
            if found:
                return found
            self.swipe()
        raise AssertionError(f"nothing starts with {start}")

    def clear(self, label):
        node = self.find(label)
        self.touch(node)
        self.shell("input", "keyevent", "123")
        for _ in range(8):
            self.shell("input", "keyevent", *["67"] * 10)
        if b"mInputShown=true" in self.shell("dumpsys", "input_method"):
            self.back()

    # ---- the flows ----
    def agree(self):
        """Nothing of the app opens before the required agreements."""
        self.shot("00-consent")
        assert not set(TABS) & set(self.words()), "the app opened without agreement"
        names = [word for word in self.words() if word.endswith("(필수)")]
        assert len(dict.fromkeys(names)) == 3, names
        read = next(word for word in self.words() if word.endswith("(필수) 보기"))
        self.tap(read)
        self.sees("초안")
        self.back()
        for name in dict.fromkeys(names):
            self.tap(name)
        self.tap("주문 준비 알림 (선택)")
        self.tap("동의하고 시작")
        self.wait_for_app()

    def set_up(self):
        document, home = self.step("enrollment"), self.step("state")["home"]
        self.tap("보드 연결 시작")
        self.type("발급 등록서", json.dumps(document, separators=(",", ":")))
        self.type("기기 식별값", document["device"])
        self.type("발급 문서의 서버 공개키 지문", document["boardSPKI"])
        self.tap("개인 기기 찾기")
        self.pressed_late("기기 등록 시작", "01-register")
        self.sees("등록을 기기에서 확인했습니다")
        self.tap("보드 설정 이어서 하기")
        self.tap("내 보드 찾기")
        self.tap("서명 시험 하기")
        self.sees("보드 서명을 확인했습니다")
        self.tap("반납받을 주소 등록")
        self.type("원래 지갑 주소", home)
        self.type("원래 지갑 주소 다시", home)
        self.tap("주소 확인")
        self.pressed_late("보드에 저장", "02-return-address")
        self.sees("반납받을 주소를 저장했습니다")
        self.tap("돌아가기")
        self.tap("이 매장에 보드 등록")
        self.sees("이 매장에 보드 계정을 등록했습니다")

    def deposit(self):
        self.step("deposit", amount=str(90000000 * self.unit))
        self.tap("입금 안내 보기")
        if self.won:
            self.sees(self.label + "는 StableNet 테스트넷에만 있습니다")
            self.shot("02-network-first")
            self.tap("확인했습니다. 주소 보기")
        self.sees("입금 주소 QR")
        self.tap("홈")
        self.sees("지금 할 일이 없습니다")
        assert not any(shown in word for shown in ("90.00", "900,000") for word in self.words()), "the balance belongs to the wallet tab"
        self.shot("03-home")
        self.tap("지갑")
        self.sees(self.wallet(90))
        self.shot("03-wallet")

    def tokens(self):
        extra = self.step("servers", change={})
        self.tap("지갑")
        self.tap("토큰 추가")
        self.type("토큰 계약 주소", extra["madeUpToken"])
        self.tap("토큰 정보 확인")
        self.sees("체인에서 읽은 정보")
        self.shot("12-add-token")
        self.tap("토큰 추가")
        self.sees("tUSDT 20.00")
        self.tap(self.labelled("tUSDT 20.00"))
        self.sees("이 자산을 보드 계정으로 보내지 마세요")
        self.shot("13-token")
        self.back()
        self.tap("잔액 숨기기")
        self.sees("••••")
        self.tap("잔액 표시")
        self.tap(self.labelled("네트워크 StableNet"))
        self.sees("보기만")
        self.shot("14-choose-network")
        self.tap(self.labelled("Ethereum Sepolia"))
        self.sees("ETH 0.50")
        assert "받기" not in self.words(), "a network the board cannot send on offers an address"
        self.shot("15-other-network")
        self.tap(self.labelled("네트워크 Ethereum"))
        self.tap(self.labelled("StableNet Testnet"))
        self.sees(self.wallet(90))

    def faucet(self):
        before = int(self.step("state")["balance"])
        self.tap("지갑")
        self.tap("시험 토큰")
        self.sees("보드가 아직 이 토큰을 보낼 수 없어 받지 않습니다")
        self.shot("12-faucet")
        button = self.labelled("받기") if "받기" in self.words() else next(word for word in self.words() if word.endswith(" 받기") and word != "시험 토큰 받기")
        assert "tKRWC 받기" not in self.words(), "a token the board cannot send is offered"
        self.tap(button)
        self.sees("보내 달라고 했습니다")
        deadline = time.monotonic() + 60
        while int(self.step("state")["balance"]) != before + 5000000 * self.unit:
            assert time.monotonic() < deadline, "the faucet paid nothing"
            time.sleep(1)
        self.sees("이후 다시 받을 수 있습니다", 60)
        self.shot("12-faucet-done")
        self.back()
        self.sees(self.wallet(95), 60)

    def send(self):
        before = int(self.step("state")["balance"])
        self.tap("지갑")
        self.tap("보내기")
        self.sees("오늘 남은 횟수")
        self.type("받는 주소", "0x" + "f1" * 20)
        self.type("금액", "50000" if self.won else "5")
        self.tap("다음")
        self.sees("처음 보내는 주소입니다")
        self.sees(self.money(5))
        self.shot("24-send-review")
        self.tap("이 내용으로 보내기")
        self.sees("보드에는 화면이 없습니다")
        if not self.find("보드로 승인"):
            self.tap("내 보드 찾기")
        self.pressed_late("보드로 승인", "25-send-approve")
        self.sees("거래가 확정되었습니다", 90)
        assert int(self.step("state")["balance"]) == before - 5000000 * self.unit, "the transfer moved another amount"
        self.shot("26-sent")
        self.tap("확인")
        self.back()
        self.tap("지갑")
        assert self.find("활동")
        assert any(word.startswith("보냄") for word in self.words()), "the transfer is not in the activity"

    def network_settings(self):
        extra = self.step("servers", change={})
        self.tap("설정")
        self.shot("16-settings")
        self.tap("네트워크")
        self.sees("정상")
        self.shot("17-networks")
        self.tap("네트워크 추가")
        for label, value in (("네트워크 이름", "Added Sepolia"), ("서버 주소 (https)", extra["wrongServer"]), ("체인 ID", "421614"), ("코인 기호", "ETH")):
            self.type(label, value.replace(" ", "%s"))
        self.tap("확인하고 저장")
        self.sees("다른 네트워크의 서버")
        self.shot("18-wrong-server")
        self.clear("서버 주소 (https)")
        self.type("서버 주소 (https)", extra["addedServer"])
        self.tap("확인하고 저장")
        self.sees("네트워크를 추가했습니다")
        self.tap("지갑")
        self.tap(self.labelled("네트워크 StableNet"))
        self.tap(self.labelled("Added Sepolia"))
        self.sees("ETH 1.25")
        self.tap("설정")
        self.tap("네트워크")
        self.tap(self.labelled("Added Sepolia"))
        self.tap("네트워크 삭제")
        self.sees("지워지지 않는 것")
        self.shot("19-remove-network")
        self.tap("네트워크 삭제")
        self.sees("네트워크를 삭제했습니다")
        self.tap(self.labelled("Base Sepolia"))
        self.tap("목록에서 숨기기")
        self.sees("네트워크를 숨겼습니다")
        self.tap(self.labelled("Base Sepolia"))
        self.tap("목록에 다시 보이기")
        self.step("servers", change={"down": ["https://ethereum-sepolia"]})
        self.tap("다시 확인")
        self.sees("연결 안 됨")
        self.step("servers", change={"down": []})
        # A removed network is not shown any more.
        self.tap("지갑")
        self.sees(self.wallet(95))

    def settings(self):
        self.tap("설정")
        self.tap("법적 고지")
        self.sees("내 동의 기록")
        self.shot("20-legal")
        self.tap("오픈소스 라이선스")
        self.sees("앱에 들어간 패키지")
        self.shot("21-licences")
        self.back()
        self.back()
        self.tap("내 보드")
        self.sees("LED 신호 보는 법")
        self.back()
        assert self.find("앱 버전"), "Back from the board left settings"
        self.tap("언어")
        self.sees("Language")
        self.shot("22-english")
        self.tap("Language")
        self.sees("언어")
        for label in ("기기·업데이트", "알림과 접근성", "도움말·진단", "등록과 매장", "반납과 초기화"):
            self.tap("설정")
            self.tap(label)
            time.sleep(1)
            self.back()
            assert set(TABS) <= set(self.words()), f"Back from {label} left the app"

    def leave_payment(self):
        self.tap("결제")
        time.sleep(1)
        for _ in range(4):
            seen = self.words()
            for label in ("결제 취소", "결제 화면 닫기", "연결 화면 닫기", "결제 목록으로"):
                if label in seen:
                    self.tap(label)
                    break
            else:
                return

    def review(self, quantity=1):
        """From the kiosk's code to the payment review on the phone."""
        self.leave_payment()
        for _ in range(3):
            self.swipe(False)
        sale = self.step("order", quantity=quantity)["sale"]
        self.tap("키오스크 QR 스캔")
        self.tap("QR 주문 연결")
        self.sees("매장 확인 번호")
        code = next(word.split()[-1] for word in self.words() if word.startswith("매장 확인 번호"))
        self.step("confirm", sale=sale, code=code)
        self.sees("결제 내용을 확인하세요")
        self.sees(self.money(8 * quantity))
        return sale

    def options(self):
        """A product with groups on the store's menu: nothing picked first, the missing group named, then one combination."""
        self.tap("둘러보기")
        self.tap("가까운 매장에서 주문")
        self.tap("근처 단말 찾기")
        self.tap("근거리 단말 후보 1 인증")
        self.tap("이 매장 메뉴 열기")
        self.sees(f"옵션 라떼 · {self.money(8)}부터")
        self.shot("07-menu-options")
        self.tap("옵션 라떼 옵션 고르기")
        self.sees("온도")
        self.shot("08-choices-start")
        self.tap(f"골라 주세요 · {self.money(8)}부터")
        self.sees("온도를 골라 주세요")
        self.shot("09-choices-missing")
        self.tap("온도 차갑게")
        self.tap(f"크기 크게 +{self.money(1)}")
        self.shot("10-choices-picked")
        self.tap(f"담기 · {self.money(9)}")
        self.sees("차갑게 · 크게")
        self.shot("11-menu-basket")
        self.tap("홈")

    def check_board(self):
        self.tap("내 보드 확인")
        self.sees("오늘 남은 한도")

    def pay(self):
        sale = self.review()
        self.check_board()
        self.pressed_late("보드로 승인", "04-approve")
        self.sees("주문이 접수되었습니다", 60)
        self.shot("05-number")
        self.tap("확인")
        self.step("cook", sale=sale, targets=["preparing", "ready"])
        self.tap("홈")
        self.sees("준비 완료", 60)
        deadline = time.monotonic() + 30
        while "주문한 상품이 준비되었습니다" not in self.shell("dumpsys", "notification", "--noredact").decode(errors="replace"):
            assert time.monotonic() < deadline, "no notification"
            time.sleep(1)
        self.step("cook", sale=sale, targets=["collected"])
        self.sees("전달됨", 60)
        self.touch(next(node for node in self.nodes() if node["words"][1].startswith("Order flow cafe 주문번호")))
        self.tap("받았어요")
        self.tap("네, 받았어요")
        self.sees("받았다고 확인해서 매장에 지급되었습니다", 60)
        self.shot("06-paid")
        self.back()
        assert set(TABS) <= set(self.words()), "Back left the app"

    def refuse(self):
        self.review()
        self.check_board()
        self.step("board", change={"answer": "refuse"})
        self.tap("보드로 승인")
        self.sees("보드에서 거절했습니다")
        self.shot("07-refused")
        self.step("board", change={"answer": "press"})

    def over_limit(self):
        self.review(quantity=4)
        self.sees("보드 한도를 넘는 금액입니다")
        self.shot("08-over-limit")

    def no_board(self):
        self.review()
        self.step("around", change={"board": False})
        self.tap("내 보드 확인")
        self.sees("주변에서 내 보드를 찾지 못했습니다")
        self.step("around", change={"board": True})

    def lost_connection(self):
        sale = self.review()
        self.check_board()
        prompts = self.step("state")["prompts"]
        self.step("board", change={"dropAfterSigning": True})
        self.tap("보드로 승인")
        self.sees("보드 연결이 끊겼습니다")
        self.shot("09-dropped")
        self.restart()
        self.tap("결제")
        self.sees("보드 승인을 이어서 확인합니다")
        self.check_board()
        self.tap("보드로 승인")
        self.sees("주문이 접수되었습니다", 60)
        assert self.step("state")["prompts"] == prompts + 1, "the board asked twice"
        self.tap("확인")
        self.step("cook", sale=sale, targets=["preparing", "unfulfillable"])

    def inner_pages(self):
        for tab, labels in (("지갑", ("받기", "반납", "토큰 추가")),
                            ("둘러보기", ("DApp 둘러보기", "외부 DApp 연결", "온체인 토큰 권한", "여행지 찾기", "테스트 시장 학습"))):
            for label in labels:
                self.tap(tab)
                self.tap(label)
                self.back()
                assert set(TABS) <= set(self.words()), f"Back from {label} left the app"

    def give_back(self):
        self.tap("홈")
        # The kitchen's refund of the last order has to be final first, and the board waits for its last quote.
        deadline = time.monotonic() + 150
        while True:
            self.tap("지갑")
            self.tap("반납")
            time.sleep(4)
            if "반납 시작" in self.words():
                break
            assert time.monotonic() < deadline, "the return stays blocked"
            self.tap("돌아가기")
            self.tap("홈")
            time.sleep(8)
        self.tap("반납 시작")
        self.sees("돌려받을 금액")
        if not self.find("보드로 승인"):
            self.tap("내 보드 찾기")
        time.sleep(100)  # the longest quote the board signed
        self.pressed_late("보드로 승인", "10-return")
        self.sees("반납이 확정되었습니다", 90)
        assert self.step("state")["balance"] == "0", "money left on the board account"
        self.shot("11-returned")
        # The added token cannot be returned, so it has to be given up by name before the key is erased.
        self.sees("보드가 보낼 수 없는 자산이 있습니다")
        assert not self.find("보드 초기화"), "a reset is offered over assets that would be lost"
        self.tap("20.00 tUSDT · 잃어도 됩니다")
        if not self.find("보드 초기화"):
            self.tap("내 보드 찾기")
        self.tap("보드 초기화")
        self.sees("보드를 초기화했습니다")
        self.tap("홈")
        self.sees("보드 연결이 필요합니다")

    def run(self):
        self.adb("logcat", "-c")
        scales = ("window_animation_scale", "transition_animation_scale", "animator_duration_scale")
        before = {name: self.shell("settings", "get", "global", name).decode().strip() for name in scales}
        for name in scales:
            self.shell("settings", "put", "global", name, "0")
        try:
            self.wait_for_app(("동의하고 시작",))
            flows = (self.agree, self.set_up, self.deposit, self.tokens, self.faucet, self.network_settings, self.settings, self.send, self.pay, self.refuse, self.over_limit, self.no_board, self.lost_connection, self.options, self.inner_pages, self.give_back)
            for flow in flows:
                name, at = flow.__name__.replace("_", " "), time.monotonic()
                try:
                    self.step("board", change={"answer": "press", "dropAfterSigning": False})
                    self.step("around", change={"board": True, "kiosk": True})
                    flow()
                    print(f"PASS {name} ({time.monotonic() - at:.0f}s)", flush=True)
                except Exception as error:
                    self.failed += 1
                    self.shot("FAILED-" + flow.__name__)
                    print(f"FAIL {name}: {error}", flush=True)
                    if flow in flows[:3]:
                        break  # the rest needs a board with money
            log = self.adb("logcat", "-d", "-v", "brief").stdout.decode(errors="replace")
            crashes = [line for line in log.splitlines() if re.search(r"AndroidRuntime.*FATAL|ANR in " + PACKAGE + r"|E/ReactNativeJS", line)]
            if crashes:
                self.failed += 1
                print("FAIL app errors:\n" + "\n".join(line[:300] for line in crashes[:5]), flush=True)
        finally:
            for name, value in before.items():
                self.shell("settings", *(("delete", "global", name) if value == "null" else ("put", "global", name, value)))
        return self.failed
