"""Verify the P05 POS fixture through a separately installed Android kiosk."""

import argparse
import atexit
import base64
import json
import pathlib
import re
import shlex
import signal
import subprocess
import time
import urllib.parse
import xml.etree.ElementTree as ET

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("lab", type=pathlib.Path)
parser.add_argument("--serial", required=True)
parser.add_argument("--package", default="com.nu54kiosk")
parser.add_argument(
    "--prepare", action="store_true", help="Collect the kiosk registration public key and exit."
)
args = parser.parse_args()
if not re.fullmatch(r"emulator-[0-9]+", args.serial):
    parser.error("provide the serial of a dedicated local Android emulator")
if not re.fullmatch(r"com\.nu54kiosk(?:\.[a-z][a-z0-9_]*)*", args.package):
    parser.error("provide the kiosk package or a kiosk test package suffix")
q = args.lab
adb = ["adb", "-s", args.serial]
package = args.package
activity = f"{package}/com.nu54kiosk.MainActivity"
MAX_FIELD_BYTES = 4096
MAX_FIXTURE_BYTES = 8192


def run(*arguments, output=False):
    result = subprocess.run([*adb, *arguments], check=True, capture_output=True)
    return result.stdout if output else None


def shell(*arguments):
    return run("shell", *arguments, output=True)


def nodes():
    try:
        dump = run(
            "exec-out", "uiautomator", "dump", "--compressed", "/proc/self/fd/1", output=True
        )
        end = dump.find(b"</hierarchy>")
        if end < 0:
            return []
        return [
            node.attrib
            for node in ET.fromstring(dump[: end + len(b"</hierarchy>")]).iter("node")
            if node.get("package") == package
        ]
    except (subprocess.CalledProcessError, ET.ParseError):
        return []


