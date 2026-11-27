#!/usr/bin/env python3
"""Run the commercial integration tests with fresh PostgreSQL and isolated Anvil fixtures."""

import argparse
import collections
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

if __name__ == "__main__":
    sys.dont_write_bytecode = True

from integration_workspace import integration_workspace, run_command, start_postgres
from contract_forge import current_compiler
from pos_upgrade_fixture import run_upgrade
from test_travel_booking_integration import published_fixture

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "products/p10-platform/database"))
from test_composition import (CAFE_PICKUP_UPGRADE_RESULT, CUSTOMER_CAFE_FORWARD_UPGRADE_RESULT,
                              CUSTOMER_CAFE_REVIEW_UPGRADE_RESULT, RENTER_ESCROW_DEADLINE_UPGRADE_RESULT,
                              RENTER_PAYMENT_ESCROW_UPGRADE_RESULT,
                              RETURN_ISSUANCE_ARCHIVE_UPGRADE_RESULT, RETURN_PICKUP_UPGRADE_RESULT,
                              RETURN_RECEIPT_UPGRADE_RESULT, TRAVEL_PICKUP_UPGRADE_RESULT, W10_GAS_UPGRADE_RESULT)

ROOT = Path(__file__).resolve().parents[1]
P05 = "products/p05-operations-backoffice"
P07 = "products/p07-indexer"
P08 = "products/p08-market-services"
P09 = "products/p09-travel-ai"
USER = "nu54_integration"
P05_SLOT_OPEN_TEST = "TestRentalSlotOpenMatchesP01"
COMPOSITION_REQUIRED = (W10_GAS_UPGRADE_RESULT, RETURN_PICKUP_UPGRADE_RESULT,
                        RETURN_ISSUANCE_ARCHIVE_UPGRADE_RESULT, RETURN_RECEIPT_UPGRADE_RESULT,
                        TRAVEL_PICKUP_UPGRADE_RESULT, CAFE_PICKUP_UPGRADE_RESULT, CUSTOMER_CAFE_FORWARD_UPGRADE_RESULT,
                        CUSTOMER_CAFE_REVIEW_UPGRADE_RESULT, RENTER_PAYMENT_ESCROW_UPGRADE_RESULT,
                        RENTER_ESCROW_DEADLINE_UPGRADE_RESULT)
RCR1_REQUIRED = {
    "--zero-withdrawal-p05": "ProtectedZeroWithdrawalHTTP/QualifiedStationIssuesOriginal/AuthenticatedExpiredOriginalReceipt",
    "--protected-withdrawal-p05": "ProtectedWithdrawalHTTP/QualifiedStationIssuesPaidOriginal/AuthenticatedExpiredOriginalReceipt",
}
K05_NEARBY_C2_TEST = "TestLoopbackK05NearbyCafeC2"
K05_RECONCILIATION_TEST = "TestLoopbackCustomerOrderReceipt/KioskCafeBeforeFinality"
DIRECT_CAFE_TEST = "TestPostgresCustomerCafeForwardReceipt"
DIRECT_CAFE_REQUIRED = {f"{DIRECT_CAFE_TEST}/{case}" for case in (
    "initial_sponsored_submission", "same_key_receipt", "legacy_unsponsored_customer")}
W10_RENTAL_REVIEW_TEST = "TestPostgresCustomerRentalReviewProof"
W10_RENTAL_DECISION_TEST = "TestPostgresCustomerRentalDecisionForward"
W10_RENTAL_DECISION_REQUIRED = {f"{W10_RENTAL_DECISION_TEST}/{case}" for case in (
    "refusal_has_no_payer_approval", "signed_approval_admits_once_and_reopens_original")}
W10_ESCROW_ANCHOR_REQUIRED = {f"TestRentalEscrowUsesReviewedFinalizedGeneration/{case}" for case in (
    "before_reviewed_origin", "at_reviewed_origin", "different_hash_at_reviewed_height",
    "later_finalized", "one_unit_short")}
