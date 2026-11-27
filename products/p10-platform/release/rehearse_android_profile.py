#!/usr/bin/env python3
"""Observe an Android practice-profile update and PackageManager rollback."""

import argparse
import re
from pathlib import Path
import time
import xml.etree.ElementTree as ET

from android_profile import CONTRACT, FORMAT, STEPS, TTL
from evidence import digest, require
from fleet import key_identity, sign, signature
from observe import android_apk, observe, run
from recover import save

class PracticeApp:
    def __init__(self, adb, device, package):
        self.adb = adb
        self.device = device
        self.package = package
        size = re.findall(r"(\d+)x(\d+)", self.call("shell","wm","size"))
        require(size,"Android display size unavailable")
        self.width, self.height = map(int,size[-1])

    def call(self,*args):
        return run(self.adb,"-s",self.device,*args)

    def launch(self):
        self.call("shell","am","force-stop",self.package)
        self.call("shell","am","start","-W","-n",self.package+"/com.nu54renter.MainActivity")
        time.sleep(2)

    def find(self,label,*,tap=False,contains=False,attempts=16,optional=False):
        for index in range(attempts):
            raw = self.call("exec-out","uiautomator","dump","/dev/tty")
            stop = raw.find("</hierarchy>")
            require(stop>=0,"Android rendered hierarchy unavailable")
            tree = ET.fromstring(raw[:stop+len("</hierarchy>")])
            require(not any("Play Protect" in (node.get("text") or "") for node in tree.iter("node")),
                    "Android Play Protect requires manual enrollment")
            for node in tree.iter("node"):
                if node.get("package")!=self.package:
                    continue
                labels=(node.get("content-desc",""),node.get("text",""))
                if not any(label in value if contains else label==value for value in labels):
                    continue
                left,top,right,bottom=map(int,re.findall(r"-?\d+",node.get("bounds","")))
                x,y=(left+right)//2,(top+bottom)//2
                if min(right-left,bottom-top)<9 or not .03*self.height < y < .97*self.height:
                    continue
                if tap:
                    if node.get("clickable")!="true" or node.get("enabled")!="true":
                        continue
                    self.call("shell","input","tap",x,y)
                    time.sleep(.5)
                return True
            high,low=round(self.height*.77),round(self.height*.34)
            first,last=(high,low) if index<attempts//2 else (low,high)
            self.call("shell","input","swipe",self.width//2,first,self.width//2,last,350)
            time.sleep(.35)
        if optional:
            return False
        raise ValueError("required Android practice action or result absent")

    def tap(self,label):
        self.find(label,tap=True)

    def source_write(self):
        self.launch()
        self.tap("결제 연습")
        self.tap("틀린 금액 찾아보기")
        self.find("비교 화면: 연습 카페, 5 연습 코인.",contains=True)
        self.tap("나중에 이어하기")
        self.launch()
        self.tap("연습 이어하기")
        self.find("비교 화면: 연습 카페, 5 연습 코인.",contains=True)
        self.tap("나중에 이어하기")

    def update(self):
        self.launch()
        self.tap("연습 이어하기")
        self.find("비교 화면: 연습 카페, 5 연습 코인.",contains=True)
        self.tap("연습 거절")
        self.find("연습을 거절했습니다")
        self.find("실제 지급 0건 · 서명 0건")
        self.tap("다른 연습 선택")
        self.tap("나중에 이어하기")
        self.tap("English")
        self.tap("Payment practice")
        self.tap("Compare matching amounts")
        self.find("Comparison display: Practice Café, 2 practice coins.",contains=True)
        self.tap("Continue later")
        self.launch()
        self.tap("Resume practice")
        self.find("Comparison display: Practice Café, 2 practice coins.",contains=True)
        self.tap("Continue later")

    def rollback(self):
        self.launch()
        self.tap("Resume practice")
        self.find("Comparison display: Practice Café, 2 practice coins.",contains=True)
        self.tap("Approve practice")
        self.find("Practice approved")
        self.find("Real payments: 0 · Signatures: 0")


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for arg in ("baseline","upgrade","aapt","apksigner","output","key","public"):
        parser.add_argument("--"+arg,type=lambda v:Path(v).resolve(),required=True)
    parser.add_argument("--adb",default="adb")
    parser.add_argument("--device",required=True)
    parser.add_argument("--preinstalled-baseline",action="store_true",
                        help="use the exact preinstalled baseline APK on an unused QA profile")
    args=parser.parse_args()
    base,new=(android_apk(file,args.aapt,args.apksigner) for file in (args.baseline,args.upgrade))
    package=base["package"]
    require(re.fullmatch(r"com\.nu54renter\.fleetqa\.r[0-9a-f]{16}",package),"Android rehearsal-only package required")
    require(new["package"]==package and base["certificate"]==new["certificate"] and new["versionCode"]>base["versionCode"]
            and new["rollbackDataPolicy"]=="retain","Android existing-profile hold: incompatible package, signer or data policy")
    require(re.fullmatch(r"emulator-[0-9]+",args.device),"Android dedicated emulator required")
    app=PracticeApp(args.adb,args.device,package)
    require(app.call("shell","getprop","ro.kernel.qemu")=="1","Android physical target outside this rehearsal")
    require(app.call("shell","getprop","ro.build.version.sdk")=="34","Android profile rehearsal is enrolled for API 34")
    listed=app.call("shell","pm","list","packages",package)
    require(listed==("package:"+package if args.preinstalled_baseline else ""),
            "unexpected QA installation; preserve the existing app")
    args.output.mkdir(mode=0o700)
    rows=[{"mode":"android-emulator","payload":digest(file),"version":apk["versionName"],
           "dimensions":{"os":"android-api-34"},**apk} for file,apk in ((args.baseline,base),(args.upgrade,new))]
    target={"package":package,"device":args.device,"adb":args.adb,"aapt":str(args.aapt),"apksigner":str(args.apksigner)}
    operation="observe-installed-baseline" if args.preinstalled_baseline else "install"
    phases=((args.baseline,rows[0],operation,None,app.source_write),
            (args.upgrade,rows[1],"enable-rollback",rows[0],app.update),
            (args.baseline,rows[0],"os-rollback",rows[1],app.rollback))
    results={}
    app.call("shell","wm","dismiss-keyguard")
    for step,(file,row,operation,source,action) in zip(STEPS,phases,strict=True):
        start=time.monotonic()
        installed=observe(file,row,{**target,"androidOperation":operation,"androidSource":source})
        action()
        record=args.output/(step+".json")
        save(record,{"observation":installed,"stage":step,"passed":True,
                     "seconds":round(time.monotonic()-start,3),"realPayments":0,"assetSignatures":0})
        results[step]={"passed":True,"result":digest(record)}
        print(f"PASS {step}",flush=True)
    now=int(time.time())
    claim={"format":FORMAT,"contract":CONTRACT,"source":rows[0]["payload"],"target":rows[1]["payload"],
           "package":package,"os":"android-api-34","certificate":base["certificate"],"rollbackDataPolicy":"retain",
           "observedAt":now,"expiresAt":now+TTL,"steps":results}
    signed=sign(claim,args.key)
    signature(claim,signed,args.public)
    save(args.output/"proof.json",{"claim":claim,"signer":key_identity(args.public),"signature":signed})


if __name__=="__main__":
    main()
