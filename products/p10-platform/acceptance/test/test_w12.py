"""Tests for the week-12 evidence tool: the log keeps only redacted values, evidence hashes are
checked, the PaymentSettled decode matches a real testnet receipt, and W12-04 timing judgement."""

import json
import re
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
import w12  # noqa: E402

# The register validator's redaction patterns (validate_design_freeze_02.py).
HEX_LEAK = re.compile(r"(?<![0-9a-fA-F])0x(?:[0-9a-fA-F]{64}|[0-9a-fA-F]{40})(?![0-9a-fA-F])")
BARE_SHA256 = re.compile(r"(?<![0-9a-fA-Fx:])[0-9a-f]{64}(?![0-9a-fA-F])")
SETTLEMENT = "0xDe7596556D35Fa62F238F074A0f0E59cF730caA4"


def redaction_problems(text: str) -> list[str]:
    out = [m.group(0) for m in HEX_LEAK.finditer(text)]
    out += [m.group(0) for m in BARE_SHA256.finditer(text) if text[max(0, m.start() - 7):m.start()] != "sha256:"]
    return out


class Sandbox(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.log = self.tmp / "week12-log.md"
        shutil.copy(w12.LOG, self.log)
        self.saved = (w12.LOG, w12.EVIDENCE)
        w12.LOG, w12.EVIDENCE = self.log, self.tmp / "evidence"
        self.manifest = lambda: w12.Manifest(w12.EVIDENCE)

    def tearDown(self):
        w12.LOG, w12.EVIDENCE = self.saved
        shutil.rmtree(self.tmp)

    def evidence_file(self, name="log.txt", body=b"approved\n"):
        p = self.tmp / name
        p.write_bytes(body)
        return p


class LogTests(Sandbox):
    def test_every_item_and_refusal_row_is_found(self):
        rows = dict(w12.item_rows(self.log.read_text()))
        self.assertEqual(sorted(k for k in rows if k.startswith("W12-")), [f"W12-{i:02}" for i in range(1, 13)])
        self.assertEqual(len([k for k in rows if not k.startswith("W12-")]), 8)  # 5 agreed + 3 device codes
        self.assertTrue(all(r == "미실행" for r in rows.values()))

    def test_record_writes_result_and_redacted_evidence(self):
        w12.main(["add", "W12-03", str(self.evidence_file())])
        m = self.manifest()
        m.note("W12-03", "tx `" + w12.short("0x" + "ab" * 32) + "` finalized")
        w12.main(["record", "W12-03", "통과"])
        w12.main(["add", "ATTESTATION_EXPIRED", str(self.evidence_file("refusal.json", b"{}"))])
        w12.main(["record", "ATTESTATION_EXPIRED", "통과"])
        text = self.log.read_text()
        row = [l for l in text.split("\n") if l.startswith("| W12-03 |")][0]
        self.assertIn("| 통과 |", row)
        self.assertIn("sha256:", row)
        self.assertIn("0xababab…abab", row)
        self.assertEqual(redaction_problems(text), [])
        # The table keeps its shape: same number of cells as the header.
        header = [l for l in text.split("\n") if l.startswith("| ID | 시험 |")][0]
        self.assertEqual(row.count("|"), header.count("|"))

    def test_record_needs_evidence(self):
        with self.assertRaises(SystemExit):
            w12.main(["record", "W12-05", "통과"])

    def test_record_refuses_unknown_results_and_rows(self):
        w12.main(["add", "W12-07", str(self.evidence_file())])
        with self.assertRaises(SystemExit):
            w12.main(["record", "W12-07", "pass"])
        with self.assertRaises(SystemExit):
            w12.main(["record", "W12-99", "통과"])


class EvidenceTests(Sandbox):
    def test_check_notices_a_changed_file_and_pending_rows(self):
        f = self.evidence_file()
        w12.main(["add", "W12-01", str(f)])
        self.assertEqual(w12.main(["check"]), 1)  # rows still pending
        stored = w12.EVIDENCE / "W12-01" / "log.txt"
        stored.write_text("edited after the fact\n")
        m = self.manifest()
        entry = m.data["items"]["W12-01"]["files"][0]
        self.assertNotEqual(w12.sha256_file(stored), entry["sha256"])

    def test_timings(self):
        ok, text = w12.judge_timings([{"ms": str(4000 + i * 100)} for i in range(20)])
        self.assertTrue(ok, text)
        self.assertFalse(w12.judge_timings([{"ms": "4000"}] * 19)[0], "fewer than 20 runs")
        self.assertFalse(w12.judge_timings([{"ms": "4000"}] * 19 + [{"ms": "10001"}])[0], "one run over 10 s")
        self.assertFalse(w12.judge_timings([{"ms": "4000", "outcome": "approved"}] * 19 + [{"ms": "4000", "outcome": "Checking"}])[0])


class ReceiptTests(unittest.TestCase):
    def test_board_payment_receipt(self):
        receipt = json.loads((HERE / "fixtures/receipt-board-payment.json").read_text())
        e = w12.decode_settled(receipt, SETTLEMENT)
        self.assertEqual(e["device"], "0xbc3152c1fd512552b3962e86c7b7c248ea17f4d8")  # the board key
        self.assertEqual(e["merchant"], "0xf92a32ceb9d940057be7b76bf4e2eb76409f61a4")
        self.assertEqual(e["amount"], 1_000_000)
        self.assertEqual(e["orderId"], "0xea4ff5e9d9680b14c5a0209810062e6612c0814522ab2e8d84caa97e70e7eaa0")
        self.assertIsNone(w12.decode_settled(receipt, "0x" + "00" * 20), "another contract's log")

    def test_short(self):
        self.assertEqual(w12.short("0xaf8adb9dcbce62b9a830e559c36a465f2f84ad2cefab54757a0455b89ee20b00"), "0xaf8adb…0b00")
        self.assertEqual(w12.short("1234"), "1234")


if __name__ == "__main__":
    unittest.main()
