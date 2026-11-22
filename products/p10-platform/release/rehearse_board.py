#!/usr/bin/env python3
"""Exercise two real counted MCUboot images through scoped host-slot fleet publication."""

import argparse
from contextlib import closing
from copy import deepcopy
import hashlib
import json
import ssl
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import time
from urllib.request import urlopen
from uuid import uuid4

from evidence import digest, read, require
from fleet import (DOMAIN, ROLES, begin, body_digest, database, finish, installation_receipt, key_identity,
                   matrix_status, receipt_payload, record_advisory, record_receipt, register, sign, vote_payload)
from observe import observe, run
from recover import save
from test_fleet import detached, keys, portal_server, ref


def candidate_files(root, input_root, signer, dimensions, resign=None):
    root.mkdir(mode=0o700)
    vendor = read(input_root/"release-manifest.json")
    require(vendor["channel"]=="custom" and vendor["board"]=="nu54v_dk/nrf54l15/cpuapp","unexpected board build target")
    recovery=vendor["serialRecovery"]
    require(recovery["layoutVerified"] is True and recovery["staging"]=="application-secondary","unverified board layout")
    counter=recovery["hardwareSecurityCounter"]
    critical=("app/zephyr/zephyr.signed.bin","release-public.pem","west-frozen.yml")
    for filename in critical:
        require(digest(input_root/filename)==vendor["artifacts"][filename],"firmware build input hash differs")
    image=root/"zephyr.signed.bin"
    key=root/"boot.pub.pem"
    if resign:
        python,imgtool,boot_private,boot_public=resign
        unsigned=input_root/"app/zephyr/zephyr.bin"
        require(digest(unsigned)==vendor["artifacts"]["app/zephyr/zephyr.bin"],"unsigned ARM input differs from its build")
        source_wire=(input_root/critical[0]).read_bytes()
        source_header=struct.unpack_from("<H",source_wire,8)[0]
        require(source_header==0x800,"unsupported board fixture header or slot geometry")
        version_parts=struct.unpack_from("<BBHI",source_wire,20)
        source_version="{}.{}.{}+{}".format(*version_parts)
        run(python,imgtool,"sign","--version",source_version,"--slot-size","0xb2000","--header-size",hex(source_header),
            "--align","16","--rom-fixed","0x10000","--security-counter",counter,"--sha","512","-k",boot_private,unsigned,image)
        shutil.copyfile(boot_public,key)
    else:
        shutil.copyfile(input_root/critical[0],image)
        shutil.copyfile(input_root/critical[1],key)
    wire=image.read_bytes()
    header,protected,payload=struct.unpack_from("<HHI",wire,8)
    major,minor,patch,build=struct.unpack_from("<BBHI",wire,20)
    version=f"{major}.{minor}.{patch}" + (f"+{build}" if build else "")
    require(32 <= header < header+payload+protected < len(wire),"invalid MCUboot archive bounds")
    measurement=hashlib.sha256(wire[:header+payload+protected]).hexdigest()
    pinned=key_identity(key)
    entry={"artifact":"board-app","dimensions":dimensions,"mode":"board-image","counter":counter,
           "bootKey":pinned,"measurement":measurement}
    observed=observe(image,{**entry,"version":version,"payload":digest(image)},{"boardKeys":{pinned:str(key)}})
    frozen=(input_root/critical[2]).read_text()
    revisions=re.findall(r"(?m)^  - name: ([\w.-]+)\n(?:    (?!revision:).+\n)*?    revision: ([0-9a-f]{40})$",frozen)
    require(any(name=="mcuboot" for name,_ in revisions),"resolved MCUboot source missing")
    components=[{"name":"west:"+name,"version":commit,"type":"library"} for name,commit in revisions]
    components.append({"name":"west:nrf","version":vendor["nrfCommit"],"type":"library"})
    private, pub=signer
    release_pub=root/"release.pub.pem"
    shutil.copyfile(pub,release_pub)
    artifact={"id":"board-app","version":version,"target":"nu54_rev1","payload":ref(image),"evidence":{}}
    image_signature=detached(image,private)
    reports={
        "provenance":{"sourceBuild":vendor,"observedImage":digest(image),"bootRoot":pinned,
                      "hostResign":bool(resign),"sourceCounter":counter},
        "sbom":{"bomFormat":"CycloneDX","specVersion":"1.6","version":1,"components":components,
                "scope":"resolved west sources from the measured build; no installed-component or vulnerability claim"},
        "rescan":{"scope":"signed image, pinned Ed25519 boot root, version, host slot and protected counter; no CVE scan",
                  "observation":observed},
        "regression":{"scope":"observed host image verification and build-reported interruption layout","observation":observed,
                      "offlineInterruptedPrefixesRejected":recovery["offlineInterruptedPrefixesRejected"]},
        "artifactSigning":{"format":"nu54-detached-ed25519-v1","signer":key_identity(pub),"payload":ref(image),
                           "publicKey":ref(release_pub),"signature":ref(image_signature)},
    }
    for role,result in reports.items():
        raw, wrapped = root/(role+".observed.json"), root/(role+".json")
        save(raw,result)
        proof={"format":f"nu54-{role}-v1","evidenceKind":"observed","artifactSha256":digest(image),"report":ref(raw)}
        if role=="provenance":
            proof.update(sourceCommit=vendor["sourceCommit"],sourceClean=not vendor["sourceDirty"])
        elif role in {"rescan","regression","artifactSigning"}:
            proof["passed"]=True
        if role=="artifactSigning":
            proof["signatureFormat"]="nu54-detached-ed25519-v1"
        save(wrapped,proof)
        artifact["evidence"][role]=ref(wrapped)
    manifest,matrix=root/"candidate.json",root/"matrix.json"
    save(manifest,{"format":"nu54-release-evidence-v1","channel":"custom","artifacts":[artifact],
                   "sourceCommit":vendor["sourceCommit"],"sourceClean":not vendor["sourceDirty"]})
    save(matrix,{"format":DOMAIN,"manifest":digest(manifest),"combinations":[entry]})
    return (manifest,detached(manifest,private),release_pub,matrix,detached(matrix,private)),entry,key,observed,vendor["nrfCommit"]


