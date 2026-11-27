#!/usr/bin/env python3
"""Exercise W04 permission denial and explicit retry in a disposable installed package."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time


def load_driver():
    file = Path(__file__).resolve().parents[1] / "subscription-smoke" / "run.py"
    spec = importlib.util.spec_from_file_location("android_ui", file)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.Android


def os_control(ui, *, suffix=None, label=None, package):
    for _ in range(8):
        for node in ui.tree():
            if node.get("package") != package:
                continue
            if suffix and not node.get("resource-id", "").endswith(suffix):
                continue
            if label and node.get("text") != label:
                continue
            x, y, right, bottom = map(int, re.findall(r"\d+", node.get("bounds")))
            ui.call("shell", "input", "tap", (x + right) // 2, (y + bottom) // 2)
            return
        time.sleep(0.4)
    raise RuntimeError(f"missing Android system control {suffix or label}")


def granted(ui):
    result = ui.call("shell", "dumpsys", "package", ui.package)
    return {
        name: re.search(r"android.permission.BLUETOOTH_" + name + r": granted=true", result) is not None
        for name in ("SCAN", "CONNECT")
    }


CLIPBOARD_PACKAGE = "org.nu54.qa.subscriptionclip"


def paste_fixture(ui, value):
    subprocess.run([*ui.command, "shell", "-T",
                    f"run-as {CLIPBOARD_PACKAGE} sh -c 'mkdir -p files && cat > files/fixture.txt'"],
                   input=value.encode(), check=True, timeout=20, stdout=subprocess.DEVNULL)
    ui.call("shell", "am", "start", "-n", CLIPBOARD_PACKAGE + "/.Clip")
    time.sleep(0.7)
    ui.call("shell", "input", "keyevent", "KEYCODE_PASTE")


def verify_fields(ui, values, stage):
    matched = set()
    for _ in range(7):
        for node in ui.tree():
            if node.get("package") != ui.package or node.get("class") != "android.widget.EditText":
                continue
            # Read accessibility values even when small bounds make them unsuitable for a tap.
            matched.update(value for value in values if value in (node.get("text"), node.get("content-desc")))
        if len(matched) == len(values):
            return
        time.sleep(0.5)
    raise RuntimeError(f"W04 enrollment fields {stage}: {len(matched)}/{len(values)} matched")


def scenario(ui):
    ui.launch()
    ui.tap("설치된 보드 연결")
    # Public synthetic document. There is no board radio counterpart in this OS test.
    device, pin = "34" * 32, "25" * 32
    document = dict(version=2, device=device, boardSPKI=pin, ca="MA==", certificateChain=["MA=="])
    encoded = json.dumps(document, separators=(",", ":"))
    for label, value in (
        ("발급 등록서", encoded),
        ("기기 식별값", device),
        ("발급 문서의 서버 공개키 지문", pin),
    ):
        ui.find(label, field=True)
        paste_fixture(ui, value)
        ui.call("shell", "input", "keyevent", "KEYCODE_BACK")
    verify_fields(ui, (encoded, device, pin), "before denial")
    ui.tap("개인 기기 찾기")
    controller = "com.google.android.permissioncontroller"
    os_control(ui, suffix=":id/permission_deny_button", package=controller)
    assert not any(granted(ui).values()), "permission was granted on the denial path"
    ui.find("주변 기기 권한이 필요합니다", contains=True)
    assert not ui.find("기기 등록 시작", attempts=1, optional=True)
    ui.tap("앱 권한 설정 열기")
    os_control(ui, label="Permissions", package="com.android.settings")
    os_control(ui, label="Nearby devices", package=controller)
    os_control(ui, label="Allow", package=controller)
    assert all(granted(ui).values()), "OS Settings did not grant both Nearby devices permissions"
    for _ in range(3):
        ui.call("shell", "input", "keyevent", "KEYCODE_BACK")
    verify_fields(ui, (encoded, device, pin), "after Settings")
    ui.tap("개인 기기 검색 다시 시도")
    if not ui.find("기기 검색을 마쳤습니다", contains=True, attempts=8, optional=True):
        if ui.find("등록 정보나 블루투스 권한을 확인", contains=True, attempts=4, optional=True):
            if ui.find("발급 등록서", field=True, attempts=4, optional=True):
                raise RuntimeError("public enrollment was not accepted after Settings returned")
            raise RuntimeError("native Bluetooth scan failed after Settings returned")
        nodes = list(ui.tree())
        print(json.dumps({"stage": "scan-completion", "fieldLengths": [
            len(n.get("text", "")) for n in nodes if n.get("class") == "android.widget.EditText"],
            "appVisible": any(n.get("package") == ui.package for n in nodes),
            "statusLabels": [name for name in ("개인 기기 찾기", "지갑 기록", "권한", "기기 검색", "키 보호")
                             if any(name in (n.get("text", "") + n.get("content-desc", "")) for n in nodes)]}),
              file=sys.stderr, flush=True)
        raise RuntimeError("native scan completion was not visible")
    assert not ui.find("앱 권한 설정 열기", attempts=1, optional=True)
    print(json.dumps({"osDenied": True, "settingsGrantedScanAndConnect": True,
                      "sameEnrollmentResumed": True, "nativeScanCompleted": True,
                      "physicalBoardRegistrationMeasured": False}))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("serial")
    parser.add_argument("apk", type=Path)
    args = parser.parse_args()
    sdk = Path(os.environ["ANDROID_HOME"])
    package = "com.nu54renter.w04permissionqa"
    aapt = next(iter(sorted(sdk.glob("build-tools/*/aapt"), reverse=True)))
    metadata = subprocess.check_output([aapt, "dump", "badging", args.apk], text=True)
    if not re.search(r"^package: name='" + re.escape(package) + r"'", metadata, re.M):
        raise ValueError("APK is not the disposable W04 package")
    ui = load_driver()(sdk / "platform-tools/adb", args.serial, package)
    if ui.call("shell", "pm", "list", "packages", package).strip():
        raise RuntimeError("disposable QA package is already installed")
    if ui.call("shell", "pm", "list", "packages", CLIPBOARD_PACKAGE).strip():
        raise RuntimeError("disposable clipboard helper is already installed")
    builder = Path(__file__).resolve().parents[1] / "subscription-smoke" / "build-clipboard.sh"
    with tempfile.TemporaryDirectory(prefix="nu54-w04-clipboard-") as directory:
        clipboard = Path(directory) / "clipboard.apk"
        subprocess.run(["bash", builder, aapt.parent, clipboard], check=True, stdout=subprocess.DEVNULL)
        ui.call("install", clipboard)
        try:
            ui.call("install", args.apk)
            try:
                scenario(ui)
            finally:
                ui.call("uninstall", package)
        finally:
            ui.call("uninstall", CLIPBOARD_PACKAGE)


if __name__ == "__main__":
    main()
