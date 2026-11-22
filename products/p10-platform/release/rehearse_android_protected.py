#!/usr/bin/env python3
"""Observe the protected C1 profile contract without a physical signing device."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import tempfile
import time
from zipfile import ZipFile

from android_profile import PROTECTED_CONTRACT as CONTRACT, PROTECTED_FORMAT as FORMAT, PROTECTED_STEPS as STEPS, TTL
from evidence import digest, read, require
from fleet import key_identity, sign, signature
from observe import android_apk, android_certificate, observe, run
from recover import save
from rehearse_android_profile import PracticeApp

PROJECT=Path(__file__).resolve().parents[3]


def profiles(apk_package):
    # One fixed public, offline C1 fixture. No signing or RPC is performed.
    fixture=read(PROJECT/"products/p02-user-app/test/fixtures/w28-local-evm.json")[0]
    board=hashlib.sha256(apk_package.encode()).hexdigest()
    sender=fixture["sender"]
    request=hashlib.sha256(("qa-request:"+apk_package).encode()).hexdigest()[:32]
    unsigned="0x"+fixture["unsigned"]
    expiry=int(time.time())-60
    scope={"version":2,"exercised":True,"mode":"personal","language":"ko",
           "verified":{"boardFingerprint":board,"address":sender,"generation":7,"network":"8283"}}
    reserved={**scope,"signing":{"kind":"evm","digest":hashlib.sha256(bytes.fromhex(unsigned[2:])).hexdigest(),
               "board":board,"generation":7,"expires":expiry,
               "evm":{"request":request,"unsigned":unsigned,"chain":"8283","genesis":fixture["genesis"],"signer":sender}}}
    pending={**scope,"pendingEvm":{"chain":"8283","raw":fixture["raw"],"hash":fixture["hash"],"sender":sender,
              "expires":expiry,"submitted":False,"genesis":fixture["genesis"],"profileRevision":1,"leaseRevision":"1",
              "evidence":{"board":board,"generation":7,"request":request,"unsigned":unsigned}}}
    return reserved,pending,fixture["hash"]


def instrumentation(args, folder, target, before, latest):
    manifest=folder/"AndroidManifest.xml"
    package="org.nu54.qa.protected."+target.rsplit(".",1)[1]
    component=package+"/org.nu54.qa.protectedprobe.Probe"
    manifest.write_text(f'<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="{package}">'
        f'<uses-sdk android:minSdkVersion="26" android:targetSdkVersion="34"/><application android:label="NU54 QA probe"/>'
        f'<instrumentation android:name="org.nu54.qa.protectedprobe.Probe" android:targetPackage="{target}"/></manifest>')
    template=(Path(__file__).parent/"consumers/android-protected/Probe.java").read_text()
    java=folder/"Probe.java"
    def literal(profile):
        return json.dumps(json.dumps(profile, separators=(",",":"),ensure_ascii=True))
    java.write_text(template.replace("__QA_TARGET__",json.dumps(target))
                    .replace("__C1_BEFORE__",literal(before)).replace("__C1_LATEST__",literal(latest)))
    classes,dex=folder/"classes",folder/"dex"
    for directory in (classes,dex):
        directory.mkdir()
    run(args.javac,"-source","8","-target","8","-classpath",args.android_jar,"-d",classes,java)
    run(args.aapt.parent/"d8","--min-api","26","--lib",args.android_jar,"--output",dex,
        classes/"org/nu54/qa/protectedprobe/Probe.class")
    package_file=folder/"manifest.apk"
    run(args.aapt.parent/"aapt2","link","-I",args.android_jar,"-o",package_file,"--manifest",manifest)
    with ZipFile(package_file,"a") as archive:
        archive.write(dex/"classes.dex","classes.dex")
    aligned,probe=folder/"aligned.apk",folder/"probe.apk"
    run(args.aapt.parent/"zipalign","-p","-f","4",package_file,aligned)
    run(args.apksigner,"sign","--ks",args.fixture_keystore,"--ks-pass","file:"+str(args.fixture_pass_file),
        "--ks-key-alias",args.fixture_alias,"--out",probe,aligned)
    require(android_certificate(probe,args.apksigner)==android_certificate(args.baseline,args.apksigner),
            "QA instrumentation signer differs")
    return probe,package,component


def profile_inspection(app, component, stage):
    output=app.call("shell","am","instrument","-w","-e","stage",stage,component)
    fields=dict(re.findall(r"(?m)^INSTRUMENTATION_RESULT: (\w+)=(.+)$",output))
    require(fields.pop("result",None)=="PASS","Android fixture did not verify production encrypted storage")
    require(set(fields)<={"record","ciphertext","ownerScope","phase","staleRejected","olderGenerationRejected","assurance","ownerKey"},
            "unexpected Android profile assurance")
    return fields


def ordinary_wallet(app, transaction=None):
    app.launch()
    app.find("지갑 프로필 선택",tap=True,attempts=1,optional=True)
    if transaction is None:
        app.find("요청 만료만으로 서명이 무효가 되지는 않습니다.",contains=True)
        app.find("미결 요청 내보내기")
    else:
        app.tap("개인 전송")
        app.find(transaction)
        app.find("서명됨 · 전송 전")
        app.find("서명된 거래는 앱의 기한이 지나도 전송될 수 있습니다.",contains=True)
        require(not app.find("만료된 미전송 초안 지우기",attempts=6,optional=True),
                "legacy clock-only discard makes protected rollback unsafe")


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for option in ("baseline","upgrade","aapt","apksigner","android-jar","output","key","public",
                   "fixture-keystore","fixture-pass-file"):
        parser.add_argument("--"+option,type=lambda value:Path(value).resolve(),required=True)
    parser.add_argument("--fixture-alias",default="qa")
    parser.add_argument("--javac",default="javac")
    parser.add_argument("--adb",default="adb")
    parser.add_argument("--device",required=True)
    parser.add_argument("--preinstalled-baseline",action="store_true",
                        help="require the exact baseline already installed; the QA probe still requires no profile")
    args=parser.parse_args()
    base,new=(android_apk(file,args.aapt,args.apksigner) for file in (args.baseline,args.upgrade))
    package=base["package"]
    require(re.fullmatch(r"com\.nu54renter\.fleetqa\.r[0-9a-f]{16}",package) and new["package"]==package and
            new["certificate"]==base["certificate"] and new["versionCode"]>base["versionCode"] and
            new["rollbackDataPolicy"]=="retain","same dedicated QA signer, increasing version and retain required")
    require(re.fullmatch(r"emulator-[0-9]+",args.device),"dedicated Android emulator required")
    app=PracticeApp(args.adb,args.device,package)
    require(app.call("shell","getprop","ro.kernel.qemu")=="1" and
            app.call("shell","getprop","ro.build.version.sdk")=="34","API 34 dedicated Android emulator required")
    listing=app.call("shell","pm","list","packages",package)
    require(listing==("package:"+package if args.preinstalled_baseline else ""),
            "unexpected QA application already installed; preserve its profile")
    args.output.mkdir(mode=0o700)
    before,latest,transaction=profiles(package)
    rows=[{"mode":"android-emulator","payload":digest(file),"version":apk["versionName"],
           "dimensions":{"os":"android-api-34"},**apk} for file,apk in ((args.baseline,base),(args.upgrade,new))]
    target={"package":package,"device":args.device,"adb":args.adb,"aapt":str(args.aapt),"apksigner":str(args.apksigner)}
    def install(file,row,action,prior=None):
        return observe(file,row,{**target,"androidOperation":action,"androidSource":prior})

    steps={}
    def record(stage, installation, profile, behavior):
        report=args.output/(stage+".json")
        save(report,{"installation":installation,"protected":profile,"behavior":behavior})
        steps[stage]={"passed":True,"result":digest(report)}

    with tempfile.TemporaryDirectory(prefix="nu54-protected-probe-") as temp:
        probe,probe_package,component=instrumentation(args,Path(temp),package,before,latest)
        require(not app.call("shell","pm","list","packages",probe_package),"existing QA instrumentation must be preserved")
        app.call("shell","wm","dismiss-keyguard")
        first=install(args.baseline,rows[0],"observe-installed-baseline" if args.preinstalled_baseline else "install")
        try:
            app.call("install","-t",probe)
            registered=app.call("shell","pm","list","instrumentation",package).splitlines()
            require(f"instrumentation:{component} (target={package})" in registered,"QA instrumentation unavailable")
            original=profile_inspection(app,component,"seed")
            ordinary_wallet(app)
            require(original["phase"]=="unresolved","baseline C1 reservation missing")
            record(STEPS[0],first,original,"unresolved-original-not-released-by-expiry")
            print("PASS baseline unresolved C1 retained through production storage and normal UI",flush=True)
            upgraded=install(args.upgrade,rows[1],"enable-rollback",rows[0])
            require(profile_inspection(app,component,"inspect")==original,"upgraded app did not open original profile")
            updated=profile_inspection(app,component,"advance")
            require(updated["phase"]=="pending" and updated["ownerScope"]==original["ownerScope"] and
                    updated.get("ownerKey")==original.get("ownerKey") and updated["record"]!=original["record"],
                    "upgraded app did not keep the original owner scope")
            ordinary_wallet(app,transaction)
            record(STEPS[1],upgraded,updated,"same-authenticated-original-pending")
            print("PASS updated C1-bound transaction retains sender and original hash",flush=True)
            reverted=install(args.baseline,rows[0],"os-rollback",rows[1])
            restored=profile_inspection(app,component,"prove")
            stale={"staleRejected","olderGenerationRejected"}
            require(all(restored.pop(key)=="true" for key in stale),"stale approval or older generation was not fenced")
            require({key:value for key,value in updated.items() if key not in stale}==restored,
                    "Android OS rollback did not keep the latest protected ciphertext")
            ordinary_wallet(app,transaction)
            record(STEPS[2],reverted,{**restored,**dict.fromkeys(stale,"true")},"rejects-stale-reservation-and-clock-only-discard")
            print("PASS PackageManager rollback retains exact latest AES-GCM state and prevents stale resurrection",flush=True)
        finally:
            if app.call("shell","pm","list","packages",probe_package)=="package:"+probe_package:
                require(app.call("uninstall",probe_package)=="Success","temporary QA instrumentation cleanup failed")
    now=int(time.time())
    claim={"format":FORMAT,"contract":CONTRACT,"source":rows[0]["payload"],"target":rows[1]["payload"],
           "package":package,"os":"android-api-34","certificate":base["certificate"],"rollbackDataPolicy":"retain",
           "scenario":"expired-unattempted-c1","authority":"synthetic-local",
           "observedAt":now,"expiresAt":now+TTL,"assurance":restored["assurance"],"ownerScope":restored["ownerScope"],
           "rawHash":transaction[2:],"steps":steps}
    signed=sign(claim,args.key)
    signature(claim,signed,args.public)
    save(args.output/"proof.json",{"claim":claim,"signer":key_identity(args.public),"signature":signed})


if __name__=="__main__":
    main()
