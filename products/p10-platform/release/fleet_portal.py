#!/usr/bin/env python3
"""Serve the verified fleet catalog and protocol registry."""

import argparse
from contextlib import closing
from datetime import datetime, timezone
from html import escape
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path

import psycopg2
import re
from urllib.parse import parse_qs, urlencode, urlsplit
from uuid import uuid4

from evidence import digest, read
from fleet import DIMENSIONS, database, text_hash, tx, uuid4_text
from recovery_operator import OperatorDenied, operator_identity, operator_tls

ROOT = Path(__file__).resolve().parents[3]
REGISTRY = ROOT / "packages/protocol/v2/service-contracts.json"
PAGES = {"D01": "프로토콜·서비스 계약", "D02": "SDK·ABI 호환표", "D03": "상호운용 시험 증거",
         "D04": "보안 공지·수정 릴리스", "D05": "공개 보드 제작·설치"}


def review_time(epoch):
    return datetime.fromtimestamp(epoch,timezone.utc).isoformat(timespec="seconds")


def tag(value):
    return escape(str(value))


def link(page, scope, **filters):
    query = urlencode({"cohort": scope, **filters})
    return f'<a href="/{page}?{tag(query)}">{tag(PAGES[page])}</a>'


def board_projection(state, filters):
    from board_reference import comparison, reference

    board=reference()
    selection={name:value for name,value in filters.items() if name in {"board","channel","image","bootRoot"}}
    state["boardReference"]=board
    state["comparison"]=comparison(board,state["targets"],state.get("channel"),selection)
    state["selection"]=selection
    return state


def board_intro(scope, model):
    record=model["boardReference"]
    result=model["comparison"]
    fields=model["selection"]
    html=f"""<p>{tag(record['catalogModel'])} · 소스 리비전 {tag(record['board'])} · 참고 회로 {tag(record['referenceVariant'])}</p>
<p>빌드 대상 <code>{tag(record['buildTarget'])}</code> · 고정 입력 <code>{tag(record['commit'])}</code></p>
<form><input type="hidden" name="cohort" value="{tag(scope)}">
<label>보드 리비전 <input name="board" value="{tag(fields.get('board',record['board']))}" required></label>
<label>서명 채널 <select name="channel"><option value="custom">제작자 서명</option><option value="official"{' selected' if fields.get('channel')=='official' else ''}>공식</option></select></label>
<label>독립 재빌드 이미지 SHA-256 <input name="image" maxlength="64" value="{tag(fields.get('image',''))}" required></label>
<label>별도 경로에서 확인한 부트 키 지문 <input name="bootRoot" maxlength="64" value="{tag(fields.get('bootRoot',''))}" required></label>
<button>설치 전 지문 대조</button></form>"""
    if result["state"]=="mismatch":
        html+="<p><b>설치 차단</b> · 리비전, 채널, 이미지 또는 부트 신뢰 키 지문이 다릅니다. 시드나 키를 등록하지 마세요.</p>"
    elif result["state"]=="fingerprints-match-evidence-pending":
        html+=f"<p>제출한 지문은 승인된 {tag(result['version'])} 이미지와 일치합니다. 실물 확인 전 설치 보류입니다.</p>"
    names={"schematic":"참고 회로도","pinctrl":"핀 제어 정의","target":"대상 장치 트리"}
    html+="<h2>출처 자료</h2><dl>"
    for item, details in record["files"].items():
        href=f"{record['upstream']}/blob/{record['commit']}/{details['file']}"
        html+=f'<dt><a href="{tag(href)}">{tag(names[item])}</a></dt><dd><code>{tag(details["sha256"])}</code></dd>'
    return html+"""</dl><p>실제 조립 BOM, 대체 부품의 공급 출처, 장착 리비전은 확인 대기입니다.</p>
<p>물리 부팅 확인 전에는 시드 입력과 보드 등록을 보류합니다. 설치 중단 뒤에는 같은 승인 이미지와 슬롯을 다시 확인합니다.</p>"""


