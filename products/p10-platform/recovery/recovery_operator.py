#!/usr/bin/env python3
"""Mutual-TLS operator access to durable D06 recovery and X05 replay state."""

import argparse
import hashlib
from html import escape
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import re
import ssl
import subprocess
import tempfile
import time
from urllib.parse import parse_qs, urlsplit
from uuid import uuid4

from cutover import (OperatorDenied, collect, db, digest, finish, open_fence, operator_guard_sql,
                     status, text_hash, validate_votes, uuid4_text, verify)
from preparation import configured, details, request
from recover import read

PATH = re.compile(r"/(?P<view>D06|X05|v1/recovery)/(?P<scope>[a-f0-9-]{36})(?:/(?P<job>[a-f0-9-]{36}))?(?:/(?P<action>fence|receipt|finish|start|stop))?")
LIMIT = 512 * 1024


def page(view, route, report, rehearsal=None, can_stop=False):
    stages={"queued":"대기","restore":"백업 복원","isolation":"일반 트래픽 차단 확인",
            "erasure":"삭제 대장 대조","replay":"거래 원장·확정 체인 재생","report":"서명 보고서 생성"}
    job_status={"queued":"복원 요청 접수","running":"격리 복원·재생 중","stopped":"격리 복원 중단",
                "hold":"전환 보류","ready":"독립 확인 대기"}
    start_panel=""
    if rehearsal:
        start_panel=f"<p>{job_status[rehearsal['state']]} · {stages[rehearsal['stage']]}</p>"
        start_panel+="<p>격리 리허설입니다. 실제 전환은 별도 세 역할의 서명이 필요합니다.</p>"
        if rehearsal.get("evidence"):
            start_panel+=f"<p>서명 보고서 <code>{escape(rehearsal['evidence'])}</code></p>"
        if can_stop and rehearsal["state"] in {"queued","running"}:
            start_panel+=f"""<form method="post" action="/D06/{route['scope']}/{route['job']}/stop">
<input type="hidden" name="job" value="{route['job']}"><button>격리 작업 중단</button></form>"""
        start_panel+=f'<p><a href="/D06/{route["scope"]}/{route["job"]}">진행 상태 조회</a></p>' 
    labels = {"fenced": "서비스 입구 차단 · 대상 전환 확인 중",
              "complete": "전환 완료", "superseded": "복귀 작업으로 대체됨"}
    title = "격리 재생과 조회 전환" if view == "X05" else "서비스 복원·승격"
    held = route["fenced"] and route["activeJob"] == route["job"]
    state = labels.get(route["state"], "전환 보류" if report and report["status"] == "hold" else
                       "독립 확인 대기" if report else "복원 작업 조회")
    plan = route["plan"] or {}
    targets = plan.get("targets", {})
    if view == "X05":
        targets = {k: v for k, v in targets.items() if k == "p07"}
    receipts = {receipt["target"]: receipt for receipt in route["targets"]}
    rows = "".join(f"<tr><td>{escape(k)}</td><td>{v}</td><td>{receipts[k]['appliedRevision'] if k in receipts else '대기'}</td></tr>"
                   for k, v in sorted(targets.items()))
    evidence_html = ""
    if report:
        validation = {k: v for k, v in report.get("reconciliation", {}).items() if k != "checkpoints" or view == "X05"}
        goals = report["goals"]
        erasure = report.get("erasure")
        evidence_html = (
            f"<h2>복원 검증</h2><p>손실 구간 {report['rpoSeconds']}초 / 목표 {goals['rpoSeconds']}초 · "
            f"검증까지 {escape(str(report.get('validationSeconds', '미측정')))} / 목표 {goals['rtoSeconds']}초</p>"
            f"<p>실제 전환 시간 {escape(str(route['rtoSeconds'])) + '초' if route.get('rtoSeconds') is not None else '대기'}</p>"
            f"<p>삭제 적용 위치 {escape(erasure['appliedThrough']) if erasure else '증거 미제공'}</p>"
            f"<details><summary>대조 결과</summary><pre>{escape(json.dumps(validation, indent=2, ensure_ascii=False))}</pre></details>"
        )
    return f"""<!doctype html>
<html lang="ko"><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>{title}</title><style>
body{{font:1rem system-ui,sans-serif;margin:clamp(.5rem,3vw,3rem) auto;padding:0 min(3vw,1rem);max-width:52rem;background:#f4f7fb;color:#132640}}
main{{background:white;padding:clamp(.5rem,3vw,2rem);overflow-wrap:anywhere;border:1px solid #d8e0ec;border-radius:.75rem}} code{{overflow-wrap:anywhere}}
h1,h2{{line-height:1.2}} table{{border-collapse:collapse;width:100%;table-layout:fixed}} td,th{{text-align:left;padding:clamp(.1rem,1vw,.7rem);border-bottom:1px solid #d8e0ec}}
pre{{overflow:auto;background:#f4f7fb;padding:1rem}}
</style><main><h1>{title}</h1><p>{state}</p>
<p>작업 <code>{escape(route['job'] or '')}</code> · 입구 revision {route['ingressRevision']}</p>
<p>일반 작업 {('차단' if held else '현재 경로 유지')}</p>
<p>현재 manifest <code>{escape(route['currentManifest'])}</code></p>
{start_panel}{evidence_html}<h2>대상별 영수증</h2><table><thead><tr><th>대상</th><th>기대 revision</th><th>적용 revision</th></tr></thead>
<tbody>{rows}</tbody></table>
<p><a href="/v1/recovery/{route['scope']}/{route['job'] or ''}">작업 JSON 조회</a></p></main></html>""".encode()


