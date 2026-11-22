"""The restart gate requires an executed, passing test."""

import json
import unittest

import regression


class RestartGateTests(unittest.TestCase):
    @staticmethod
    def output(*events):
        return "\n".join(json.dumps({"Action": action, **({"Test": name} if name else {})}) for action, name in events)

    def test_selected_test_ran_and_passed(self):
        name = regression.RESTART_TEST
        self.assertTrue(regression.restart_passed(self.output(("run", name), ("pass", name), ("pass", None))))

    def test_skip_and_empty_selector_are_failures(self):
        name = regression.RESTART_TEST
        self.assertFalse(regression.restart_passed(self.output(("run", name), ("skip", name), ("pass", None))))
        self.assertFalse(regression.restart_passed(self.output(("pass", None))))
        self.assertFalse(regression.restart_passed(self.output(("pass", name))))


if __name__ == "__main__":
    unittest.main()
