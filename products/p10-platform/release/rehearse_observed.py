#!/usr/bin/env python3
"""Measure actual SDK archives through a custom PostgreSQL publication rehearsal."""

import argparse
from contextlib import closing
import json
import hashlib
import ssl
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
from urllib.request import urlopen
from uuid import uuid4

from evidence import digest, read, require
from fleet import (DIMENSIONS, DOMAIN, ROLES, begin, body_digest, database, finish, installation_receipt, key_identity,
                   matrix_status, record_advisory, record_receipt, register, sign, vote_payload)
from observe import run, sdk
from recover import save
from test_fleet import browser_layout, detached, keys, portal_server, ref, reject


def observed_candidate(directory, sdk_file, provenance_file, version, dimensions, pair, npm_cache, scanner_cache):
    directory.mkdir(mode=0o700)
    archive = directory / sdk_file.name
    shutil.copyfile(sdk_file, archive)
    private, public = pair
    local_public = directory / "release.pub.pem"
    shutil.copyfile(public,local_public)
    provenance = read(provenance_file)
    require(provenance["artifactSha256"] == digest(archive) and provenance["evidenceKind"] == "observed",
            "SDK source evidence differs from the exact archive")
    payload = ref(archive)
    payload_signature = detached(archive,private)
    with tempfile.TemporaryDirectory(prefix="n54-measured-sdk-") as scratch:
        installed = Path(scratch)
        observation = sdk(archive,{"version":version},{"package":"@nu54/protocol","npmCache":str(npm_cache)},installed)
        sbom = json.loads(run("npm","sbom","--sbom-format=cyclonedx","--omit=dev",cwd=installed))
        scan = json.loads(run("trivy","fs","--cache-dir",scanner_cache,"--skip-db-update","--skip-java-db-update",
            "--offline-scan","--skip-version-check","--scanners","vuln","--format","json",installed))
        results = scan.get("Results") or []
        require(any(item.get("Type")=="npm" and item.get("Packages") for item in results),
                "scanner did not observe installed npm packages")
        require(not any(item.get("Vulnerabilities") for item in results),"SDK scan has unresolved findings")
    artifact = {"id":"protocol-ts","version":version,"target":"node","payload":payload,"evidence":{}}
    raw = {"provenance":provenance,"sbom":sbom,"regression":observation,
           "rescan":{"scan":scan,"db":read(scanner_cache/"db/metadata.json")},
           "artifactSigning":{"format":"nu54-detached-ed25519-v1","signer":key_identity(public),"payload":payload,
                     "publicKey":ref(local_public),"signature":ref(payload_signature)}}
    for role,result in raw.items():
        inner, outer = directory / (role+".observed.json"), directory / (role+".json")
        save(inner,result)
        claim={"format":f"nu54-{role}-v1","evidenceKind":"observed","artifactSha256":payload["sha256"],"report":ref(inner)}
        if role == "provenance":
            claim |= {field:provenance[field] for field in ("sourceClean","sourceCommit")}
        if role in {"regression","rescan","artifactSigning"}:
            claim["passed"]=True
        if role == "artifactSigning":
            claim["signatureFormat"]="nu54-detached-ed25519-v1"
        save(outer,claim)
        artifact["evidence"][role]=ref(outer)
    manifest, matrix = directory / "candidate.json", directory / "matrix.json"
    save(manifest,{"format":"nu54-release-evidence-v1","channel":"custom","artifacts":[artifact],
                   "sourceCommit":provenance["sourceCommit"],"sourceClean":provenance["sourceClean"]})
    save(matrix,{"format":DOMAIN,"manifest":digest(manifest),
                 "combinations":[{"artifact":"protocol-ts","dimensions":dimensions,"mode":"sdk","counter":0}]})
    return manifest, detached(manifest,private), local_public, matrix, detached(matrix,private)


def private_review_browser(cli, folder, origin, scope, profile):
    session="-s=n54review-"+uuid4().hex[:12]
    config=folder/"observed-private-browser.json"
    save(config,{"browser":{"browserName":"chromium","launchOptions":{"channel":"chrome"},
        "contextOptions":{"ignoreHTTPSErrors":True,"viewport":{"width":390,"height":760},
            "clientCertificates":[{"origin":origin,"certPath":profile["cert"],"keyPath":profile["key"]}]}}})
    def command(*args,check=True):
        return subprocess.run([cli,session,*args],cwd=folder,check=check,capture_output=True,text=True,timeout=25).stdout
    try:
        command("open","--config="+str(config),f"{origin}/D04?cohort={scope}")
        state=command("eval","() => ({ review:document.body.innerText.includes('수정 후보 검토'),hold:document.body.innerText.includes('배포 보류') })")
        require('"review": true' in state and '"hold": true' in state,"authenticated Chrome review states absent")
        command("goto",f"{origin}/D03?cohort={scope}")
        state=command("eval","() => document.body.innerText.includes('설치 전 검토') && document.body.innerText.includes('protocol-ts')")
        require("### Result\ntrue" in state,"authenticated Chrome pending install proof absent")
    finally:
        command("close",check=False)