def rejects(operation, expected, *args):
    try:
        operation(*args)
    except ValueError as error:
        require(expected in str(error),f"unexpected hold cause: {error}")
    else:
        raise AssertionError("forbidden board operation passed")


def chrome_board(cli, folder, output, origin, scope, profile, hold):
    session="-s=board-"+uuid4().hex[:12]
    config=folder/"board-browser.json"
    save(config,{"browser":{"browserName":"chromium","launchOptions":{"channel":"chrome"},
        "contextOptions":{"ignoreHTTPSErrors":True,"viewport":{"width":390,"height":760},
            "clientCertificates":[{"origin":origin,"certPath":profile["cert"],"keyPath":profile["key"]}]}}})
    def command(*args,check=True):
        completed=subprocess.run([cli,session,*args],cwd=folder,capture_output=True,text=True,check=check,timeout=25)
        require(not check or "### Error" not in completed.stdout,"Chrome board command failed")
        return completed.stdout
    try:
        for page,heading in (("D04","보류 이유"),("D05","설치 보류")):
            address=f"{origin}/{page}?cohort={scope}"
            if page=="D04":
                command("open","--config="+str(config),address)
            else:
                command("goto",address)
            phrases=json.dumps([heading,hold["owner"],hold["reason"]],ensure_ascii=False)
            result=command("eval",f"""() => {{
                document.documentElement.style.fontSize='200%';
                return {{ visible:{phrases}.every(label=>document.body.innerText.includes(label)),
                         fits:document.documentElement.scrollWidth <= innerWidth }};
            }}""")
            require('"visible": true' in result and '"fits": true' in result,"board hold not visible at mobile zoom")
            command("screenshot","--filename="+str(output/f"{page}-hold.png"))
            print(f"PASS mTLS Chrome {page} board hold 390px/200%",flush=True)
    finally:
        command("close",check=False)


