#!/usr/bin/env python3
"""Publish measured Protocol SDK interoperability in an isolated local catalog."""

import argparse
from contextlib import closing
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
from urllib.request import urlopen
from uuid import uuid4

from build_sdk import write_evidence
from evidence import digest, openssl, read, require, resolve, verify
from fleet import (DOMAIN, ROLES, begin, database, finish, installation_receipt, key_identity,
                   record_receipt, register, sign, vote_payload)
from interop import VECTORS
from observe import run
from recover import save
from test_fleet import detached, keys, portal_server, ref, reject


def prepare(source, output, signer, rows):
    supplied=source/"local-release.json"
    verify(supplied,source/"local-release.sig",source/"local-release.pub.pem",read(source/"local-trust.json"),"custom")
    release=read(supplied)
    artifacts=[item for item in release["artifacts"] if item["id"] in rows]
    require(len(artifacts)==len(rows),"signed SDK artifacts missing")
    private,public=signer
    trusted=output/"release.pub.pem"
    shutil.copyfile(public,trusted)
    for artifact in artifacts:
        payload=resolve(source,artifact["payload"])
        archived=output/payload.name
        shutil.copyfile(payload,archived)
        for role,wrapper in artifact["evidence"].items():
            if role=="artifactSigning":
                continue
            wrapped=resolve(source,wrapper)
            document=read(wrapped)
            require(document["evidenceKind"]=="observed","SDK original evidence is not observed")
            report=resolve(source,document["report"])
            shutil.copyfile(wrapped,output/wrapped.name)
            if not (output/report.name).exists():
                shutil.copyfile(report,output/report.name)
        path=output/(artifact["id"]+".payload.sig")
        path.write_bytes(openssl(["pkeyutl","-sign","-rawin","-inkey",private,"-in",archived]))
        raw=output/(artifact["id"]+".local-signature.json")
        save(raw,{"format":"nu54-detached-ed25519-v1","publicKey":ref(trusted),"signature":ref(path),
                  "payload":ref(archived),"signer":key_identity(trusted)})
        write_evidence(output,artifact,"artifactSigning",raw,passed=True,signatureFormat="nu54-detached-ed25519-v1")
    release["artifacts"]=artifacts
    manifest,matrix=output/"interop.json",output/"matrix.json"
    save(manifest,release)
    save(matrix,{"format":DOMAIN,"manifest":digest(manifest),"combinations":list(rows.values())})
    return manifest,detached(manifest,private),trusted,matrix,detached(matrix,private)


def browser_check(cli, root, url, checksum, all_languages):
    session="-s=d03-"+uuid4().hex[:12]
    conf=root/"browser.json"
    save(conf,{"browser":{"browserName":"chromium","launchOptions":{"channel":"chrome"},
                         "contextOptions":{"viewport":{"width":390,"height":760}}}})
    def cli_run(*args):
        result=subprocess.run([cli,session,*args],cwd=root,capture_output=True,text=True,timeout=30,check=True).stdout
        require("### Error" not in result,"Chrome operation failed")
        return result
    try:
        cli_run("open","--config="+str(conf),url)
        snapshot=cli_run("snapshot")
        labels=["C 최소 프로젝트","TypeScript tarball 소비자","실물 링크 지원 보류",checksum]
        if all_languages:
            labels+=["Go 모듈 소비자","Python wheel 소비자","v1 프레임 분할, 순서 오류 및 재시작","v2 프레임 재접속 검증 자료 미제공"]
        require(all(label in snapshot for label in labels),"browser D03 evidence scope differs")
        viewport=cli_run("eval","""() => { document.documentElement.style.fontSize='200%';
          return document.documentElement.scrollWidth<=innerWidth; }""")
        require("true" in viewport,"browser D03 viewport overflow")
        partial=re.search(r'link "증거 부족 조합 보기" \[ref=(e\d+)\]',snapshot)
        require(partial,"D03 partial action missing")
        cli_run("click",partial[1])
        require("물리 링크 증거가 부족한 조합" in cli_run("snapshot"),"D03 partial URL does not show held evidence")
        print("PASS Chrome D03 start/partial, host scope and physical hold at 390px/200%",flush=True)
    finally:
        cli_run("close")


