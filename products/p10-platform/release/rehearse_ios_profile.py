#!/usr/bin/env python3
"""Measure practice continuity across two sealed QA iOS candidates."""

import argparse
import json
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import time

from evidence import digest, read, require
from fleet import key_identity, sign
from ios_profile import CONTRACT, FORMAT, STEPS, TTL
from observe import ios_profile_identity, observe, run
from recover import save

TESTS = ("testSavePractice", "testReadAndUpdate", "testReadAfterRollback")


def recorded(command, log, timeout):
    with log.open("xb") as output:
        return subprocess.run(command, stdout=output, stderr=subprocess.STDOUT, timeout=timeout).returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("stage", "output", "key", "public"):
        parser.add_argument("--" + name, required=True, type=lambda path: Path(path).resolve())
    parser.add_argument("--device", required=True)
    parser.add_argument("--runtime", default="ios-27-0-simulator")
    args = parser.parse_args()
    package = read(args.stage/"preparation.json")["package"]
    require(re.fullmatch(r"com\.nu54renter\.fleetqa\.run[a-f0-9]{32}", package), "QA-only app bundle required")
    apps = subprocess.run(["xcrun","simctl","get_app_container",args.device,package,"app"], capture_output=True,timeout=15)
    require(apps.returncode != 0 and b"No such file or directory" in apps.stderr, "fresh QA installation required")
    args.output.mkdir(mode=0o700)
    suite = Path(__file__).parent/"consumers"/"ios-profile"/"SceneSmoke.swift"
    project = Path(__file__).resolve().parents[3]/"scripts"/"ios"/"scene-smoke"/"SceneSmoke.xcodeproj"
    results = {}
    archives = {}
    identities=set()
    for name in ("baseline","upgrade"):
        folder=args.stage/name
        sealed=read(folder/"sealed.json")
        payload=folder/"application.tar.gz"
        require(digest(payload)==sealed["archive"],"iOS profile package archive changed")
        with tempfile.TemporaryDirectory(prefix="nu54-ios-profile-preflight-") as extracted:
            with tarfile.open(payload) as bundle:
                bundle.extractall(extracted,filter="data")
            app=Path(extracted)/"NU54Renter.app"
            run("codesign","--verify","--deep","--strict",app)
            actual=ios_profile_identity(app,package)
            require(sealed["target"].get("profileIdentity")==actual,"iOS profile requires precompiled QA identity")
            identities.add(actual)
    require(len(identities)==1,"iOS profile identities differ across the candidate pair")
    installed = False
    with tempfile.TemporaryDirectory(prefix="nu54-ios-profile-") as work:
        root = Path(work)
        shutil.copytree(project,root/"SceneSmoke.xcodeproj")
        (root/"SceneSmoke.swift").write_text(suite.read_text().replace("__NU54_QA_PACKAGE__",package))
        destination = "platform=iOS Simulator,id="+args.device
        build = ["xcodebuild","build-for-testing","-project",str(root/"SceneSmoke.xcodeproj"),"-scheme","SceneSmoke",
                 "-destination",destination,"-derivedDataPath",str(root/"build")]
        require(recorded(build,args.output/"build.log",180)==0, "iOS profile XCTest build failed")
        test_run = next((root/"build").rglob("SceneSmoke_*.xctestrun"))
        try:
            for step,test,version in zip(STEPS,TESTS,("baseline","upgrade","baseline"),strict=True):
                folder=args.stage/version
                sealed=read(folder/"sealed.json")
                payload=folder/"application.tar.gz"
                archives[version]=sealed["archive"]
                installed=True
                observation=observe(payload,{"mode":"ios-simulator","payload":sealed["archive"],"version":sealed["version"],
                    "dimensions":{"os":args.runtime},**sealed["target"]},{"package":package,"device":args.device})
                save(args.output/(step+".installation.json"),observation)
                xcresult=args.output/(step+".xcresult")
                test_command=["xcodebuild","test-without-building","-xctestrun",str(test_run),"-destination",destination,
                              "-parallel-testing-enabled","NO","-only-testing:SceneSmoke/SceneSmoke/"+test,
                              "-resultBundlePath",str(xcresult)]
                code=recorded(test_command,args.output/(step+".log"),180)
                summary=json.loads(run("xcrun","xcresulttool","get","test-results","summary","--path",xcresult))
                counts={key:summary[key] for key in ("totalTestCount","passedTests","failedTests","skippedTests")}
                passed=code==0 and counts=={"totalTestCount":1,"passedTests":1,"failedTests":0,"skippedTests":0}
                result=args.output/(step+".result.json")
                save(result,{"test":test,"counts":counts,"exitCode":code,"observation":observation})
                results[step]={"passed":passed,"result":digest(result)}
        finally:
            if installed:
                run("xcrun","simctl","uninstall",args.device,package)
    now=int(time.time())
    claim={"format":FORMAT,"contract":CONTRACT,"source":archives["baseline"],"target":archives["upgrade"],
           "package":package,"os":args.runtime,"identity":next(iter(identities)),"observedAt":now,"expiresAt":now+TTL,"steps":results}
    signed=sign(claim,args.key)
    from fleet import signature
    signature(claim,signed,args.public)
    save(args.output/"proof.json",{"claim":claim,"signer":key_identity(args.public),"signature":signed})
    require(all(step["passed"] for step in results.values()),"previous-profile compatibility is on hold")


if __name__=="__main__":
    main()
