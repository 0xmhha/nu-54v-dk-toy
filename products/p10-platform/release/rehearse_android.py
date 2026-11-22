#!/usr/bin/env python3
"""Run a P10 Android rehearsal on three dedicated emulators and PostgreSQL."""

import argparse
from contextlib import closing
from copy import deepcopy
import hashlib
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time
from uuid import uuid4
from zipfile import ZipFile

from android_profile import PROTECTED_CONTRACT, PROTECTED_SCOPE
from evidence import digest, read, require
from fleet import (ANDROID_FIELDS, DOMAIN, ROLES, begin, database, finish, installation_receipt, key_identity,
                   matrix_status, record_receipt, register, sign, vote_payload)
from fleet_portal import projection
from observe import android_apk, android_device_digest, android_installed, run
from recover import save
from rehearse_board import rejects
from test_fleet import detached, keys, ref


def candidate(root, apk_path, signer, dimensions, args, profile, proof=None):
    root.mkdir(mode=0o700)
    private,public=signer
    apk=root/"application.apk"
    shutil.copyfile(apk_path,apk)
    copied_key=root/"publisher.pub.pem"
    shutil.copyfile(public,copied_key)
    identity=android_apk(apk,args.aapt,args.apksigner)
    archive_digest=digest(apk)
    with ZipFile(apk) as archive:
        files=[info for info in archive.infolist() if info.filename.startswith(("lib/","assets/")) and not info.is_dir()]
        components=[]
        for info in files:
            with archive.open(info) as component:
                checksum=hashlib.file_digest(component,"sha256").hexdigest()
            components.append({"name":info.filename,"version":identity["versionName"],"type":"file",
                               "hashes":[{"alg":"SHA-256","content":checksum}]})
    source=run("git","rev-parse","HEAD")
    clean=not run("git","status","--porcelain")
    reports={
        "provenance":{"scope":"local QA fleet wrapping of a measured native Android build","apk":identity,"payload":ref(apk)},
        "sbom":{"bomFormat":"CycloneDX","specVersion":"1.6","version":1,"components":components,
                "scope":"native libraries and assets present in the actual APK"},
        "rescan":{"scope":"observed APK signature and manifest identity","apk":identity},
        "regression":{"scope":"verified Android package metadata and rollback policy","apk":identity},
        "artifactSigning":{"format":"nu54-detached-ed25519-v1","signer":key_identity(public),"payload":ref(apk),
                           "publicKey":ref(copied_key),"signature":ref(detached(apk,private))},
    }
    proofs={}
    for role,item in reports.items():
        raw, wrapper=root/(role+".observed.json"),root/(role+".json")
        save(raw,item)
        record={"format":f"nu54-{role}-v1","evidenceKind":"observed","artifactSha256":archive_digest,"report":ref(raw)}
        if role=="provenance":
            record.update(sourceCommit=source,sourceClean=clean)
        if role in {"rescan","regression","artifactSigning"}:
            record["passed"]=True
        if role=="artifactSigning":
            record["signatureFormat"]="nu54-detached-ed25519-v1"
        save(wrapper,record)
        proofs[role]=ref(wrapper)
    manifest,matrix=root/"candidate.json",root/"matrix.json"
    artifact={"id":"p02-android","version":identity["versionName"],"target":"android-arm64","payload":ref(apk),"evidence":proofs}
    save(manifest,{"format":"nu54-release-evidence-v1","channel":"custom","sourceCommit":source,
                   "sourceClean":clean,"artifacts":[artifact]})
    combination={"artifact":artifact["id"],"dimensions":dimensions,"mode":"android-emulator","counter":0,
                 "profile":profile,**{key:identity[key] for key in ANDROID_FIELDS}}
    if proof:
        combination["profileProof"]=proof
    save(matrix,{"format":DOMAIN,"manifest":digest(manifest),"combinations":[combination]})
    return manifest,detached(manifest,private),copied_key,matrix,detached(matrix,private)


