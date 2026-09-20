#!/usr/bin/env python3
"""Generate the one-page product and communication architecture diagram."""

from html import escape
from pathlib import Path


WIDTH, HEIGHT = 2400, 1500
HERE = Path(__file__).resolve().parent
OUTPUT = HERE / "product-architecture-one-page.svg"


def lines(x, y, values, *, size=24, color="#334155", weight=400, anchor="middle", gap=34):
    spans = []
    for index, value in enumerate(values):
        dy = 0 if index == 0 else gap
        spans.append(
            f'<tspan x="{x}" dy="{dy}">{escape(value)}</tspan>'
        )
    return (
        f'<text x="{x}" y="{y}" text-anchor="{anchor}" '
        f'font-size="{size}" font-weight="{weight}" fill="{color}">'
        + "".join(spans)
        + "</text>"
    )


def zone(x, y, width, height, title, subtitle, fill, stroke):
    return "".join(
        [
            f'<rect x="{x}" y="{y}" width="{width}" height="{height}" rx="24" '
            f'fill="{fill}" stroke="{stroke}" stroke-width="3"/>',
            f'<rect x="{x}" y="{y}" width="{width}" height="72" rx="24" '
            f'fill="{stroke}" opacity="0.12"/>',
            f'<text x="{x + 28}" y="{y + 34}" font-size="24" font-weight="700" fill="#0f172a">'
            f'{escape(title)}</text>',
            f'<text x="{x + 28}" y="{y + 58}" font-size="16" fill="#475569">'
            f'{escape(subtitle)}</text>',
        ]
    )


def product(x, y, width, height, code, title, work, fill, stroke):
    work_lines = work.split("|")
    heading_y = y + 42
    work_y = y + 90
    heading = escape(title) if not code else f'{escape(code)} <tspan font-weight="650">· {escape(title)}</tspan>'
    return "".join(
        [
            f'<g filter="url(#shadow)"><rect x="{x}" y="{y}" width="{width}" height="{height}" '
            f'rx="18" fill="{fill}" stroke="{stroke}" stroke-width="3"/></g>',
            f'<rect x="{x}" y="{y}" width="10" height="{height}" rx="5" fill="{stroke}"/>',
            f'<text x="{x + 28}" y="{heading_y}" font-size="26" font-weight="800" fill="#0f172a">'
            f'{heading}</text>',
            lines(x + width / 2, work_y, work_lines, size=20, color="#334155", gap=30),
        ]
    )


def path(d, *, color="#64748b", width=4, both=False, dash=None):
    dash_attr = f' stroke-dasharray="{dash}"' if dash else ""
    start = ' marker-start="url(#arrowStart)"' if both else ""
    return (
        f'<path d="{d}" fill="none" stroke="{color}" stroke-width="{width}" '
        f'stroke-linecap="round" stroke-linejoin="round" marker-end="url(#arrowEnd)"'
        f'{start}{dash_attr}/>'
    )


def badge(x, y, label, fill="#ffffff", stroke="#64748b"):
    width = max(54, 22 * len(label) + 26)
    return "".join(
        [
            f'<rect x="{x - width / 2}" y="{y - 22}" width="{width}" height="40" rx="20" '
            f'fill="{fill}" stroke="{stroke}" stroke-width="2"/>',
            f'<text x="{x}" y="{y + 7}" text-anchor="middle" font-size="19" '
            f'font-weight="800" fill="#334155">{escape(label)}</text>',
        ]
    )


def legend_item(x, y, code, title, detail, color):
    return "".join(
        [
            f'<circle cx="{x}" cy="{y}" r="20" fill="{color}"/>',
            f'<text x="{x}" y="{y + 7}" text-anchor="middle" font-size="16" '
            f'font-weight="800" fill="#ffffff">{escape(code)}</text>',
            f'<text x="{x + 34}" y="{y - 2}" font-size="18" font-weight="700" fill="#0f172a">'
            f'{escape(title)}</text>',
            f'<text x="{x + 34}" y="{y + 22}" font-size="15" fill="#475569">'
            f'{escape(detail)}</text>',
        ]
    )


