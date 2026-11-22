"""Keep only known stages, XCTest counts and aggregate P03 outcomes."""

import json
from pathlib import Path
import re
import sys


def read(directory, name):
    file = directory / name
    return file.read_text(errors="replace") if file.exists() else ""


def summarize(directory, status):
    evidence = {"exitStatus": status}
    try:
        apple = json.loads(read(directory, "apple-summary.json"))
    except ValueError:
        apple = {}
    for field in ("passedTests", "failedTests", "skippedTests", "totalTestCount", "duration"):
        if isinstance(apple.get(field), (int, float)):
            evidence[field] = apple[field]
    for job in ("ui-build", "app-build"):
        log = read(directory, job + ".log")
        if log:
            evidence[job + "Succeeded"] = bool(re.search(r"(?m)^\*\* (?:TEST )?BUILD SUCCEEDED \*\*$", log))
    ui = read(directory, "ui.log")
    duration = re.search(r"(?m)^Test Case '-\[SceneSmoke\.SceneSmoke test\w+\]' (?:passed|failed) \(([0-9.]+) seconds\)\.$", ui)
    if duration:
        evidence["uiSeconds"] = float(duration[1])
    stage = (
        "market_wallet_selected|market_quote_visible|market_quote_expired|market_quote_replaced|"
        "market_consent_reviewed|market_original_hash_retained|market_receipt_confirmed|"
        "w20_entry_and_wallet_choice|w20_reopened_enrolled_wallet|w20_registered_provider_(?:login|pending|active)"
    )
    if ui:
        evidence["stages"] = re.findall(rf"(?m)^P03_STAGE ({stage})$", ui)
    backend = read(directory, "backend.log")
    if backend:
        evidence["backendSetup"] = {
            "postgresStarted": "server started" in backend,
            "legacyUpgradePassed": "PASS P03 migration preserves legacy wallet" in backend,
            "signRefusalUpgradePassed": "PASS P03 v6 migration preserves legacy approval and signature receipts" in backend,
            "walletGatePassed": "--- PASS: TestPostgresWallet" in backend,
            "refusalGatePassed": "--- PASS: TestPostgresRecoveryRefusalAndReview" in backend,
            "ceremonyStarted": "=== RUN   TestRealCeremonies" in backend,
        }
        outcomes = (
            "market wallet independently approved an isolated order in iOS Safari|"
            "expired browser market quote emitted no wallet signature|"
            "verified two active browser-enrolled wallets with distinct public keys and signature receipts|"
            "verified one completed DKG and zero signing requests for the registered OIDC subject"
        )
        evidence["database"] = re.findall(rf"(?m)^\s+(?:process_test|browser_lab_test)\.go:\d+: ({outcomes})$", backend)
        result = re.search(r"(?m)^--- (PASS|FAIL): TestRealCeremonies \(([0-9.]+)s\)$", backend)
        if result:
            evidence["ceremony"] = {"status": result[1], "seconds": float(result[2])}
    evidence["recoveryProgress"] = [int(x) for x in re.findall(r"(?m)^P03_RECOVERY ([0-9]+)$", ui)]
    if "P03_RECOVERY 3" in ui:
        recovery_only = ui.split("P03_RECOVERY 3", 1)[1].split("P03_RECOVERY 4", 1)[0]
        taps = len(re.findall(r'(?m)^\s*t = .* Tap "독립 복구 확인" Button$', recovery_only))
        evidence["recoveryBrowserTapCount"] = taps
        marker = re.search(r"(?m)^P03_RECOVERY_BROWSER_FIRST_TAP ([01])$", recovery_only)
        if marker:
            evidence["recoveryBrowserFirstTapOpened"] = marker[1] == "1"
        elif taps == 1 and ("P03_RECOVERY 4" in ui or "P03_DENIED_UI:" in ui):
            evidence["recoveryBrowserFirstTapOpened"] = True
    evidence["transferFieldProgress"] = [int(x) for x in re.findall(r"(?m)^P03_TRANSFER_FIELD ([1-5])$", ui)]
    evidence["assertionLines"] = [int(x) for x in re.findall(r"SceneSmoke[.]swift:(\d+)(?::\d+)?: error:", ui)]
    evidence["testRunnerTimeExceeded"] = "exceeded execution time allowance" in ui
    evidence["recoveryLedgerVerified"] = "verified closed wallet, same-key epoch-2 recovery, old-participant fences, new-committee acknowledgements and revoked original credential" in backend
    for field, marker, log in (
        ("signRefusalOriginalUiVerified", "P03_SIGN_REFUSAL_ORIGINAL_UI", ui),
        ("signRefusalBrowserCommitted", "P03_SIGN_REFUSAL_DELIVERED:", backend),
        ("signRefusalTerminalUiVerified", "P03_SIGN_REFUSAL_TERMINAL_UI", ui),
        ("signRefusalFreshUiVerified", "P03_SIGN_REFUSAL_FRESH_UI", ui),
        ("signRefusalLedgerVerified", "P03_SIGN_REFUSAL_DB:", backend),
        ("recoveryRefusalUiVerified", "P03_DENIED_UI:", ui),
        ("recoveryRefusalLedgerVerified", "P03_DENIED_DB:", backend),
        ("recoveryReviewPermitVerified", "P03_REVIEW_DB:", backend),
        ("lostFreezeReceiptUiVerified", "P03_RESUME_RECOVERY_UI", ui),
        ("lostClosureReceiptUiVerified", "P03_RESUME_CLOSURE_UI", ui),
        ("receiptCardinalityVerified", "P03_RESUME_DB:", backend),
        ("partialAckUiVerified", "P03_PARTIAL_UI ", ui),
        ("partialAckLedgerVerified", "P03_PARTIAL_DB:", backend),
    ):
        evidence[field] = marker in log
    return evidence


if __name__ == "__main__":
    path = Path(sys.argv[1])
    (path / "summary.json").write_text(json.dumps(summarize(path, int(sys.argv[2])), indent=2) + "\n")
