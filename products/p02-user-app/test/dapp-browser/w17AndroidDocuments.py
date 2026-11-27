"""Drive only the isolated W17 Android app and system picker with synthetic files."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import time
import xml.etree.ElementTree as ET

APP = "com.nu54renter.wallet.sendqa"
TMP_DUMP = "/data/local/tmp/nu54-w17-ui.xml"


def run(args):
    def adb(*command):
        return subprocess.check_output([str(args.adb), "-s", args.device, *command], text=True, timeout=15)

    def shell(*command):
        return adb("shell", *command)

    def nodes():
        for _ in range(3):
            try:
                shell("uiautomator","dump","--compressed",TMP_DUMP)
                return list(ET.fromstring(shell("cat",TMP_DUMP)).iter("node"))
            except (subprocess.CalledProcessError,subprocess.TimeoutExpired,ET.ParseError):
                time.sleep(0.6)
        return []

    def labels(node):
        return (node.get("text",""),node.get("content-desc",""))

    def bounds(node):
        return tuple(map(int,re.findall(r"\d+",node.get("bounds",""))))

    def find(label, scroll=False, prefix=False, optional=False, attempts=9, resource=None, contains=()):
        for retry in range(attempts):
            for node in nodes():
                if (resource is None or node.get("resource-id")==resource) and any(
                        (value.startswith(label) if prefix else value==label) and
                        all(fragment in value for fragment in contains) for value in labels(node)):
                    box=bounds(node)
                    if node.get("enabled")=="true" and len(box)==4 and box[2]-box[0]>=10 and box[3]-box[1]>=10:
                        return node
            if scroll:
                size = re.findall(r"(\d+)x(\d+)",shell("wm","size"))[-1]
                width,height=map(int,size)
                marks = (0.76,0.46) if retry<5 else (0.45,0.77)
                shell("input","swipe",str(width//2),str(int(height*marks[0])),str(width//2),str(int(height*marks[1])),"340")
            else:
                time.sleep(0.35)
        if optional:
            return None
        stage="original transaction ID" if re.fullmatch(r"[0-9a-f]{64}",label) else label
        raise RuntimeError("Expected native action unavailable: "+stage)

    def tap(node):
        x0,y0,x1,y1=bounds(node)
        shell("input","tap",str((x0+x1)//2),str((y0+y1)//2))

    def click(label, scroll=True):
        tap(find(label,scroll=scroll))
        print("Advanced native control", label, flush=True)

    def launch():
        shell("am","force-stop",APP)
        shell("am","start","-n",APP+"/com.nu54renter.MainActivity")
        click("다른 체인 보내기")
        click("Bitcoin 공동서명")

    launch()
    click("개인 기기 찾기")
    click("공동서명 지갑 확인 · Loopback P01 demo board")
    click("보드에 공동서명 정책 등록")
    click("공동서명 PSBT 가져오기")
    click("PSBT 파일 또는 QR 이미지 선택")
    match=find(args.document,prefix=True,optional=True,attempts=2)
    if match is None:
        def downloads():
            return (find("Downloads",optional=True,attempts=1,resource="android:id/title") or
                    find("다운로드",optional=True,attempts=1,resource="android:id/title"))
        root=downloads()
        if root is None:
            roots=(find("Show roots",optional=True,attempts=1) or
                   find("루트 표시",optional=True,attempts=1))
            if roots is None:
                raise RuntimeError("System document root navigation unavailable")
            tap(roots)
            root=downloads()
        if root is None:
            raise RuntimeError("System Downloads root unavailable")
        tap(root)
        match=find(args.document,prefix=True)
    tap(match)
    click("PSBT 원문 검토")
    assert find("입력 UTXO",prefix=True,scroll=True) is not None
    assert find("자기 거스름돈",prefix=True,scroll=True) is not None
    if args.review:
        expected=json.loads(args.review.read_text())
        for item in expected["inputs"]:
            find("입력 UTXO",prefix=True,scroll=True,contains=(item["outpoint"],str(item["satoshis"])+" sat",item["scriptPubKey"]))
        for item in expected["allOutputs"]:
            change=item["changePathClaim"]
            values=(item["address"],str(item["satoshis"])+" sat",item["scriptPubKey"])
            if change:
                values+=("m/48'/1'/0'/2'/1/0",)
            find("자기 거스름돈" if change else "외부 출력",prefix=True,scroll=True,contains=values)
        find("입력 합계",prefix=True,scroll=True,contains=(str(expected["inputTotal"]),str(expected["outputTotal"])))
        find("수수료",prefix=True,scroll=True,contains=(str(expected["feeSatoshis"])+" sat",expected["minimumFeeRateSatPerVbyte"]+" sat/vB"))
    click("거절하고 초안으로")
    click("PSBT 원문 검토")
    click("보드에 부분 서명 요청")
    find("서명 1개.",prefix=True,scroll=True)
    original=args.original.read_text().strip()
    find(original,scroll=True)
    launch()
    find(original,scroll=True)
    click("원 거래 상태 재조회")
    find("원 거래가 확정되었습니다.",scroll=True)
    print("PASS one document, two reviewed inputs, protected original ID and independent local confirmation")


if __name__=="__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--adb",type=Path,required=True)
    parser.add_argument("--device",required=True)
    parser.add_argument("--document",required=True)
    parser.add_argument("--original",type=Path,required=True)
    parser.add_argument("--review",type=Path)
    run(parser.parse_args())
