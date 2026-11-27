"""Quote review transition tests for the Android UI driver."""

import contextlib
import io
import unittest
import xml.etree.ElementTree as ET

from orders import MarketDevice


AMOUNT = "2.34602 LAQ"
PREVIOUS = f"직전 예상 {AMOUNT} · 새 견적 승인 필요"
APPROVAL = "견적 확인 및 학습 지갑에서 승인"


def frame(deadline, *, previous=False, enabled=True):
    labels = [f"예상 {AMOUNT}", f"가격 시각 10:30:00 · 승인 기한 {deadline}"]
    if previous:
        labels.append(PREVIOUS)
    return [
        *(ET.Element("node", {"text": label}) for label in labels),
        ET.Element("node", {"content-desc": APPROVAL, "enabled": str(enabled).lower(), "clickable": "true"}),
    ]


class RenewedQuoteTest(unittest.TestCase):
    def device(self, frames):
        device = MarketDevice.__new__(MarketDevice)
        updates = iter(frames)
        device.hierarchy = lambda: next(updates)
        device.scroll = lambda *, up, short=False: None
        return device

    def test_same_amount_alone_cannot_be_mistaken_for_fresh_quote(self):
        device = self.device([
            frame("10:31:00"), frame("10:32:00", previous=True, enabled=False),
            frame("10:32:00", previous=True),
        ])
        self.assertEqual(device.review_renewal(AMOUNT, "승인 기한 10:31:00", APPROVAL, attempts=3),
                         "승인 기한 10:32:00")

    def test_available_approval_cannot_replace_the_previous_amount(self):
        device = self.device([frame("10:32:00")])
        output = io.StringIO()
        with contextlib.redirect_stderr(output), self.assertRaisesRegex(RuntimeError, "renewal"):
            device.review_renewal(AMOUNT, "승인 기한 10:31:00", APPROVAL, attempts=1)
        self.assertIn("10:32:00", output.getvalue())
        self.assertIn('"previous": false', output.getvalue())


if __name__ == "__main__":
    unittest.main()