W10_ESCROW_PRESENTATION_REQUIRED = {f"TestRentalEscrowPresentationSignsEarliestAuthorityDeadline/{case}" for case in (
    "payer_enrollment_first", "chain_evidence_first", "original_quote_first",
    "rounded_deadline_already_reached")}
W11_RECEIPT_REQUIRED = {
    "TestLoopbackCustomerOrderReceipt/KioskNearbyPaidSeparateOriginal",
    "TestLoopbackCustomerOrderReceipt/KioskCafeAfterFulfillment",
    "TestLoopbackCustomerOrderReceipt/KioskNearbyPickupSeparateOrder",
    "TestLoopbackCustomerOrderReceipt/W11NearbyClosedOriginal",
}
P05_COMMERCE_REQUIRED = {
    "TestP01AcceptsTwoP05Quotes", "TestPostgresLastStockAndSinglePayerWithRestart",
    "TestPostgresTerminalModeUsesActiveOperatorAndRevision", "TestPostgresQRPairingRequiresMatchingTerminalAndCustomer",
    "TestPostgresRegisteredPOSSelection",
    "TestPostgresProtectedCloseCandidate", "TestProtectedCloseExecutorPreservesExactAmbiguousTransaction",
    "TestTravelPublicLookupAndLastSeatsTLS", "TestTravelRejectsWrongModeAndMixedCommerceEnvelope",
    "TestFirmwareVerifiesCountedReleaseImage", "TestPostgresOperatorCatalogRealTLS",
    "TestPostgresOperatorCatalogRealTLS/OriginalMenuDenialSurvivesRestock",
    "TestPostgresOperatorCatalogRealTLS/NearbyPrivateReceiptAndPublicCheckout",
    "TestPickupCartCommitsExactApprovedTime", "TestPickupWindowSerializesLastPlaceAndRetainsTheOriginalSale",
    "TestPickupLifecycleEnvelopeCannotMixBookingAndTerminalChanges",
    "TestPostgresPublicMenuCancellationFencesHandoff", "TestPostgresRentalEvidenceWriterScope",
    "TestPostgresPrivateRentalCollectorAuthority", "TestPostgresRentalRuntimeWriterAuthenticatesBoardAndP07",
    "TestPostgresCustomerContract",
    P05_SLOT_OPEN_TEST,
}


def verify_w10_rental_decision(pilot_output, scratch, dsn, env):
    if pilot_output is not None:
        return 0
    peer = scratch / "w10-c2-board"
    run(["sh", "packages/protocol/v2/testdata/test-transport.sh"],
        env={**env, "NU54_MOBILE_TEST_PEER_OUTPUT": str(peer)})
    return verified_test(["go", "test", "-json", "-count=1", "-run", f"^{W10_RENTAL_DECISION_TEST}$",
        f"./{P05}/internal/settlement"], W10_RENTAL_DECISION_REQUIRED, {
            **env, "P05_SETTLEMENT_TEST_DATABASE_URL": dsn("customer_cafe"),
            "NU54_W10_RENTER_BOARD_BIN": str(peer),
            "NU54_W10_RENTER_BOARD_BRIDGE": str(ROOT / "scripts/android/w10-smoke/renter-board.mts"),
        })


def verify_k05_nearby_c2(pilot_output, scratch, dsn, env):
    if pilot_output is not None:
        return 0
    return verified_test(
        ["sh", f"{P05}/test/settlement/anvil.sh", "-json", "-run", f"^{K05_NEARBY_C2_TEST}$"],
        {K05_NEARBY_C2_TEST},
        {**env, "P05_SETTLEMENT_TEST_DATABASE_URL": dsn("settlement_cafe"),
         "NU54_P01_INSTALL_TEST": str(scratch / "p01" / "test_install")},
    )


def run(args, env, **kwargs):
    return run_command(args, cwd=ROOT, env=env, check=True, **kwargs)


