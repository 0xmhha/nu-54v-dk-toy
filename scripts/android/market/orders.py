"""Drive the installed Android custom market test app through its ordinary UI."""

import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import urllib.request
import xml.etree.ElementTree as ET


class MarketDevice:
    def __init__(self, serial, package):
        assert re.fullmatch(r"emulator-\d+", serial)
        assert re.fullmatch(r"com\.nu54renter\.marketqa\.r[0-9a-f]{12}", package)
        self.command = [str(Path(os.environ["ANDROID_HOME"]) / "platform-tools/adb"), "-s", serial]
        self.package = package
        assert self.adb("shell", "getprop", "ro.kernel.qemu").strip() == "1"
        sizes = re.findall(r"(\d+)x(\d+)", self.adb("shell", "wm", "size"))
        self.width, self.height = map(int, sizes[-1])

    def adb(self, *args):
        return subprocess.check_output([*self.command, *map(str, args)], text=True, timeout=45)

    def hierarchy(self):
        for _ in range(5):
            raw = self.adb("exec-out", "uiautomator", "dump", "/dev/tty")
            end = raw.find("</hierarchy>")
            if end >= 0:
                return [node for node in ET.fromstring(raw[:end + 12]).iter("node")
                        if node.get("package") == self.package]
        raise RuntimeError("Android test app hierarchy unavailable")

    @staticmethod
    def labels(node):
        return node.get("text", ""), node.get("content-desc", "")

    def locate(self, label, *, edit=False, clickable=False, attempts=14):
        for attempt in range(attempts):
            for node in self.hierarchy():
                if label not in self.labels(node):
                    continue
                if edit and node.get("class") != "android.widget.EditText":
                    continue
                if clickable and (node.get("clickable") != "true" or node.get("enabled") != "true"):
                    continue
                left, top, right, bottom = map(int, re.findall(r"-?\d+", node.get("bounds", "")))
                x, y = (left + right) // 2, (top + bottom) // 2
                if min(right - left, bottom - top) >= 56 and 0.09 * self.height < y < 0.92 * self.height:
                    return x, y
            self.scroll(up=attempt % 6 < 3)
        raise RuntimeError("Android market control unavailable: " + label)

    def scroll(self, *, up, short=False):
        low, high = (0.43, 0.59) if short else (0.27, 0.75)
        start, end = (high, low) if up else (low, high)
        self.adb("shell", "input", "swipe", self.width // 2, int(self.height * start),
                 self.width // 2, int(self.height * end), 260)

    def visible(self, prefix):
        return any(label.startswith(prefix) for node in self.hierarchy() for label in self.labels(node))

    def value(self, prefix, pattern, attempts=12):
        for attempt in range(attempts):
            for node in self.hierarchy():
                for label in self.labels(node):
                    if label.startswith(prefix) and (match := re.search(pattern, label)):
                        return match.group()
            if attempt >= 2:
                self.scroll(up=attempt % 6 < 3)
        raise RuntimeError("Android market value unavailable: " + prefix)

    def review_renewal(self, amount, previous_deadline, approval, attempts=12):
        previous_text = f"직전 예상 {amount} · 새 견적 승인 필요"
        frames = []
        for attempt in range(attempts):
            nodes = self.hierarchy()
            labels = {label for node in nodes for label in self.labels(node) if label}
            deadline = next((label.split(" · ", 1)[1] for label in labels
                             if label.startswith("가격 시각 ") and " · 승인 기한 " in label), None)
            previous = previous_text in labels
            current = f"예상 {amount}" in labels
            approval_enabled = any(approval in self.labels(node) and node.get("enabled") == "true"
                                   and node.get("clickable") == "true" for node in nodes)
            if current and previous and deadline and deadline != previous_deadline and approval_enabled:
                if frames:
                    print("Quote renewal before ready: " + json.dumps(frames, ensure_ascii=False), flush=True)
                return deadline
            frames.append({
                "deadline": deadline, "previous": previous, "current": current,
                "approval_enabled": approval_enabled,
                "related_labels": sorted(label for label in labels
                                         if label.startswith(("직전 예상", "새 견적", "현재 가격과 입력", "견적 조건"))),
            })
            if attempt >= 2 and attempt + 1 < attempts:
                self.scroll(up=attempt % 8 < 5, short=True)
        print("Quote renewal accessibility frames: " + json.dumps(frames, ensure_ascii=False),
              file=sys.stderr, flush=True)
        raise RuntimeError("Android market quote renewal did not expose the old amount and a fresh approval")

    def tap(self, label):
        x, y = self.locate(label, clickable=True)
        self.adb("shell", "input", "tap", x, y)
        return x, y

    def fill(self, label, value):
        x, y = self.locate(label, edit=True)
        self.adb("shell", "input", "tap", x, y)
        self.adb("shell", "input", "text", value)
        if "mInputShown=true" in self.adb("shell", "dumpsys", "input_method"):
            self.adb("shell", "input", "keyevent", "KEYCODE_BACK")
        if not self.visible(str(value)):
            raise RuntimeError("Market input never displayed the entered value: " + label)

    def check_field(self, label, expected):
        self.locate(label, edit=True)
        assert any(label in self.labels(node) and node.get("text") == expected
                   for node in self.hierarchy()), f"Reviewed input changed: {label}"

    def enter(self):
        self.adb("shell", "am", "force-stop", self.package)
        self.adb("shell", "am", "start", "-W", "-n", self.package + "/com.nu54renter.MainActivity")
        self.tap("테스트 시장 학습")


def verify_learning_home(device):
    def warnings():
        for text in ("테스트 자산으로 연습하기", "StableNet 테스트넷 · 테스트 자산만", "실물 자산이나 실거래가 아닙니다."):
            device.value(text, r"^" + re.escape(text) + r"$")
        assert not device.visible("견적 받기"), "fresh home must not offer an executable quote"

    device.enter()
    warnings()
    screenshot = os.environ.get("NU54_ANDROID_MARKET_HOME_SCREEN")
    if screenshot:
        with Path(screenshot).open("xb") as result:
            subprocess.run([*device.command, "exec-out", "screencap", "-p"], stdout=result, check=True, timeout=15)
    for label in ("교환", "유동성", "포지션", "모의 환전"):
        device.tap(label)
        device.value(label + " 연습", re.escape(label) + r" 연습")
    device.locate("시장 운영 모니터")
    assert any("시장 운영 모니터" in device.labels(node) and node.get("enabled") == "false"
               for node in device.hierarchy()), "unconfigured operator access must be unavailable"
    device.tap("여행")
    device.value("여행지 찾기", r"^여행지 찾기$")
    device.tap("학습 시장으로 돌아가기")
    device.value("모의 환전 연습", r"^모의 환전 연습")
    warnings()
    device.tap("지갑으로 돌아가기")
    device.adb("shell", "am", "start", "-W", "-a", "android.intent.action.VIEW",
               "-d", "nu54://market-learning", "-p", device.package)
    warnings()
    print("L01 signed Android home and learning return exposed testnet/test-asset warnings; all topics and travel return worked before enrollment; unconfigured operator access stayed disabled.", flush=True)


def request(url, payload=None):
    body = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=8) as response:
        return json.load(response)