def execute(args, db, reader, private):
    scope=str(uuid4())
    package=android_apk(args.baseline,args.aapt,args.apksigner)["package"]
    require(re.fullmatch(r"com\.nu54renter\.fleetqa\.r[0-9a-f]{16}",package),"QA Android package required")
    baseline_digest=digest(args.baseline)
    for device in args.device:
        require(run(args.adb,"-s",device,"shell","getprop","ro.kernel.qemu")=="1" and
                run(args.adb,"-s",device,"shell","getprop","ro.build.version.sdk")=="34","enrolled Android API 34 emulator required")
        installed=run(args.adb,"-s",device,"shell","pm","list","packages",package)
        if installed:
            require(installed=="package:"+package,"unexpected Android QA package")
            with tempfile.TemporaryDirectory(prefix="n54-android-baseline-") as folder:
                apk=android_installed(args.adb,device,package,Path(folder))
                require(digest(apk)==baseline_digest,"preserve existing Android app with a different payload")
    dims={"product":"P02","os":"android-api-34","board":"emulator-arm64","chain":"unscoped","wire":2,"sdk":"none","abi":"android-arm64"}
    names=("android-a",) if args.protected_local else ("android-a","android-b","android-c")
    approved_profile=PROTECTED_SCOPE if args.protected_local else "practice-profile"
    observers={name:keys(private,name) for name in names}
    roles={role:(str(uuid4()),*keys(private,role)) for role in ROLES}
    subjects={"baseline":(args.baseline,"fresh-install-only",None),
              "upgrade":(args.upgrade,approved_profile,read(args.profile_proof)),
              "unsafe-default":(args.unsafe,approved_profile,None),
              "protected-wallet":(args.upgrade,"protected-wallet",None)}
    expiry_private,expiry_public=keys(private,"simulated-expiry")
    expired={**subjects["upgrade"][2]["claim"],"observedAt":int(time.time())-120,"expiresAt":int(time.time())-60}
    expired_key=key_identity(expiry_public)
    subjects["expired-proof"]=(args.upgrade,approved_profile,
                               {"claim":expired,"signer":expired_key,"signature":sign(expired,expiry_private)})
    publishers={label:keys(private,"release-"+label) for label in subjects}
    bundles={label:candidate(args.output/label,path,publishers[label],dims,args,profile,proof)
             for label,(path,profile,proof) in subjects.items()}
    granted_key=key_identity(args.profile_key)
    scope_targets={name:{"artifact":"p02-android","key":str(observers[name][1]),"dimensions":dims,"package":package,
                         "device":device,"adb":args.adb,"aapt":str(args.aapt),"apksigner":str(args.apksigner)}
                   for name,device in zip(names,args.device,strict=True)}
    policy={"cohorts":{scope:{"public":True,"channel":"custom","approvalSeconds":600,"targets":scope_targets}},
            "principals":{actor:{"person":role,"issuer":"isolated-android-qa","subject":role,"roles":[role],
                                 "cohorts":[scope],"key":str(public)} for role,(actor,_,public) in roles.items()},
            "releaseTrust":{"format":"nu54-release-trust-v1","channels":{"official":{},"custom":{
                key_identity(public):["p02-android"] for _,public in publishers.values()}}},
            "androidProfileVerifiers":{granted_key:{"key":str(args.profile_key),"packages":[package],"os":[dims["os"]],
                                                    "contracts":[PROTECTED_CONTRACT] if args.protected_local else []},
                                       expired_key:{"key":str(expiry_public),"packages":[package],"os":[dims["os"]],
                                                    "contracts":[PROTECTED_CONTRACT] if args.protected_local else []}}}
    public_keys=args.output/"public-keys"
    public_keys.mkdir(mode=0o700)
    for actor,_,public in roles.values():
        shutil.copyfile(public,public_keys/f"{actor}.pem")
    for name,(_,public) in observers.items():
        shutil.copyfile(public,public_keys/f"{name}.pem")
    shutil.copyfile(args.profile_key,public_keys/"profile.pub.pem")
    manifests={}
    for label,(manifest,sig,pub,matrix,matrix_sig) in bundles.items():
        manifests[label]=register(db,manifest,sig,pub,policy,matrix,matrix_sig)

    def approve(label, revision, parent=None, action="publish", supersedes=None, ttl=300):
        now=int(time.time())
        plan={"id":str(uuid4()),"action":action,"cohort":scope,"revision":revision,"parent":manifests[parent] if parent else None,
              "candidate":key_identity(publishers[label][1]) if action=="revoke" else manifests[label],
              "targets":{} if action=="revoke" else dict.fromkeys(names,revision),"public":True,
              "advisory":None,"issued":now,"expires":now+ttl}
        if supersedes:
            plan["supersedes"]=supersedes
        return {"plan":plan,"votes":{role:{"actor":actor,"signature":sign(vote_payload(plan,role,actor),private)}
                for role,(actor,private,_) in roles.items()}}

    def start(label, approval):
        begin(db,approval,policy)
        save(args.output/(label+".approvals.json"),approval)

    def collect(label, approval, candidate_name, target, *, record=True):
        manifest,sig,pub,*_=bundles[candidate_name]
        receipt=installation_receipt(db,approval,target,observers[target][0],manifest,sig,pub,policy)
        if record:
            record_receipt(db,approval,receipt,policy)
        save(args.output/(label+"."+target+".receipt.json"),receipt)
        return receipt

    baseline=approve("baseline",0)
    if not args.protected_local:
        duplicate=deepcopy(policy)
        duplicate["cohorts"][scope]["targets"][names[-1]]["device"]=args.device[0]
        rejects(begin,"Android targets must be distinct",db,baseline,duplicate)
    start("baseline",baseline)
    for name in names:
        collect("baseline",baseline,"baseline",name)
    finish(db,baseline,policy)
    for target in scope_targets.values():
        target["androidProfile"]=approved_profile
    rejects(begin,"incompatible APK",db,approve("unsafe-default",1,"baseline"),policy)
    rejects(begin,"security-state continuity unverified",db,approve("protected-wallet",1,"baseline"),policy)
    rejects(begin,"continuity evidence stale",db,approve("expired-proof",1,"baseline"),policy)
    denied=deepcopy(policy)
    denied["androidProfileVerifiers"].clear()
    rejects(begin,"not independently enrolled" if args.protected_local else "continuity verifier not enrolled",
            db,approve("upgrade",1,"baseline"),denied)
    held=matrix_status(db,policy,scope)
    require(held["manifest"]==manifests["baseline"] and held["pending"] is None,"unsafe candidate staged a transition")
    partial=approve("upgrade",1,"baseline",ttl=30)
    start("partial",partial)
    # The local protected proof covers one emulator. Delay its sole ACK to
    # exercise expiry before publication; the practice cohort has three targets.
    late=collect("partial",partial,"upgrade",names[0],record=not args.protected_local)
    rejects(finish,"installation receipts incomplete",db,partial,policy)
    time.sleep(max(0,partial["plan"]["expires"]-time.time()+.12))
    stopped=matrix_status(db,policy,scope)
    require(stopped["pendingStatus"]["state"]=="expired" and
            all(row["status"]=="hold" for row in stopped["targets"]),"partial Android campaign did not hold")
    save(args.output/"expired-hold.json",stopped)
    upgraded=approve("upgrade",1,"baseline","resume",partial["plan"]["id"])
    start("upgrade",upgraded)
    rejects(record_receipt,"receipt body differs",db,upgraded,late,policy)
    for name in names:
        collect("upgrade",upgraded,"upgrade",name)
    finish(db,upgraded,policy)
    save(args.output/"upgrade.status.json",matrix_status(db,policy,scope))
    supported=projection(reader,"D02",scope,{})
    require(len(supported["targets"])==1 and supported["targets"][0]["android_profile"]==approved_profile,
            "catalog Android profile scope differs")
    save(args.output/"upgrade.support.json",supported)
    start("revoke",approve("upgrade",2,"upgrade","revoke"))
    require(matrix_status(db,policy,scope)["state"]=="revoked","Android upgrade publisher not revoked")
    require(not projection(reader,"D02",scope,{})["targets"],"withdrawn Android candidate remains public")
    rollback=approve("baseline",2,"upgrade","rollback")
    start("rollback",rollback)
    for name in names:
        collect("rollback",rollback,"baseline",name)
    finish(db,rollback,policy)
    status=matrix_status(db,policy,scope)
    require(status["manifest"]==manifests["baseline"] and status["revision"]==3 and status["pending"] is None,
            "Android QA fleet rollback did not restore the baseline")
    supported=projection(reader,"D02",scope,{})
    require(len(supported["targets"])==1 and supported["targets"][0]["android_profile"]=="fresh-install-only",
            "reverted Android catalog claimed broader profile support")
    require(not projection(reader,"D04",scope,{})["advisories"] and not projection(reader,"D05",scope,{})["targets"],
            "Android profile rehearsal leaked into unrelated projections")
    save(args.output/"rollback.support.json",supported)
    save(args.output/"summary.json",{"cohort":scope,"status":status,"candidate":manifests["upgrade"],
            "scope":approved_profile,"targets":len(names),
            "unsafeDefaultHeld":True,"protectedWalletHeld":True,"expiredProofHeld":True,"absentVerifierHeld":True,
            "lateReceiptRejected":True,"upgradedSignerRevoked":True})
    print(f"PASS {len(names)} distinct Android emulator(s), {approved_profile}, protected-wallet hold, late ACK and OS rollback",flush=True)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for flag in ("baseline","upgrade","unsafe","profile-proof","profile-key","aapt","apksigner","output"):
        parser.add_argument("--"+flag,required=True,type=lambda value:Path(value).resolve())
    parser.add_argument("--adb",default="adb")
    parser.add_argument("--protected-local",action="store_true",
                        help="only the narrow protected-profile contract on a single dedicated API 34 emulator")
    parser.add_argument("--device",required=True,action="append")
    parser.add_argument("--existing-postgres",required=True,action="store_true")
    args=parser.parse_args()
    count=1 if args.protected_local else 3
    require(len(args.device)==len({android_device_digest(device) for device in args.device})==count,
            f"{count} distinct dedicated Android emulator(s) required")
    args.output.mkdir(mode=0o700)
    name="n54and_"+uuid4().hex[:12]
    login=name+"_fleet"
    provisioned=False
    subprocess.run(["createdb","-w",name],check=True)
    try:
        migration=Path(__file__).resolve().parents[1]/"database/migrate.py"
        subprocess.run([sys.executable,str(migration),"p10"],check=True,
                       env={**os.environ,"P10_MIGRATION_DATABASE_URL":name})
        with closing(database(name)) as owner:
            owner.execute(f"CREATE ROLE {login} LOGIN IN ROLE nu54_p10_fleet_service")
            provisioned=True
        with tempfile.TemporaryDirectory(prefix="n54-android-fleet-keys-") as private:
            with closing(database(f"dbname={name} user={login}")) as service, closing(database(name)) as catalog:
                catalog.execute("SET ROLE nu54_p10_fleet_catalog")
                execute(args,service,catalog,Path(private))
        subprocess.run(["pg_dump","-w","-Fc","-d",name,"-f",str(args.output/"authority.pgdump")],check=True)
    finally:
        subprocess.run(["dropdb","-w",name],check=True)
        if provisioned:
            with closing(database("postgres")) as owner:
                owner.execute(f"DROP ROLE {login}")


if __name__=="__main__":
    main()
