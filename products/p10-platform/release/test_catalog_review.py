"""Exercise local SDK file verification and scoped advisory review in Chrome."""

import argparse
from contextlib import closing
from datetime import timezone
import hashlib
import json
import os
from pathlib import Path
import ssl
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlencode
from urllib.request import urlopen
from uuid import uuid4

from evidence import digest, read, require
from fleet import (ROLES, begin, database, finish, installation_receipt, key_identity, record_advisory,
                   record_receipt, register, sign, vote_payload)
from recover import save
from test_cutover import blocked_request
from test_erasure import get, tls_fixtures
from test_fleet import keys, portal_server, reject

HERE=Path(__file__).resolve().parent


def session(cli, root, url, code, client=None):
    context={"viewport":{"width":390,"height":760}}
    if client:
        context.update(ignoreHTTPSErrors=True,clientCertificates=[{
            "origin":url.split("/D0")[0],"certPath":client["cert"],"keyPath":client["key"]}])
    session_id=uuid4().hex[:12]
    name="-s=n54catalog-"+session_id
    config=root/(session_id+".json")
    save(config,{"browser":{"browserName":"chromium","launchOptions":{"channel":"chrome"},"contextOptions":context}})
    def run(*args):
        command=subprocess.run([cli,name,*args],cwd=root,capture_output=True,text=True,check=True,timeout=35)
        require("### Error" not in command.stdout,command.stdout)
        return command.stdout
    try:
        run("open","--config="+str(config),url)
        return run("run-code",code)
    finally:
        run("close")


def local_verification(args, root, public, scope, bundle, wrong, alternate):
    manifest,sig,key,*_=bundle
    artifact=read(manifest)["artifacts"][0]
    good=manifest.parent/artifact["payload"]["file"]
    corrupt=root/"tampered.tgz"
    content=good.read_bytes()
    corrupt.write_bytes(content[:-1]+bytes([content[-1]^1]))
    corrupted_sig=root/"bad-manifest.sig"
    raw=sig.read_bytes()
    corrupted_sig.write_bytes(raw[:-1]+bytes([raw[-1]^1]))
    files={"manifest":str(manifest),"signature":str(sig),"key":str(key),"archive":str(corrupt)}
    return session(args.browser,root,public+"/D02?"+urlencode({"cohort":scope}),f"""async page => {{
       const form=page.locator('form[data-local-verify]');
       const files={json.dumps(files)};
       const leaked=[];
       page.on('request', request => {{ if(request.method()!=='GET') leaked.push(request.url()); }});
       for (const [field,path] of Object.entries(files)) await form.locator('[name='+field+']').setInputFiles(path);
       const result=form.getByRole('status');
       const submit=form.getByRole('button',{{name:'출처 검증'}});
       await submit.click();
       await page.waitForFunction(()=>document.querySelector('output')?.dataset.state==='invalid');
       if (!(await result.textContent()).includes('SDK 아카이브의 해시')) throw Error('wrong archive reason');
       await form.locator('[name=archive]').setInputFiles({json.dumps(str(good))});
       await form.locator('[name=key]').setInputFiles({json.dumps(str(wrong))});
       await submit.click();
       await page.waitForFunction(()=>document.querySelector('output')?.dataset.state==='invalid');
       if (!(await result.textContent()).includes('공개 키 지문')) throw Error('wrong signer reason');
       await form.locator('[name=key]').setInputFiles(files.key);
       await form.locator('[name=signature]').setInputFiles({json.dumps(str(corrupted_sig))});
       await submit.click();
       await page.waitForFunction(()=>document.querySelector('output')?.dataset.state==='invalid');
       if (!(await result.textContent()).includes('manifest 서명')) throw Error('bad signature reason');
       await form.locator('[name=signature]').setInputFiles(files.signature);
       await submit.click();
       await page.waitForFunction(()=>document.querySelector('output')?.dataset.state==='valid');
       if (leaked.length) throw Error('local file data left the browser');
       const fits=await page.evaluate(()=>{{ document.documentElement.style.fontSize='200%';
         return document.documentElement.scrollWidth<=innerWidth; }});
       if (!fits) throw Error('D02 overflow');
       await page.getByRole('link',{{name:'다른 릴리스 선택'}}).click();
       await page.getByRole('heading',{{name:'공개 릴리스 선택'}}).waitFor();
       await page.getByRole('link',{{name:{json.dumps("custom · "+alternate)}}}).click();
       await page.getByRole('heading',{{name:'설치 전 SDK 출처 확인'}}).waitFor();
       if (!await page.locator('option').filter({{hasText:'protocol-ts 0.2.0'}}).count()) throw Error('different release missing');
       return 'D02 wrong SHA and foreign key held, same release verified locally';
    }}""")


