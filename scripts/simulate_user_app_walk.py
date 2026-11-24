"""Taps through the user app on an emulator. Run by simulate_user_app.py --walk."""

import html
import json
import re
import time

PACKAGE = "com.nu54renter"
TABS = ("홈", "결제", "지갑", "둘러보기")


class Walk:
    def __init__(self, adb, step, evidence):
        self.adb, self.step, self.evidence = adb, step, evidence
        size = re.search(rb"(\d+)x(\d+)", self.shell("wm", "size"))
        self.width, self.height = int(size[1]), int(size[2])
        self.failed = 0

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

    def wait_for_app(self):
        deadline = time.monotonic() + 150
        while not set(TABS) <= set(self.words()):
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

    # ---- the flows ----
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
        self.step("deposit", amount="90000000")
        self.tap("입금 안내 보기")
        self.tap("네트워크 주소 설정")
        self.type("검증된 HTTPS RPC 주소", "https://rpc.test")
        self.tap("지문 확인 후 읽기 전용 RPC 저장")
        self.sees("서버를 저장했습니다")
        self.back()
        self.sees("입금 주소 QR")
        self.tap("홈")
        self.sees("90.00 TestUSDC")
        self.shot("03-home")

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
        return sale

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
        for tab, labels in (("지갑", ("6개 체인 · 읽기 전용", "EVM 수신", "개인 전송", "지급 기록 관리", "다른 체인 받기", "다른 체인 보내기")),
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
            self.tap("관리")
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
            self.wait_for_app()
            flows = (self.set_up, self.deposit, self.pay, self.refuse, self.over_limit, self.no_board, self.lost_connection, self.inner_pages, self.give_back)
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
                    if flow in flows[:2]:
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
