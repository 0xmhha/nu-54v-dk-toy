#!/usr/bin/env python3
"""Rehearse P02 release and rollback on three pre-authorized QA simulators."""

import argparse
from contextlib import closing
from copy import deepcopy
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
from uuid import uuid4

from evidence import digest, read, require
from fleet import (DOMAIN, ROLES, begin, body_digest, database, finish, installation_receipt, key_identity,
                   matrix_status, receipt_payload, record_receipt, register, sign, vote_payload)
from fleet_portal import projection
from observe import bundle_content, ios_device_digest, ios_profile_identity, run
from recover import save
from rehearse_board import rejects
from test_fleet import detached, keys, ref


def app_info(app):
    return plistlib.loads((app / "Info.plist").read_bytes())


def seal(source, output, package, version, build, preserve_identity=False):
    original = app_info(source)
    run("codesign", "--verify", "--deep", "--strict", source)
    require("iPhoneSimulator" in original["CFBundleSupportedPlatforms"], "input is not a simulator app")
    require("arm64" in run("lipo", "-archs", source/original["CFBundleExecutable"]).split(), "ARM simulator executable required")
    profile_identity=ios_profile_identity(source,package) if preserve_identity else None
    original_digest = digest(source/original["CFBundleExecutable"])
    original_content = bundle_content(source)
    with tempfile.TemporaryDirectory(prefix="n54-ios-package-") as path:
        app=Path(path)/source.name
        shutil.copytree(source,app,symlinks=True)
        plugins=app/"PlugIns"
        if plugins.is_dir() and not preserve_identity:
            for test in plugins.glob("*.xctest"):
                shutil.rmtree(test)
        values=original
        if preserve_identity:
            version,build=values["CFBundleShortVersionString"],values["CFBundleVersion"]
            require(ios_profile_identity(app,package)==profile_identity,"copied iOS profile identity differs")
        else:
            values={**original,"CFBundleIdentifier":package,"CFBundleShortVersionString":version,"CFBundleVersion":build}
            (app/"Info.plist").write_bytes(plistlib.dumps(values))
            run("codesign","--force","--sign","-","--identifier",package,app)
        run("codesign","--verify","--deep","--strict",app)
        target={"bundleId":package,"bundleBuild":build,"content":bundle_content(app),
                "executable":digest(app/values["CFBundleExecutable"])}
        if profile_identity:
            target["profileIdentity"]=profile_identity
        frameworks=[]
        for item in sorted((app/"Frameworks").iterdir()):
            if item.suffix==".framework":
                info=app_info(item)
                frameworks.append({"name":info["CFBundleIdentifier"],"version":info["CFBundleVersion"],
                                   "type":"framework","hashes":[{"alg":"SHA-256","content":digest(item/info["CFBundleExecutable"])}]})
            elif item.suffix==".dylib":
                version_match=re.search(r"current version ([0-9.]+)",run("otool","-L",item))
                require(version_match,"bundled library lacks a version")
                frameworks.append({"name":item.name,"version":version_match[1],"type":"library",
                                   "hashes":[{"alg":"SHA-256","content":digest(item)}]})
        with tarfile.open(output/"application.tar.gz","w:gz") as archive:
            archive.add(app,arcname="NU54Renter.app")
    save(output/"sealed.json",{"version":version,"target":target,"original":{"package":original["CFBundleIdentifier"],
         "version":original["CFBundleShortVersionString"],"executable":original_digest,"content":original_content},
         "sbom":frameworks,"archive":digest(output/"application.tar.gz")})