def projection(db, page, scope, filters, review=False):
    if page == "D01":
        data = read(REGISTRY)
        operations = data["operations"]
        family = filters.get("family", "")
        role = filters.get("role", "")
        interface_path = REGISTRY.with_name("interfaces.json")
        interfaces = read(interface_path)["interfaces"]
        return {"errorCodes": data["errorCodes"], "recovery": data["recovery"],
                "commandReplay": data["commandReplay"], "selectedFamily": family, "selectedRole": role,
                "roles": sorted({role for value in operations.values() for role in value["roles"]}),
                "registry": digest(REGISTRY), "schemaVersion": data["schemaVersion"],
                "interfaces": [entry for entry in interfaces if not family or entry["id"] + "/" == family],
                "paymentQuote": read(REGISTRY.with_name("schema.json"))["quote"] if family in {"", "C2/"} else None,
                "operations": {name: value for name, value in operations.items()
                    if name.startswith(family) and (not role or role in value["roles"])},
                "contractFamilies": [entry["id"] for entry in interfaces]}
    if review:
        snapshot = db.execute("SELECT platform.fleet_review_snapshot(%s) AS report", (scope,)).fetchone()["report"]
        if snapshot is None:
            return None
        if page in {"D04","D05"}:
            overlay=db.execute("SELECT platform.fleet_review_overlay(%s) AS report",(scope,)).fetchone()["report"]
            for advisory in snapshot["advisories"]:
                state=overlay.get(advisory["id"],{})
                advisory["checks"]=state.get("checks",{})
                advisory["holdRevision"]=0
                if state.get("published"):
                    advisory["state"]="published"
                if state.get("hold"):
                    hold=state["hold"]
                    advisory.update(state="hold",holdRevision=hold["revision"],owner=hold["owner"],
                                    reason=hold["reason"],reviewBy=hold["reviewBy"])
            if page=="D04" and filters.get("case"):
                snapshot["advisories"]=[item for item in snapshot["advisories"] if item["id"]==filters["case"]]
                if len(snapshot["advisories"])!=1:
                    return None
            snapshot["workflow"]=filters.get("view","start")
        return {"state": "private-review", "review": True, **snapshot}
    if page == "D04":
        return {"state": "public", "advisories": [dict(row, state="published") for row in db.execute(
            """SELECT id::text,component,affected,versions,manifest
               FROM fleet_public_advisory WHERE cohort=%s ORDER BY id""", (scope,))]}
    row = db.execute("SELECT revision,manifest,signer,channel FROM fleet_public_cohort WHERE id=%s", (scope,)).fetchone()
    if not row:
        state={"state":"unpublished","targets":[]}
        if page=="D03":
            state["interopView"]="partial" if filters.get("view")=="partial" else "start"
        return board_projection(state,filters) if page=="D05" else state
    state = {**row, "state": "published", "releaseSigner": row["signer"]}
    state["targets"] = []
    for item in db.execute("""SELECT name,dimensions,version,mode,payload,counter,observation,boot_key,measurement,ios_profile,android_profile,interop
                              FROM fleet_public_target WHERE cohort=%s ORDER BY name""", (scope,)):
        if any(key in DIMENSIONS and str(item["dimensions"][key]) != value for key,value in filters.items()):
            continue
        if page == "D05" and item["dimensions"]["product"] != "P01":
            continue
        support = {"sdk":"consumer-install","abi":"abi-consumer","service":"service-install","physical":"physical"}
        state["targets"].append({**item,"status":support.get(item["mode"],"local-observation")})
    if page=="D03":
        state["interopView"]="partial" if filters.get("view")=="partial" else "start"
        if state["interopView"]=="partial":
            state["targets"]=[item for item in state["targets"] if item["mode"]!="physical"]
    return board_projection(state,filters) if page=="D05" else state