parts = [
    f'<svg xmlns="http://www.w3.org/2000/svg" width="{WIDTH}" height="{HEIGHT}" viewBox="0 0 {WIDTH} {HEIGHT}">',
    """<defs>
      <filter id="shadow" x="-20%" y="-20%" width="140%" height="150%">
        <feDropShadow dx="0" dy="5" stdDeviation="8" flood-color="#0f172a" flood-opacity="0.10"/>
      </filter>
      <marker id="arrowEnd" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
        <path d="M 0 0 L 10 5 L 0 10 z" fill="#64748b"/>
      </marker>
      <marker id="arrowStart" viewBox="0 0 10 10" refX="1" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
        <path d="M 10 0 L 0 5 L 10 10 z" fill="#64748b"/>
      </marker>
      <style>text { font-family: "Apple SD Gothic Neo", "Noto Sans KR", Arial, sans-serif; }</style>
    </defs>""",
    '<rect width="2400" height="1500" fill="#f8fafc"/>',
    '<text x="70" y="58" font-size="38" font-weight="850" fill="#0f172a">NU-54V-DK Travel Wallet Ecosystem</text>',
    '<text x="70" y="93" font-size="20" fill="#475569">10개 제품의 책임, 실행 위치, 통신 경계를 한 페이지에 정리한 구현 전 아키텍처</text>',
    '<text x="2330" y="76" text-anchor="end" font-size="17" fill="#64748b">WBS 기준 · DF-20260920-01</text>',
    zone(50, 125, 610, 875, "LOCAL", "사용자 · 매장 · 실기기", "#f7fbff", "#3b82f6"),
    zone(700, 125, 1060, 875, "CLOUD", "서비스 · 운영 · 데이터", "#f6fcf8", "#22a55b"),
    zone(1800, 125, 550, 875, "BLOCKCHAIN", "StableNet Testnet · Chain ID 8283", "#fbf8ff", "#8b5cf6"),
]

# Connections are rendered first so product cards stay legible above them.
parts.extend(
    [
        path("M355 420 L355 485", both=True),
        badge(355, 452, "C1", "#e8f2ff", "#3b82f6"),
        path("M235 405 C90 480 90 760 235 790", both=True),
        badge(65, 605, "C2", "#e8f2ff", "#3b82f6"),
        path("M610 585 C680 585 680 315 750 315", both=True),
        badge(680, 472, "C3/C8", "#e8f2ff", "#3b82f6"),
        path("M610 835 C675 835 675 345 750 345", both=True),
        badge(678, 715, "C3", "#e8f2ff", "#3b82f6"),
        path("M610 555 C690 555 680 180 1250 180 L1250 210", both=True),
        badge(930, 180, "C4", "#eaf8ef", "#22a55b"),
        path("M1200 250 C1450 165 1650 165 1840 250"),
        badge(1510, 165, "C5", "#f1ebff", "#8b5cf6"),
        path("M1710 300 L1840 300"),
        badge(1775, 300, "C5", "#f1ebff", "#8b5cf6"),
        path("M2075 420 L2075 500"),
        path("M2075 720 L2075 620", both=True),
        path("M1840 560 C1780 560 1780 690 1600 690 L1320 690 C1260 690 1230 670 1230 660"),
        badge(1600, 690, "C6", "#f1ebff", "#8b5cf6"),
        path("M1230 480 C1120 430 1010 420 970 400"),
        path("M1080 560 L1040 560", both=True),
        path("M1380 560 L1420 560", both=True),
        path("M1230 660 L1230 750", both=True),
        badge(1230, 705, "C7", "#eaf8ef", "#22a55b"),
        path("M1000 400 C1060 455 1060 700 1100 750", both=True),
        badge(1060, 700, "C8", "#eaf8ef", "#22a55b"),
        path("M1100 400 C1200 440 1450 440 1570 480", both=True),
        badge(1400, 440, "C8", "#eaf8ef", "#22a55b"),
        path("M1720 565 C1790 565 1780 820 1840 820"),
        badge(1780, 700, "C5", "#f1ebff", "#8b5cf6"),
    ]
)