def candidate(root, signer, dimensions, source_commit, source_clean, profile_proof=None):
    sealed=read(root/"sealed.json")
    payload=root/"application.tar.gz"
    require(digest(payload)==sealed["archive"],"sealed iOS payload changed")
    private,public=signer
    release_public=root/"release.pub.pem"
    shutil.copyfile(public,release_public)
    artifact={"id":"p02-ios","version":sealed["version"],"target":"simulator-arm64","payload":ref(payload),"evidence":{}}
    reports={
        "provenance":{"scope":"local QA repackaging of an independently measured simulator application","input":sealed["original"],
                      "output":sealed["target"]},
        "sbom":{"bomFormat":"CycloneDX","specVersion":"1.6","version":1,"components":sealed["sbom"],
                "scope":"installed app bundle native frameworks and libraries inventory"},
        "rescan":{"scope":"strict ad hoc simulator codesign verification and archive integrity","archive":sealed["archive"]},
        "regression":{"scope":"simulator ARM architecture and verified signed bundle identity","bundle":sealed["target"]},
        "artifactSigning":{"format":"nu54-detached-ed25519-v1","signer":key_identity(public),"payload":ref(payload),
                           "publicKey":ref(release_public),"signature":ref(detached(payload,private))},
    }
    for role,report in reports.items():
        raw,wrapper=root/(role+".observed.json"),root/(role+".json")
        save(raw,report)
        evidence={"format":f"nu54-{role}-v1","evidenceKind":"observed","artifactSha256":digest(payload),"report":ref(raw)}
        if role=="provenance":
            evidence.update(sourceCommit=source_commit,sourceClean=source_clean)
        if role in {"rescan","regression","artifactSigning"}:
            evidence["passed"]=True
        if role=="artifactSigning":
            evidence["signatureFormat"]="nu54-detached-ed25519-v1"
        save(wrapper,evidence)
        artifact["evidence"][role]=ref(wrapper)
    manifest,matrix=root/"candidate.json",root/"matrix.json"
    entry={"artifact":"p02-ios","dimensions":dimensions,"mode":"ios-simulator","counter":0,**sealed["target"]}
    entry["profile"]="existing-profile" if profile_proof else "fresh-install-only"
    if profile_proof:
        entry["profileProof"]=profile_proof
    save(manifest,{"format":"nu54-release-evidence-v1","channel":"custom","artifacts":[artifact],
                   "sourceCommit":source_commit,"sourceClean":source_clean})
    save(matrix,{"format":DOMAIN,"manifest":digest(manifest),"combinations":[entry]})
    return manifest,detached(manifest,private),release_public,matrix,detached(matrix,private)