def archive_verifier(scope, model):
    sdk=[row for row in model["targets"] if row["mode"] in {"sdk","abi"}]
    if not sdk or not model.get("manifest"):
        return ""
    choices="".join(f'<option value="{tag(row["name"])}" data-version="{tag(row["version"])}" '
                    f'data-payload="{tag(row["payload"])}">{tag(row["name"])} {tag(row["version"])}</option>' for row in sdk)
    return f"""<section aria-label="SDK 출처 확인"><h2>설치 전 SDK 출처 확인</h2>
<p>브라우저에서만 파일의 해시와 릴리스 서명을 검사합니다.</p>
<p>등록한 키 <code>{tag(model['releaseSigner'])}</code> · 채널 {tag(model['channel'])}</p>
<form method="dialog" data-local-verify data-signer="{tag(model['releaseSigner'])}"
      data-manifest="{tag(model['manifest'])}" data-channel="{tag(model['channel'])}" data-scope="{tag(scope)}">
<label>SDK 조합 <select name="artifact">{choices}</select></label>
<label>로컬 SDK 아카이브 <input type="file" name="archive" required></label>
<label>서명 manifest <input type="file" name="manifest" required></label>
<label>manifest 서명 <input type="file" name="signature" required></label>
<label>릴리스 공개 키 <input type="file" name="key" required></label>
<button>출처 검증</button><p><output role="status" aria-live="polite"></output></p></form>
<p>{link("D05",scope)} · <a href="/D02">다른 릴리스 선택</a></p>
<script src="/assets/catalog-verify.js" defer></script></section>"""


def review_controls(scope, model, case):
    chosen=model.get("workflow","start")
    parts=[]
    for artifact,versions in sorted(case["oldVersions"].items()):
        parts.append(f"<p><b>{tag(artifact)}</b> · 잠금 버전 {tag(', '.join(versions))}</p>")
        checks=case["checks"].get(artifact,{})
        if chosen=="review":
            for key,label in (("sbom","후보 SBOM"),("rescan","보안 재검사"),("regression","제품 회귀"),("artifactSigning","산출물 서명")):
                item=checks.get(key)
                if not item:
                    parts.append(f"<p>{label} · 검증 자료 미제공</p>")
                    continue
                labels={"wrapper":"검증 기록","report":"보고서 해시","kind":"근거 유형","payload":"대상 해시",
                        "passed":"검증 결과","checks":"검사 수","components":"구성 요소 수","scanTargets":"스캔 대상 수",
                        "findings":"발견 항목 수","signature":"서명 해시","signer":"서명자 지문"}
                evidence="".join(f"<dt>{tag(labels[field])}</dt><dd><code>{tag(value)}</code></dd>"
                                 for field,value in item.items() if field in labels)
                parts.append(f"<details><summary>{label}</summary><dl>{evidence}</dl></details>")
    links=[(view,label) for view,label in (("review","수정 증거 검토"),("hold","보류 사유 기록")) if view!=chosen]
    parts.extend(f'<p><a href="/D04?{tag(urlencode({"cohort":scope,"case":case["id"],"view":view}))}">{label}</a></p>'
                 for view,label in links)
    roles=model.get("holdRoles",[])
    if chosen=="hold" and roles and case["state"]!="published":
        options="".join(f'<option value="{tag(role)}">{tag(role)}</option>' for role in roles)
        fields={"case":case["id"],"candidate":case["candidate"],"revision":str(case["holdRevision"]),"request":str(uuid4())}
        hidden="".join(f'<input type="hidden" name="{name}" value="{tag(value)}">' for name,value in fields.items())
        parts.append(f"""<form method="post" action="/D04/hold?cohort={tag(scope)}">{hidden}
<label>담당 역할 <select name="owner" required>{options}</select></label>
<label>보류 이유 <textarea name="reason" maxlength="512" required></textarea></label>
<label>다음 검토 시각 UTC <input name="reviewBy" required placeholder="2030-01-01T12:30Z"></label>
<button>보류 저장</button></form>""")
    return "".join(parts)