def exercise(args,db,root,catalog):
    scope=str(uuid4())
    subjects={role:(str(uuid4()),*keys(root,role)) for role in ROLES}
    signer=keys(root,"publisher-candidate")
    checksum=digest(VECTORS)
    runtime="node-"+run("node","--version").removeprefix("v")
    rows={}
    targets={}
    formats=[("c","c","abi","host"),("typescript","ts","sdk",runtime)]
    if args.go_proxy and args.python_wheels:
        formats += [("go","go","abi","host"),("python","python","sdk","host")]
    installers={language:keys(root,language) for language,*_ in formats}
    for language,suffix,mode,os_name in formats:
        artifact="protocol-"+suffix
        dims={"product":"P10","os":os_name,"board":"host","chain":"unscoped","wire":2,"sdk":language,"abi":"protocol-v2"}
        rows[artifact]={"artifact":artifact,"dimensions":dims,"mode":mode,"counter":0,
                        "interop":{"language":language,"vectors":checksum}}
        targets[language]={"artifact":artifact,"dimensions":dims,"key":str(installers[language][1]),
                           "package":"@nu54/protocol","npmCache":str(args.npm_cache),
                           "goProxy":str(args.go_proxy),"pythonWheels":str(args.python_wheels)}
    manifest,sig,pub,matrix,matrix_sig=prepare(args.source,args.output,signer,rows)
    policy={"cohorts":{scope:{"public":True,"channel":"custom","approvalSeconds":600,"targets":targets}},
            "principals":{actor:{"person":role,"issuer":"interop-qa","subject":role,"roles":[role],
                                 "cohorts":[scope],"key":str(public)} for role,(actor,_,public) in subjects.items()},
            "releaseTrust":{"format":"nu54-release-trust-v1","channels":{"official":{},"custom":{
                key_identity(pub):list(rows)}}}}
    reference=register(db,manifest,sig,pub,policy,matrix,matrix_sig)
    now=int(time.time())
    plan={"id":str(uuid4()),"action":"publish","cohort":scope,"revision":0,"parent":None,"candidate":reference,
          "targets":dict.fromkeys(targets,0),"public":True,"advisory":None,"issued":now,"expires":now+300}
    bundle={"plan":plan,"votes":{role:{"actor":actor,"signature":sign(vote_payload(plan,role,actor),private)}
                for role,(actor,private,_) in subjects.items()}}
    begin(db,bundle,policy)
    reject(finish,db,bundle,policy)
    for name,(key,public) in installers.items():
        receipt=installation_receipt(db,bundle,name,key,manifest,sig,pub,policy)
        record_receipt(db,bundle,receipt,policy)
        save(args.output/(name+".receipt.json"),receipt)
        shutil.copyfile(public,args.output/(name+".pub.pem"))
    finished=finish(db,bundle,policy)
    policy_file=root/"policy.json"
    save(policy_file,policy)
    with portal_server(catalog,policy_file) as origin:
        with urlopen(f"{origin}/api/D03?cohort={scope}",timeout=5) as response:
            projected=json.load(response)
        require({row["interop"]["language"] for row in projected["targets"]}==set(installers),
                "published D03 omits actual host consumer")
        require(all(row["interop"]["vectors"]==checksum and row["mode"]!="physical" for row in projected["targets"]),
                "D03 overstates measured scopes")
        if args.browser:
            browser_check(args.browser,root,f"{origin}/D03?cohort={scope}",checksum,len(installers)==4)
    save(args.output/"summary.json",{"manifest":reference,"receiptDigest":finished,"projection":projected})
    print("PASS signed installed host consumers: "+", ".join(sorted(installers)),flush=True)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ("source","output","npm-cache"):
        parser.add_argument("--"+name,type=lambda value:Path(value).resolve(),required=True)
    parser.add_argument("--go-proxy",type=lambda value:Path(value).resolve())
    parser.add_argument("--python-wheels",type=lambda value:Path(value).resolve())
    parser.add_argument("--browser")
    parser.add_argument("--existing-postgres",required=True,action="store_true")
    args=parser.parse_args()
    require(bool(args.go_proxy)==bool(args.python_wheels),"provide both offline Go proxy and Python wheelhouse")
    args.output.mkdir(mode=0o700)
    name="n54interop_"+uuid4().hex[:12]
    service,catalog=name+"_fleet",name+"_web"
    provisioned=False
    subprocess.run(["createdb","-w",name],check=True)
    try:
        migration=Path(__file__).resolve().parents[1]/"database/migrate.py"
        subprocess.run([sys.executable,str(migration),"p10"],check=True,
                       env={**os.environ,"P10_MIGRATION_DATABASE_URL":name})
        with closing(database(name)) as owner:
            owner.execute(f"CREATE ROLE {service} LOGIN IN ROLE nu54_p10_fleet_service")
            owner.execute(f"CREATE ROLE {catalog} LOGIN IN ROLE nu54_p10_fleet_catalog")
            provisioned=True
        with tempfile.TemporaryDirectory(prefix="n54interop-") as directory:
            with closing(database(f"dbname={name} user={service}")) as db:
                exercise(args,db,Path(directory),f"dbname={name} user={catalog}")
    finally:
        subprocess.run(["dropdb","-w",name],check=True)
        if provisioned:
            with closing(database("postgres")) as admin:
                admin.execute(f"DROP ROLE {service},{catalog}")


if __name__=="__main__":
    main()