def wait(label, seconds=12, show=False, sensitive=False):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        for node in nodes():
            if label in (node.get("text"), node.get("content-desc")):
                return node
        if show:
            center = str(width // 2)
            shell(
                "input", "swipe", center, str(height * 3 // 4), center, str(height * 2 // 5), "330"
            )
        time.sleep(0.18)
    raise AssertionError(
        "A test value did not appear." if sensitive else f"Missing UI label: {label}"
    )


def tap(label, show=False, sensitive=False):
    node = wait(label, show=show, sensitive=sensitive)
    x1, y1, x2, y2 = map(int, re.findall(r"[0-9]+", node["bounds"]))
    shell("input", "tap", str((x1 + x2) // 2), str((y1 + y2) // 2))


def type_in(label, value):
    wait(label, show=True)
    encoded = value.encode()
    if not 0 < len(encoded) <= MAX_FIELD_BYTES:
        raise ValueError("Fixture field is outside the input limit.")
    payload = base64.b64encode(encoded).decode()
    command = shlex.join(
        [
            "am",
            "instrument",
            "-w",
            "--no-window-animation",
            "-e",
            "target",
            package,
            "-e",
            "label",
            label,
            "-e",
            "payload",
            payload,
            "local.poslab/.Input",
        ]
    )
    for _ in range(6):
        try:
            entered = b"POS_QA_INPUT_OK" in shell(command)
        except subprocess.CalledProcessError:
            entered = False
        if entered:
            time.sleep(0.25)
            return
        time.sleep(0.5)
        wait(label, show=True)
    raise RuntimeError(f"Could not enter value in {label}")


def read_fixture(name, seconds=220):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            with (q / name).open("rb") as fixture:
                data = fixture.read(MAX_FIXTURE_BYTES + 1)
            if len(data) > MAX_FIXTURE_BYTES:
                raise RuntimeError(f"Fixture exceeds file limit: {name}")
            return json.loads(data)
        except (FileNotFoundError, ValueError):
            time.sleep(0.2)
    raise RuntimeError(f"Fixture did not write {name}")


def snapshot(name):
    (q / name).write_bytes(run("exec-out", "screencap", "-p", output=True))


def cleanup(port):
    for command in (("reverse", "--remove", port), ("shell", "pm", "clear", package)):
        try:
            run(*command)
        except subprocess.CalledProcessError:
            print("POS_QA_DEVICE_CLEANUP_FAILED")


def stop(signum, _frame):
    raise SystemExit(128 + signum)


signal.signal(signal.SIGTERM, stop)
width, height = map(int, re.findall(r"(\d+)x(\d+)", shell("wm", "size").decode())[-1])
shell("am", "force-stop", package)
shell("am", "start", "-n", activity)
if args.prepare:
    tap("등록 공개키 확인")
    for _ in range(6):
        for node in nodes():
            match = re.fullmatch(r"[A-Za-z0-9+/]{122}==", node.get("text", ""))
            if match:
                (q / "terminal-spki.der").write_bytes(base64.b64decode(match.group()))
                raise SystemExit(0)
        time.sleep(0.5)
    raise RuntimeError("The kiosk did not display its registration public key.")

profile = read_fixture("terminal-profile.json")
upstream = urllib.parse.urlsplit(profile["endpoint"])
if upstream.hostname != "127.0.0.1" or upstream.scheme != "https" or not upstream.port:
    raise SystemExit("The fixture endpoint must be local HTTPS.")
port = f"tcp:{upstream.port}"
wait("매장 발급서", show=True)
atexit.register(cleanup, port)
blocked = signal.pthread_sigmask(signal.SIG_BLOCK, {signal.SIGINT, signal.SIGTERM})
try:
    run("reverse", port, port)
finally:
    signal.pthread_sigmask(signal.SIG_SETMASK, blocked)
type_in("매장 발급서", json.dumps(profile, separators=(",", ":")))
type_in("독립 확인한 서버 공개키 지문", profile["pin"])
tap("등록서 확인", show=True)
wait("새 고객 주문", 20)
print(
    "Android enrolled distinct terminal using app-generated Android Keystore key and pinned mTLS",
    flush=True,
)
order = read_fixture("orders.json")


def open_order(value, next_order=False):
    tap("다른 POS 원주문 열기" if next_order else "새 고객 주문", show=next_order)
    wait("POS 결제 전용")
    type_in("POS 인계 코드", value)
    tap("POS 원주문 조회", show=True)


open_order(order["handoff"])
wait("POS 주문 S-0142")
wait("금액 6.50 TestUSDC")
wait("이 주문을 여는 일회용 QR", show=True)
assert not any(n.get("content-desc") == "주문 접수 요청" for n in nodes())
snapshot("pending-preview.png")
(q / "preview-open").touch()
pairs = read_fixture("pairs.json")
tap("휴대전화 확인 번호 조회", show=True)
wait(f"확인 번호 {pairs['other']}와 일치", show=True, sensitive=True)
tap(f"확인 번호 {pairs['chosen']}와 일치", show=True, sensitive=True)
wait("선택한 지급자의 견적을 기록하고 있습니다")
(q / "locking-seen").touch()
assert not any(n.get("content-desc") == "이 주문을 여는 일회용 QR" for n in nodes())
try:
    wait("기존 견적 · 선택한 폰에서 결제를 계속하세요", show=True)
except AssertionError:
    for status in (
        "지급자 확인됨 · 견적 기록 대기",
        "결합 결과를 확인하지 못했습니다 · 같은 원주문을 조회하세요",
        "다른 지급자가 선택됐습니다",
        "기존 지급자 결합을 확인해야 합니다 · 운영자 확인 대기",
    ):
        if any(status in (n.get("text"), n.get("content-desc")) for n in nodes()):
            print(f"POS state after the issuance attempt: {status}", flush=True)
            break
    raise
wait(f"연결한 휴대전화 · {pairs['chosen']}", show=True, sensitive=True)
assert not any(n.get("content-desc") == "이 주문을 여는 일회용 QR" for n in nodes())
snapshot("quote-issued.png")
(q / "payer-bound").touch()
receipt = read_fixture("receipt.json")["receipt"]
tap("POS 원주문 다시 조회", show=True)
wait("서버 지급 확인")
wait(f"같은 영수증 {receipt}", sensitive=True)
wait("POS 주문 S-0142")
wait("금액 6.50 TestUSDC")
assert not any(n.get("content-desc") == "이 주문을 여는 일회용 QR" for n in nodes())
snapshot("settled-first-open.png")
shell("am", "force-stop", package)
shell("am", "start", "-n", activity)
wait("새 고객 주문")
assert not any(receipt in (n.get("text") or "") for n in nodes())
open_order(order["handoff"])
wait(f"같은 영수증 {receipt}", sensitive=True)
wait("POS 주문 S-0142")
wait("금액 6.50 TestUSDC")
snapshot("settled-after-restart.png")
other_order = read_fixture("competing-order.json")
open_order(other_order["handoff"], next_order=True)
wait("POS 주문 S-0143")
(q / "competing-open").touch()
competing = read_fixture("competing-pairs.json")
tap("휴대전화 확인 번호 조회", show=True)
wait(f"확인 번호 {competing['chosen']}와 일치", show=True, sensitive=True)
(q / "competing-visible").touch()
read_fixture("external-bound.json")
tap(f"확인 번호 {competing['chosen']}와 일치", sensitive=True)
wait("다른 지급자가 선택됐습니다", show=True)
assert not any(
    any(code in (n.get("text", "") + n.get("content-desc", "")) for code in competing.values())
    for n in nodes()
)
snapshot("competing-conflict.png")
(q / "competing-checked").touch()
(q / "ui-completed").write_text(
    "passed enrolled terminal, candidate selection, and exact original receipt after restart\n"
)
print(
    "Android selected one of two POS payers, hid public locator after binding, and reopened the exact Anvil-settled receipt after a process restart.",
    flush=True,
)
