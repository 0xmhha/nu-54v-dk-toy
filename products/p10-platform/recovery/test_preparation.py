"""Exercise the ordinary D06 start, resume, cancellation and signed result paths."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import socket
import ssl
import subprocess
import sys
import tempfile
import time
from uuid import uuid4

from cutover import db, h, open_fence, q, verify_evidence
from preparation import check_report
from recover import file_digest, save, write_backup
from test_cutover import blocked_request, check_browser, key, reject, signed
from test_erasure import get, tls_fixtures

HERE=Path(__file__).resolve().parent


def browser_start(cli, folder, client, origin, scope, *, stop=False):
    browser="-s=d06-"+uuid4().hex[:12]
    config=folder/(browser[3:]+".json")
    save(config,{"browser":{"browserName":"chromium","launchOptions":{"channel":"chrome"},"contextOptions":{
        "ignoreHTTPSErrors":True,"viewport":{"width":390,"height":760},
        "clientCertificates":[{"origin":origin,"certPath":client["cert"],"keyPath":client["key"]}]}}})

    def command(*args):
        checked=subprocess.run([cli,browser,*args],cwd=folder,check=True,capture_output=True,text=True,timeout=30)
        assert "### Error" not in checked.stdout,checked.stdout
        return checked.stdout

    try:
        command("open","--config="+str(config),origin+f"/D06/{scope}")
        snapshot=command("snapshot")
        button=re.search(r'button "격리 복원 요청" \[ref=((?:f\d+)?e\d+)\]',snapshot)
        assert button,snapshot
        command("click",button[1])
        current=command("snapshot")
        match=re.search(rf"/D06/{scope}/([a-f0-9-]{{36}})",current)
        assert match and "복원 요청 접수" in current,current
        summary=command("eval","""() => {
          document.documentElement.style.fontSize='200%';
          return document.documentElement.scrollWidth===innerWidth;
        }""")
        assert "true" in summary,summary
        action="start"
        if stop:
            cancel=re.search(r'button "격리 작업 중단" \[ref=((?:f\d+)?e\d+)\]',current)
            assert cancel,current
            command("click",cancel[1])
            assert "격리 복원 중단" in command("snapshot")
            action="start and stop"
        print(f"PASS Chrome mTLS normal D06 {action} form at 390px/200%",flush=True)
        return match[1]
    finally:
        command("close")


def run_scenario(control, folder, source=None, rpc=None, output=None):
    scope, other, actor, viewer, applier=(str(uuid4()) for _ in range(5))
    report_private, report_public=key(folder,"report")
    backup_private, backup_public=key(folder,"backup")
    profiles, policy=folder/"preparation.json",folder/"policy.json"
    reports=folder/"reports"
    reports.mkdir(mode=0o700)
    certificates,_,_=tls_fixtures(folder,browser=bool(os.environ.get("NU54_RECOVERY_BROWSER_CLI")))
    server, writer, reader, outsider=(certificates[name] for name in ("ordinary","mutation","restore","traveler"))

    def fingerprint(profile):
        return hashlib.sha256(ssl.PEM_cert_to_DER_cert(Path(profile["cert"]).read_text())).hexdigest()

    goal=str(uuid4())
    archive=folder/"archive"
    tools=Path(os.environ["POSTGRES_BIN"]) if "POSTGRES_BIN" in os.environ else None
    if source:
        write_backup(argparse.Namespace(scope=scope,p05=source,p07=source,postgres_bin=tools,
                                       archive=archive,sign_key=backup_private))
    profile={"archive":str(archive),"backupKey":backup_public,"rpc":rpc or "http://127.0.0.1:1","synthetic":True,
             "goalRef":goal,"goals":{"rpoSeconds":900,"rtoSeconds":3600},
             "snapshot":file_digest(archive/"manifest.json") if source else "0"*64}
    save(profiles,{"scopes":{scope:{"p05-p07":profile}}})
    signers={}
    principals={}
    for name, identity in (("requester",actor),("verifier",viewer),("applier",applier)):
        private,public=key(folder,name)
        signers[name]=(identity,private)
        principals[identity]={"person":name,"issuer":"rehearsal-test","subject":name,"publicKey":public,
                              "scopes":[scope],"roles":[name],"expiresAt":int(time.time())+600}
    spec={"rehearsalReportKey":report_public,"requireManagedRehearsal":True,
          "targets":{target:key(folder,target)[1] for target in ("p05","p07")},
          "goalRef":goal,"goals":profile["goals"],"maxApprovalSeconds":180,"synthetic":True}
    enrollment={"scopes":{scope:spec},"operatorCertificates":{fingerprint(writer):actor,fingerprint(reader):viewer},
                "principals":principals}
    save(policy,enrollment)
    reader_login,writer_login,worker_login=["n54prep_"+uuid4().hex[:12] for _ in range(3)]
    db(control,f"""
