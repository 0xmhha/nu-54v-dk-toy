"""Regression cases for the runtime evidence ledger."""
import unittest

from check_journeys import check_evidence_anchors, check_runtime_coverage


PAGES = [
    {"id": "A01", "title": "첫 화면", "states": [{"id": "start"}, {"id": "wait"}]},
    {"id": "B01", "title": "다음 화면", "states": [{"id": "start"}]},
]
HEADER = "| 페이지 | 제품 UI에서 관측 | 증거 연결 대기 | 실행 근거 |\n|---|---|---|---|\n"
FIRST = "| [A01 첫 화면](book.html#A01) | [start](book.html#A01/start) | [wait](book.html#A01/wait) | [시험](runtime-acceptance.md#proof) |\n"
SECOND = "| [B01 다음 화면](book.html#B01) | — | [start](book.html#B01/start) | — |\n"


class RuntimeCoverageTest(unittest.TestCase):
    def test_exact_partition(self):
        self.assertEqual(check_runtime_coverage(PAGES, HEADER + FIRST + SECOND), (2, 3, 1, 2))

    def test_invalid_ledger(self):
        for name, value in [
            ("missing page", HEADER + FIRST),
            ("duplicate page", HEADER + FIRST + SECOND + SECOND),
            ("mislabeled page", HEADER + FIRST.replace("첫 화면", "다른 화면") + SECOND),
            ("wrong page target", HEADER + FIRST.replace("book.html#A01)", "book.html#B01)") + SECOND),
            ("missing state", HEADER + FIRST.replace("[wait](book.html#A01/wait)", "—") + SECOND),
            ("duplicate state", HEADER + FIRST.replace("[wait](book.html#A01/wait)", "[start](book.html#A01/start) · [wait](book.html#A01/wait)") + SECOND),
            ("mislabeled state", HEADER + FIRST.replace("[wait]", "[waiting]") + SECOND),
            ("other page state", HEADER + FIRST.replace("book.html#A01/start", "book.html#B01/start") + SECOND),
            ("no observation evidence", HEADER + FIRST.replace("[시험](runtime-acceptance.md#proof)", "—") + SECOND),
            ("prose without evidence", HEADER + FIRST.replace("[시험](runtime-acceptance.md#proof)", "시험") + SECOND),
            ("design link as evidence", HEADER + FIRST.replace("runtime-acceptance.md#proof", "book.html#A01/start") + SECOND),
        ]:
            with self.subTest(name=name), self.assertRaises(ValueError):
                check_runtime_coverage(PAGES, value)


class EvidenceAnchorTest(unittest.TestCase):
    def test_known_heading(self):
        ledger = FIRST.replace("#proof", "#실제-제목-w10")
        self.assertEqual(check_evidence_anchors(ledger * 2, "### 실제 제목 (W10)\n"), 1)

    def test_existing_file_with_wrong_heading(self):
        with self.assertRaisesRegex(ValueError, "missing.*proof"):
            check_evidence_anchors(FIRST, "### Existing Heading\n")

    def test_heading_in_fenced_example_does_not_count(self):
        with self.assertRaises(ValueError):
            check_evidence_anchors(FIRST, "```markdown\n### Proof\n```\n")

    def test_duplicate_heading_is_ambiguous(self):
        with self.assertRaisesRegex(ValueError, "duplicate.*proof"):
            check_evidence_anchors(FIRST, "### Proof\n### Proof\n")


if __name__ == "__main__":
    unittest.main()
