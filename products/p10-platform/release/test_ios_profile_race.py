"""Test profile expiry across real PostgreSQL waits with isolated signed fixtures."""

import argparse
from concurrent.futures import ThreadPoolExecutor
from contextlib import closing
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
from uuid import uuid4

from evidence import read
from fleet import (ROLES, begin, body_digest, database, finish, key_identity, packed, receipt_payload,
                   record_receipt, sign, vote_payload)
from ios_profile import CONTRACT, FORMAT, STEPS
from observe import ios_device_digest
from recover import save
from test_fleet import keys


class Fixture:
    def __init__(self, db, root):
        self.scope, self.target, self.device = str(uuid4()), "qa-ios", str(uuid4())
        self.profiles = ("fresh-install-only", "existing-profile")
        self.package = "com.nu54renter.fleetqa.run" + uuid4().hex
        self.dims = {"product":"P02","os":"ios-27-0-simulator","board":"simulator-arm64","chain":"unscoped",
                     "wire":2,"sdk":"none","abi":"ios-arm64"}
        self.manifests = ("a"*64,"b"*64)
        self.payloads = ("c"*64,"d"*64)
        self.publisher = keys(root,"release-key")
        self.profile_key = keys(root,"profile")
        self.observer = keys(root,"observer")
        self.actors = {role:(str(uuid4()),*keys(root,role)) for role in ROLES}
        publisher = key_identity(self.publisher[1])
        verifier = key_identity(self.profile_key[1])
        self.policy = {"cohorts":{self.scope:{"public":True,"channel":"custom","approvalSeconds":600,
            "targets":{self.target:{"artifact":"p02-ios","key":str(self.observer[1]),"dimensions":self.dims,
                                   "package":self.package,"device":self.device,"iosProfile":"existing-profile"}}}},
            "principals":{actor:{"person":role,"issuer":"test","subject":role,"roles":[role],"cohorts":[self.scope],
                                  "key":str(pub)} for role,(actor,_,pub) in self.actors.items()},
            "releaseTrust":{"format":"nu54-release-trust-v1","channels":{"official":{},"custom":{publisher:["p02-ios"]}}},
            "iosProfileVerifiers":{verifier:{"key":str(self.profile_key[1]),"packages":[self.package],"os":[self.dims["os"]]}}}
        self.rows = [{"artifact":"p02-ios","dimensions":self.dims,"combinations":[self.dims],
                      "mode":"ios-simulator","counter":0,"bundleId":self.package,"bundleBuild":str(index+1),
                      "content":str(index+1)*64,"executable":str(index+3)*64,"profileIdentity":"f"*64,
                      "version":f"1.0.{index+1}","payload":self.payloads[index],"evidence":{},
                      "buildRelease":str(index+5)*64,"fixtureEvidence":True,"profile":self.profiles[index]}
                     for index in range(2)]
        self.seed(db,publisher)

    def seed(self, db, publisher):
        for index,row in enumerate(self.rows):
            ref=bytes.fromhex(row["buildRelease"])
            field=bytes.fromhex(self.manifests[index])
            db.execute("INSERT INTO build_release VALUES (%s,2,%s,'custom',%s,%s,%s,%s)",
                       (ref,row["version"],bytes.fromhex(publisher),field,field,field))
            db.execute("INSERT INTO fleet_candidate VALUES (%s,%s,%s)",
                       (self.manifests[index],publisher,packed({"channel":"custom","artifacts":{"p02-ios":row}})))
        db.execute("INSERT INTO fleet_cohort(id,current,revision) VALUES (%s,%s,1)",(self.scope,self.manifests[0]))
        db.execute("INSERT INTO fleet_target VALUES (%s,%s,1,%s)",(self.scope,self.target,self.manifests[0]))

    def proof(self, db, expires):
        claim={"format":FORMAT,"contract":CONTRACT,"source":self.payloads[0],"target":self.payloads[1],
               "package":self.package,"os":self.dims["os"],"identity":"f"*64,"observedAt":expires-60,
               "expiresAt":expires,"steps":{step:{"passed":True,"result":"e"*64} for step in STEPS}}
        secret,public=self.profile_key
        self.rows[1]["profileProof"]={"claim":claim,"signer":key_identity(public),"signature":sign(claim,secret)}
        db.execute("UPDATE fleet_candidate SET metadata=%s WHERE id=%s",
                   (packed({"channel":"custom","artifacts":{"p02-ios":self.rows[1]}}),self.manifests[1]))

    def plan(self, db, action="publish", previous=None, ttl=90):
        now=clock(db)
        plan={"id":str(uuid4()),"action":action,"cohort":self.scope,"revision":1,"candidate":self.manifests[1],
              "parent":self.manifests[0],"advisory":None,"public":True,"issued":now,"expires":now+ttl,
              "targets":{self.target:1}}
        if previous:
            plan["supersedes"]=previous["plan"]["id"]
        return {"plan":plan,"votes":{role:{"actor":actor,"signature":sign(vote_payload(plan,role,actor),secret)}
                for role,(actor,secret,_) in self.actors.items()}}

    def receipt(self, bundle):
        row=self.rows[1]
        details={"package":self.package,"runtime":"com.apple.CoreSimulator.SimRuntime.iOS-27-0",
                 "build":row["bundleBuild"],"content":row["content"],"executable":row["executable"],
                 "deviceDigest":ios_device_digest(self.device),"profileIdentity":row["profileIdentity"]}
        observation={"input":row["payload"],"mode":row["mode"],"version":row["version"],"observed":True,"details":details}
        payload=receipt_payload(bundle["plan"],self.target,row,body_digest(observation))
        return {"receipt":payload,"observation":observation,"signature":sign(payload,self.observer[0])}