parts.extend(
    [
        product(100, 210, 510, 210, "P01", "NU-54V-DK", "HW Wallet · Key Lifecycle · Payment|FOTA · Passkey · Record · Find · Stamp|BLE session · Resource Arbitration", "#e8f2ff", "#3b82f6"),
        product(100, 485, 510, 200, "P02", "User App", "Social Login · Device Setup · FOTA|HW/Cloud Wallet · Approval · Receipt|Travel · DApp · Asset Experience", "#e8f2ff", "#3b82f6"),
        product(100, 750, 510, 180, "P04", "RN Kiosk", "Merchant · Menu · Order · Payment|Refund · Sales · Settlement · Roles", "#e8f2ff", "#3b82f6"),
        product(750, 210, 450, 190, "P10", "Common Platform", "API · AuthZ · Policy · Audit|Config · CI · Release · Observability|Acceptance · Recovery · Handover", "#eaf8ef", "#22a55b"),
        product(1250, 210, 460, 180, "P03", "Cloud MPC Wallet", "2-of-3 DKG · Account Binding|Approval · Sign · Refresh · Recovery", "#eaf8ef", "#22a55b"),
        product(740, 480, 300, 180, "P05", "Backoffice", "Merchant · Device|Rental · FOTA|Reconcile · Audit", "#eaf8ef", "#22a55b"),
        product(1080, 480, 300, 180, "P07", "Indexer", "RPC Ingest · Reorg|Decode · Projection|Query · Explorer", "#eaf8ef", "#22a55b"),
        product(1420, 480, 300, 180, "P08", "Market Service", "DEX · TEST FX|Perpetual · Oracle|Keeper · Risk", "#eaf8ef", "#22a55b"),
        product(990, 750, 480, 180, "P09", "Travel & AI", "Place · Verified Review · Footprint|Audio · AI Course · Challenge · Stamp|Consent · Deletion · Benefit", "#eaf8ef", "#22a55b"),
        product(1840, 210, 470, 210, "P06", "Contract Product", "dummy USDC · WKRC · Smart Account|DID · CafePass/STO · x402|Deployment Manifest · ABI · Events", "#f1ebff", "#8b5cf6"),
        product(1840, 500, 470, 120, "", "Chain Event Log", "Blocks · Transactions · Events", "#f1ebff", "#8b5cf6"),
        product(1840, 720, 470, 200, "", "Market Contracts", "AMM Pool · Swap · LP · TEST FX|Position · Funding · Liquidation|Oracle · Keeper Execution", "#f1ebff", "#8b5cf6"),
    ]
)

# External providers and communication legend.
parts.extend(
    [
        zone(50, 1040, 2300, 380, "EXTERNAL & COMMUNICATION", "외부 제공자와 경계 간 프로토콜", "#ffffff", "#f59e0b"),
        product(90, 1135, 310, 150, "", "Google · Apple", "OIDC · PKCE|Identity Binding", "#fff5dc", "#f59e0b"),
        product(430, 1135, 390, 150, "", "Kakao Local · OpenAI", "Place · Transcription|Recommendation Provider API", "#fff5dc", "#f59e0b"),
        path("M245 1135 C245 1020 330 980 355 685", dash="12 10"),
        path("M625 1135 C840 1060 1080 1010 1230 930", dash="12 10"),
        badge(520, 1030, "C9", "#fff5dc", "#f59e0b"),
        legend_item(900, 1130, "C1", "기기 ↔ 유저 앱", "BLE CBOR v1 · SMP · CTAP · Audio", "#3b82f6"),
        legend_item(900, 1210, "C2", "기기 ↔ 키오스크", "임시 BLE 결제 승인 · 물리 확인", "#3b82f6"),
        legend_item(900, 1290, "C3", "Local ↔ Cloud", "HTTPS · JSON/JCS · 인증 세션", "#3b82f6"),
        legend_item(1390, 1130, "C4", "앱 ↔ Cloud MPC", "별도 모바일 승인 · 지갑 결과", "#22a55b"),
        legend_item(1390, 1210, "C5", "Cloud ↔ Chain", "EOA/MPC 제출 · 컨트랙트 호출", "#8b5cf6"),
        legend_item(1390, 1290, "C6", "Chain → Indexer", "RPC · Log · Reorg · Backfill", "#8b5cf6"),
        legend_item(1880, 1130, "C7", "Indexer → Cloud", "결제 · 상품 · 대사 Projection", "#22a55b"),
        legend_item(1880, 1210, "C8", "앱 ↔ 도메인", "여행 · 시장 Intent/Result는 P10 API 경유", "#22a55b"),
        legend_item(1880, 1290, "C9", "External Provider", "OIDC · 장소 · 전사 · 추천 API", "#f59e0b"),
        '<text x="70" y="1470" font-size="15" fill="#64748b">Source: product-worklist-and-12week-wbs.md · 실선은 제품 간 런타임 경계, 점선은 외부 제공자 연동을 뜻한다.</text>',
        "</svg>",
    ]
)

OUTPUT.write_text("".join(parts), encoding="utf-8")
print(OUTPUT)