def operator_identity(connection, policy, client_ca):
    try:
        return verified_operator_identity(connection,policy,client_ca)
    except (ValueError, OSError, KeyError, AttributeError, ssl.SSLError, subprocess.TimeoutExpired) as exc:
        if isinstance(exc, OperatorDenied):
            raise
        raise OperatorDenied("operator certificate authorization unavailable") from exc


def verified_operator_identity(connection, policy, client_ca):
    leaf = connection.getpeercert(binary_form=True)
    chain_api = getattr(connection, "get_verified_chain", None)
    if chain_api is None:
        # CPython 3.11/3.12 expose the handshake-verified chain on the SSL object.
        chain_api = getattr(connection._sslobj, "get_verified_chain", None)
    if not leaf or not chain_api:
        raise OperatorDenied("operator TLS chain unavailable")
    chain = [item if isinstance(item, bytes) else ssl.PEM_cert_to_DER_cert(item.public_bytes()) for item in chain_api()]
    if not chain or chain[0] != leaf:
        raise OperatorDenied("operator TLS chain differs")
    fingerprints = [hashlib.sha256(item).hexdigest() for item in chain]
    if set(fingerprints).intersection(policy.get("revokedCertificates", [])):
        raise OperatorDenied("operator certificate withdrawn")
    actor = policy.get("operatorCertificates", {}).get(fingerprints[0])
    grant = policy.get("principals", {}).get(actor)
    if not grant or type(grant.get("expiresAt")) is not int or grant["expiresAt"] <= time.time():
        raise OperatorDenied("operator enrollment expired")
    with tempfile.TemporaryDirectory(prefix="n54-operator-chain-") as directory:
        files = []
        deadline = grant["expiresAt"]
        for index, certificate in enumerate(chain):
            path = Path(directory) / f"{index}.pem"
            path.write_text(ssl.DER_cert_to_PEM_cert(certificate))
            files.append(path)
            cert = ssl._ssl._test_decode_cert(str(path))
            deadline = min(deadline, int(ssl.cert_time_to_seconds(cert["notAfter"])))
        args = ["openssl","verify","-purpose","sslclient","-no-CApath","-no-CAstore","-CAfile",str(client_ca)]
        if len(files) > 1:
            issuers = Path(directory) / "issuers.pem"
            issuers.write_bytes(b"".join(item.read_bytes() for item in files[1:]))
            args += ["-untrusted",str(issuers)]
        result = subprocess.run([*args,str(files[0])],capture_output=True,timeout=4,check=False)
        if result.returncode or deadline <= time.time():
            raise OperatorDenied("operator TLS trust ended")
    return actor, grant, deadline