def rpc_response(method, *arguments):
    return request("http://127.0.0.1:8285", {"jsonrpc": "2.0", "id": 1, "method": method,
                                           "params": arguments})


def rpc(method, *arguments):
    reply = rpc_response(method, *arguments)
    if "error" in reply:
        raise RuntimeError("Disposable Anvil rejected " + method)
    return reply["result"]


def words(*values):
    return "".join(f"{value:064x}" for value in values)


def send(actor, target, data):
    return rpc("eth_sendTransaction", {"from": actor, "to": target, "data": data})


def main(serial, package, profile_path):
    device = MarketDevice(serial, package)
    if os.environ.get("NU54_ANDROID_MARKET_HOME_ONLY") == "1":
        verify_learning_home(device)
        return
    profile = json.loads(Path(profile_path).read_bytes())
    deployment = profile["deploymentHash"]
    venue, base, quote = (profile[name]["address"] for name in ("venue", "base", "quote"))
    assert int(rpc("eth_chainId"), 16) == 8283 and "anvil" in rpc("web3_clientVersion").lower()
    assert rpc("eth_getBlockByHash", deployment, False)["hash"] == deployment
    actors = rpc("eth_accounts")

    def publish(price=1_200_000):
        block = rpc("eth_getBlockByNumber", "latest", False)
        for reporter in actors[2:4]:
            send(reporter, venue, "0x07640c1f" + words(price, int(block["timestamp"], 16)))
        rpc("anvil_mine", "0x3")

    def pending(attempts=12):
        return device.value("원래 거래 해시 ", r"0x[0-9a-fA-F]{64}", attempts)

    def approve(label="견적 확인 및 학습 지갑에서 승인", restart=False):
        before = rpc("eth_getTransactionCount", wallet, "pending")
        tap_x, tap_y = device.tap(label)
        try:
            original = pending()
        except RuntimeError:
            changed = rpc("eth_getTransactionCount", wallet, "pending") != before
            print(f"Selected enabled control at ({tap_x},{tap_y}); signed nonce changed: {changed}", flush=True)
            raise
        if restart:
            device.enter()
            assert pending() == original
        rpc("anvil_mine", "0x4")
        device.tap("원래 거래 확정 확인")
        device.value("거래 확정 · 블록", r"\d+")
        assert rpc("eth_getTransactionReceipt", original)["status"] == "0x1"
        return original

    def offer(group, action, inputs, expected="", price=1_200_000):
        if os.environ.get("NU54_ANDROID_MARKET_REENTER") == "1":
            device.enter()
        device.tap(group)
        device.tap(action)
        for label, value in inputs:
            device.fill(label, value)
        publish(price)
        device.tap("가격과 잔액 확인")
        device.tap("견적 받기")
        if expected:
            device.value(expected, re.escape(expected))

    device.enter()
    device.fill("학습 배포 블록 해시", deployment)
    device.tap("격리 학습 지갑 만들기")
    wallet = device.value("전용 학습 지갑 ", r"0x[0-9a-fA-F]{40}")
    print("Android release enrolled the exact HTTPS deployment", flush=True)
    rpc("anvil_setBalance", wallet, "0x3635c9adc5dea00000")
    for token, amount in [(base, 100_000_000_000), (quote, 1_000_000_000)]:
        send(actors[0], token, "0x40c10f19" + "0" * 24 + wallet[2:] + words(amount))

    def balance(token):
        return int(rpc("eth_call", {"to": token, "data": "0x70a08231" + "0" * 24 + wallet[2:]}, "latest"), 16)

    def shares():
        return int(rpc("eth_call", {"to": venue, "data": "0xce7c2ac2" + "0" * 24 + wallet[2:]}, "latest"), 16)

    def balances():
        return balance(base), balance(quote)

    def check_fx(holdings):
        offer("모의 환전", "LAB → LAQ 모의 환전", [("가상 매도 LAB", 2), ("최소 가상 수령 LAQ", 2)], "예상 2.4 LAQ")
        publish(1_210_000)
        device.tap("견적 확인 및 학습 지갑에서 승인")
        device.value("견적 조건이 달라졌거나 네트워크 응답을 확인할 수 없습니다.", "견적")
        assert not device.visible("원래 거래 해시 ")
        device.tap("새 견적 받기")
        device.value("예상 2.42 LAQ", r"2\.42")
        print("Waiting for the reviewed quote's real wall-clock deadline", flush=True)
        time.sleep(72)
        device.tap("견적 확인 및 학습 지갑에서 승인")
        device.value("견적 조건이 달라졌거나 네트워크 응답을 확인할 수 없습니다.", "견적")
        assert not device.visible("원래 거래 해시 ")
        publish(1_210_000)
        device.tap("가격과 잔액 확인")
        device.tap("새 견적 받기")
        device.value("예상 2.42 LAQ", r"2\.42")
        approve(restart=True)
        device.value("실제 2.42 LAQ", r"2\.42")
        assert balances() == holdings
        print("Changed and expired quotes refused before signing; fresh FX kept its original hash and balances", flush=True)
        offer("모의 환전", "LAQ → LAB 모의 환전", [("가상 매도 LAQ", "2.42"), ("최소 가상 수령 LAB", 2)], "예상 2 LAB", price=1_210_000)
        approve()
        device.value("실제 2 LAB", "2")
        assert balances() == holdings
        print("Reverse FX confirmed in exact LAB units without moving either token", flush=True)

    if os.environ.get("NU54_ANDROID_MARKET_SCENARIO") == "position-review":
        fields = [("격리 증거금 LAQ", 12), ("포지션 수량 LAB", 1)]
        endpoint = "http://127.0.0.1:8284/v1/"
        remaining = 300 - time.time() % 300
        if remaining < 45:
            time.sleep(remaining + 1)
        offer("포지션", "매도 포지션 열기", fields)
        for prefix, pattern in [
            ("위험 조건을 확인하세요", r"위험 조건"),
            ("매도 · 1 LAB", r"매도 · 1 LAB"),
            ("격리 증거금 12 LAQ · 최대 ", r"1배"),
            ("기준 가격 1.2 LAQ/LAB · 관측 ", r"\d"),
            ("다음 예상 펀딩 ", r"예상 수령 0\.0012 LAQ"),
            ("펀딩 전 예상 청산 조건 · ", r"12 LAQ/LAB 이상"),
            ("첫 예상 펀딩 후 · ", r"12\.001091 LAQ/LAB 이상"),
        ]:
            device.value(prefix, pattern)
        # Explicitly seed one real existing position; the stale review below
        # must leave this original open and publish no additional effect.
        approve("LAQ 최대 12 사용 허용 승인")
        opened = approve()
        print("L05 reviewed short risk and one original on-chain position", flush=True)
        publish()
        device.tap("가격과 잔액 확인")
        device.tap("견적 받기")
        device.value("위험 조건을 확인하세요", r"위험 조건")
        stale_approval = "LAQ 최대 12 사용 허용 승인"
        device.locate(stale_approval, clickable=True)
        initial = request(endpoint + "positions/1")
        price_version = int(request(endpoint + "profile")["priceVersion"])
        history = request(endpoint + "positions/1/history")
        wallet_before = rpc("eth_getTransactionCount", wallet, "pending")
        keeper_before = rpc("eth_getTransactionCount", actors[4], "pending")
        checkpoint = rpc("evm_snapshot")
        head = rpc("eth_getBlockByNumber", "latest", False)
        rpc("evm_setNextBlockTimestamp", int(head["timestamp"], 16) + 121)
        rpc("evm_mine")
        assert request(endpoint + "profile")["state"] == "price-unavailable"
        liquidation = rpc_response("eth_call", {
            "from": actors[4], "to": venue, "data": "0x20dc2088" + words(1, 1, price_version),
        }, "latest")
        assert liquidation.get("error", {}).get("data") == "0x00bfc921", "expected Market.InvalidPrice"
        # Ask for approval from the existing review before manually refreshing.
        # Its submission must revalidate the read head and discard the offer.
        device.tap(stale_approval)
        device.value("신규 위험 증가가 중지됐어요", r"신규 위험 증가")
        device.tap("포지션 1 확인")
        device.value(opened, re.escape(opened))
        assert not device.visible("위험 조건을 확인하세요")
        assert not device.visible("원래 거래 해시 ")
        assert not any(node.get("enabled") == "true" for node in device.hierarchy()
                       if any(label in {"견적 받기", "새 견적 받기", "견적 확인 및 학습 지갑에서 승인", stale_approval}
                              for label in device.labels(node)))
        assert request(endpoint + "positions/1")["position"] == initial["position"]
        assert request(endpoint + "positions/1/history") == history
        assert rpc("eth_getTransactionCount", wallet, "pending") == wallet_before
        assert rpc("eth_getTransactionCount", actors[4], "pending") == keeper_before
        print("L05 stale user action and keeper rejected, original open position/history and both nonces unchanged", flush=True)
        assert rpc("evm_revert", checkpoint)
        publish()
        device.tap("가격과 잔액 확인")
        assert not device.visible("위험 조건을 확인하세요"), "Recovery must not restore a stale approval"
        assert not device.visible(stale_approval), "Recovery must not restore the previous sign button"
        device.tap("견적 받기")
        device.value("위험 조건을 확인하세요", r"위험 조건")
        assert rpc("eth_getTransactionCount", wallet, "pending") == wallet_before
        assert request(endpoint + "positions/1/history") == history
        print("L05 signed Android reviewed real short terms, held stale price/keeper execution, retained original position and history, and required explicit fresh review without a second transaction", flush=True)
        return

    if os.environ.get("NU54_ANDROID_MARKET_SCENARIO") == "fx":
        check_fx((100_000_000_000, 1_000_000_000))
        return

    offer("유동성", "유동성 공급", [("최대 지급 LAB", 100), ("최대 지급 LAQ", 120), ("최소 발행 지분", 100)],
          "예상 100 LP 지분")
    approve("LAB 최대 100 사용 허용 승인")
    approve("LAQ 최대 120 사용 허용 승인")
    approve()
    assert balances() == (90_000_000_000, 880_000_000) and shares() == 10_000_000_000
    print("Both allowances and 100 LP confirmed from the ordinary screen", flush=True)

    if os.environ.get("NU54_ANDROID_MARKET_SCENARIO") == "liquidity-pool":
        original = (rpc("eth_getTransactionCount", wallet, "pending"), balances(), shares())

        def same_pool():
            device.value("테스트 풀 선택", r"^테스트 풀 선택$")
            device.value("풀 주소 ", re.escape(venue))
            device.value("보유 LP 100 LP", r"^보유 LP 100 LP$")

        device.tap("유동성")
        same_pool()
        for action, field, amount in [
            ("유동성 공급", "최대 지급 LAB", "1"),
            ("유동성 회수", "소각 지분", "10"),
        ]:
            device.tap(action)
            device.fill(field, amount)
            device.tap("풀로 돌아가기")
            same_pool()
            assert (rpc("eth_getTransactionCount", wallet, "pending"), balances(), shares()) == original
        device.tap("학습 홈")
        device.value("테스트 자산으로 연습하기", r"^테스트 자산으로 연습하기$")
        device.tap("유동성")
        same_pool()
        assert (rpc("eth_getTransactionCount", wallet, "pending"), balances(), shares()) == original
        print("L03 signed pool entry matches the enrolled venue and 100 finalized LP; both reviews returned to the same pool with no wallet nonce, token or LP change", flush=True)
        return

    offer("교환", "LAB → LAQ 교환", [("판매 LAB", 2), ("최소 수령 LAQ", 2)], "예상 2.34602 LAQ")
    approve("LAB 최대 2 사용 허용 승인")
    if os.environ.get("NU54_ANDROID_MARKET_SCENARIO") == "exchange-expired":
        reviewed = "2.34602 LAQ"
        nonce = rpc("eth_getTransactionCount", wallet, "pending")
        holdings = balances()
        deadline = device.value("가격 시각 ", r"승인 기한 .+")
        print(f"L02/expired reviewed {reviewed} with {deadline}; waiting beyond the 60-second quote window",
              flush=True)
        time.sleep(65)
        assert request("http://127.0.0.1:8284/v1/profile")["state"] == "ready"
        device.tap("견적 확인 및 학습 지갑에서 승인")
        device.value("견적 조건이 달라졌거나 네트워크 응답을 확인할 수 없습니다.", "견적")
        assert not device.visible("원래 거래 해시 ")
        assert rpc("eth_getTransactionCount", wallet, "pending") == nonce
        assert balances() == holdings
        print(f"L02/expired rejected before submission with no hash, wallet nonce still {int(nonce, 16)}, "
              f"and balances still {holdings}", flush=True)

        device.check_field("판매 LAB", "2")
        device.check_field("최소 수령 LAQ", "2")
        publish()
        device.tap("가격과 잔액 확인")
        x, y = device.tap("새 견적 받기")
        print(f"L02 renewal request tapped at ({x}, {y}) on {device.width}×{device.height}", flush=True)
        fresh_deadline = device.review_renewal(reviewed, deadline, "견적 확인 및 학습 지갑에서 승인")
        assert rpc("eth_getTransactionCount", wallet, "pending") == nonce
        assert balances() == holdings
        print(f"L02/expired kept both inputs and showed the previous amount beside the new offer, {fresh_deadline}",
              flush=True)
        original = approve(restart=True)
        device.value(f"실제 {reviewed}", re.escape(reviewed))
        assert balances() == (89_800_000_000, 882_346_020)
        print(f"L02/expired fresh explicit approval confirmed {reviewed} under {original}", flush=True)
        return

    approve()
    device.value("실제 2.34602 LAQ", r"2\.34602")
    assert balances() == (89_800_000_000, 882_346_020)
    print("Swap 2 LAB to 2.34602 LAQ confirmed on the same screen", flush=True)

    offer("유동성", "유동성 회수", [("소각 지분", 10), ("최소 수령 LAB", 1), ("최소 수령 LAQ", 1)],
          "예상 10.2 LAB · 11.765398 LAQ")
    approve(restart=True)
    device.value("실제 10.2 LAB · 11.765398 LAQ", r"11\.765398")
    holdings = balances()
    assert holdings == (90_820_000_000, 894_111_418) and shares() == 9_000_000_000
    print("Single withdrawal approval burned ten LP; both assets and original hash verified after restart", flush=True)

    offer("포지션", "매수 포지션 열기", [("격리 증거금 LAQ", 10), ("포지션 수량 LAB", 1)])
    approve("LAQ 최대 10 사용 허용 승인")
    opened = approve(restart=True)
    assert balances() == (holdings[0], holdings[1] - 10_000_000)
    device.tap("포지션")
    device.tap("포지션 1 확인")
    device.value(opened, re.escape(opened))

    # Funding advances only this disposable chain. Revert before any new
    # user signature so the native signer still uses the actual wall time.
    snapshot = rpc("evm_snapshot")
    endpoint = "http://127.0.0.1:8284/v1/"
    period = int(request(endpoint + "positions/1")["position"]["nextPeriod"])
    rpc("evm_setNextBlockTimestamp", period * 300 + 1)
    rpc("evm_mine")
    publish()
    version = int(request(endpoint + "profile")["priceVersion"])
    funding = send(actors[4], venue, "0x325fab67" + words(1, 1, period, version))
    rpc("anvil_mine", "0x4")
    device.tap("포지션 1 확인")
    device.value(funding, re.escape(funding))
    assert rpc("eth_getTransactionReceipt", funding)["status"] == "0x1"
    assert rpc("evm_revert", snapshot)
    device.tap("포지션 1 확인")
    device.value("포지션 1 ·", r"버전 1$")
    assert not any(event["txHash"] == funding for event in request(endpoint + "positions/1/history"))
    publish()
    device.tap("가격과 잔액 확인")
    device.tap("이 포지션 종료 견적 준비")
    device.tap("견적 받기")
    approve()
    assert balances() == holdings and request(endpoint + "positions/1")["position"]["closed"]
    print("Position opened, original funding visible, rollback reconciled and collateral returned on close", flush=True)

    offer("포지션", "매도 포지션 열기", [("격리 증거금 LAQ", 10), ("포지션 수량 LAB", 1)])
    approve("LAQ 최대 10 사용 허용 승인")
    approve()
    assert not request(endpoint + "positions/2")["position"]["long"]
    assert balances() == (holdings[0], holdings[1] - 10_000_000)
    publish(20_000_000)
    version = int(request(endpoint + "profile")["priceVersion"])
    liquidation = send(actors[4], venue, "0x20dc2088" + words(2, 1, version))
    rpc("anvil_mine", "0x4")
    device.tap("포지션 2 확인")
    device.value(liquidation, re.escape(liquidation))
    assert rpc("eth_getTransactionReceipt", liquidation)["status"] == "0x1"
    assert request(endpoint + "positions/2")["position"]["closed"]
    holdings = (holdings[0], holdings[1] - 10_000_000)
    assert balances() == holdings
    print("Short position liquidated by the isolated keeper with its original hash visible", flush=True)

    check_fx(holdings)


if __name__ == "__main__":
    main(*sys.argv[1:])
