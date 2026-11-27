#!/usr/bin/env python3
"""Exercise the normal K03 route on an enrolled, isolated Android release."""
import argparse
import json
from pathlib import Path
import re
from urllib.parse import urlsplit
from urllib.request import urlopen
from uuid import UUID, uuid4

from run import Android


def open_shop(device, branch):
    device.call("shell", "am", "force-stop", device.package)
    device.call("shell", "am", "start", "-a", "android.intent.action.VIEW",
                "-d", "nu54://shop/" + branch, "-p", device.package)


def deny_nearby(device):
    for _ in range(4):
        for node in device.tree():
            resource = node.get("resource-id", "").split("/")[-1]
            if "permissioncontroller" not in node.get("package", "") or resource not in {
                "permission_deny_button", "permission_deny_and_dont_ask_again_button",
            }:
                continue
            left, top, right, bottom = map(int, re.findall(r"\d+", node.get("bounds")))
            device.call("shell", "input", "tap", (left + right) // 2, (top + bottom) // 2)
            return
    raise RuntimeError("expected the Android Nearby permission prompt on a fresh enrollment")


def online(device, fixture, branch, other):
    device.launch()
    device.tap("가까운 매장에서 주문")
    assert not device.find("이 매장 메뉴 열기", attempts=1, optional=True)
    device.tap("근처 단말 찾기")
    deny_nearby(device)
    device.find("매장 입구의 QR을 스캔하세요")
    device.find("카메라로 매장 QR 확인")
    device.find("매장 입구 QR 주소", field=True)
    device.call("shell", "input", "text", "nu54://shop/" + branch)
    device.call("shell", "input", "keyevent", "KEYCODE_BACK")
    device.tap("매장 입구 QR 확인")
    device.find(f"{fixture['name']}에서 주문")

    open_shop(device, other)
    device.find("등록 지점의 매장 입구 QR을 확인하세요.")
    assert not device.find("이 매장 메뉴 열기", attempts=1, optional=True)

    device.launch()
    device.tap("가까운 매장에서 주문")
    device.find("간판의 지점명", field=True)
    device.call("shell", "input", "text", "Other%sbranch%s" + uuid4().hex)
    device.call("shell", "input", "keyevent", "KEYCODE_BACK")
    device.tap("온라인으로 매장 확인")
    device.find("입력한 지점명이 확인된 지점명과 다릅니다.")
    device.find("이 매장 메뉴 열기")
    for node in device.tree():
        if node.get("clickable") == "true" and "이 매장 메뉴 열기" in (
            node.get("text"), node.get("content-desc"),
        ):
            assert node.get("enabled") == "false"

    open_shop(device, branch)
    device.find(f"{fixture['name']}에서 주문")
    assert not device.find("간판의 지점명", attempts=1, optional=True)
    device.tap("이 매장 메뉴 열기")
    device.find(f"{fixture['name']} 메뉴")
    device.tap(fixture["menuItem"])
    device.tap("주문 접수 요청")
    device.find("주문이 연결되었습니다. 보드 승인 대기 중입니다.")


def offline(device, fixture, branch):
    original = str(UUID(fixture["originalSale"]))
    resume = urlsplit(fixture["resumeURL"])
    if resume.scheme != "http" or resume.hostname != "127.0.0.1" or resume.username or resume.password:
        raise ValueError("shop smoke restore control must be a loopback HTTP endpoint")
    open_shop(device, branch)
    device.find("새 주문을 시작할 수 없습니다")
    assert not device.find("이 매장 메뉴 열기", attempts=1, optional=True)
    assert not device.find("주문 접수 요청", attempts=1, optional=True)
    device.find("원래 주문 상태 확인")
    with urlopen(resume.geturl(), timeout=10) as response:
        assert response.read(128).decode().strip() == original
    device.tap("원래 주문 상태 확인")
    device.find(original)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("serial")
    parser.add_argument("phase", choices=("online", "offline"))
    parser.add_argument("fixture", type=Path)
    parser.add_argument("--adb", default="adb")
    args = parser.parse_args()
    fixture = json.loads(args.fixture.read_bytes())
    package = fixture["bundle"]
    if not re.fullmatch(r"com\.nu54renter\.[A-Za-z0-9_.]+", package):
        parser.error("the fixture must select an isolated P02 package")
    branch = str(UUID(fixture["branch"]))
    other = str(UUID(fixture["otherBranch"]))
    if branch == other:
        parser.error("expected two different fixture branches")
    device = Android(args.adb, args.serial, package)
    if args.phase == "online":
        online(device, fixture, branch, other)
    else:
        offline(device, fixture, branch)
    print(f"PASS K03 Android {args.phase}")


if __name__ == "__main__":
    main()