def clock(db):
    return int(db.execute("SELECT extract(epoch FROM clock_timestamp()) AS now").fetchone()["now"])


def wait_until_locked(db, pid, statement):
    deadline=time.monotonic()+5
    while time.monotonic()<deadline:
        row=db.execute("SELECT wait_event_type,query FROM pg_stat_activity WHERE pid=%s",(pid,)).fetchone()
        if row["wait_event_type"]=="Lock" and statement in row["query"]:
            return
        time.sleep(.03)
    raise AssertionError(f"worker did not reach locked statement {statement}")


def refresh_test(dsn, root, phase):
    with closing(database(dsn)) as owner, closing(database(dsn)) as worker, closing(database(dsn)) as blocker:
        fixture=Fixture(owner,root)
        bundle=fixture.plan(owner)
        fixture.proof(owner,clock(owner)+60)
        prior=None
        if phase=="resume":
            prior=fixture.plan(owner,ttl=4)
            begin(owner,prior,fixture.policy)
            record_receipt(owner,prior,fixture.receipt(prior),fixture.policy)
            while clock(owner)<prior["plan"]["expires"]:
                time.sleep(.025)
            bundle=fixture.plan(owner,"resume",prior)
        policy_file=root/"policy.json"
        save(policy_file,fixture.policy)

        def reload_policy():
            return read(policy_file)

        action=lambda:begin(worker,bundle,fixture.policy,policy_source=reload_policy)
        lock="SELECT id FROM fleet_cohort WHERE id=%s FOR NO KEY UPDATE"
        params=(fixture.scope,)
        expected_statement="UPDATE fleet_cohort SET pending"
        if phase in {"record","finish"}:
            begin(owner,bundle,fixture.policy)
            receipt=fixture.receipt(bundle)
            if phase=="record":
                action=lambda:record_receipt(worker,bundle,receipt,fixture.policy,policy_source=reload_policy)
                lock,params,expected_statement="LOCK TABLE fleet_receipt IN SHARE MODE",(),"INSERT INTO fleet_receipt"
            else:
                record_receipt(owner,bundle,receipt,fixture.policy)
                action=lambda:finish(worker,bundle,fixture.policy,policy_source=reload_policy)
                lock,params,expected_statement="LOCK TABLE promotion IN SHARE MODE",(),"INSERT INTO promotion"
        expires=clock(owner)+(60 if phase=="policy" else 5)
        fixture.proof(owner,expires)
        blocker.connection.autocommit=False
        blocker.execute(lock,params)
        observations={}
        started=time.monotonic()
        try:
            with ThreadPoolExecutor(max_workers=1) as pool:
                future=pool.submit(action)
                try:
                    wait_until_locked(owner,worker.connection.get_backend_pid(),expected_statement)
                    assert clock(owner)<expires,"fixture did not enter SQL wait with live proof"
                    if phase=="policy":
                        changed={**fixture.policy,"iosProfileVerifiers":{}}
                        temporary=root/"policy.next"
                        save(temporary,changed)
                        temporary.replace(policy_file)
                    else:
                        while clock(owner)<expires:
                            time.sleep(.025)
                finally:
                    blocker.connection.rollback()
                try:
                    future.result(timeout=10)
                    observations["denied"]=False
                except ValueError as error:
                    expected = "continuity verifier not enrolled" if phase=="policy" else "continuity evidence stale"
                    assert expected in str(error),error
                    observations["denied"]=True
        finally:
            blocker.connection.rollback()
        cohort=owner.execute("SELECT current,pending,revision FROM fleet_cohort WHERE id=%s",(fixture.scope,)).fetchone()
        job=owner.execute("SELECT result FROM fleet_operation WHERE id=%s",(bundle["plan"]["id"],)).fetchone()
        receipts=owner.execute("SELECT count(*) AS total FROM fleet_receipt WHERE job=%s",(bundle["plan"]["id"],)).fetchone()["total"]
        assert clock(owner)<bundle["plan"]["expires"],"plan must remain live"
        observations.update(seconds=round(time.monotonic()-started,2),cohort=dict(cohort),job=job,receipts=receipts)
        print(phase,json.dumps(observations),flush=True)
        assert observations["denied"],f"{phase} accepted a proof invalidated during PostgreSQL wait"
        assert cohort["current"]==fixture.manifests[0] and cohort["revision"]==1
        target=owner.execute("SELECT current,revision FROM fleet_target WHERE cohort=%s AND name=%s",
                             (fixture.scope,fixture.target)).fetchone()
        assert target["current"]==fixture.manifests[0] and target["revision"]==1
        if phase in {"begin","policy","resume"}:
            assert job is None and cohort["pending"]==(prior["plan"]["id"] if prior else None)
        else:
            assert job["result"] is None and cohort["pending"]==bundle["plan"]["id"]
        assert receipts==int(phase=="finish")
        assert owner.execute("SELECT count(*) AS total FROM promotion").fetchone()["total"]==0
        assert owner.execute("SELECT count(*) AS total FROM test_evidence").fetchone()["total"]==0
        if prior:
            old=owner.execute("SELECT superseded_by FROM fleet_operation WHERE id=%s",(prior["plan"]["id"],)).fetchone()
            assert old["superseded_by"] is None
            total=owner.execute("SELECT count(*) AS total FROM fleet_receipt WHERE job=%s",(prior["plan"]["id"],)).fetchone()
            assert total["total"]==1
        if phase=="policy":
            ordinary=fixture.plan(owner)
            revoke={**ordinary["plan"],"action":"revoke","candidate":key_identity(fixture.profile_key[1]),"targets":{}}
            signed={"plan":revoke,"votes":{role:{"actor":actor,"signature":sign(vote_payload(revoke,role,actor),key)}
                for role,(actor,key,_) in fixture.actors.items()}}
            begin(owner,signed,fixture.policy)
            try:
                begin(owner,fixture.plan(owner),fixture.policy)
            except ValueError as error:
                assert "continuity verifier revoked" in str(error),error
            else:
                raise AssertionError("durable signed verifier revocation was ignored")
            print("PASS durable verifier revocation with independent signed approvals",flush=True)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--existing-postgres",action="store_true",required=True)
    parser.add_argument("--phase",choices=("begin","record","finish","resume","policy"),action="append")
    args=parser.parse_args()
    for phase in args.phase or ("begin","record","finish","resume","policy"):
        dbname="n54prov_"+uuid4().hex[:12]
        subprocess.run(["createdb","-w",dbname],check=True)
        try:
            migration=Path(__file__).resolve().parents[1]/"database/migrate.py"
            subprocess.run([sys.executable,str(migration),"p10"],check=True,
                           env={**os.environ,"P10_MIGRATION_DATABASE_URL":dbname},stdout=subprocess.DEVNULL)
            with tempfile.TemporaryDirectory(prefix="n54-profile-race-") as directory:
                refresh_test(dbname,Path(directory),phase)
        finally:
            subprocess.run(["dropdb","-w",dbname],check=True)


if __name__=="__main__":
    main()