def exercise(args, db, private, devices, source):
    scope=str(uuid4())
    names=("ios-a","ios-b","ios-c")
    dimensions={"product":"P02","os":args.runtime,"board":"simulator-arm64","chain":"unscoped",
                "wire":2,"sdk":"none","abi":"ios-arm64"}
    actors={role:(str(uuid4()),*keys(private,role)) for role in ROLES}
    observers={name:keys(private,name) for name in names}
    publishers={name:keys(private,name) for name in ("baseline","upgrade")}
    profile=read(args.profile_proof) if args.profile_proof else None
    packages={name:candidate(args.output/name,value,dimensions,**source,profile_proof=profile if name=="upgrade" else None)
              for name,value in publishers.items()}
    policy={"cohorts":{scope:{"public":True,"channel":"custom","approvalSeconds":600,"targets":{
        name:{"artifact":"p02-ios","key":str(observers[name][1]),"dimensions":dimensions,"package":args.package,"device":device}
        for name,device in zip(names,devices,strict=True)}}},
        "principals":{actor:{"person":role,"issuer":"qa-ios-rehearsal","subject":role,"roles":[role],
                            "cohorts":[scope],"key":str(pub)} for role,(actor,_,pub) in actors.items()},
        "releaseTrust":{"format":"nu54-release-trust-v1","channels":{"official":{},"custom":{
            key_identity(pub):["p02-ios"] for _,pub in publishers.values()}}}}
    if profile:
        require(args.profile_key is not None,"independent profile verifier public key required")
        policy["iosProfileVerifiers"]={key_identity(args.profile_key):{
            "key":str(args.profile_key),"packages":[args.package],"os":[args.runtime]}}
    identities=args.output/"public-keys"
    identities.mkdir(mode=0o700,exist_ok=True)
    for actor,_,public in actors.values():
        shutil.copyfile(public,identities/f"{actor}.pem")
    for target,(_,public) in observers.items():
        shutil.copyfile(public,identities/f"{target}.pem")
    for manifest,signature,pub,matrix,matrixsig in packages.values():
        register(db,manifest,signature,pub,policy,matrix,matrixsig)

    def plan(name, revision, parent=None, action="publish", predecessor=None, ttl=300):
        now=int(time.time())
        claim={"id":str(uuid4()),"action":action,"cohort":scope,"revision":revision,
               "candidate":key_identity(publishers[name][1]) if action=="revoke" else digest(packages[name][0]),
               "parent":digest(packages[parent][0]) if parent else None,"public":True,"advisory":None,
               "issued":now,"expires":now+ttl,"targets":{} if action=="revoke" else dict.fromkeys(names,revision)}
        if predecessor:
            claim["supersedes"]=predecessor
        return {"plan":claim,"votes":{role:{"actor":actor,"signature":sign(vote_payload(claim,role,actor),key)}
                 for role,(actor,key,_) in actors.items()}}

    def start(label, approval):
        save(args.output/(label+".approvals.json"),approval)
        begin(db,approval,policy)

    def install(label, release, approval, target, negative=False):
        manifest,signature,pub,*_=packages[release]
        key=observers[target][0]
        if negative:
            package=manifest.parent/"application.tar.gz"
            with package.open("r+b") as archive:
                first=archive.read(1)
                archive.seek(0)
                archive.write(bytes([first[0]^1]))
                archive.flush()
                try:
                    rejects(installation_receipt,"file digest mismatch",db,approval,target,key,manifest,signature,pub,policy)
                finally:
                    archive.seek(0)
                    archive.write(first)
        receipt=installation_receipt(db,approval,target,key,manifest,signature,pub,policy)
        if negative:
            invalid=deepcopy(receipt)
            invalid["observation"]["details"]["deviceDigest"]="0"*64
            invalid["receipt"]=receipt_payload(approval["plan"],target,receipt["receipt"],body_digest(invalid["observation"]))
            invalid["signature"]=sign(invalid["receipt"],key)
            rejects(record_receipt,"iOS installed application evidence differs",db,approval,invalid,policy)
        record_receipt(db,approval,receipt,policy)
        record_receipt(db,approval,receipt,policy)
        save(args.output/(label+"."+target+".receipt.json"),receipt)
        return receipt

    original=plan("baseline",0)
    bad=deepcopy(policy)
    bad["cohorts"][scope]["targets"]["ios-c"]["device"]=devices[0].lower()
    rejects(begin,"targets must be distinct",db,original,bad)
    start("baseline",original)
    for target in names:
        install("baseline","baseline",original,target,negative=target==names[0])
    wrong=deepcopy(policy)
    wrong["cohorts"][scope]["targets"]["ios-a"]["device"]=devices[2]
    rejects(finish,"iOS installed application evidence differs",db,original,wrong)
    finish(db,original,policy)
    pending=plan("upgrade",1,"baseline",ttl=30)
    if profile:
        for target in policy["cohorts"][scope]["targets"].values():
            target["iosProfile"]="existing-profile"
        denied=deepcopy(policy)
        denied["iosProfileVerifiers"].clear()
        rejects(begin,"continuity verifier not enrolled",db,pending,denied)
    else:
        denied=deepcopy(policy)
        denied["cohorts"][scope]["targets"]["ios-a"]["iosProfile"]="existing-profile"
        rejects(begin,"signed scope absent",db,pending,denied)
    start("partial",pending)
    legacy=install("partial","upgrade",pending,names[0])
    rejects(finish,"installation receipts incomplete",db,pending,policy)
    require(matrix_status(db,policy,scope)["manifest"]==original["plan"]["candidate"],"partial receipt changed active release")
    time.sleep(max(0,pending["plan"]["expires"]-time.time()+.15))
    hold=matrix_status(db,policy,scope)
    require(hold["pendingStatus"]["state"]=="expired" and all(row["status"]=="hold" for row in hold["targets"]),
            "expired upgrade did not hold the cohort")
    save(args.output/"expired-hold.json",hold)
    upgrade=plan("upgrade",1,"baseline","resume",pending["plan"]["id"])
    start("upgrade",upgrade)
    rejects(record_receipt,"receipt body differs",db,upgrade,legacy,policy)
    for target in names:
        install("upgrade","upgrade",upgrade,target)
    finish(db,upgrade,policy)
    status=matrix_status(db,policy,scope)
    require(status["manifest"]==upgrade["plan"]["candidate"] and all(row["status"]=="local-observation" for row in status["targets"]),
            "upgraded simulator release not current")
    save(args.output/"upgrade.status.json",status)
    revoke=plan("upgrade",2,"upgrade","revoke")
    start("revoke",revoke)
    require(matrix_status(db,policy,scope)["state"]=="revoked","revocation did not fence upgraded release")
    rollback=plan("baseline",2,"upgrade","rollback")
    start("rollback",rollback)
    for target in names:
        install("rollback","baseline",rollback,target)
    finish(db,rollback,policy)
    status=matrix_status(db,policy,scope)
    require(status["manifest"]==original["plan"]["candidate"] and status["revision"]==3 and status["pending"] is None
            and all(row["status"]=="local-observation" for row in status["targets"]),
            "rollback did not restore the observed baseline")
    return scope, {"scope":"local custom iOS simulator rehearsal using an unlaunched QA installation ID",
        "status":status,"upgradedManifest":upgrade["plan"]["candidate"],"approvalExpiryHold":True,
        "lateReceiptRejected":True,"duplicateDeviceRejected":True,"wrongDeviceReceiptRejected":True,
        "upgradePublisherRevoked":True,"simulatorTargets":len(devices)}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output",type=lambda value:Path(value).resolve(),required=True)
    parser.add_argument("--prepare",nargs=2,type=lambda value:Path(value).resolve(),metavar=("BASELINE_APP","UPGRADE_APP"))
    parser.add_argument("--device",action="append",default=[])
    parser.add_argument("--precompiled-qa-identity",action="store_true")
    parser.add_argument("--runtime",default="ios-27-0-simulator")
    parser.add_argument("--existing-postgres",action="store_true")
    parser.add_argument("--profile-proof",type=Path)
    parser.add_argument("--profile-key",type=Path)
    args=parser.parse_args()
    if args.prepare:
        args.output.mkdir(mode=0o700)
        package=app_info(args.prepare[0])["CFBundleIdentifier"] if args.precompiled_qa_identity else (
            "com.nu54renter.fleetqa.run"+uuid4().hex)
        require(re.fullmatch(r"com\.nu54renter\.fleetqa\.run[a-f0-9]{32}",package),"rehearsal-only bundle identifier required")
        for index,(name,source) in enumerate(zip(("baseline","upgrade"),args.prepare,strict=True),1):
            folder=args.output/name
            folder.mkdir(mode=0o700)
            seal(source,folder,package,f"1.0.{index}",str(index),args.precompiled_qa_identity)
        before,after=(read(args.output/name/"sealed.json") for name in ("baseline","upgrade"))
        require(before["original"]["executable"]!=after["original"]["executable"] and before["version"]!=after["version"],
                "two distinct real ARM binary builds and versions required")
        if args.precompiled_qa_identity:
            require(before["target"]["profileIdentity"]==after["target"]["profileIdentity"],
                    "candidate application identities differ")
        save(args.output/"preparation.json",{"package":package,"source_clean":not run("git","status","--porcelain"),
             "source_commit":run("git","rev-parse","HEAD")})
        print(package,flush=True)
        return
    require(args.existing_postgres,"isolated PostgreSQL is required")
    require(bool(args.profile_proof)==bool(args.profile_key),"profile proof and independent public key must be supplied together")
    devices=args.device
    require(len(devices)==len({ios_device_digest(device) for device in devices})==3,"three distinct QA simulators required")
    prepared=read(args.output/"preparation.json")
    args.package=prepared.pop("package")
    require(re.fullmatch(r"com\.nu54renter\.fleetqa\.run[a-f0-9]{32}",args.package),"rehearsal-only bundle identifier required")
    booted=json.loads(run("xcrun","simctl","list","devices","booted","-j"))["devices"]
    compatible={row["udid"].upper() for runtime,rows in booted.items()
                if runtime.removeprefix("com.apple.CoreSimulator.SimRuntime.").lower()+"-simulator"==args.runtime
                for row in rows if row["state"]=="Booted"}
    for device in devices:
        require(device.upper() in compatible,"QA simulator must be booted on the signed compatibility runtime")
        result=subprocess.run(["xcrun","simctl","get_app_container",device,args.package,"app"],capture_output=True)
        require(result.returncode!=0 and b"No such file or directory" in result.stderr,
                "fresh QA identifier absent on all devices required")
    dbname="n54ios_"+uuid4().hex[:12]
    login=dbname+"_fleet"
    provisioned=False
    subprocess.run(["createdb","-w",dbname],check=True)
    try:
        migration=Path(__file__).resolve().parents[1]/"database/migrate.py"
        subprocess.run([sys.executable,str(migration),"p10"],env={**os.environ,"P10_MIGRATION_DATABASE_URL":dbname},check=True)
        with closing(database(dbname)) as owner:
            owner.execute(f"CREATE ROLE {login} LOGIN IN ROLE nu54_p10_fleet_service")
            provisioned=True
        with tempfile.TemporaryDirectory(prefix="n54-ios-keys-") as private:
            with closing(database(f"dbname={dbname} user={login}")) as service:
                scope,summary=exercise(args,service,Path(private),devices,prepared)
        subprocess.run(["pg_dump","-w","-Fc","-d",dbname,"-f",str(args.output/"authority.pgdump")],check=True)
        with closing(database(dbname)) as catalog:
            catalog.execute("SET ROLE nu54_p10_fleet_catalog")
            support=projection(catalog,"D02",scope,{})
            advisories=projection(catalog,"D04",scope,{})
            boards=projection(catalog,"D05",scope,{})
            require(len(support["targets"])==1,
                    "catalog should display one support combination for the three identical simulator targets")
            row=support["targets"][0]
            require(row["status"]=="local-observation" and row["mode"]=="ios-simulator" and row["version"]=="1.0.1",
                    "catalog did not display the restored local simulator combination")
            save(args.output/"support.json",support)
            require(not advisories["advisories"] and not boards["targets"],
                    "iOS rehearsal leaked into public advisories or board support")
        save(args.output/"summary.json",summary)
        print("PASS three actual iOS simulators, partial hold and late ACK isolation, upgrade signer revocation, full rollback",flush=True)
    finally:
        subprocess.run(["dropdb","-w",dbname],check=True)
        if provisioned:
            with closing(database("postgres")) as owner:
                owner.execute(f"DROP ROLE {login}")


if __name__=="__main__":
    main()