def operator_tls(cert, key, client_ca):
    tls = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH, cafile=client_ca)
    tls.minimum_version = ssl.TLSVersion.TLSv1_3
    tls.verify_mode = ssl.CERT_REQUIRED
    tls.load_cert_chain(cert, key)
    return tls


class Handler(BaseHTTPRequestHandler):
    server_version = "NU54-Recovery"

    def log_message(self, fmt, *args):
        # Work IDs and tenant scope aliases stay out of the web access log.
        return

    def reply(self, code, payload, media="application/json"):
        if media == "application/json" and not isinstance(payload,bytes):
            payload = json.dumps(payload, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header("Content-Type", media + "; charset=utf-8")
        self.send_header("Cache-Control", "private, no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; form-action 'self'")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def resolve(self):
        path = urlsplit(self.path)
        matched = PATH.fullmatch(path.path) if not path.query else None
        if not matched:
            raise OperatorDenied("unknown recovery resource")
        return matched.groupdict()

    def authenticate(self, scope, permission):
        try:
            policy = read(self.server.policy_path)
            actor, grant, deadline = operator_identity(self.connection, policy, self.server.client_ca)
        except (ValueError, OSError, KeyError, ssl.SSLError, subprocess.TimeoutExpired) as exc:
            raise OperatorDenied("operator trust unavailable") from exc
        if scope not in grant.get("scopes", ()) or (permission != "read" and permission not in grant.get("roles", ())):
            raise OperatorDenied("operator role or scope differs")
        return (actor,permission,deadline), policy

    def read_access(self, scope):
        auth, _ = self.authenticate(scope,"read")
        allowed = operator_guard_sql(scope,auth)
        if db(self.server.database,f"SELECT {allowed};") != "t":
            raise OperatorDenied("operator database enrollment ended")
        # The enrollment query can itself wait on PostgreSQL.
        current, _ = self.authenticate(scope,"read")
        if current[0] != auth[0]:
            raise OperatorDenied("operator grant changed")
        return current[0]

    def report_file(self, evidence):
        return self.server.reports / (text_hash(evidence) + ".json")

    def render_index(self, scope, jobs, profiles, write):
        entries="".join(f'<li><a href="/D06/{scope}/{row["job"]}">{escape(row["profile"])} · {row["job"]}</a></li>'
                        for row in jobs)
        cards=""
        for name, profile in sorted(profiles.items()):
            targets="P05 거래 원장 · P07 확정 체크포인트"
            if profile.get("erasure"):
                targets+=" · P09 삭제 대장"
            goal=profile["goals"]
            form=f"""<form method="post" action="/D06/{scope}/start">
<input type="hidden" name="profile" value="{escape(name)}"><input type="hidden" name="job" value="{uuid4()}">
<button>격리 복원 요청</button></form>""" if write else ""
            channel="합성 훈련" if profile["synthetic"] else "운영 증거 격리 리허설"
            cards+=f"""<article><h2>{escape(name)}</h2><p>{targets}</p>
<p>{channel} · RPO {escape(str(goal['rpoSeconds']))}초 · 검증 목표 {escape(str(goal['rtoSeconds']))}초</p>{form}</article>"""
        return f"""<!doctype html><html lang="ko"><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>서비스 복원·승격</title><style>body{{font:1rem system-ui;max-width:52rem;margin:auto;padding:1em;overflow-wrap:anywhere}}
button{{font:inherit;max-width:100%}}</style><h1>격리 환경에 복원할 범위</h1>
<p>작업자 준비 → 독립 검증 → 승인 후 전환</p>{cards}<h2>작업 내역</h2><ul>{entries}</ul></html>""".encode()

    def may_request(self, scope):
        if not self.server.control or not self.server.preparation:
            return False
        try:
            authorization,_=self.authenticate(scope,"requester")
            return db(self.server.control,f"SELECT {operator_guard_sql(scope,authorization)};")=="t"
        except OperatorDenied:
            return False

    def submit_rehearsal(self, values):
        if not self.server.control or not self.server.preparation or values["view"] not in {"D06","v1/recovery"}:
            raise OperatorDenied("rehearsal unavailable")
        scope,action=values["scope"],values["action"]
        authorization,policy=self.authenticate(scope,"requester")
        identity=authorization[0]
        size=int(self.headers.get("Content-Length",-1))
        if not 0<size<=4096:
            return self.reply(400,{"error":"invalid_command"})
        form=self.headers.get_content_type()=="application/x-www-form-urlencoded"
        if form:
            if self.headers.get("Origin")!="https://"+self.headers.get("Host","") or self.headers.get("Sec-Fetch-Site","same-origin")!="same-origin":
                raise OperatorDenied("rehearsal form origin differs")
            fields=parse_qs(self.rfile.read(size).decode(),max_num_fields=2,strict_parsing=True)
            if any(len(value)!=1 for value in fields.values()):
                raise ValueError("rehearsal request fields differ")
            body={key:value[0] for key,value in fields.items()}
        elif self.headers.get_content_type()=="application/json":
            from evidence import parse
            body=parse(self.rfile.read(size))
        else:
            return self.reply(400,{"error":"invalid_command"})
        stop=action=="stop"
        if stop:
            if set(body)!={"job"} or body["job"]!=values["job"]:
                raise ValueError("rehearsal target differs")
            job=uuid4_text(values["job"])
            profile=""
            identity_digest="0"*64
        else:
            if values["job"] or set(body)!={"profile","job"}:
                raise ValueError("rehearsal request fields differ")
            job=uuid4_text(body["job"])
            profile=body["profile"]
            identity_digest=digest(configured(read(self.server.preparation),scope,profile))

        def authorize():
            fresh,current=self.authenticate(scope,"requester")
            if fresh[0]!=identity or current!=policy:
                raise OperatorDenied("operator grant changed")
            if not stop and digest(configured(read(self.server.preparation),scope,profile))!=identity_digest:
                raise OperatorDenied("restore profile changed")
            return fresh
        request(self.server.control,scope,job,profile,identity_digest,authorize,stop=stop)
        if form:
            self.send_response(303)
            self.send_header("Location",f"/D06/{scope}/{job}")
            self.send_header("Cache-Control","private, no-store")
            self.send_header("Content-Length","0")
            self.end_headers()
        else:
            self.reply(202,{"job":job})

    def do_GET(self):
        try:
            values = self.resolve()
            actor = self.read_access(values["scope"])
            if values["action"]:
                return self.reply(404, {"error": "not_found"})
            scope=values["scope"]
            if values["job"] is None:
                if not self.server.preparation or values["view"]!="D06":
                    raise OperatorDenied("unknown rehearsal resource")
                profiles=read(self.server.preparation).get("scopes",{}).get(scope,{})
                payload=self.render_index(scope,details(self.server.database,scope),profiles,self.may_request(scope))
                if self.read_access(scope)!=actor:
                    raise OperatorDenied("operator grant changed")
                return self.reply(200,payload,"text/html")
            route = status(self.server.database, scope, values["job"])
            rehearsal=None
            if self.server.preparation:
                rows=details(self.server.database,scope,values["job"])
                rehearsal=rows[0] if rows else None
            report = None
            if rehearsal and route["job"] is None:
                route["job"]=rehearsal["job"]
                if rehearsal["evidence"]:
                    artifact=self.report_file(rehearsal["evidence"])
                    raw=artifact.read_bytes()
                    if hashlib.sha256(raw).hexdigest()!=rehearsal["evidence"]:
                        raise ValueError("rehearsal report digest differs")
                    spec=read(self.server.policy_path)["scopes"][scope]
                    verify({"scope":scope,"job":rehearsal["job"],"report":rehearsal["evidence"]},
                           rehearsal["signature"],Path(spec["rehearsalReportKey"]))
                    from evidence import parse
                    report=parse(raw)
            elif route["job"] is None:
                # Inspection can be queried before a signed plan is approved.
                artifact = self.server.reports / values["scope"] / (values["job"] + ".json")
                if not artifact.is_file():
                    return self.reply(404, {"error": "not_found"})
                report = read(artifact)
                route["job"] = values["job"]
            else:
                artifact = self.report_file(route["plan"]["evidence"])
                if artifact.is_file():
                    raw = artifact.read_bytes()
                    if hashlib.sha256(raw).hexdigest() != route["plan"]["evidence"]:
                        raise ValueError("report digest differs")
                    report = json.loads(raw)
            if report and (report["scope"], report["job"]) != (values["scope"], values["job"]):
                return self.reply(404, {"error": "not_found"})
            data = page(values["view"],route,report,rehearsal,self.may_request(scope) if rehearsal else False) if values["view"] != "v1/recovery" else json.dumps({"route":route,"report":report,"rehearsal":rehearsal},ensure_ascii=False).encode()
            if self.read_access(values["scope"]) != actor:
                raise OperatorDenied("operator grant changed")
            self.reply(200,data,"text/html" if values["view"] != "v1/recovery" else "application/json")
        except (ValueError, KeyError, TypeError):
            self.reply(404, {"error": "not_found"})
        except (OSError, RuntimeError):
            self.reply(503, {"error": "status_unavailable"})

    def do_POST(self):
        try:
            values = self.resolve()
            action = values["action"]
            if action in {"start","stop"}:
                return self.submit_rehearsal(values)
            role = "requester" if action == "fence" else "applier"
            if values["view"] != "v1/recovery" or not action or not self.server.control:
                raise OperatorDenied("unknown recovery transition")
            authorization, policy = self.authenticate(values["scope"],role)
            actor = authorization[0]
            length = int(self.headers.get("Content-Length", -1))
            if not 0 < length <= LIMIT or self.headers.get_content_type() != "application/json":
                return self.reply(400, {"error": "invalid_command"})
            from evidence import parse
            body = parse(self.rfile.read(length))
            bundle = body["bundle"]
            plan, _ = validate_votes(bundle, policy)
            if (plan["scope"], plan["job"], bundle["votes"][role]["actor"]) != (values["scope"], values["job"], actor):
                return self.reply(404, {"error": "not_found"})
            def authorize():
                current, fresh = self.authenticate(values["scope"],role)
                if current[0] != actor or fresh != policy:
                    raise OperatorDenied("operator policy changed")
                try:
                    validate_votes(bundle,fresh)
                except (ValueError,KeyError,OSError) as exc:
                    raise OperatorDenied("operator approval trust changed") from exc
                return current

            report = self.report_file(plan["evidence"])
            if action == "fence":
                open_fence(self.server.control, bundle, report, policy, authorize=authorize)
            elif action == "receipt":
                collect(self.server.control, bundle, body["receipt"], policy, authorize=authorize)
            else:
                finish(self.server.control, bundle, policy, report, authorize=authorize)
            self.reply(200, {"job":plan["job"],"status":"accepted"})
        except OperatorDenied:
            self.reply(404, {"error":"not_found"})
        except (ValueError, KeyError, TypeError, RuntimeError, OSError):
            self.reply(409, {"error": "recovery_held"})


def main():
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument("--bind", default="127.0.0.1")
    args.add_argument("--port", type=int, default=8444)
    args.add_argument("--policy", required=True, type=Path)
    args.add_argument("--reports", required=True, type=Path)
    args.add_argument("--status-database", required=True)
    args.add_argument("--control-database")
    args.add_argument("--rehearsal-config",type=Path)
    args.add_argument("--cert", required=True)
    args.add_argument("--key", required=True)
    args.add_argument("--client-ca", required=True)
    config = args.parse_args()
    tls = operator_tls(config.cert, config.key, config.client_ca)
    with ThreadingHTTPServer((config.bind, config.port), Handler) as server:
        server.socket = tls.wrap_socket(server.socket, server_side=True)
        server.policy_path, server.client_ca = config.policy, config.client_ca
        server.reports, server.database = config.reports, config.status_database
        server.control = config.control_database
        server.preparation = config.rehearsal_config
        server.serve_forever()


if __name__ == "__main__":
    main()
