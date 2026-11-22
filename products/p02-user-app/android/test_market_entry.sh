#!/usr/bin/env bash
# Exercise ordinary market entry in an isolated custom release install.
set -euo pipefail
adb_bin="${ANDROID_HOME:?Set ANDROID_HOME}/platform-tools/adb"
serial="${1:?Pass an isolated emulator serial}"
apk="${2:?Pass a signed custom release APK}"
expected_deployment=${3:-}
[[ "$serial" =~ ^emulator-[0-9]+$ ]]
[[ -z "$expected_deployment" || "$expected_deployment" =~ ^0x[0-9a-fA-F]{64}$ ]]
adb() { "$adb_bin" -s "$serial" "$@"; }
adb install -r "$apk"
adb shell am start -W -n com.nu54renter.custom/com.nu54renter.MainActivity >/dev/null
screen="/sdcard/nu54-market-entry-$$.xml"
dump() {
  adb shell uiautomator dump "$screen" >/dev/null
  adb shell cat "$screen"
}
trap 'adb shell rm -f "$screen" >/dev/null' EXIT
tap() {
  local position x y
  position=$(dump | python3 -c '
import re, sys, xml.etree.ElementTree as ET
for node in ET.parse(sys.stdin).iter("node"):
    if sys.argv[1] in {node.get("text"), node.get("content-desc")}:
        x1,y1,x2,y2 = map(int,re.findall(r"\d+", node.get("bounds", "")))
        print((x1+x2)//2, (y1+y2)//2)
        break
else:
    raise SystemExit("Missing market app control: " + sys.argv[1])' "$1")
  read -r x y <<<"$position"
  adb shell input tap "$x" "$y"
}
tap "테스트 시장 학습"
if [[ -n "$expected_deployment" ]]; then
  if dump | grep '전용 학습 지갑 0x' >/dev/null; then
    tap "가격과 잔액 확인"
    for _ in {1..8}; do
      if dump | grep 'LAB 0 · LAQ 0 · LP 0' >/dev/null; then
        echo "Android custom release retained the enrolled wallet and refreshed balances over pinned HTTPS."
        exit 0
      fi
      sleep 1
    done
    echo "Android release did not refresh balances over its pinned HTTPS deployment" >&2
    exit 1
  fi
  tap "학습 배포 블록 해시"
  adb shell input text "$expected_deployment"
  if adb shell dumpsys input_method | grep 'mInputShown=true' >/dev/null; then
    adb shell input keyevent KEYCODE_BACK
  fi
  tap "격리 학습 지갑 만들기"
  for _ in {1..8}; do
    if dump | python3 -c '
import sys, xml.etree.ElementTree as ET
labels = [node.get(key) or "" for node in ET.parse(sys.stdin).iter("node") for key in ("text","content-desc")]
sys.exit(0 if any(label.startswith("전용 학습 지갑 0x") for label in labels) else 1)'; then
      echo "Android custom release enrolled the market wallet through its pinned HTTPS test deployment."
      exit 0
    fi
    sleep 1
  done
  echo "Pinned deployment did not enroll the wallet" >&2
  exit 1
else
  dump | python3 -c '
import sys, xml.etree.ElementTree as ET
labels = {node.get(key) for node in ET.parse(sys.stdin).iter("node") for key in ("text","content-desc")}
assert "승인된 테스트 시장 배포 설정이 필요합니다." in labels, "Missing installed-deployment gate"
assert "격리 학습 지갑 만들기" not in labels, "Unconfigured release offered wallet setup"
print("Android custom release exposed market entry and blocked unconfigured wallet setup.")'
fi