def build_p05_host_inputs(build, env):
    run(["cmake", "-S", "products/p01-device-firmware/test", "-B", str(build), "-DCMAKE_BUILD_TYPE=Debug"], env)
    run(["cmake", "--build", str(build), "--target", "test_install", "test_session"], env)
    return {
        "NU54_P01_INSTALL_TEST": str(build / "test_install"),
        "P05_W12_SLOT_OPEN_HOST": str(build / "test_session"),
    }


def verified_test(args, required, env):
    # go test -json can succeed even if the requested integration never ran.
    proc = run_command(args, cwd=ROOT, env=env, stdout=subprocess.PIPE, text=True)
    events = collections.defaultdict(collections.Counter)
    skips = []
    for line in proc.stdout.splitlines(keepends=True):
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            print(line, end="", flush=True)
            continue
        action = event.get("Action")
        name = event.get("Test")
        if action in {"output", "build-output"}:
            print(event["Output"], end="", flush=True)
        if name:
            events[name][action] += 1
        if action == "skip":
            skips.append(name or event.get("Package"))
    missing = sorted(name for name in required if events[name]["run"] != 1 or events[name]["pass"] != 1)
    if proc.returncode or skips or missing:
        raise SystemExit(f"Integration suite failed (exit={proc.returncode}, skipped={skips}, required not passed={missing})")
    print(f"PASS {len(required)} required named tests, skips=0", flush=True)
    return len(required)


def verify_composition(proc):
    results = collections.Counter(proc.stdout.splitlines())
    failed = [marker for marker in COMPOSITION_REQUIRED if results[marker] != 1]
    if proc.returncode or failed:
        raise SystemExit(f"P05 database composition failed (exit={proc.returncode}, missing or repeated={failed})")
    return len(COMPOSITION_REQUIRED)