def render(page, scope, model):
    pages = ("D03", "D04", "D05") if model.get("review") else PAGES
    nav = " · ".join(link(item, scope) for item in pages)
    if page=="D02" and "releases" in model:
        html="<h2>공개 릴리스 선택</h2>"
        if not model["releases"]:
            html+="<p>공개 지원 조합이 없습니다.</p>"
        for release in model["releases"]:
            html+=f'<p><a href="/D02?cohort={tag(release["id"])}">{tag(release["channel"])} · {tag(release["manifest"])}</a></p>'
    elif page == "D01":
        html = f"<p>스키마 v{tag(model['schemaVersion'])} <code>{tag(model['registry'])}</code></p>"
        def options(values, selected):
            return "".join(f'<option value="{tag(value)}"{" selected" if value == selected else ""}>{tag(label)}</option>'
                           for value, label in values)

        filters = options([(family + "/", family) for family in model["contractFamilies"]], model["selectedFamily"])
        roles = options([(role, role) for role in model["roles"]], model["selectedRole"])
        html += f"""<form><input type="hidden" name="cohort" value="{tag(scope)}">
<label>계약 <select name="family"><option value="">전체</option>{filters}</select></label>
<label>역할 <select name="role"><option value="">전체</option>{roles}</select></label><button>조회</button></form>"""
        html += "".join(f"<p><b>{tag(entry['id'])}</b> · {tag(entry['source'])} → {tag(entry['target'])} · {tag(entry['auth'])}</p>"
                        for entry in model["interfaces"])
        if model["paymentQuote"] is not None:
            quote_fields = " · ".join(tag(name) for name, _ in model["paymentQuote"]["fields"])
            html += f"<details><summary>pay.prepare</summary><p>{quote_fields}</p></details>"
        html += "".join(f"<details><summary>{tag(name)}</summary><pre>{tag(json.dumps(row, indent=2))}</pre></details>"
                        for name, row in model["operations"].items())
        replay_labels = {
            "key": "작업 키", "sameIntent": "동일 작업 키와 본문", "changedIntent": "본문 충돌",
            "unknownResult": "결과 불명 대사", "scope": "테넌트와 객체 범위",
        }
        html += "<details><summary>명령 멱등성과 결과 대사</summary><dl>"
        html += "".join(f"<dt>{tag(replay_labels[key])}</dt><dd>{tag(rule)}</dd>"
                        for key, rule in model["commandReplay"].items())
        html += "</dl></details><details><summary>오류 코드와 복구</summary><dl>"
        html += "".join(f"<dt><code>{tag(code)}</code></dt><dd>{tag(model['recovery'].get(code, '—'))}</dd>"
                        for code in model["errorCodes"])
        html += "</dl></details>"
    elif page == "D03" and model.get("review"):
        pending = model["pending"]
        html = "<p>설치 전 검토</p>"
        if pending:
            reasons={"expired":"승인 만료 · 설치 보류","revoked":"서명 키 철회 · 설치 보류","installing":"설치 영수증 대기"}
            html += f"<p>{tag(reasons[pending['state']])}</p><pre>{tag(json.dumps(pending,indent=2))}</pre>"
        else:
            html += "<p>대기 중인 후보가 없습니다.</p>"
    elif page == "D05" and model.get("review"):
        html = "<p>보드 설치 검토</p>"
        for case in model["advisories"]:
            if case.get("boardCandidate") and case["state"]=="hold":
                html += f"<article><h2>설치 보류</h2><p>{tag(case.get('reason','보드 후보의 검사 조건이 충족되지 않았습니다.'))}</p>"
                html += f"<p>담당 역할 {tag(case['owner'])} · 재검토 {tag(review_time(case['reviewBy']))}</p>"
                html += f"<p>보류 후보 <code>{tag(case['candidate'])}</code></p></article>"
        html += link("D04",scope)
    elif page == "D04":
        html = "<p>SBOM 구성 요소, 영향 버전, 수정 후보 및 재검토 상태</p>"
        for advisory in model["advisories"]:
            versions = advisory.get("versions",advisory.get("newVersions",{}))
            patched = ", ".join(version for items in versions.values() for version in items)
            states = {"published":"공개 승인","hold":"배포 보류","review":"수정 후보 검토"}
            state = states.get(advisory["state"],advisory["state"])
            html += f"<article><h2>{tag(advisory['component'])}</h2><p>{tag(state)}</p>"
            html += f"<dl><dt>영향 버전</dt><dd>{tag(', '.join(advisory['affected']))}</dd><dt>수정 후보 버전</dt><dd>{tag(patched)}</dd></dl>"
            if model.get("review"):
                html += f"<p>담당 역할 {tag(advisory['owner'])} · 재검토 {tag(review_time(advisory['reviewBy']))}</p>"
                if advisory.get("reason"):
                    html += f"<p>보류 이유 {tag(advisory['reason'])}</p>"
                html += f"<p>검토 후보 manifest <code>{tag(advisory['candidate'])}</code></p>"
                html += review_controls(scope,model,advisory)
            else:
                html += f"<p>승인된 manifest <code>{tag(advisory['manifest'])}</code></p>"
            html += link("D03",scope) + "</article>"
        if not model["advisories"]:
            html += "<p>대상 채널의 검증 결과가 없습니다.</p>"
    else:
        html = f"<p>배포 상태 {tag(model['state'])}</p>"
        if page=="D05":
            html+=board_intro(scope,model)
        if page=="D03":
            view=model["interopView"]
            label="물리 링크 증거가 부족한 조합" if view=="partial" else "상호운용 증거 목록"
            target="start" if view=="partial" else "partial"
            action="증거 목록으로" if view=="partial" else "증거 부족 조합 보기"
            path=urlencode({"cohort":scope,"view":target})
            html+=f'<h2>{tag(label)}</h2><p><a href="/D03?{tag(path)}">{tag(action)}</a> · {link("D02",scope)}</p>'
        if page == "D02":
            options = " ".join(f'<label>{tag(field)} <input name="{field}"></label>' for field in DIMENSIONS)
            html += f'<form><input type="hidden" name="cohort" value="{tag(scope)}">{options}<button>조합 조회</button></form>'
            html += archive_verifier(scope,model)
        if not model["targets"]:
            html += "<p>일치하는 설치 증거가 없습니다. 지원 상태는 보류입니다.</p>"
        for row in model["targets"]:
            dimensions = " · ".join(tag(row["dimensions"][key]) for key in DIMENSIONS)
            html += f"<article><h2>{tag(row['name'])}</h2><p>{dimensions}</p><p>{tag(row['version'])} · {tag(row['status'])}</p>"
            if page == "D02":
                if row.get("ios_profile"):
                    label = {"fresh-install-only": "새 설치만 관측", "existing-profile": "연습 기록의 업데이트 및 복귀 관측"}
                    html += f"<p>{tag(label[row['ios_profile']])}</p>"
                if row.get("android_profile"):
                    android_labels={"practice-profile":"Android 연습 기록 유지 및 OS 롤백 관측",
                                    "protected-profile-local":"에뮬레이터의 C1 보호 기록 보존 및 OS 롤백 관측"}
                    label=android_labels.get(row["android_profile"],"새 설치만 관측")
                    html += f"<p>{tag(label)}</p>"
                html += f"<p>출시 manifest <code>{tag(model['manifest'])}</code></p>"
            if page == "D03":
                matrix={
                    "quote-bytes-and-signature":"원문 바이트·서명 대조",
                    "invalid-vectors":"기형·만료·재사용 입력 거절",
                    "fragments-and-reconnect":"분할 프레임과 재연결·재개",
                    "service-denial":"서비스 계약 권한 거절",
                    "v1-fragments-reset":"v1 프레임 분할, 순서 오류 및 재시작",
                }
                evidence=row.get("interop")
                if evidence:
                    language={"c":"C 최소 프로젝트","typescript":"TypeScript tarball 소비자",
                              "go":"Go 모듈 소비자","python":"Python wheel 소비자"}[evidence["language"]]
                    html+=f"<h3>{tag(language)}</h3><p>독립 호스트 프로토콜 시험</p>"
                    html+=f"<p>교차 언어 벡터 원문 <code>{tag(evidence['vectors'])}</code></p><ul>"
                    html+="".join(f"<li>{tag(matrix[check])}</li>" for check in evidence["checks"])
                    html+="</ul>"
                    if "fragments-and-reconnect" not in evidence["checks"]:
                        html+="<p>v2 프레임 재접속 검증 자료 미제공</p>"
                    html+="<p>실물 링크 지원 보류 · 실제 리비전과 OS 증거 대기</p>"
                else:
                    label="실기 설치 관측" if row["mode"]=="physical" else "상호운용 검증 자료 없음 · 지원 보류"
                    html+=f"<p>{tag(label)}</p>"
                html+=f"<p>서명 설치 기록 <code>{tag(row['observation'])}</code></p>"
            if page == "D05":
                html += f"<p>릴리스 키 지문 <code>{tag(model['releaseSigner'])}</code></p>"
                html += f"<p>보안 카운터 {tag(row['counter'])}</p><p>설치 이미지 <code>{tag(row['payload'])}</code></p>"
                if row.get("boot_key"):
                    html += f"<p>부트 신뢰 키 <code>{tag(row['boot_key'])}</code></p><p>보호 구간 해시 <code>{tag(row['measurement'])}</code></p>"
                html += "<p>채널과 승인 부트 키를 확인한 뒤 설치합니다. 모의 슬롯 결과는 현장 부팅 근거와 구분합니다.</p>"
            html += "</article>"
    return f"""<!doctype html><html lang="ko"><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>{PAGES[page]}</title><style>
body{{max-width:50rem;margin:clamp(.5rem,4vw,4rem) auto;padding:0 3vw;font:1rem system-ui;color:#16283f;background:#f5f7fc;overflow-wrap:anywhere}}
main{{padding:clamp(.5rem,3vw,2rem);background:white;border:1px solid #dce4f0;border-radius:.65rem}}
article{{border-top:1px solid #dce4f0}} pre{{overflow:auto;white-space:pre-wrap}}
label{{display:inline-flex;gap:.3em;flex-wrap:wrap;min-width:0;max-width:100%}}
input,select,button,textarea{{box-sizing:border-box;max-width:100%;min-width:0;font:inherit}}
</style><nav>{nav}</nav><main><h1>{PAGES[page]}</h1>{html}</main></html>""".encode()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        return

    def reviewer(self, scope, policy):
        actor, grant, _ = operator_identity(self.connection,policy,self.server.client_ca)
        if scope not in policy.get("cohorts",{}) or scope not in grant.get("cohorts",()) or "release-review" not in grant.get("roles",()):
            raise OperatorDenied("review role or scope differs")
        return actor

    def hold_identity(self,scope,owner=None):
        policy=read(self.server.policy_path)
        actor,grant,deadline=operator_identity(self.connection,policy,self.server.client_ca)
        roles=grant.get("holdRoles",[])
        if (scope not in policy.get("cohorts",{}) or scope not in grant.get("cohorts",()) or
                not {"release-review","release-hold"}<=set(grant.get("roles",())) or
                owner is not None and owner not in roles):
            raise OperatorDenied("release hold authorization differs")
        return actor,deadline,roles

    def hold_roles(self, scope):
        if not self.server.hold_database:
            return []
        try:
            actor,_,roles=self.hold_identity(scope)
        except OperatorDenied:
            return []
        with closing(database(self.server.hold_database)) as db:
            return [role for role in roles if db.execute("SELECT platform.fleet_hold_authorized(%s,%s,%s,false) AS allowed",
                                                        (scope,actor,role)).fetchone()["allowed"]]

    def do_POST(self):
        path=urlsplit(self.path)
        if not self.server.review or not self.server.hold_database or path.path not in {"/D04/hold","/api/D04/hold"}:
            return self.send_error(404)
        try:
            query=parse_qs(path.query,max_num_fields=1,strict_parsing=True)
            scope=uuid4_text(query["cohort"][0])
            actor,_,_=self.hold_identity(scope)
            size=int(self.headers.get("Content-Length","-1"))
            if not 0<size<=4096:
                return self.send_error(400)
            media=self.headers.get_content_type()
            if media=="application/x-www-form-urlencoded":
                if (self.headers.get("Origin")!="https://"+self.headers.get("Host","") or
                        self.headers.get("Sec-Fetch-Site","same-origin")!="same-origin"):
                    raise OperatorDenied("hold form origin differs")
                fields=parse_qs(self.rfile.read(size).decode(),max_num_fields=7,strict_parsing=True)
                if any(len(value)!=1 for value in fields.values()):
                    return self.send_error(400)
                body={key:value[0] for key,value in fields.items()}
            elif media=="application/json" and path.path=="/api/D04/hold":
                from evidence import parse
                body=parse(self.rfile.read(size))
            else:
                return self.send_error(400)
            if set(body)!={"case","candidate","revision","request","owner","reason","reviewBy"}:
                return self.send_error(400)
            advisory=uuid4_text(body["case"])
            candidate=text_hash(body["candidate"])
            token=uuid4_text(body["request"])
            version=int(body["revision"])
            owner,reason=body["owner"],body["reason"].strip()
            when=body["reviewBy"]
            if not (0<=version<2**63-1 and re.fullmatch(r"[a-z][a-z0-9-]{0,63}",owner) and
                    0<len(reason)<=512 and re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}Z",when)):
                return self.send_error(400)
            deadline=int(datetime.strptime(when,"%Y-%m-%dT%H:%MZ").replace(tzinfo=timezone.utc).timestamp())

            with closing(database(self.server.hold_database)) as db:
                def authorize():
                    current,expires,_=self.hold_identity(scope,owner)
                    if current!=actor or not db.execute(
                       "SELECT platform.fleet_hold_authorized(%s,%s,%s,true) AND clock_timestamp()<to_timestamp(%s) AS allowed",
                       (scope,actor,owner,expires)).fetchone()["allowed"]:
                        raise OperatorDenied("fleet hold enrollment changed")
                    return expires

                def write():
                    expires=authorize()
                    result=db.execute("SELECT platform.place_fleet_hold(%s,%s,%s,%s,%s,%s,%s,%s,%s,%s) AS placed",
                                      (scope,advisory,candidate,version,token,actor,owner,reason,deadline,expires)).fetchone()
                    authorize()
                    return result["placed"]

                if not tx(db,write):
                    return self.send_error(404)
            if path.path=="/api/D04/hold":
                payload=json.dumps({"case":advisory,"revision":version+1}).encode()
                self.send_response(200)
                self.send_header("Content-Type","application/json; charset=utf-8")
                self.send_header("Cache-Control","private, no-store")
                self.send_header("X-Content-Type-Options","nosniff")
                self.send_header("Content-Length",str(len(payload)))
                self.end_headers()
                return self.wfile.write(payload)
            query=urlencode({"cohort":scope,"case":advisory,"view":"hold"})
            self.send_response(303)
            self.send_header("Location","/D04?"+query)
            self.send_header("Cache-Control","private, no-store")
            self.send_header("Content-Length","0")
            self.end_headers()
        except OperatorDenied:
            self.send_error(404)
        except (ValueError,TypeError,KeyError,OverflowError):
            self.send_error(400)
        except (psycopg2.Error,OSError,RuntimeError):
            self.send_error(503)

    def do_GET(self):
        path = urlsplit(self.path)
        if path.path=="/assets/catalog-verify.js" and not path.query and not self.server.review:
            body=Path(__file__).with_name("catalog_verify.js").read_bytes()
            self.send_response(200)
            self.send_header("Content-Type","text/javascript; charset=utf-8")
            self.send_header("X-Content-Type-Options","nosniff")
            self.send_header("Cache-Control","no-store")
            self.send_header("Content-Length",str(len(body)))
            self.end_headers()
            return self.wfile.write(body)
        match = re.fullmatch(r"/(?:(api)/)?(D0[1-5])", path.path)
        try:
            args = parse_qs(path.query, max_num_fields=10)
            if match is None or any(len(values) != 1 for values in args.values()):
                return self.send_error(404)
            api, page = match.groups()
            params = {name: values[0] for name, values in args.items()}
            scope = params.pop("cohort", "")
            policy = read(self.server.policy_path)
            spec = policy.get("cohorts", {}).get(scope)
            review = self.server.review
            if review:
                if not (page in {"D03","D04","D05"} and spec):
                    raise OperatorDenied("unknown private review")
                actor = self.reviewer(scope,policy)
            elif page != "D01" and not (page=="D02" and not scope and not params) and (not spec or spec.get("public") is not True):
                return self.send_error(404)
            if page == "D01":
                model = projection(None, page, scope, params)
            else:
                with closing(database(self.server.database)) as db:
                    db.execute("SET default_transaction_read_only TO on")
                    if page=="D02" and not scope:
                        allowed=[key for key,item in policy["cohorts"].items() if item.get("public") is True]
                        model={"releases":db.execute("""SELECT id::text,manifest,channel FROM fleet_public_cohort
                                                       WHERE id=ANY(%s::uuid[]) ORDER BY manifest""",(allowed,)).fetchall()}
                    else:
                        model = projection(db, page, scope, params, review=review)
            if model is None:
                return self.send_error(404)
            if review and page=="D04":
                model["holdRoles"]=self.hold_roles(scope)
            body = json.dumps(model, ensure_ascii=False).encode() if api else render(page, scope, model)
            if review:
                with closing(database(self.server.database)) as db:
                    permitted = db.execute("SELECT platform.fleet_review_authorized(%s) AS allowed",(scope,)).fetchone()
                if not permitted["allowed"] or self.reviewer(scope,read(self.server.policy_path)) != actor:
                    raise OperatorDenied("review enrollment changed")
        except OperatorDenied:
            return self.send_error(404)
        except (psycopg2.Error, ValueError, KeyError, OSError):
            return self.send_error(503)
        self.send_response(200)
        self.send_header("Content-Type", ("application/json" if api else "text/html") + "; charset=utf-8")
        scripts="; script-src 'self'; connect-src 'self'" if page=="D02" and not api else ""
        self.send_header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; form-action 'self'"+scripts)
        self.send_header("Cache-Control", "private, no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", required=True)
    parser.add_argument("--policy", type=Path, required=True)
    parser.add_argument("--port", type=int, default=8445)
    parser.add_argument("--review", action="store_true")
    parser.add_argument("--hold-database")
    for label in ("cert", "key", "client-ca"):
        parser.add_argument("--" + label, type=Path)
    args = parser.parse_args()
    if args.review and not all((args.cert,args.key,args.client_ca)):
        parser.error("private review requires the operator mTLS certificate, key and client CA")
    with ThreadingHTTPServer(("127.0.0.1", args.port), Handler) as server:
        server.database, server.policy_path, server.review = args.database, args.policy, args.review
        server.client_ca = args.client_ca
        server.hold_database=args.hold_database
        if args.review:
            tls = operator_tls(args.cert,args.key,args.client_ca)
            server.socket = tls.wrap_socket(server.socket, server_side=True)
        server.serve_forever()


if __name__ == "__main__":
    main()