def chrome_board_comparison(cli, folder, origin, scope, row):
    from urllib.parse import urlencode

    session="-s=d05-"+uuid4().hex[:12]
    config=folder/"board-public-browser.json"
    save(config,{"browser":{"browserName":"chromium","launchOptions":{"channel":"chrome"},
                             "contextOptions":{"viewport":{"width":390,"height":760}}}})

    def action(*items):
        ran=subprocess.run([cli,session,*items],cwd=folder,capture_output=True,text=True,check=True,timeout=30).stdout
        require("### Error" not in ran,"D05 Chrome browser action failed")
        return ran

    url=f"{origin}/D05?cohort={scope}"
    try:
        action("open","--config="+str(config),url)
        panel=action("snapshot")
        field_labels={"독립 재빌드 이미지 SHA-256":row["payload"],"별도 경로에서 확인한 부트 키 지문":row["boot_key"]}
        for label,value in field_labels.items():
            field=re.search(r'textbox "'+re.escape(label)+r'" \[ref=(e\d+)\]',panel)
            require(field,"D05 required input absent")
            action("fill",field[1],value)
        panel=action("snapshot")
        submit=re.search(r'button "설치 전 지문 대조" \[ref=(e\d+)\]',panel)
        require(submit,"D05 compare action absent")
        action("click",submit[1])
        panel=action("snapshot")
        require("실물 확인 전 설치 보류" in panel and "실제 조립 BOM" in panel,"matching reference overstates hardware")
        invalid=urlencode({"cohort":scope,"board":row["dimensions"]["board"],"channel":"custom",
                           "bootRoot":row["boot_key"],"image":"0"*64})
        action("goto",f"{origin}/D05?{invalid}")
        panel=action("snapshot")
        require("설치 차단" in panel and "시드나 키를 등록하지 마세요" in panel,"mismatched board image not held")
        viewport=action("eval","""() => { document.documentElement.style.fontSize='200%';
          return document.documentElement.scrollWidth<=innerWidth; }""")
        require("true" in viewport,"D05 comparison viewport overflow")
        print("PASS Chrome D05 clean-build input comparison, physical hold and image mismatch at 390px/200%",flush=True)
    finally:
        action("close")