def rehearse(args, name, service, private):
    scope=str(uuid4())
    old, fixed=args.before_version,args.fixed_version
    # Test statements describe an intentional compatibility retirement. Security
    # scanner output is captured separately from that rehearsal decision.
    runtime = run("node","--version").removeprefix("v")
    dimensions=dict(zip(DIMENSIONS,("P10","node-"+runtime,"host","unscoped",1,"typescript","protocol-v1"),strict=True))
    actors={role:(str(uuid4()),*keys(private,role)) for role in ROLES}
    signers={candidate:keys(private,candidate) for candidate in ("fallback","before","fixed")}
    targets={target:keys(private,target) for target in ("consumer-a","consumer-b","consumer-c")}
    policy={"cohorts":{scope:{"channel":"custom","public":True,"approvalSeconds":600,"targets":{
        target:{"key":str(pub),"artifact":"protocol-ts","dimensions":dimensions,"package":"@nu54/protocol",
                "npmCache":str(args.npm_cache)} for target,(_,pub) in targets.items()}}},
        "principals":{actor:{"person":role,"issuer":"local-rehearsal","subject":role,"roles":[role],"cohorts":[scope],
                            "key":str(pub)} for role,(actor,_,pub) in actors.items()},
        "releaseTrust":{"format":"nu54-release-trust-v1","channels":{"official":{},"custom":{
            key_identity(pub):["protocol-ts"] for _,pub in signers.values()}}}}
    public_keys=args.output/"observer-keys"
    public_keys.mkdir(mode=0o700)
    for target,(_,pub) in targets.items():
        shutil.copyfile(pub,public_keys/f"{target}.pub.pem")
    for role,(actor,_,pub) in actors.items():
        shutil.copyfile(pub,public_keys/f"{role}-{actor}.pub.pem")
    candidates={}
    source_specs={"before":(args.before_sdk,args.before_provenance,old),
                  "fixed":(args.fixed_sdk,args.fixed_provenance,fixed),
                  "fallback":(args.fixed_sdk,args.fixed_provenance,fixed)}
    for label,(archive,provenance,version) in source_specs.items():
        candidates[label]=observed_candidate(args.output/label,archive,provenance,version,dimensions,signers[label],
                                               args.npm_cache,args.trivy_cache)

    def manifest(item):
        return digest(candidates[item][0])

    def signed(item,revision,parent=None,action="publish",advisory=None):
        now=int(time.time())
        target = key_identity(signers[item][1]) if action=="revoke" else manifest(item)
        plan={"id":str(uuid4()),"action":action,"cohort":scope,"revision":revision,"candidate":target,
              "parent":manifest(parent) if parent else None,
              "targets":{owner:revision for owner in targets} if action!="revoke" else {},
              "advisory":advisory,"public":True,"issued":now,"expires":now+300}
        votes={role:{"actor":actor,"signature":sign(vote_payload(plan,role,actor),key)} for role,(actor,key,_) in actors.items()}
        return {"plan":plan,"votes":votes}

    def publish(db,item,bundle):
        begin(db,bundle,policy)
        receipts = args.output / item / bundle["plan"]["id"]
        receipts.mkdir(mode=0o700)
        save(receipts/"approvals.json",bundle)
        for index,(target,(key,_)) in enumerate(targets.items()):
            candidate, release_sig, public, _, _=candidates[item]
            receipt=installation_receipt(db,bundle,target,key,candidate,release_sig,public,policy)
            record_receipt(db,bundle,receipt,policy)
            if index==0:
                reject(finish,db,bundle,policy)
            save(receipts/f"{target}.receipt.json",receipt)
        return finish(db,bundle,policy)

    with closing(database(name)) as admin, closing(database(f"dbname={name} user={service}")) as db:
        for candidate, sig, pub, matrix, matrix_sig in candidates.values():
            register(db,candidate,sig,pub,policy,matrix,matrix_sig)
        publish(db,"fallback",signed("fallback",0))
        publish(db,"before",signed("before",1,"fallback"))
        case={"id":str(uuid4()),"cohort":scope,"component":"pkg:npm/%40nu54/protocol","affected":[old],
              "before":manifest("before"),"candidate":manifest("fixed"),"owner":"custom-rehearsal",
              "reviewBy":int(time.time())+600}
        review=record_advisory(db,case,policy)
        require(review["state"]=="review" and review["newVersions"]=={"protocol-ts":[fixed]},"observed package diff differs")
        hold={**case,"id":str(uuid4()),"before":manifest("fixed"),"candidate":manifest("fallback"),"affected":[fixed]}
        require(record_advisory(db,hold,policy)["state"]=="hold","unresolved real SBOM entry left review")
        bundle=signed("fixed",2,"before",advisory={"id":case["id"],"digest":body_digest(review)})
        two={"plan":bundle["plan"],"votes":{role:vote for role,vote in bundle["votes"].items() if role!="publisher"}}
        reject(begin,db,two,policy)
        begin(db,bundle,policy)
        from test_erasure import get, tls_fixtures

        tls,_,_=tls_fixtures(private,browser=bool(args.browser))
        reviewer=str(uuid4())
        cert=ssl.PEM_cert_to_DER_cert(Path(tls["restore"]["cert"]).read_text())
        policy["operatorCertificates"]={hashlib.sha256(cert).hexdigest():reviewer}
        policy["principals"][reviewer]={"roles":["release-review"],"cohorts":[scope],"expiresAt":int(time.time())+600}
        policy_file=private/"fleet-policy.json"
        save(policy_file,policy)
        catalog, reviewer_login=["n54obs_"+uuid4().hex[:12] for _ in range(2)]
        admin.execute(f"CREATE ROLE {catalog} LOGIN IN ROLE nu54_p10_fleet_catalog")
        admin.execute(f"CREATE ROLE {reviewer_login} LOGIN IN ROLE nu54_p10_fleet_review")
        admin.execute("INSERT INTO fleet_review_enrollment VALUES (%s,%s,now()+interval '10 minutes')",(reviewer_login,scope))
        server=tls["ordinary"]
        flags=["--review","--cert",server["cert"],"--key",server["key"],"--client-ca",server["ca"]]
        try:
            with portal_server(f"dbname={name} user={catalog}",policy_file) as origin, \
                 portal_server(f"dbname={name} user={reviewer_login}",policy_file,flags) as portal:
                url=f"/api/D04?cohort={scope}"
                with urlopen(origin+url,timeout=4) as response:
                    require(not json.load(response)["advisories"],"unapproved observed report escaped to public D04")
                code,private_report=get(tls["restore"],portal,url)
                phases=({row["state"] for row in json.loads(private_report)["advisories"]} if code==200 else set())
                require(phases=={"review","hold"},"private reviewer lacks expected observed SBOM decisions")
                require(get(tls["traveler"],portal,url)[0]==404,"nonreviewer saw private report")
                if args.browser:
                    private_review_browser(args.browser,private,portal,scope,tls["restore"])
                    print("PASS mTLS Chrome D04 observed candidate review and hold, D03 pending installation",flush=True)
                print("PASS only two approvals and premature publication denied",flush=True)
                patched=publish(db,"fixed",bundle)
                with urlopen(origin+url,timeout=4) as response:
                    public=json.load(response)["advisories"]
                require(len(public)==1 and public[0]["manifest"]==manifest("fixed") and "owner" not in public[0],
                        "published observed candidate absent from D04")
                save(args.output/"published-advisory.json",public)
                if args.browser:
                    browser_layout(args.browser,private,origin,scope,artifact="protocol-ts",
                                   unsupported_wire="2",component=case["component"])
        finally:
            admin.execute(f"DROP ROLE {catalog}, {reviewer_login}")
        revocation=signed("before",3,"fixed",action="revoke")
        begin(db,revocation,policy)
        save(args.output/"revocation.approvals.json",revocation)
        reject(begin,db,signed("before",3,"fixed",action="rollback"),policy)
        publish(db,"fallback",signed("fallback",3,"fixed",action="rollback"))
        end=matrix_status(db,policy,scope)
        require(end["revision"]==4 and all(row["status"]=="consumer-install" for row in end["targets"]),
                "last verified release did not resume")
    summary={"scope":scope,"observedCandidate":manifest("fixed"),"publishedResult":patched,"targetReceipts":len(targets),
             "affectedVersion":old,"fixedVersion":fixed,"rollbackManifest":end["manifest"],"revision":end["revision"],"node":runtime}
    save(args.output/"summary.json",summary)
    print(json.dumps(summary,sort_keys=True),flush=True)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--existing-postgres",action="store_true",required=True)
    for argument in ("before-sdk","fixed-sdk","before-provenance","fixed-provenance","output","npm-cache","trivy-cache"):
        parser.add_argument("--"+argument,type=lambda value:Path(value).resolve(),required=True)
    for argument in ("before-version","fixed-version"):
        parser.add_argument("--"+argument,required=True)
    parser.add_argument("--browser")
    args=parser.parse_args()
    args.output.mkdir(mode=0o700)
    name="n54obs_"+uuid4().hex[:12]
    service=name+"_service"
    role_created=False
    subprocess.run(["createdb","-w",name],check=True)
    try:
        migrate=Path(__file__).resolve().parents[1]/"database/migrate.py"
        subprocess.run([sys.executable,str(migrate),"p10"],env={**os.environ,"P10_MIGRATION_DATABASE_URL":name},check=True)
        with closing(database(name)) as admin:
            admin.execute(f"CREATE ROLE {service} LOGIN IN ROLE nu54_p10_fleet_service")
            role_created=True
        with tempfile.TemporaryDirectory(prefix="n54-observed-keys-") as private:
            rehearse(args,name,service,Path(private))
        subprocess.run(["pg_dump","-w","-Fc","-d",name,"-f",str(args.output/"authority.pgdump")],check=True)
    finally:
        subprocess.run(["dropdb","-w",name],check=True)
        if role_created:
            with closing(database("postgres")) as admin:
                admin.execute(f"DROP ROLE {service}")


if __name__ == "__main__":
    main()