def exercise(args, name, root, roles):
    service,catalog,reviewer,holder=roles
    scope, other, actor=(str(uuid4()) for _ in range(3))
    bundles={label:tuple(args.source/label/item for item in ("candidate.json","candidate.sig","release.pub.pem","matrix.json","matrix.sig"))
             for label in ("before","fixed")}
    target="consumer"
    observer=keys(root,"observer")
    approvers={role:(str(uuid4()),*keys(root,role)) for role in ROLES}
    _,wrong=keys(root,"wrong-signer")
    dims=read(bundles["before"][3])["combinations"][0]["dimensions"]
    certs,_,_=tls_fixtures(root,browser=True)
    server,operator,outsider=certs["ordinary"],certs["restore"],certs["traveler"]
    actor_pem=Path(operator["cert"]).read_text()
    actor_fingerprint=hashlib.sha256(ssl.PEM_cert_to_DER_cert(actor_pem)).hexdigest()
    grant={"roles":["release-review","release-hold"],"holdRoles":["release-reviewer"],
           "cohorts":[scope],"expiresAt":int(time.time())+300}
    policy={"cohorts":{cohort:{"public":True,"channel":"custom","approvalSeconds":600,"targets":{
                    target:{"artifact":"protocol-ts","key":str(observer[1]),"dimensions":dims,
                            "package":"@nu54/protocol","npmCache":str(args.npm_cache)}}} for cohort in (scope,other)},
            "principals":{ident:{"person":role,"issuer":"test-idp","subject":role,"roles":[role],"cohorts":[scope,other],
                                 "key":str(pub)} for role,(ident,_,pub) in approvers.items()},
            "operatorCertificates":{actor_fingerprint:actor},
            "releaseTrust":{"format":"nu54-release-trust-v1","channels":{"official":{},"custom":{
                key_identity(candidate[2]):["protocol-ts"] for candidate in bundles.values()}}}}
    policy["principals"][actor]=grant
    manifests={label: digest(candidate[0]) for label,candidate in bundles.items()}
    policy_path=root/"policy.json"
    save(policy_path,policy)

    def approved(label,revision,parent=None,*,cohort=scope):
        now=int(time.time())
        plan={"id":str(uuid4()),"action":"publish","cohort":cohort,"revision":revision,
              "candidate":manifests[label],"parent":manifests[parent] if parent else None,
              "targets":{target:revision},"public":True,"advisory":None,"issued":now,"expires":now+120}
        return {"plan":plan,"votes":{role:{"actor":ident,"signature":sign(vote_payload(plan,role,ident),private)}
                for role,(ident,private,_) in approvers.items()}}

    with closing(database(service)) as db, closing(database(name)) as admin:
        for manifest,sig,key,matrix,matrix_sig in bundles.values():
            register(db,manifest,sig,key,policy,matrix,matrix_sig)
        initial=approved("before",0)
        begin(db,initial,policy)
        receipt=installation_receipt(db,initial,target,observer[0],*bundles["before"][:3],policy)
        record_receipt(db,initial,receipt,policy)
        finish(db,initial,policy)
        alternate=approved("fixed",0,cohort=other)
        begin(db,alternate,policy)
        observed=installation_receipt(db,alternate,target,observer[0],*bundles["fixed"][:3],policy)
        record_receipt(db,alternate,observed,policy)
        finish(db,alternate,policy)
        cases={cohort:{"id":str(uuid4()),"cohort":cohort,"component":"pkg:npm/%40nu54/protocol","affected":["0.1.0"],
                "before":manifests["before"],"candidate":manifests["fixed"],"owner":"release-operations",
                "reviewBy":int(time.time())+900} for cohort in (scope,other)}
        for case in cases.values():
            require(record_advisory(db,case,policy)["state"]=="review","actual SDK SBOM delta is not ready for review")
        admin.execute("INSERT INTO fleet_review_enrollment VALUES (%s,%s,now()+interval '5 minutes')",(reviewer,scope))
        admin.execute("INSERT INTO fleet_hold_enrollment VALUES (%s,%s,%s,'release-reviewer',now()+interval '5 minutes')",
                      (holder,actor,scope))
        expected=cases[scope]
        query={"cohort":scope,"case":expected["id"]}
        hold_url="/D04?"+urlencode({**query,"view":"hold"})
        review_url="/D04?"+urlencode({**query,"view":"review"})
        write_url="/api/D04/hold?"+urlencode({"cohort":scope})
        payload={"case":expected["id"],"candidate":expected["candidate"],"request":str(uuid4()),"revision":0,
                 "owner":"release-reviewer","reason":"Scoped QA hold after reviewing the installed SDK",
                 "reviewBy":time.strftime("%Y-%m-%dT%H:%MZ",time.gmtime(time.time()+3600))}
        flags=["--review","--cert",server["cert"],"--key",server["key"],"--client-ca",server["ca"]]
        if not os.environ.get("NU54_REVIEW_EXPECT_LEGACY"):
            flags.extend(("--hold-database",f"dbname={name} user={holder}"))
        with portal_server(f"dbname={name} user={catalog}",policy_path) as public, \
             portal_server(f"dbname={name} user={reviewer}",policy_path,flags) as private:
            assert get(outsider,private,review_url)[0]==404
            assert get(operator,private,review_url.replace(expected["id"],cases[other]["id"]))[0]==404
            assert get(operator,private,review_url.replace(scope,other))[0]==404
            assert get(operator,private,write_url,{**payload,"case":cases[other]["id"]})[0]==404
            assert get(operator,private,write_url,{**payload,"candidate":"f"*64})[0]==404
            result=local_verification(args,root,public,scope,bundles["before"],wrong,manifests["fixed"])
            require("D02 wrong SHA" in result,"Chrome D02 local verification did not finish")
            print("PASS D02 actual SDK tamper, foreign key and bad signature refused; same archive verified, alternate active release, uploads=0",flush=True)
            assert "보류 저장".encode() in get(operator,private,hold_url)[1],"normal D04 hold form missing"

            def withdraw():
                temporary=root/"withdraw.json"
                save(temporary,{**policy,"operatorCertificates":{}})
                temporary.replace(policy_path)
            try:
                result=blocked_request(name,"SELECT pg_advisory_xact_lock(440060)",holder,
                                       lambda:get(operator,private,write_url,payload),withdraw)
                assert result[0]==404,result
                assert admin.execute("SELECT count(*) AS total FROM fleet_advisory_hold").fetchone()["total"]==0
            finally:
                renewed=root/"renewed.json"
                save(renewed,policy)
                renewed.replace(policy_path)

            result=session(args.browser,root,private+"/D04?"+urlencode({"cohort":scope}),f"""async page => {{
                await page.getByRole('link',{{name:'수정 증거 검토'}}).click();
                for (const title of ['후보 SBOM','보안 재검사','제품 회귀','산출물 서명']) {{
                    await page.getByText(title,{{exact:true}}).click();
                }}
                for (const text of ['보고서 해시','서명 해시','서명자 지문','검증 결과']) {{
                    if (!(await page.locator('main').innerText()).includes(text)) throw Error('missing '+text);
                }}
                await page.getByRole('link',{{name:'보류 사유 기록'}}).click();
                await page.getByLabel('담당 역할').selectOption('release-reviewer');
                await page.getByLabel('보류 이유').fill({json.dumps(payload["reason"])});
                await page.getByLabel('다음 검토 시각 UTC').fill({json.dumps(payload["reviewBy"])});
                await page.getByRole('button',{{name:'보류 저장'}}).click();
                await page.getByText({json.dumps(payload["reason"])},{{exact:false}}).first().waitFor();
                return 'D04 hold stored';
            }}""",operator)
            require("D04 hold stored" in result,"Chrome hold workflow did not complete")
            result=session(args.browser,root,private+hold_url,f"""async page => {{
                await page.getByText({json.dumps(payload["reason"])},{{exact:false}}).first().waitFor();
                const fits=await page.evaluate(()=>{{document.documentElement.style.fontSize='200%';
                    return document.documentElement.scrollWidth<=innerWidth;}});
                if (!fits) throw Error('D04 overflow');
                return 'D04 reopened in fresh Chrome';
            }}""",operator)
            require("D04 reopened" in result,"D04 hold absent from new browser")
            row=admin.execute("SELECT request_id,owner_role,reason,review_at FROM fleet_advisory_hold WHERE advisory=%s",
                              (expected["id"],)).fetchone()
            assert (row["owner_role"],row["reason"],row["review_at"].astimezone(timezone.utc)
                    .strftime("%Y-%m-%dT%H:%MZ"))==(payload["owner"],payload["reason"],payload["reviewBy"])
            replay={**payload,"request":str(row["request_id"])}
            assert get(operator,private,write_url,replay)[0]==200
            assert get(operator,private,write_url,{**replay,"reason":"unexpected different"})[0]==404
            assert get(operator,private,write_url,{**replay,"request":str(uuid4())})[0]==404
            assert admin.execute("SELECT count(*) AS total FROM fleet_advisory_hold").fetchone()["total"]==1
            assert get(outsider,private,hold_url)[0]==404
            with urlopen(public+"/api/D04?"+urlencode({"cohort":scope}),timeout=5) as response:
                assert json.load(response)["advisories"]==[]
            with urlopen(public+"/api/D02?"+urlencode({"cohort":other}),timeout=5) as response:
                assert json.load(response)["manifest"]==manifests["fixed"]
            reject(begin,db,approved("fixed",1,"before"),policy)
            active=admin.execute("SELECT current,pending FROM fleet_cohort WHERE id=%s",(scope,)).fetchone()
            assert active["current"]==manifests["before"] and active["pending"] is None
            print("PASS D04 actual evidence details, actor withdrawal during lock, persisted hold, case isolation and no bypass",flush=True)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--existing-postgres",required=True,action="store_true")
    for label in ("source","npm-cache","browser"):
        parser.add_argument("--"+label,type=Path,required=True)
    args=parser.parse_args()
    name="n54review_"+uuid4().hex[:12]
    suffixes=("service","catalog","review","hold")
    logins={role:name+"_"+role for role in suffixes}
    subprocess.run(["createdb","-w",name],check=True)
    provisioned=False
    try:
        subprocess.run([sys.executable,str(HERE.parent/"database/migrate.py"),"p10"],check=True,
                       env={**os.environ,"P10_MIGRATION_DATABASE_URL":name})
        with closing(database(name)) as db:
            for role,login in logins.items():
                db.execute(f"CREATE ROLE {login} LOGIN IN ROLE nu54_p10_fleet_{role}")
        provisioned=True
        with tempfile.TemporaryDirectory(prefix="n54catalog-") as directory:
            exercise(args,name,Path(directory),(f"dbname={name} user={logins['service']}",
                                                logins["catalog"],logins["review"],logins["hold"]))
    finally:
        subprocess.run(["dropdb","-w",name],check=True)
        if provisioned:
            with closing(database("postgres")) as db:
                db.execute(f"DROP ROLE {','.join(logins.values())}")


if __name__=="__main__":
    main()