def exercise(args, db, owner, private):
    scope,other_scope=str(uuid4()),str(uuid4())
    dimensions={"product":"P01","os":"zephyr","board":"nu54_rev1","chain":"unscoped","wire":2,
                "sdk":"none","abi":"mcuboot-app"}
    actors={role:(str(uuid4()),*keys(private,role)) for role in ROLES}
    installers={name:keys(private,name) for name in ("slot-a","slot-b","slot-c")}
    source_labels=("prior","upgrade","foreign","safe") if args.resume_rehearsal else ("prior","upgrade","foreign")
    release_keys={name:keys(private,name) for name in source_labels}
    boot_private,boot_public=private/"mcuboot-test-key.pem",private/"mcuboot-test-key.pub"
    run(args.mcuboot_python,args.imgtool,"keygen","-k",boot_private,"-t","ed25519")
    run(args.mcuboot_python,args.imgtool,"getpub","-e","pem","-k",boot_private,"-o",boot_public)
    same_root=(args.mcuboot_python,args.imgtool,boot_private,boot_public)
    sources={"prior":(args.prior_build,same_root),"upgrade":(args.upgrade_build,same_root),
             "foreign":(args.upgrade_build,None),"safe":(args.upgrade_build,same_root)}
    states={}
    for name in source_labels:
        source, resign = sources[name]
        states[name]=candidate_files(args.output/name,source,release_keys[name],dimensions,resign)
    prior,current=states["prior"],states["upgrade"]
    require(prior[1]["counter"]<current[1]["counter"],"candidate counter must increase")
    require(prior[1]["bootKey"]==current[1]["bootKey"]!=states["foreign"][1]["bootKey"],
            "rehearsal requires one immutable boot root and one independently genuine foreign-root candidate")
    boot_keys={value[1]["bootKey"]:str(value[2]) for value in states.values()}
    policy={"cohorts":{scope:{"public":True,"channel":"custom","approvalSeconds":600,"targets":{
        name:{"artifact":"board-app","key":str(pub),"dimensions":dimensions,"boardKeys":boot_keys.copy()}
        for name,(_,pub) in installers.items()}}},
        "principals":{actor:{"person":role,"issuer":"host-board-rehearsal","subject":role,"roles":[role],
                              "cohorts":[scope,other_scope],"key":str(pub)} for role,(actor,_,pub) in actors.items()},
        "releaseTrust":{"format":"nu54-release-trust-v1","channels":{"official":{},"custom":{
            key_identity(pub):["board-app"] for _,pub in release_keys.values()}}}}
    policy["cohorts"][other_scope]=deepcopy(policy["cohorts"][scope])
    for bundle,*_ in states.values():
        register(db,*bundle[:3],policy,*bundle[3:])
    public_keys=args.output/"observer-keys"
    public_keys.mkdir(mode=0o700)
    for actor,_,pub in actors.values():
        shutil.copyfile(pub,public_keys/f"{actor}.pem")
    for name,(_,pub) in installers.items():
        shutil.copyfile(pub,public_keys/f"{name}.pem")

    def candidate(name):
        return digest(states[name][0][0])

    def approval(name,revision,parent=None,action="publish",*,cohort=scope,supersedes=None,ttl=300):
        now=int(time.time())
        selected=key_identity(release_keys[name][1]) if action=="revoke" else candidate(name)
        plan={"id":str(uuid4()),"action":action,"cohort":cohort,"revision":revision,"candidate":selected,
              "parent":candidate(parent) if parent else None,"targets":{} if action=="revoke" else
              {target:revision for target in installers},"advisory":None,"public":True,"issued":now,"expires":now+ttl}
        if supersedes is not None:
            plan["supersedes"]=supersedes
        votes={role:{"actor":actor,"signature":sign(vote_payload(plan,role,actor),key)}
               for role,(actor,key,_) in actors.items()}
        return {"plan":plan,"votes":votes}

    def publish(name, released, *, negative=False, output_name=None):
        package,entry,_,_,_=states[name]
        manifest,signature,publisher,*_=package
        if negative:
            wrong=deepcopy(policy)
            del wrong["cohorts"][scope]["targets"]["slot-a"]["boardKeys"][entry["bootKey"]]
            rejects(begin,"not independently enrolled",db,released,wrong)
        begin(db,released,policy)
        evidence_path=args.output/(output_name or name)
        evidence_path.mkdir(mode=0o700,exist_ok=True)
        save(evidence_path/"approvals.json",released)
        result=None
        for index,(target,(key,_)) in enumerate(installers.items()):
            if negative and index==0:
                image=manifest.parent/"zephyr.signed.bin"
                original=image.read_bytes()
                try:
                    image.write_bytes(original[:-1]+bytes([original[-1]^1]))
                    rejects(installation_receipt,"file digest mismatch",db,released,target,key,manifest,signature,publisher,policy)
                finally:
                    image.write_bytes(original)
            receipt=installation_receipt(db,released,target,key,manifest,signature,publisher,policy)
            if negative and index==0:
                forged=deepcopy(receipt)
                forged["observation"]["details"]["counter"]-=1
                row={**entry,"payload":receipt["receipt"]["payload"],"version":receipt["receipt"]["version"]}
                forged["receipt"]=receipt_payload(released["plan"],target,row,body_digest(forged["observation"]))
                forged["signature"]=sign(forged["receipt"],key)
                rejects(record_receipt,"installed-slot evidence differs",db,released,forged,policy)
                rejects(finish,"installation receipts incomplete",db,released,policy)
                assert matrix_status(db,policy,scope)["manifest"]==candidate("prior")
            record_receipt(db,released,receipt,policy)
            record_receipt(db,released,receipt,policy)
            save(evidence_path/f"{target}.receipt.json",receipt)
        result=finish(db,released,policy)
        return result

    publish("prior",approval("prior",0))
    foreign=approval("foreign",1,"prior")
    rejects(begin,"boot root would change",db,foreign,policy)
    save(args.output/"foreign-root.rejected-plan.json",foreign)
    require(matrix_status(db,policy,scope)["pending"] is None,"foreign root staged a transition")
    published_name="upgrade"
    if args.resume_rehearsal:
        publish("prior",approval("prior",0,cohort=other_scope),output_name="unaffected-cohort")
        other_before=matrix_status(db,policy,other_scope)
        old=approval("upgrade",1,"prior",ttl=7)
        begin(db,old,policy)
        save(args.output/"expired.approvals.json",old)
        manifest,sig,pub,*_=states["upgrade"][0]
        first=installation_receipt(db,old,"slot-a",installers["slot-a"][0],manifest,sig,pub,policy)
        record_receipt(db,old,first,policy)
        save(args.output/"expired.slot-a.receipt.json",first)
        time.sleep(max(0,old["plan"]["expires"]-time.time()+.15))
        stale=matrix_status(db,policy,scope)
        require(stale["pendingStatus"]["state"]=="expired" and all(row["status"]=="hold" for row in stale["targets"]),
                "expired partial placement was not held")
        assert matrix_status(db,policy,other_scope)==other_before
        rejects(begin,"already has a pending",db,approval("upgrade",1,"prior"),policy)
        rejects(begin,"renew the same candidate",db,approval("safe",1,"prior","resume",supersedes=old["plan"]["id"]),policy)
        renewed=approval("upgrade",1,"prior","resume",supersedes=old["plan"]["id"])
        begin(db,renewed,policy)
        save(args.output/"renewed.approvals.json",renewed)
        old_job=db.execute("SELECT superseded_by FROM fleet_operation WHERE id=%s",(old["plan"]["id"],)).fetchone()
        require(old_job["superseded_by"]==renewed["plan"]["id"],"old receipts were not linked to the replacement")
        rejects(record_receipt,"expired or outside policy",db,old,first,policy)
        rejects(record_receipt,"receipt body differs",db,renewed,first,policy)
        second=installation_receipt(db,renewed,"slot-b",installers["slot-b"][0],manifest,sig,pub,policy)
        record_receipt(db,renewed,second,policy)
        save(args.output/"renewed.slot-b.receipt.json",second)
        pending_revocation=approval("upgrade",1,"prior","revoke")
        begin(db,pending_revocation,policy)
        save(args.output/"pending-signer-revocation.json",pending_revocation)
        held=matrix_status(db,policy,scope)
        require(held["pendingStatus"]["state"]=="revoked","revoked partial placement was not held")
        assert matrix_status(db,policy,other_scope)==other_before
        rejects(begin,"counter would decrease",db,approval("prior",1,"prior","resume",supersedes=renewed["plan"]["id"]),policy)
        rejects(record_receipt,"revoked in this cohort",db,renewed,second,policy)
        replacement=approval("safe",1,"prior","resume",supersedes=renewed["plan"]["id"])
        published=publish("safe",replacement,negative=True)
        published_name="safe"
        retained=db.execute("SELECT count(*) AS n FROM fleet_receipt WHERE job IN (%s,%s)",
                            (old["plan"]["id"],renewed["plan"]["id"])).fetchone()["n"]
        assert retained==2 and matrix_status(db,policy,other_scope)==other_before
        print("PASS partial TTL/revocation renewal, older acknowledgments isolated, counter floor and unaffected active cohort",flush=True)
    else:
        published=publish("upgrade",approval("upgrade",1,"prior"),negative=True)
    rollback=approval("prior",2,published_name,"rollback")
    rejects(begin,"counter would decrease",db,rollback,policy)
    hold={"id":str(uuid4()),"cohort":scope,"component":"west:nrf","affected":[current[4]],"before":candidate(published_name),
          "candidate":candidate("prior"),"owner":"board-release-reviewer","reviewBy":int(time.time())+300,
          "reason":"MCUboot 보안 카운터가 낮아지는 복귀는 보류합니다."}
    hold_report=record_advisory(db,hold,policy)
    require(hold_report["state"]=="hold","unsafe rollback left review")
    save(args.output/"rollback.hold.json",hold_report)
    revoke=approval("prior",2,published_name,"revoke")
    begin(db,revoke,policy)
    save(args.output/"prior-signer-revocation.json",revoke)
    rejects(begin,"revoked in this cohort",db,approval("prior",2,published_name,"rollback"),policy)
    status=matrix_status(db,policy,scope)
    require(status["manifest"]==candidate(published_name) and status["revision"]==2 and status["pending"] is None
            and all(row["status"]=="local-observation" for row in status["targets"]),"unsafe rollback changed the active board fleet")
    from test_erasure import get, tls_fixtures

    tls,_,_=tls_fixtures(private,browser=bool(args.browser))
    reviewer=str(uuid4())
    cert=ssl.PEM_cert_to_DER_cert(Path(tls["restore"]["cert"]).read_text())
    policy["operatorCertificates"]={hashlib.sha256(cert).hexdigest():reviewer}
    policy["principals"][reviewer]={"roles":["release-review"],"cohorts":[scope],"expiresAt":int(time.time())+300}
    policy_file=private/"board-policy.json"
    save(policy_file,policy)
    name=owner.connection.info.dbname
    catalog,review=["n54boardui_"+uuid4().hex[:12] for _ in range(2)]
    owner.execute(f"CREATE ROLE {catalog} LOGIN IN ROLE nu54_p10_fleet_catalog")
    owner.execute(f"CREATE ROLE {review} LOGIN IN ROLE nu54_p10_fleet_review")
    owner.execute("INSERT INTO fleet_review_enrollment VALUES (%s,%s,now()+interval '5 minutes')",(review,scope))
    server=tls["ordinary"]
    flags=["--review","--cert",server["cert"],"--key",server["key"],"--client-ca",server["ca"]]
    try:
        from fleet_portal import projection, render
        with closing(database(f"dbname={name} user={review}")) as reader:
            model=projection(reader,"D04",scope,{},review=True)
            require(model is not None and b"board-release-reviewer" in render("D04",scope,model),"review role lost held board case")
        with portal_server(f"dbname={name} user={catalog}",policy_file) as public, \
             portal_server(f"dbname={name} user={review}",policy_file,flags) as protected:
            with urlopen(f"{public}/api/D05?cohort={scope}",timeout=4) as response:
                board=json.load(response)["targets"]
            assert len(board)==1 and board[0]["counter"]==current[1]["counter"] and board[0]["boot_key"]==current[1]["bootKey"]
            for page, heading in (("D04","보류 이유"),("D05","설치 보류")):
                code,body=get(tls["restore"],protected,f"/{page}?cohort={scope}")
                assert code==200 and heading.encode() in body and hold["owner"].encode() in body and hold["reason"].encode() in body, (
                    page,code,body.decode(errors="replace")[-2500:])
                assert get(tls["traveler"],protected,f"/{page}?cohort={scope}")[0]==404
            save(args.output/"board.public.json",board)
            if args.browser:
                chrome_board(args.browser,private,args.output,protected,scope,tls["restore"],hold)
                chrome_board_comparison(args.browser,private,public,scope,board[0])
    finally:
        owner.execute(f"DROP ROLE {catalog}, {review}")
    summary={"active":status["manifest"],"result":published,"counterBefore":prior[1]["counter"],"counterAfter":current[1]["counter"],
             "targetReceipts":len(installers),"mode":"host-file","priorSignerRevoked":True,"rollback":"hold",
             "bootRoot":current[1]["bootKey"],"foreignRootDenied":True,"resumeRehearsal":args.resume_rehearsal}
    save(args.output/"summary.json",summary)
    print(summary,flush=True)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--existing-postgres",action="store_true",required=True)
    for argument in ("prior-build","upgrade-build","output","mcuboot-python","imgtool"):
        parser.add_argument("--"+argument,type=lambda path:Path(path).resolve(),required=True)
    parser.add_argument("--browser")
    parser.add_argument("--resume-rehearsal",action="store_true")
    args=parser.parse_args()
    args.output.mkdir(mode=0o700)
    name="n54board_"+uuid4().hex[:12]
    login=name+"_fleet"
    provisioned=False
    subprocess.run(["createdb","-w",name],check=True)
    try:
        migration=Path(__file__).resolve().parents[1]/"database/migrate.py"
        subprocess.run([sys.executable,str(migration),"p10"],env={**os.environ,"P10_MIGRATION_DATABASE_URL":name},check=True)
        with closing(database(name)) as owner:
            owner.execute(f"CREATE ROLE {login} LOGIN IN ROLE nu54_p10_fleet_service")
            provisioned=True
        with tempfile.TemporaryDirectory(prefix="n54-board-keys-") as private:
            with closing(database(f"dbname={name} user={login}")) as service, closing(database(name)) as owner:
                exercise(args,service,owner,Path(private))
        subprocess.run(["pg_dump","-w","-Fc","-d",name,"-f",str(args.output/"authority.pgdump")],check=True)
    finally:
        subprocess.run(["dropdb","-w",name],check=True)
        if provisioned:
            with closing(database("postgres")) as owner:
                owner.execute(f"DROP ROLE {login}")


if __name__=="__main__":
    main()