def secure_env():
    return {k: v for k, v in os.environ.items() if not k.startswith(("PG", "P05_", "P07_", "P08_", "P09_"))
            and k not in {"ANVIL", "FORGE", "CAST", "NU54_P01_INSTALL_TEST", "NU54_PROTECTED_WITHDRAWAL_RECEIPTS"}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--postgres-bin", type=Path, default=os.environ.get("POSTGRES_BIN"), help="directory containing the local PostgreSQL tools")
    parser.add_argument("--pilot-output", type=Path, help="empty folder for local pilot measurements")
    parser.add_argument("--public-hold-seconds", type=int, default=300, choices=range(1, 301), metavar="1–300")
    parser.add_argument("--bulk", action="store_true", help="extend the pilot with 100 whole-process payments and inboxes")
    parser.add_argument("--independent", action="store_true", help="normal opsd issuance with 100 independent customers")
    parser.add_argument("--independent-same-sku", action="store_true",
                        help="run the independent pilot with two options of one SKU")
    parser.add_argument("--human-units", action="store_true", help="run 10, 20 and 3 TestUSDC using six-decimal units")
    args = parser.parse_args()
    if args.independent_same_sku:
        if args.public_hold_seconds != 300:
            parser.error("--independent-same-sku requires the full 300-second public hold")
        args.independent = True
    if args.independent:
        args.bulk = True
    if args.bulk and not args.pilot_output:
        parser.error("--bulk requires --pilot-output")

    def pg(tool):
        return str(args.postgres_bin.resolve() / tool) if args.postgres_bin else tool

    suite_env = secure_env()
    suite_env.pop("NU54_K10_INSTALLED_RUNNER", None)
    suite_env.pop("NU54_K10_LIVE_HOLD", None)
    with integration_workspace("nu54-pg-", pg("pg_ctl"), "data", suite_env) as (scratch, env):
        run(["python3", "scripts/verify_ci_inputs.py", "go", "node", "foundry", "cast"], env)
        expected = json.loads((ROOT / ".github/inputs.json").read_text())["versions"]["foundry"]
        anvil = run(["anvil", "--version"], env, capture_output=True, text=True).stdout
        if re.search(rf"(?m)^anvil Version: {re.escape(expected)}(?:-v\S+)?(?:\s|$)", anvil) is None:
            raise SystemExit("Anvil does not match the pinned Foundry release")

        pilot = {}
        if args.pilot_output:
            output = args.pilot_output.resolve()
            output.mkdir(parents=True, exist_ok=True, mode=0o700)
            if any(output.iterdir()):
                parser.error("--pilot-output must be empty")
            pilot = {"P05_PILOT_OUTPUT": str(output), "P05_PILOT_PUBLIC_HOLD_SECONDS": str(args.public_hold_seconds)}
            if args.bulk:
                pilot["P05_PILOT_BULK"] = "1"
            if args.independent:
                pilot["P05_PILOT_INDEPENDENT"] = "1"
            if args.independent_same_sku:
                pilot["P05_PILOT_SAME_SKU"] = "1"
            if args.human_units:
                pilot["P05_PILOT_HUMAN_UNITS"] = "1"
            version_commands = {"go": ["go", "version"], "node": ["node", "--version"],
                                "postgres": [pg("postgres"), "--version"], "anvil": ["anvil", "--version"]}
            versions = {name: run(command, env, capture_output=True, text=True).stdout.splitlines()[0]
                        for name, command in version_commands.items()}
            with (output / "environment.json").open("x") as report:
                json.dump({"mode": "local synthetic pilot", "toolVersions": versions,
                           "requestedPublicHoldSeconds": args.public_hold_seconds, "bulkEnabled": args.bulk,
                           "independentCartMode": ("same-sku-two-options" if args.independent_same_sku
                                                   else "two-skus" if args.independent else None)}, report, indent=2)
                report.write("\n")

        data = scratch / "data"
        socket = scratch / "socket"
        socket.mkdir(mode=0o700)
        run([pg("initdb"), "-D", str(data), "-A", "trust", "--no-locale", "--encoding=UTF8", "-U", USER], env)
        start_postgres(pg("pg_ctl"), data, scratch / "postgres.log",
                       f"-c listen_addresses='' -c unix_socket_directories={socket} -c unix_socket_permissions=0700",
                       env, cwd=ROOT)
        local = {**env, "PGHOST": str(socket), "PGUSER": USER, "PGPORT": "5432", "PGSSLMODE": "disable",
                 "PATH": (str(args.postgres_bin.resolve()) + os.pathsep + env["PATH"]) if args.postgres_bin else env["PATH"]}
        run_upgrade(scratch, local)
        composition = run_command(["python3", "products/p10-platform/database/test_composition.py",
            "--socket", str(socket), "--port", "5432", "--user", USER], cwd=ROOT, env=local,
            stdout=subprocess.PIPE, text=True)
        print(composition.stdout, end="", flush=True)
        sql_required = 1 + verify_composition(composition)  # K11 is checked by run_upgrade above.
        run(["python3", "products/p10-platform/recovery/test_cutover.py", "--existing-postgres"], local)
        run(["python3", "products/p10-platform/recovery/test_preparation.py", "--existing-postgres"], local)
        run(["python3", "products/p10-platform/release/test_fleet.py", "--existing-postgres"], local)
        run(["python3", "products/p10-platform/release/test_ios_profile_race.py", "--existing-postgres"], local)
        # The rehearsal owns two detached postmasters and handles termination.
        run(["python3", "products/p10-platform/recovery/test_erasure.py", "--existing-postgres"], local,
            termination_grace=120)
        databases = {"commerce": "nu54_commerce_gating", "pickup": "nu54_commerce_pickup_strict",
                     "settlement": "nu54_settlement_gating", "rental": "nu54_commerce_generation_gating",
                     "chainview": "nu54_chainview_gating", "generation": "nu54_generation_gating", "market": "nu54_market_gating", "places": "nu54_p09_commerce_gating",
                     "publication": "nu54_p09_commerce_publication", "private": "nu54_p09_private_strict",
                     "journal": "nu54_p09_erasure_strict"}
        if args.pilot_output is None:
            databases["settlement_cafe"] = "nu54_settlement_k05_c2"
            databases["customer_cafe"] = "nu54_settlement_w10_customer"
        for key, name in databases.items():
            run([pg("createdb"), name], local)
            owners = {"settlement_cafe": ("p05", "p07"), "customer_cafe": ("p05",), "commerce": ("p05",), "pickup": ("p05",), "settlement": ("p05", "p07"), "rental": ("p05",), "chainview": ("p07",), "generation": ("p07",), "market": ("p08",),
                      "places": ("p09",), "publication": ("p09",), "private": ("p09",), "journal": ("p09",)}[key]
            for owner in owners:
                run(["python3", "products/p10-platform/database/migrate.py", owner],
                    {**local, f"{owner.upper()}_MIGRATION_DATABASE_URL": name})
        p05_host_inputs = build_p05_host_inputs(scratch / "p01", env)
        contracts = scratch / "p06-contracts"
        run(["python3", "scripts/contract_forge.py", "p06", "build", "--quiet"],
            {**env, "FOUNDRY_OUT": str(contracts), "FOUNDRY_CACHE_PATH": str(scratch / "p06-forge-cache")})

        def dsn(database):
            return f"host={socket} port=5432 user={USER} dbname={databases[database]} sslmode=disable"

        run([pg("psql"), "-X", "-v", "ON_ERROR_STOP=1", "-d", databases["market"], "-c",
             "CREATE ROLE marketkeeper LOGIN IN ROLE nu54_p08_keeper; CREATE ROLE marketreader LOGIN IN ROLE nu54_p08_inspector"], local)
        total_required = 0
        total_required += verified_test(["bash", f"{P08}/test/anvil.sh", "-json", "-run", "^TestIsolatedMarketExecutionAndKeeperRestart$"], {
            "TestIsolatedMarketExecutionAndKeeperRestart",
        }, {**env, "P08_TEST_DATABASE_URL": dsn("market") + " user=marketkeeper",
            "P08_TEST_INSPECT_DATABASE_URL": dsn("market") + " user=marketreader"})
        # P08's fixture and services have stopped before the merchant integration runs.
        place_fixture = scratch / "places"
        place_fixture.mkdir(mode=0o700)
        with published_fixture(place_fixture, local, dsn("places")) as public_url:
            commerce_required = P05_COMMERCE_REQUIRED
            if args.pilot_output is None:
                commerce_required = commerce_required | W10_ESCROW_PRESENTATION_REQUIRED
            # The protected close child requires both live daemons in the dedicated case below.
            total_required += verified_test(["go", "test", "-json", "-count=1", "-skip",
                "^(TestProtectedCloseActualDaemons|TestInstalledW10SponsoredSettlement)$", f"./{P05}/internal/commerce"],
                commerce_required, {**env, **p05_host_inputs, "P05_COMMERCE_TEST_DATABASE_URL": dsn("commerce"),
                "P05_PICKUP_TEST_DATABASE_URL": dsn("pickup"),
                "P05_TRAVEL_TEST_DATABASE_URL": dsn("commerce"), "P09_TRAVEL_TEST_PUBLIC_URL": public_url})
        opsd = scratch / "rental-opsd"
        run(["go", "build", "-o", str(opsd), f"./{P05}/cmd/opsd"], env)
        total_required += verified_test(["go", "test", "-json", "-count=1", "-run", "^TestOpsdRentalAuthorityReadAudience$", f"./{P05}/cmd/opsd"],
            {"TestOpsdRentalAuthorityReadAudience"}, {**env, "P05_COMMERCE_TEST_DATABASE_URL": dsn("rental"),
            "P05_074_TEST_OPSD": str(opsd)})
        total_required += verified_test(["go", "test", "-json", "-count=1", f"./{P05}/internal/authority"], {
            "TestCommitRequiresCurrentAuthorityAfterDatabaseWait",
            "TestLiveGrantRecheckedAfterItsOwnRowLock",
            "TestLiveGrantRecheckedAfterItsOwnRowLock/current",
            "TestLiveGrantRecheckedAfterItsOwnRowLock/expired",
            "TestLiveGrantRecheckedAfterItsOwnRowLock/revoked",
        }, {**env, "P05_AUTHORITY_TEST_DATABASE_URL": dsn("commerce")})
        total_required += verified_test(["go", "test", "-json", "-count=1", f"./{P09}/internal/places"], {
            "TestPostgresSignedPublication", "TestSignedRegionLookupHasSameMapListPlaceIDs",
        }, {**env, "P09_TEST_READ_URL": dsn("publication") + " user=travelread",
            "P09_TEST_WRITE_URL": dsn("publication") + " user=travelwrite"})
        recordsd = scratch / "recordsd"
        run(["go", "build", "-o", str(recordsd), f"./{P09}/cmd/recordsd"], env)
        total_required += verified_test(["go", "test", "-json", "-count=1", f"./{P09}/internal/records"], {
            "TestPrivateConsentMutualTLS", "TestSelectedErasureAndRestore",
            "TestScopedProcessorRevocation", "TestItinerarySourcesAndCAS",
        }, {**env, "P09_PRIVATE_TEST_DSN": dsn("private"), "P09_PRIVATE_TEST_BIN": str(recordsd),
            "P09_ERASURE_TEST_DSN": dsn("journal")})
        settlement_required = {
            "TestLoopbackSettlementEvidenceAndRefund", "TestLoopbackCustomerOrderReceipt",
            "TestPostgresRefundCapacityFinanceRolesAndDayCutoff",
            "TestPostgresFinanceConsoleScopesAndReplay", "TestQuoteBoundaryOriginalArchive",
            "TestLoopbackFinanceOriginalOrder", "TestPostgresFinancePendingOrderProjection",
            "TestLoopbackFinanceRevertedEvidence", "TestLoopbackMerchantGasSponsorship",
            "TestLoopbackSponsoredBoardPayment",
            "TestLoopbackSettlementEvidenceAndRefund/FinalizedRefundRetry",
            "TestPostgresRelayCustodyAndDeliveryRecovery",
            "TestPostgresOperatorAccessRequiresTwoIdentities", "TestPostgresLegacyCashoutDispatchFence",
            "TestLoopbackPayoutApprovalAndFinalizedRotation", "TestLoopbackManagedRentalFinalWithdrawal",
            "TestLoopbackManagedRentalFinalWithdrawal/LegacyCashOutChangedAmount",
        }
        if args.pilot_output is None:
            settlement_required.update({
                "TestLoopbackSettlementEvidenceAndRefund/SubscriptionCycleLateFinality",
                "TestLoopbackSettlementEvidenceAndRefund/TravelBookingPaymentsAndPickup",
                "TestLoopbackSettlementEvidenceAndRefund/TravelPickupOriginalPendingToReady",
            })
            settlement_required.update(W11_RECEIPT_REQUIRED | W10_ESCROW_ANCHOR_REQUIRED | {K05_RECONCILIATION_TEST, W10_RENTAL_REVIEW_TEST})
        if env.get("NU54_REPRICED_ORDER_TEST") == "1" or env.get("NU54_REPRICED_NATIVE_LAB"):
            settlement_required.update({
                "TestLoopbackCustomerOrderReceipt/RepricedOriginalOrder",
                "TestLoopbackCustomerOrderReceipt/OriginalOldQuoteExpiresUnpaid",
            })
        # The QR receipt fixture leaves an accepted submission for each original sale.
        # General settlement workers must not pick up these isolated fixture orders.
        if args.pilot_output is None:
            total_required += verified_test(["go", "test", "-json", "-count=1", "-run", f"^{DIRECT_CAFE_TEST}$",
                f"./{P05}/internal/settlement"], DIRECT_CAFE_REQUIRED,
                {**env, "P05_SETTLEMENT_TEST_DATABASE_URL": dsn("customer_cafe")})
        total_required += verify_w10_rental_decision(args.pilot_output, scratch, dsn, env)
        broad_exclusions = (f"^(TestLoopback(InstalledW10Fixture|W10InstalledFinalizer|K05NearbyCafeC2)"
                            f"|{DIRECT_CAFE_TEST}|{W10_RENTAL_DECISION_TEST})$")
        total_required += verified_test(["sh", f"{P05}/test/settlement/anvil.sh", "-json", "-skip", broad_exclusions], settlement_required, {**env, **pilot, "P05_SETTLEMENT_TEST_DATABASE_URL": dsn("settlement"),
            "NU54_REFUND_RETRY_TEST": "1",
            "NU54_TRAVEL_T06_HOST": "1" if args.pilot_output is None else "0",
            "NU54_TRAVEL_NATIVE_T06": "0",
            "NU54_W11_UNPAID_HOST": "1" if args.pilot_output is None else "0",
            "NU54_RECOVERY_REHEARSAL_OUTPUT": env.get("NU54_RECOVERY_REHEARSAL_OUTPUT", str(scratch / "recovery")),
            "NU54_P01_INSTALL_TEST": str(scratch / "p01" / "test_install")})
        total_required += verify_k05_nearby_c2(args.pilot_output, scratch, dsn, env)
        # This fixture deploys its own contracts and changes the token's withdrawal policy.
        withdrawal_group = "TestLoopbackManagedRentalFinalWithdrawal"
        total_required += verified_test(["sh", f"{P05}/test/settlement/anvil.sh", "-json", "-run",
            f"^{withdrawal_group}$/^ProtectedWithdrawalReceipts$"], {withdrawal_group + "/ProtectedWithdrawalReceipts"},
            {**env, "P05_SETTLEMENT_TEST_DATABASE_URL": dsn("settlement"), "NU54_PROTECTED_WITHDRAWAL_RECEIPTS": "1"})
        v1_generation = {
            "TestSharedRJP1Projection", "TestSharedRFR1Receipt",
            "TestV1GenerationSameLimitsDistinctTypedNonces", "TestGenerationReaderRealV1PostgresMutualTLS",
        }
        v1_filter = "^(" + "|".join(sorted(v1_generation)) + ")$"
        total_required += verified_test(["sh", f"{P07}/test/chainview-anvil.sh", "-json", "-skip", v1_filter], {
            "TestAnvilActualReceiptsAndDivergentFork", "TestPostgresAtomicReplayAndFinality",
            "TestAnvilActualReceiptsAndDivergentFork/evidence",
            "TestAnvilActualReceiptsAndDivergentFork/explorer-operations",
            "TestAnvilActualReceiptsAndDivergentFork/explorer-boundary",
            "TestAnvilActualReceiptsAndDivergentFork/read-revision",
        }, {**env, "P07_CHAINVIEW_TEST_URL": dsn("chainview")})
        daemon_test = "TestGenerationReaderRealV1PostgresMutualTLS/DedicatedDaemon"
        total_required += verified_test(["python3", "-B", f"{P07}/test/generation-v1.py", "--existing-postgres", "--combined-p05", "--json"],
            v1_generation | {daemon_test, daemon_test + "/ActualP05AuthorityAndWriter"},
            {**env, "P07_GENERATION_TEST_DATABASE_URL": dsn("generation"), "P05_O09_COMBINED_DATABASE_URL": dsn("rental"),
            "P07_GENERATION_TEST_ARTIFACTS": str(contracts)})
        # Each scenario provisions its own cluster-wide roles. The rollover cases start at migration 077.
        pg_directory = Path(shutil.which("pg_ctl", path=local["PATH"])).resolve().parent
        compiler = current_compiler(env.get("FOUNDRY_SOLC"), offline=True)
        reconciler = {"P05_O09_WITHDRAWAL_RECONCILIATION": "1"}
        v1_origin = {"P05_O09_RENTER_V1_ORIGIN": "1", "P05_O09_RENTER_ASSESSMENT": "mixed"}
        for options, scenario, settings in (
            (("--protected-close-p05",), "ActualProtectedCloseWithLostResponse", {}),
            (("--supervised-close-p05",), "SupervisedOperatorHTTP", {}),
            (("--protected-withdrawal-p05",), "ProtectedWithdrawalHTTP/ReconcileWithoutOperatorRequest", reconciler),
            (("--zero-withdrawal-p05",), "ProtectedZeroWithdrawalHTTP/QualifiedStationIssuesOriginal", {}),
            (("--recover-withdrawal-p05",), "ProtectedWithdrawalRestartHTTP", {}),
            (("--rollover-close-p05", "unchanged"), "ProtectedCloseLegacyRollover", {}),
            (("--rollover-close-p05", "renewed"), "ProtectedCloseLegacyReauthorization", {}),
            (("--recover-withdrawal-p05",), "ProtectedWithdrawalTerminalRetryHTTP",
             {**reconciler, "P05_O09_RETRY_WITHDRAWAL": "1"}),
            (("--recover-withdrawal-p05",), "ProtectedWithdrawalRevertedAlreadyPaidHTTP/Upgrade081To082",
             {**reconciler, "P05_O09_RETRY_WITHDRAWAL": "third-party", "P05_O09_RECEIPT_ATTRIBUTION_UPGRADE": "1"}),
            (("--combined-p05",), "RenterReturnHandoverHTTP/Upgrade083To084",
             {"P05_O09_RENTER_RETURN": "1", "P05_O09_RENTER_ATTESTATION_UPGRADE": "1"}),
            (("--combined-p05",), "RenterReturnPendingAssessmentHTTP", {"P05_O09_RENTER_ASSESSMENT": "positive"}),
            (("--combined-p05",), "RenterReturnVerifiedZeroHTTP", {"P05_O09_RENTER_ASSESSMENT": "zero"}),
            (("--combined-p05",), "RenterReturnLimitOnlyHTTP", {"P05_O09_RENTER_ASSESSMENT": "limit"}),
            (("--combined-p05",), "RenterReturnMixedPendingHTTP/Upgrade085To086",
             {"P05_O09_RENTER_ASSESSMENT": "mixed", "P05_O09_RENTER_TYPED_UPGRADE": "1"}),
            (("--combined-p05",), "RenterV1OrderOriginHTTP", v1_origin),
            (("--combined-p05",), "Upgrade087To088RenterV1Origin/RejectHistoricalFirstReservation",
             {**v1_origin, "P05_O09_RENTER_V1_UPGRADE": "1", "P05_O09_RENTER_V1_FREEZE_FENCE": "1"}),
        ):
            # Require both the top-level scenario and any migration/worker subtest it selects.
            required = {daemon_test + "/" + name for name in (scenario.partition("/")[0], scenario)}
            if options == ("--protected-withdrawal-p05",):
                required.add(daemon_test + "/ProtectedWithdrawalHTTP/QualifiedStationIssuesPaidOriginal")
            if args.pilot_output is None and options[0] in RCR1_REQUIRED:
                required.add(daemon_test + "/" + RCR1_REQUIRED[options[0]])
                settings = {**settings, "P05_O09_RCR1_TEST": "1"}
            total_required += verified_test(["python3", "-B", f"{P07}/test/generation-v1.py",
                "--pg-bin", str(pg_directory), "--solc", str(compiler), *options, "--json"],
                required, {**env, **settings})
        total_required += verified_test(["go", "test", "-json", "-count=1", f"./{P07}/internal/evidencev2", f"./{P07}/internal/ingest"], {
            "TestPostgresActiveEvidence", "TestPostgresIdempotentRestartAndDuplicate", "TestC7MutualTLS",
            "TestRechecksAuthorityBeforeReleasingObservation",
        }, {**env, "P07_EVIDENCE_TEST_DATABASE_URL": dsn("chainview"), "P07_TEST_DATABASE_URL": dsn("chainview")})
        print(f"PASS commercial integration gate, {total_required} required tests, skips=0", flush=True)
        print(f"PASS commercial SQL upgrade gate, {sql_required} required fixtures "
              "(K11 075→076, W10 089→090, W12/W11 092→094, O09/W11 094→096, O09 096→097, T06 097→098, K05 098→099, W10 099→100, W10 100→101, W10 101→102, W10 102→103), skips=0", flush=True)


if __name__ == "__main__":
    main()
