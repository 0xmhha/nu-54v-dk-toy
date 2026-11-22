import unittest

from fleet_portal import REGISTRY, projection, read, render


class PublicServiceContractTest(unittest.TestCase):
    def test_scoped_operations_and_recovery_share_the_registry(self):
        registry = read(REGISTRY)
        result = projection(None, "D01", "", {"family": "C3/", "role": "terminal"})
        self.assertEqual(result["operations"], {
            name: contract for name, contract in registry["operations"].items()
            if name.startswith("C3/") and "terminal" in contract["roles"]
        })
        self.assertEqual(result["errorCodes"], registry["errorCodes"])
        self.assertEqual(result["recovery"], registry["recovery"])
        self.assertEqual(result["commandReplay"], registry["commandReplay"])
        html = render("D01", "", result).decode()
        for item in ("C3/order.create", "REPLAY_CONFLICT", "UNKNOWN_RESULT",
                     "reconcile-prior-receipt", "reconcile-before-new-authority", "intentDigest"):
            self.assertIn(item, html)
        self.assertNotIn("C3/subscription.offers", html)
