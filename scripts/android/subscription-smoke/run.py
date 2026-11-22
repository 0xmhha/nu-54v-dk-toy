#!/usr/bin/env python3
"""Drive a shipping P02 APK on a disposable Android emulator against the loopback P05 lab."""
import argparse
import base64
import contextlib
import json
import os
from pathlib import Path
import re
import subprocess
import time
from urllib.parse import urlsplit
import xml.etree.ElementTree as ET

CLIP = "org.nu54.qa.subscriptionclip"
REGISTRATION = (
    "고객 주문 등록서",
    "독립 확인한 주문 서버 지문",
    "별도 발급한 비공개 주문 서명 지문",
)


def await_file(path, timeout=180):
    until = time.monotonic() + timeout
    while time.monotonic() < until:
        if (path.parent / "ui-aborted").exists():
            raise RuntimeError("paired native fixture stopped")
        if path.is_file():
            return path.read_text().strip()
        time.sleep(0.25)
    raise RuntimeError(f"native fixture did not publish {path.name}")


def atomic_json(path, data):
    temp = path.with_suffix(".tmp")
    temp.write_text(json.dumps(data, separators=(",", ":")))
    temp.replace(path)


class Android:
    def __init__(self, adb, serial, package):
        self.command = [str(adb), "-s", serial]
        self.package = package
        if not serial.startswith("emulator-") or self.call("shell", "getprop", "ro.kernel.qemu").strip() != "1":
            raise ValueError("use an isolated Android emulator")
        self.width, self.height = map(int, re.findall(r"(\d+)x(\d+)", self.call("shell", "wm", "size"))[-1])

    def call(self, *args):
        return subprocess.check_output([*self.command, *map(str, args)], timeout=120).decode()

    def launch(self):
        self.call("shell", "am", "force-stop", self.package)
        self.call("shell", "am", "start", "-n", self.package + "/com.nu54renter.MainActivity")

    def tree(self):
        for _ in range(5):
            raw = self.call("exec-out", "uiautomator", "dump", "/dev/tty")
            if "</hierarchy>" in raw:
                return ET.fromstring(raw.split("</hierarchy>", 1)[0] + "</hierarchy>").iter("node")
            time.sleep(0.25)
        raise RuntimeError("Android accessibility hierarchy unavailable")

    def find(self, label, *, tap=False, contains=False, attempts=10, optional=False, field=False):
        for attempt in range(attempts):
            for node in self.tree():
                if node.get("package") != self.package or (field and node.get("class") != "android.widget.EditText"):
                    continue
                names = (node.get("text", ""), node.get("content-desc", ""))
                if not any(label in name if contains else label == name for name in names):
                    continue
                x, y, right, bottom = map(int, re.findall(r"-?\d+", node.get("bounds")))
                if min(right - x, bottom - y) < 8:
                    continue
                if tap or field:
                    if not field and (node.get("clickable") != "true" or node.get("enabled") != "true"):
                        continue
                    self.call("shell", "input", "tap", (x + right) // 2, (y + bottom) // 2)
                    time.sleep(0.3)
                return True
            if attempt + 1 < attempts:
                start, end = (0.73, 0.35) if attempt < attempts // 2 else (0.34, 0.75)
                self.call("shell", "input", "swipe", self.width // 2, int(self.height * start),
                          self.width // 2, int(self.height * end), 250)
        if optional:
            return False
        raise RuntimeError("missing expected synthetic reference" if re.fullmatch(r"[\da-f-]{36}", label)
                           else f"missing native UI element: {label}")

    def tap(self, label):
        self.find(label, tap=True)

    def enroll(self, profile, tls_pin, presentation_pin):
        self.launch()
        self.tap("내 결제")
        for label, value in zip(REGISTRATION, (json.dumps(profile, separators=(",", ":")), tls_pin, presentation_pin)):
            subprocess.run([*self.command, "shell", "-T",
                            f"run-as {CLIP} sh -c 'mkdir -p files && cat > files/fixture.txt'"],
                           input=value.encode(), check=True, stdout=subprocess.DEVNULL, timeout=20)
            self.call("shell", "am", "start", "-n", CLIP + "/.Clip")
            time.sleep(0.7)
            self.find(label, field=True)
            self.call("shell", "input", "keyevent", "KEYCODE_PASTE")
            self.call("shell", "input", "keyevent", "KEYCODE_BACK")
        self.tap("주문 등록 확인")

    def public_key(self):
        self.launch()
        self.tap("내 결제")
        self.tap("주문 등록 공개키 보기")
        for _ in range(6):
            for node in self.tree():
                key = node.get("text", "")
                if node.get("package") == self.package and re.fullmatch(r"MFkwEwYH[A-Za-z0-9+/=]{112,124}", key):
                    if len(base64.b64decode(key, validate=True)) == 91:
                        return key
            self.call("shell", "input", "swipe", self.width // 2, self.height * 3 // 4,
                      self.width // 2, self.height * 2 // 5, 250)
        raise RuntimeError("normal enrollment did not expose a P-256 public key")

    def deny_notification(self):
        self.tap("회차 알림 허용")
        for _ in range(6):
            for node in self.tree():
                if "permissioncontroller" not in node.get("package", "") or not node.get("resource-id", "").endswith("permission_deny_button"):
                    continue
                x, y, right, bottom = map(int, re.findall(r"\d+", node.get("bounds")))
                self.call("shell", "input", "tap", (x + right) // 2, (y + bottom) // 2)
                self.find("알림을 허용하지 않았습니다. 미결 목록은 사용할 수 있습니다.")
                return
        raise RuntimeError("fresh OS notification permission dialog not displayed")

    def schedule(self, *, restart=False):
        if restart:
            self.launch()
            self.tap("내 결제")
        self.tap("구독 관리")


def terms(device, fixture):
    device.schedule()
    device.tap("도시 산책 패스 구독 조건 보기")
    device.find("회차 상한 4.00 TestUSDC", contains=True)
    device.find("회차마다 보드에서 직접 승인합니다.")
    device.tap("조건에 동의")
    device.find("구독 중")
    (fixture / "accepted").touch()
    original = await_file(fixture / "original.json")
    device.tap("일시정지")
    device.find("구독 재개")
    (fixture / "paused").touch()
    await_file(fixture / "pause-verified")
    device.tap("구독 재개")
    device.find("구독 중")
    (fixture / "resumed").touch()
    await_file(fixture / "offered")
    device.tap("구독 목록 새로고침")
    device.find("새 약관에 동의하기 전 미래 청구는 보류됩니다.")
    device.tap("변경 조건 확인 · v3")
    device.find("회차 상한 5.00 TestUSDC", contains=True)
    device.find("도시 산책 패스 · v2 · 4.00 TestUSDC", contains=True)
    (fixture / "reviewing-change").touch()
    await_file(fixture / "consent-ready")
    device.tap("조건에 동의")
    device.find("도시 산책 패스 · v3 · 5.00 TestUSDC", contains=True)
    (fixture / "changed").touch()
    await_file(fixture / "change-verified")
    device.tap("해지 검토")
    device.find("미래 회차를 해지합니다. 기존 발급 견적은 계속 조회할 수 있습니다.")
    device.tap("돌아가기")
    device.tap("해지 검토")
    device.tap("미래 회차 해지")
    device.schedule(restart=True)
    device.find("해지됨")
    device.find("도시 산책 패스 · v3 · 5.00 TestUSDC", contains=True)
    opened = device.find("같은 회차 결제 검토", tap=True, attempts=2, optional=True)
    if not opened:
        device.tap("기존 회차 재조회")
    device.find(json.loads(original)["sale"])
    device.find("약관 v2", contains=True)
    (fixture / "finished").touch()
    await_file(fixture / "result.json")


def preview(device, fixture, sale):
    for attempt in range(2):
        device.schedule(restart=attempt != 0)
        device.find("같은 회차 결제 검토")
        if attempt == 0:
            device.deny_notification()
        device.tap("같은 회차 결제 검토")
        device.find(sale)
        device.find("보드 요약 검토")
        assert not device.find("서버 지급 확인", attempts=1, optional=True)
    (fixture / "ui-completed").touch()


def lifecycle(device, fixture, sale):
    assert await_file(fixture / "included") == sale
    device.schedule(restart=True)
    device.tap("지급 결과 조회")
    device.find(sale)
    device.find("지급 결과 확인 중")
    assert not device.find("보드 요약 검토", attempts=1, optional=True)
    (fixture / "included-seen").touch()
    assert await_file(fixture / "confirmed") == sale
    device.schedule(restart=True)
    device.find("지급 확정", contains=True)
    device.tap("기존 회차 보기")
    device.find(sale)
    device.find("서버 지급 확인")
    device.find(await_file(fixture / "receipt-ref"), contains=True)
    assert not device.find("픽업 번호", contains=True, attempts=1, optional=True)
    (fixture / "confirmed-seen").touch()

    skipped = await_file(fixture / "skip-open")
    assert skipped != sale
    device.schedule(restart=True)
    device.tap("같은 회차 결제 검토")
    device.find(skipped)
    device.tap("구독 일정")
    device.tap("회차 건너뛰기")
    device.find("종료", contains=True)
    device.schedule(restart=True)
    device.find("종료", contains=True)
    (fixture / "skip-open-seen").touch()

    unpaid = await_file(fixture / "reconcile")
    assert unpaid not in {sale, skipped}
    device.schedule(restart=True)
    device.tap("기존 회차 재조회")
    device.find(unpaid)
    device.find("기한 내 지급 여부를 대사 중입니다.", contains=True)
    device.find("서명 견적 기한", contains=True)
    assert not device.find("보드 요약 검토", attempts=1, optional=True)
    (fixture / "reconcile-seen").touch()
    assert await_file(fixture / "expired") == unpaid
    device.tap("기존 회차 재조회")
    device.find("견적 만료가 확정되었습니다.", contains=True)
    device.find(unpaid)
    (fixture / "expired-seen").touch()
    await_file(fixture / "lifecycle-report.json")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("serial", help="ADB serial of a fresh API 33+ emulator")
    parser.add_argument("package", help="installed isolated P02 package")
    parser.add_argument("clipboard", type=Path, help="APK from build-clipboard.sh")
    parser.add_argument("fixture", type=Path, help="private fixture directory with mode 0700")
    parser.add_argument("--mode", choices=("preview", "terms", "lifecycle"), default="preview")
    parser.add_argument("--publish-key-only", action="store_true", help="collect the public SPKI before a mixed native run")
    parser.add_argument("--adb", default="adb")
    args = parser.parse_args()
    os.umask(0o077)
    fixture = args.fixture.resolve()
    fixture.mkdir(mode=0o700, parents=True, exist_ok=True)
    completed = ("ui-completed", "ui-aborted", "finished", "lifecycle-report.json")
    if fixture.stat().st_mode & 0o077 or any((fixture / name).exists() for name in completed):
        parser.error("use a fresh private lab directory")
    if not re.fullmatch(r"com\.nu54renter\.[A-Za-z0-9_.]+", args.package):
        parser.error("use an isolated P02 app id suffix")
    device = Android(args.adb, args.serial, args.package)
    if not (fixture / "public-keys.json").exists():
        atomic_json(fixture / "public-keys.json", {"android": device.public_key()})
    if args.publish_key_only:
        return
    if f"package:{CLIP}" in device.call("shell", "pm", "list", "packages", CLIP).splitlines():
        parser.error("use a fresh isolated emulator without the clipboard fixture installed")
    installed = False
    port = None
    try:
        doc = json.loads(await_file(fixture / "preview.json", timeout=660))
        profile = doc["profiles"]["android"]
        endpoint = urlsplit(profile["endpoint"])
        if endpoint.scheme != "https" or endpoint.hostname != "127.0.0.1" or endpoint.path != "/v2/commerce" or profile["payment"]["chain"] != 8283:
            raise ValueError("expected the loopback Anvil P05 fixture")
        port = f"tcp:{endpoint.port}"
        device.call("reverse", port, port)
        device.call("install", args.clipboard)
        installed = True
        device.enroll(profile, doc["tlsPin"], doc["presentationPin"])
        if args.mode == "terms":
            terms(device, fixture)
        else:
            sale = doc["invoice"]["saleRef"]
            preview(device, fixture, sale)
            if args.mode == "lifecycle":
                lifecycle(device, fixture, sale)
        print(f"PASS Android native subscription {args.mode}")
    except Exception:
        (fixture / "ui-aborted").touch()
        raise
    finally:
        if port:
            with contextlib.suppress(subprocess.SubprocessError):
                device.call("reverse", "--remove", port)
        if installed:
            with contextlib.suppress(subprocess.SubprocessError):
                device.call("uninstall", CLIP)


if __name__ == "__main__":
    main()