INSERT INTO platform.recovery_route VALUES ({q(scope)},0,NULL,{h('a'*64)}),({q(other)},0,NULL,{h('b'*64)});
CREATE ROLE {reader_login} LOGIN IN ROLE nu54_p10_recovery_status;
CREATE ROLE {writer_login} LOGIN IN ROLE nu54_p10_recovery_coordinator;
CREATE ROLE {worker_login} LOGIN IN ROLE nu54_p10_recovery_worker;
INSERT INTO platform.recovery_operator_enrollment VALUES
('{reader_login}',{q(actor)},{q(scope)},'read',now()+interval '10 minutes'),
('{reader_login}',{q(viewer)},{q(scope)},'read',now()+interval '10 minutes'),
('{writer_login}',{q(actor)},{q(scope)},'requester',now()+interval '10 minutes');""")
    with socket.socket() as probe:
        probe.bind(("127.0.0.1",0))
        port=probe.getsockname()[1]
    origin=f"https://127.0.0.1:{port}"
    worker=f"dbname={control} user={worker_login}"
    command=[sys.executable,str(HERE/"recovery_operator.py"),"--port",str(port),"--policy",str(policy),
             "--reports",str(reports),"--status-database",f"dbname={control} user={reader_login}",
             "--control-database",f"dbname={control} user={writer_login}",
             "--cert",server["cert"],"--key",server["key"],"--client-ca",server["ca"]]
    if not os.environ.get("NU54_REHEARSAL_EXPECT_LEGACY"):
        command.extend(("--rehearsal-config",str(profiles)))
    with subprocess.Popen(command) as process:
        try:
            deadline=time.monotonic()+10
            while True:
                try:
                    with socket.create_connection(("127.0.0.1",port),timeout=.5):
                        break
                except OSError:
                    if time.monotonic()>deadline or process.poll() is not None:
                        raise
                    time.sleep(.1)
            assert get(writer,origin,f"/D06/{scope}")[0]==200,"normal D06 preparation page unavailable"
            assert get(outsider,origin,f"/D06/{scope}")[0]==404
            assert get(writer,origin,f"/D06/{other}")[0]==404
            assert "격리 복원 요청".encode() not in get(reader,origin,f"/D06/{scope}")[1]
            cli=os.environ.get("NU54_RECOVERY_BROWSER_CLI")
            job=browser_start(cli,folder,writer,origin,scope) if cli else str(uuid4())
            start=f"/v1/recovery/{scope}/start"
            payload={"job":job,"profile":"p05-p07"}
            assert get(reader,origin,start,payload)[0]==404
            assert get(writer,origin,start,payload)[0]==202
            assert get(writer,origin,start,payload)[0]==202
            assert get(writer,origin,start,{**payload,"profile":"another"})[0]==404
            assert get(writer,origin,start,{**payload,"archive":"/arbitrary"})[0]==409
            withdrawn=str(uuid4())
            def revoke():
                changed=folder/"policy-next.json"
                save(changed,{**enrollment,"operatorCertificates":{fingerprint(reader):viewer}})
                changed.replace(policy)
            try:
                blocked=blocked_request(control,
                    f"SELECT pg_advisory_xact_lock(hashtextextended({q(scope)}::text,440054));",
                    writer_login,lambda:get(writer,origin,start,{**payload,"job":withdrawn}),revoke)
                assert blocked[0]==404,blocked
                assert db(control,f"SELECT count(*) FROM platform.recovery_rehearsal WHERE job_id={q(withdrawn)};")=="0"
                print("PASS scoped D06 requester withdrawn during PostgreSQL wait: zero jobs",flush=True)
            finally:
                changed=folder/"policy-restored.json"
                save(changed,enrollment)
                changed.replace(policy)
            assert db(control,f"SELECT count(*) FROM platform.recovery_rehearsal WHERE job_id={q(job)};")=="1"
            stopped=browser_start(cli,folder,writer,origin,scope,stop=True) if cli else str(uuid4())
            assert get(writer,origin,start,{**payload,"job":stopped})[0]==202
            assert get(writer,origin,f"/v1/recovery/{scope}/{stopped}/stop",{"job":stopped})[0]==202
            assert get(writer,origin,f"/v1/recovery/{scope}/{stopped}/stop",{"job":stopped})[0]==202
            assert "격리 복원 중단".encode() in get(writer,origin,f"/D06/{scope}/{stopped}")[1]
            assert get(writer,origin,f"/D06/{scope}/{job}")[0]==200
            run_worker=[sys.executable,str(HERE/"preparation.py"),"--database",worker,"--config",str(profiles),
                        "--isolation-root","/tmp","--reports",str(reports),"--report-key",str(report_private)]
            if tools:
                run_worker.extend(("--postgres-bin",str(tools)))
            subprocess.run(run_worker,check=True)
            code,body=get(writer,origin,f"/v1/recovery/{scope}/{job}")
            assert code==200, (code,body)
            data=json.loads(body)
            assert data["rehearsal"]["state"]==("ready" if source else "hold"),data
            assert not data["route"]["fenced"] and data["route"]["ingressRevision"]==0
            assert db(control,"SELECT count(*) FROM platform.recovery_run;")=="0"
            if source:
                assert data["report"]["ordinaryTrafficFenced"] and data["report"]["reconciliation"]["matched"]
                assert data["rehearsal"]["signature"] and data["report"]["reconciliation"]["paidRefunded"]>=3
                code,body=get(writer,origin,f"/D06/{scope}/{job}")
                assert code==200 and "독립 확인 대기".encode() in body
                if cli:
                    check_browser(cli,folder,writer,origin,f"/D06/{scope}/{job}",code,body)
                now=int(time.time())
                plan={"kind":"cutover","job":job,"scope":scope,"priorJob":None,"manifest":data["report"]["manifest"],
                      "incumbent":"a"*64,"routeRevision":0,"targets":dict.fromkeys(("p05","p07"),0),
                      "evidence":data["rehearsal"]["evidence"],"goals":spec["goals"],"goalRef":goal,
                      "issuedAt":now,"expiresAt":now+120}
                prepared=reports/(plan["evidence"]+".json")
                bundle=signed(plan,signers)
                assert check_report(control,plan,spec,prepared,enrollment,bundle)
                assert verify_evidence(plan,spec,prepared)["job"]==job
                reject(open_fence,control,{**bundle,"votes":{"requester":bundle["votes"]["requester"]}},prepared,enrollment)
                assert db(control,"SELECT count(*) FROM platform.recovery_run;")=="0"
            if output:
                save(output/"managed-d06.json",data)
            result="signed actual restore/replay" if source else "missing archive held"
            print(f"PASS same D06 job, scope isolation, owner stop, {result}, untouched ingress",flush=True)
        finally:
            process.terminate()
            process.wait(timeout=8)
            db(control,f"DROP ROLE {reader_login},{writer_login},{worker_login};")


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--existing-postgres",action="store_true",required=True)
    parser.add_argument("--source")
    parser.add_argument("--rpc")
    parser.add_argument("--output",type=Path)
    args=parser.parse_args()
    name="n54prep_"+uuid4().hex[:12]
    subprocess.run(["createdb","-w",name],check=True)
    try:
        subprocess.run([sys.executable,str(HERE.parent/"database/migrate.py"),"p10"],
                       env={**os.environ,"P10_MIGRATION_DATABASE_URL":name},check=True)
        with tempfile.TemporaryDirectory(prefix="n54prep-") as folder:
            run_scenario(name,Path(folder),args.source,args.rpc,args.output)
    finally:
        subprocess.run(["dropdb","-w",name],check=True)


if __name__=="__main__":
    main()
