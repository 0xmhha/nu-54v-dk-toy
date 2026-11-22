#!/usr/bin/env python3
"""Run normal P02 enrollment and signed nearby review on an owned Android emulator."""
import contextlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "subscription-smoke"))
from run import Android, CLIP


def main():
    origin = sys.argv[1]
    remote = urlsplit(origin)
    if remote.scheme != "http" or remote.hostname != "127.0.0.1" or not remote.port:
        raise ValueError("W10 control endpoint must be loopback")
    device = Android(os.environ.get("NU54_W10_ADB", "adb"), os.environ["NU54_W10_ANDROID_SERIAL"],
                     os.environ["NU54_W10_ANDROID_PACKAGE"])
    try:
        scenario(device, origin, remote)
    except Exception:
        dump_screen(device)
        raise


def dump_screen(device):
    """Print the visible accessibility nodes so a failed installed run can be diagnosed after cleanup."""
    seen = set()
    keep = os.environ.get("NU54_W10_DIAGNOSTICS")
    if keep:  # optional directory outside the run workspace, which is removed on exit
        with contextlib.suppress(Exception):
            target = Path(keep)
            target.mkdir(parents=True, exist_ok=True)
            stamp = time.strftime("%H%M%S")
            (target/f"{stamp}-screen.png").write_bytes(
                subprocess.check_output([*device.command, "exec-out", "screencap", "-p"], timeout=30))
            (target/f"{stamp}-hierarchy.xml").write_text(device.call("exec-out", "uiautomator", "dump", "/dev/tty"))
            (target/f"{stamp}-logcat.txt").write_text(device.call("logcat", "-d", "-v", "threadtime"))
            print(f"W10 diagnostics saved under {target}/{stamp}-*", file=sys.stderr, flush=True)
    with contextlib.suppress(Exception):
        crash =device.call("logcat", "-d", "-b", "main,crash", "-s", "AndroidRuntime:E", "ReactNativeJS:E",
                            "ReactNative:E", "unknown:ReactNative:E")
        manager = device.call("logcat", "-d", "-s", "ActivityManager:I", "ActivityTaskManager:I")
        for row in (crash.splitlines()[-60:] + [r for r in manager.splitlines() if device.package in r][-15:]):
            print("SYSLOG " + row[:240], file=sys.stderr, flush=True)
    with contextlib.suppress(Exception):
        for _ in range(8):  # scroll back to the top, then walk down the whole screen
            device.call("shell", "input", "swipe", device.width // 2, int(device.height * 0.3),
                        device.width // 2, int(device.height * 0.8), 150)
        for _ in range(10):
            for node in device.tree():
                text = node.get("text", "") or node.get("content-desc", "")
                if text and (node.get("package"), text) not in seen:
                    seen.add((node.get("package"), text))
                    print(f"SCREEN {node.get('package')} {node.get('class')} {text[:160]!r}", file=sys.stderr, flush=True)
            device.call("shell", "input", "swipe", device.width // 2, int(device.height * 0.75),
                        device.width // 2, int(device.height * 0.35), 250)
    with contextlib.suppress(Exception):
        log = device.call("logcat", "-d", "-s", "ReactNativeJS:*")
        for row in str(log).splitlines()[-40:]:
            print("RNLOG " + row[:240], file=sys.stderr, flush=True)


def scenario(device, origin, remote):
    token = os.environ["NU54_K10_TOKEN"]

    def call(path, body=None, *, timeout=4):
        with urlopen(Request(origin + path, data=body or b"", method="POST",
                             headers={"Authorization": "Bearer " + token}), timeout=timeout) as response:
            return json.load(response)

    ports = [f"tcp:{remote.port}"]
    device.call("reverse", ports[0], ports[0])
    try:
        input_doc = call("/enroll", device.public_key().encode())
        profile = json.loads(input_doc["registration"])
        endpoint = urlsplit(profile["endpoint"])
        assert endpoint.hostname == "127.0.0.1" and endpoint.port
        ports.append(f"tcp:{endpoint.port}")
        device.call("reverse", ports[-1], ports[-1])
        device.enroll(profile, input_doc["pin"], input_doc["presentationPin"])
        device.find("구독 관리")
        print("W10 registered customer app", flush=True)

        if os.environ.get("NU54_W10_RENTER_INSTALLED") == "1":
            renter_decisions(device, call, input_doc["branch"])
            return

        def shop():
            device.launch()
            device.tap("가까운 매장에서 주문")
            device.tap("근처 단말 찾기")
            device.tap("근거리 단말 후보 1 인증")
            device.tap("이 매장 메뉴 열기")
            device.find("여울 커피 성수점 메뉴")

        shop()
        device.tap("Iced tea cold 추가")
        device.tap("차 따뜻함 추가")
        call("/arm")
        device.tap("주문 접수 요청")
        device.find("발급 여부 미확인")
        print("W10 observed the lost original quote reply", flush=True)
        shop()
        device.tap("원래 요청 재조회")
        device.find("Iced tea · cold ×1 · 8.00 TestUSDC")
        device.find("차 · 따뜻함 ×1 · 7.00 TestUSDC")
        device.find("가스 비용 · 매장 부담")
        device.find("등록된 릴레이 " + input_doc["relay"])
        device.find("인증한 가맹점 견적", contains=True)
        device.find("검토한 주문 ", contains=True)
        summary = next(
            (node.get("text", "") for node in device.tree()
             if "검토한 주문 " in node.get("text", "") and "15.00 TestUSDC" in node.get("text", "")), "")
        original = re.search(r"검토한 주문 ([a-f0-9-]{36})", summary)[1]
        device.tap("원견적 상세")
        device.find("수령 주소 0x0000000000000000000000000000000000003001")
        device.tap("견적 확인으로")
        if device.find("같은 견적의 가스 부담 다시 확인", attempts=1, optional=True):
            device.tap("같은 견적의 가스 부담 다시 확인")
        device.tap("검증한 보드 찾기")
        device.tap("검증 보드에서 결제 검토")
        device.find("보드가 원견적을 검증했습니다")
        print("W10 observed authenticated board review", flush=True)
        cutoff = time.monotonic() + 6
        while True:
            try:
                call("/owner/sw2")
                break
            except HTTPError:
                if time.monotonic() >= cutoff:
                    raise
                time.sleep(0.15)
        device.find("보드에서 결제를 거절했습니다.", contains=True)
        assert not device.find("지급 요청 제출", attempts=2, optional=True)
        device.tap("같은 주문의 지급 결과 확인")
        device.find(original, contains=True)
        assert not device.find("보드 서명 확인", attempts=1, optional=True)
        print("PASS Android sponsored original review, C board SW2 and same-order status", flush=True)
    finally:
        for port in reversed(ports):
            with contextlib.suppress(subprocess.SubprocessError):
                device.call("reverse", "--remove", port)


def renter_decisions(device, call, branch):
    def home():
        device.launch()
        device.tap("내 결제")
        device.find("구독 관리")

    def discover():
        device.tap("근처 단말 찾기")
        device.tap("근거리 단말 후보 1 인증")
        device.tap("이 매장 메뉴 열기")
        # The board review needs the terminal link; a direct online menu cannot reach the board.
        if not device.find("온라인 단말 중계", attempts=3, optional=True):
            raise RuntimeError("W10 renter menu opened without the nearby terminal relay")

    def review():
        device.tap("이 주문의 결제 내용 검토")
        # Sponsorship and escrow proofs last at most 30 s; a slow emulator must request them again.
        if device.find("같은 견적의 후원과 예치금 다시 확인", attempts=1, optional=True):
            device.tap("같은 견적의 후원과 예치금 다시 확인")
        device.tap("대여 보드 원견적 검토 확인")
        device.find("보드가 원견적을 검증했습니다")
        labels = {node.get("text") for node in device.tree() if node.get("package") == device.package}
        assert {"결제 원천 · 대여 지갑 예치금", "가스 비용 · 매장 부담"} <= labels

    qr = call("/rental/original", b"QR")["sale"]
    home()
    locator = f"nu54://order?v=2&branch={branch}&order={qr}&expires={int(time.time()) + 55}"
    subprocess.run([*device.command, "shell", "-T",
                    f"run-as {CLIP} sh -c 'mkdir -p files && cat > files/fixture.txt'"],
                   input=locator.encode(), check=True, stdout=subprocess.DEVNULL, timeout=20)
    device.call("shell", "am", "start", "-n", CLIP + "/.Clip")
    device.find("매장 주문 QR 내용", field=True)
    device.call("shell", "input", "keyevent", "KEYCODE_PASTE")
    device.call("shell", "input", "keyevent", "KEYCODE_BACK")
    discover()
    device.tap("QR 주문 연결")
    review()
    call("/rental/sw1")
    device.tap("대여 보드 결정 확인")
    device.find("보드에서 이 원견적의 지급을 승인했습니다.", contains=True)
    call("/rental/arm", qr.encode())
    device.tap("지급 요청 제출")
    device.find("네트워크 응답을 기다립니다.", contains=True)
    original_hash = call("/rental/broadcast", qr.encode(), timeout=45)["hash"]
    home()
    device.tap(f"매장 주문 {qr}")
    device.tap("같은 주문의 지급 결과 확인")
    device.find(original_hash, contains=True)
    assert not device.find("영수증 번호", contains=True, attempts=1, optional=True)
    assert call("/rental/finalize", qr.encode(), timeout=45)["hash"] == original_hash
    device.tap("같은 주문의 지급 결과 확인")
    device.find("영수증 번호", contains=True)
    device.find(original_hash, contains=True)
    print("PASS Android W10 renter QR SW1, lost C3, original W11 pending and final hash", flush=True)

    w09 = call("/rental/original", b"W09")["sale"]
    home()
    discover()
    device.tap(f"매장 주문 {w09}")
    review()
    call("/rental/sw2")
    device.tap("대여 보드 결정 확인")
    device.find("보드에서 이 원견적의 지급을 거절했습니다.", contains=True)
    assert not device.find("지급 요청 제출", attempts=1, optional=True)
    call("/rental/refused", w09.encode())
    home()
    device.tap(f"매장 주문 {w09}")
    device.find(w09, contains=True)
    print("PASS Android W10 renter W09 SW2, original remains, zero settlement", flush=True)


if __name__ == "__main__":
    main()
