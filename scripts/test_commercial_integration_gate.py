import json
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import test_commercial_integrations as gate


class CommercialGateTest(unittest.TestCase):
    def test_composition_requires_each_upgrade_once(self):
        expected = (gate.W10_GAS_UPGRADE_RESULT, gate.RETURN_PICKUP_UPGRADE_RESULT,
                    gate.RETURN_ISSUANCE_ARCHIVE_UPGRADE_RESULT, gate.RETURN_RECEIPT_UPGRADE_RESULT,
                    gate.TRAVEL_PICKUP_UPGRADE_RESULT, gate.CAFE_PICKUP_UPGRADE_RESULT,
                    gate.CUSTOMER_CAFE_FORWARD_UPGRADE_RESULT, gate.CUSTOMER_CAFE_REVIEW_UPGRADE_RESULT,
                    gate.RENTER_PAYMENT_ESCROW_UPGRADE_RESULT, gate.RENTER_ESCROW_DEADLINE_UPGRADE_RESULT)
        for marker in expected:
            without = tuple(line for line in expected if line != marker)
            skipped = (*without, marker.replace("PASS ", "SKIP ", 1))
            for lines in (without, (*expected, marker), skipped):
                with self.subTest(lines=lines), self.assertRaisesRegex(SystemExit, "missing or repeated"):
                    gate.verify_composition(SimpleNamespace(stdout="\n".join(lines), returncode=0))
        combined = "\n".join(expected)
        self.assertEqual(gate.verify_composition(SimpleNamespace(stdout=combined, returncode=0)), 10)
        with self.assertRaisesRegex(SystemExit, "exit=1"):
            gate.verify_composition(SimpleNamespace(stdout=combined, returncode=1))

    def test_rental_escrow_requires_anchor_and_signed_deadline_cases(self):
        anchors = {"TestRentalEscrowUsesReviewedFinalizedGeneration/" + child for child in (
            "before_reviewed_origin", "at_reviewed_origin", "different_hash_at_reviewed_height",
            "later_finalized", "one_unit_short")}
        deadlines = {"TestRentalEscrowPresentationSignsEarliestAuthorityDeadline/" + child for child in (
            "payer_enrollment_first", "chain_evidence_first", "original_quote_first",
            "rounded_deadline_already_reached")}
        self.assertEqual(gate.W10_ESCROW_ANCHOR_REQUIRED, anchors)
        self.assertEqual(gate.W10_ESCROW_PRESENTATION_REQUIRED, deadlines)
        for group in (anchors, deadlines):
            for omitted in group:
                stdout = "\n".join(json.dumps({"Action": action, "Test": test})
                                   for test in group - {omitted} for action in ("run", "pass"))
                with self.subTest(omitted=omitted), patch.object(gate, "run_command",
                        return_value=SimpleNamespace(stdout=stdout, returncode=0)):
                    with self.assertRaisesRegex(SystemExit, omitted):
                        gate.verified_test([], group, {})

    def test_w10_requires_actual_customer_rental_review(self):
        marker = "TestPostgresCustomerRentalReviewProof"
        self.assertEqual(gate.W10_RENTAL_REVIEW_TEST, marker)
        for actions in (["run", "skip"], ["run", "pass", "pass"], ["run", "pass"]):
            stdout = "\n".join(json.dumps({"Action": action, "Test": marker}) for action in actions)
            with self.subTest(actions=actions), patch.object(gate, "run_command",
                    return_value=SimpleNamespace(stdout=stdout, returncode=0)):
                if actions == ["run", "pass"]:
                    self.assertEqual(gate.verified_test([], {marker}, {}), 1)
                else:
                    with self.assertRaisesRegex(SystemExit, marker):
                        gate.verified_test([], {marker}, {})

    def test_w10_decision_requires_isolated_real_board_and_both_customer_outcomes(self):
        marker = "TestPostgresCustomerRentalDecisionForward"
        required = {f"{marker}/refusal_has_no_payer_approval",
                    f"{marker}/signed_approval_admits_once_and_reopens_original"}
        self.assertEqual(gate.W10_RENTAL_DECISION_REQUIRED, required)
        with patch.object(gate, "run") as run, patch.object(gate, "verified_test", return_value=2) as verify:
            gate.verify_w10_rental_decision(None, Path("/private/suite"), lambda name: name, {})
            self.assertEqual(run.call_args.kwargs["env"]["NU54_MOBILE_TEST_PEER_OUTPUT"], "/private/suite/w10-c2-board")
            self.assertEqual(verify.call_args.args[1], required)
            self.assertEqual(verify.call_args.args[2]["NU54_W10_RENTER_BOARD_BIN"], "/private/suite/w10-c2-board")
            self.assertEqual(verify.call_args.args[2]["P05_SETTLEMENT_TEST_DATABASE_URL"], "customer_cafe")
        with patch.object(gate, "run") as run, patch.object(gate, "verified_test") as verify:
            self.assertEqual(gate.verify_w10_rental_decision(Path("/pilot"), Path("/private/suite"), None, {}), 0)
            run.assert_not_called()
            verify.assert_not_called()

    def test_direct_customer_cafe_requires_all_three_receipt_cases(self):
        name = "TestPostgresCustomerCafeForwardReceipt"
        cases = {name + "/" + child for child in (
            "initial_sponsored_submission", "same_key_receipt", "legacy_unsponsored_customer")}
        self.assertEqual(gate.DIRECT_CAFE_REQUIRED, cases)
        for omitted in cases:
            stdout = "\n".join(json.dumps({"Action": action, "Test": test})
                               for test in {name} | (cases - {omitted}) for action in ("run", "pass"))
            with self.subTest(omitted=omitted), patch.object(gate, "run_command",
                    return_value=SimpleNamespace(stdout=stdout, returncode=0)):
                with self.assertRaisesRegex(SystemExit, omitted):
                    gate.verified_test([], gate.DIRECT_CAFE_REQUIRED, {})

    def test_k05_and_w11_require_each_original_receipt_case(self):
        self.assertEqual(gate.K05_RECONCILIATION_TEST, "TestLoopbackCustomerOrderReceipt/KioskCafeBeforeFinality")
        required = gate.W11_RECEIPT_REQUIRED | {gate.K05_RECONCILIATION_TEST}
        self.assertEqual(len(required), 5)
        for omitted in required:
            stdout = "\n".join(json.dumps({"Action": action, "Test": name})
                               for name in required - {omitted} for action in ("run", "pass"))
            with self.subTest(omitted=omitted):
                with patch.object(gate, "run_command", return_value=SimpleNamespace(returncode=0, stdout=stdout)):
                    with self.assertRaisesRegex(SystemExit, omitted):
                        gate.verified_test([], required, {})

    def test_k05_requires_the_separate_c2_original_only_in_normal_gate(self):
        name = "TestLoopbackK05NearbyCafeC2"
        scratch = Path("/private/suite")
        dsn = lambda database: f"dbname={database}"
        for actions in ([], ["run", "skip"], ["run", "pass", "pass"], ["run", "pass"]):
            output = "\n".join(json.dumps({"Action": action, "Test": name}) for action in actions)
            with self.subTest(actions=actions), patch.object(gate, "run_command",
                    return_value=SimpleNamespace(returncode=0, stdout=output)) as run:
                if actions == ["run", "pass"]:
                    self.assertEqual(gate.verify_k05_nearby_c2(None, scratch, dsn, {}), 1)
                else:
                    with self.assertRaises(SystemExit):
                        gate.verify_k05_nearby_c2(None, scratch, dsn, {})
                self.assertEqual(run.call_args.args[0][-2:], ["-run", f"^{name}$"])
                self.assertEqual(run.call_args.kwargs["env"]["P05_SETTLEMENT_TEST_DATABASE_URL"],
                                 "dbname=settlement_cafe")
                self.assertNotIn("P05_PILOT_HUMAN_UNITS", run.call_args.kwargs["env"])
        with patch.object(gate, "run_command") as run:
            self.assertEqual(gate.verify_k05_nearby_c2(Path("/pilot"), scratch, dsn, {}), 0)
            run.assert_not_called()

    def test_builds_and_wires_required_p01_producers(self):
        build = Path("/private/suite/p01")
        with patch.object(gate, "run") as run:
            inputs = gate.build_p05_host_inputs(build, {})
        self.assertEqual(inputs["P05_W12_SLOT_OPEN_HOST"], str(build / "test_session"))
        self.assertEqual(inputs["NU54_P01_INSTALL_TEST"], str(build / "test_install"))
        self.assertEqual(run.call_args.args[0][-2:], ["test_install", "test_session"])

    def test_required_slot_open_must_actually_run_and_pass(self):
        name = gate.P05_SLOT_OPEN_TEST
        self.assertIn(name, gate.P05_COMMERCE_REQUIRED)
        for actions, expected in [
            ([], "required not passed"),
            (["run", "skip"], "skipped="),
            (["run", "pass", "pass"], "required not passed"),
        ]:
            with self.subTest(actions=actions):
                stdout = "\n".join(json.dumps({"Action": action, "Test": name}) for action in actions)
                with patch.object(gate, "run_command", return_value=SimpleNamespace(returncode=0, stdout=stdout)):
                    with self.assertRaisesRegex(SystemExit, expected):
                        gate.verified_test([], {name}, {})
        stdout = "\n".join(json.dumps({"Action": action, "Test": name}) for action in ("run", "pass"))
        with patch.object(gate, "run_command", return_value=SimpleNamespace(returncode=0, stdout=stdout)):
            self.assertEqual(gate.verified_test([], {name}, {}), 1)


if __name__ == "__main__":
    unittest.main()
