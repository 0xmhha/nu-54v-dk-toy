#!/usr/bin/env python3
"""The user app simulation's route without a store registration: what a run binds, for whom, and what it walks.

No emulator, bundler, server or database is started: the world is stood in for.
Run it with `python3 -B scripts/test_simulate_user_app_entrance.py`.
"""

from contextlib import contextmanager, nullcontext, redirect_stderr, redirect_stdout
import io
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import simulate_user_app as simulation
import simulate_user_app_entrance_walk as entrance
import simulate_user_app_walk as registration
import simulate_user_app_won_walk as won_wallet

ROOT = Path(__file__).resolve().parents[1]
FLOW = ROOT / "products/p05-operations-backoffice/test/order-flow"


def adb(*args, timeout=90):
    return subprocess.CompletedProcess(args, 0, b"Physical size: 1080x2400\n" if args[1:] == ("wm", "size") else b"", b"")


class Flows(unittest.TestCase):
    def test_each_part_names_flows_the_walk_has_and_starts_a_phone_of_its_own(self):
        self.assertEqual(list(entrance.PARTS), ["stores", "orders"])
        for part, names in entrance.PARTS.items():
            walk = entrance.EntranceWalk(adb, None, Path("unused"), "won", "", part)
            self.assertEqual([flow.__name__ for flow in walk.flows()], list(names))
            self.assertEqual(names[0], "agree")
            self.assertEqual(len(set(names)), len(names))
        self.assertIn("set_up", entrance.PARTS["orders"][:2])
        self.assertNotIn("set_up", entrance.PARTS["stores"])

    def test_the_walk_with_a_registration_keeps_its_flows(self):
        walk = registration.Walk(adb, None, Path("unused"))
        self.assertEqual(len(walk.flows()), 16)
        self.assertEqual(walk.NEEDED, ("agree", "set_up", "deposit"))
        self.assertEqual([flow.__name__ for flow in walk.flows()][:3], ["agree", "set_up", "deposit"])

    def test_the_won_wallet_has_its_own_flows_after_the_setup(self):
        walk = won_wallet.WonWalletWalk(adb, None, Path("unused"), "won")
        self.assertEqual([flow.__name__ for flow in walk.flows()], ["agree", "set_up", "part_of_a_won", "decimal_refused", "whole_won_sent", "below_one_won"])
        self.assertEqual(walk.NEEDED, ("agree", "set_up", "part_of_a_won"))

    def test_won_are_whole_and_dollars_have_cents(self):
        won, usd = (entrance.EntranceWalk(adb, None, Path("unused"), token, "", "orders") for token in ("won", "usd"))
        self.assertEqual([won.price(2_999, 299), won.price(0, 0), won.price(902_999, 90_299)], ["2,999원", "0원", "902,999원"])
        self.assertEqual([usd.price(2_999, 299), usd.price(0, 0), usd.price(902_999, 90_299)], ["2.99 TestUSDC", "0.00 TestUSDC", "902.99 TestUSDC"])
        self.assertEqual([won.of("아메리카노", "치즈케이크"), usd.of("아메리카노", "치즈케이크", "치즈케이크")], ["11,000원", "17.50 TestUSDC"])

    def test_a_screenshot_has_the_words_of_its_screen_beside_it(self):
        screen = ('<hierarchy><node text="Order flow cafe" content-desc="" /><node text="" content-desc="지금 영업 중" /><node text="메뉴 보고 주문" content-desc="메뉴 보고 주문" />'
                  '<node text="보드 결제 · tKRW &amp; 원" content-desc="" /><node text="홈" content-desc="홈" /></hierarchy>')

        def device(*args, timeout=90):
            out = b"PNG" if args[:2] == ("exec-out", "screencap") else screen.encode() if args[:2] == ("exec-out", "cat") else adb(*args).stdout
            return subprocess.CompletedProcess(args, 0, out, b"")

        with tempfile.TemporaryDirectory() as storage:
            walk = entrance.EntranceWalk(device, None, Path(storage), "won", "", "stores")
            walk.shot("009-store-open")
            walk.shot("136-payment-approve", words=False)
            self.assertEqual(sorted(path.name for path in Path(storage).iterdir()), ["009-store-open.png", "009-store-open.txt", "136-payment-approve.png"])
            # As the phone reads them out, once each, without the tabs of the app.
            self.assertEqual((Path(storage) / "009-store-open.txt").read_text(), "Order flow cafe\n지금 영업 중\n메뉴 보고 주문\n보드 결제 · tKRW & 원\n")

    def test_a_failed_flow_is_a_line_and_the_walk_goes_on_unless_the_rest_needs_it(self):
        class Walk(entrance.EntranceWalk):
            def wait_for_app(self, words=()):
                pass

            def shot(self, name):
                pass

            def flows(self):
                def agree():
                    pass

                def shortfall():
                    raise AssertionError("the basket was emptied")

                def pay():
                    pass

                def set_up():
                    raise AssertionError("no board")

                def pickup():
                    pass
                return (agree, shortfall, pay, set_up, pickup)

        printed = io.StringIO()
        with redirect_stdout(printed):
            failed = Walk(adb, lambda name, **values: None, Path("unused"), "won", "", "orders").run()
        self.assertEqual(failed, 2)
        self.assertEqual(printed.getvalue().splitlines(), ["PASS agree (0s)", "FAIL shortfall: the basket was emptied", "PASS pay (0s)", "FAIL set up: no board"])


class Fixture(unittest.TestCase):
    """The walk, the host and the server's fixture name the same stores and prices."""

    def test_the_stores_and_products_of_the_walk_are_those_of_the_fixture(self):
        fixture = (FLOW / "testdata/order_flow_anvil_test.go").read_text()
        host = (FLOW / "simulator/host.test.ts").read_text()
        named = dict(re.findall(r'\{"(\w+)", "([^"]+)", (?:true|false)\}', fixture))
        self.assertEqual(set(named.values()) | {"Order flow cafe"}, set(entrance.STORES))
        pages = re.search(r"const pages: Record<string, object> = \{(.*?)\n\};", host, re.DOTALL)[1]
        self.assertEqual(set(re.findall(r"^  (\w+): \{", pages, re.MULTILINE)), set(named) | {"cafe"})
        products = {korean: (int(won.replace("_", "")), int(cents.replace("_", ""))) for korean, won, cents in
                    re.findall(r'\{"\w+", "\w+", "([^"]+)", ([\d_]+), ([\d_]+), \d+\}', fixture)}
        self.assertEqual(products, entrance.PRICES)


class Run(unittest.TestCase):
    """simulate_user_app.main() with the emulator, the world and the walk stood in for."""

    def run_simulation(self, *arguments, failed=()):
        worlds, walks, printed = [], [], io.StringIO()

        class Walk:
            def __init__(self, *started):
                walks.append(started[3:])

            def run(self):
                return dict(failed).get(walks[-1][-1], 0) if walks[-1] and walks[-1][-1] in entrance.PARTS else 0

        @contextmanager
        def world(args, _adb):
            worlds.append(args.route)
            yield mock.Mock()

        with mock.patch.object(simulation, "emulator", lambda *given: (adb, "")), mock.patch.object(simulation, "world", world), \
                mock.patch.object(entrance, "EntranceWalk", Walk), mock.patch.object(registration, "Walk", Walk), \
                mock.patch.object(won_wallet, "WonWalletWalk", Walk), \
                redirect_stdout(printed), redirect_stderr(printed):
            try:
                ended = simulation.main(["emulator-5554", *arguments])
            except SystemExit as stopped:
                ended = stopped.code
        return ended, printed.getvalue(), worlds, walks

    def setUp(self):
        storage = tempfile.TemporaryDirectory()
        self.addCleanup(storage.cleanup)
        self.evidence = Path(storage.name) / "evidence"

    def test_the_route_walks_its_two_parts_each_in_a_world_of_its_own(self):
        ended, printed, worlds, walks = self.run_simulation("--route", "entrance", "--token", "won", "--walk", str(self.evidence))
        self.assertIsNone(ended)
        self.assertEqual(worlds, ["entrance", "entrance"])
        self.assertEqual(walks, [("won", "", "stores"), ("won", "", "orders")])
        self.assertEqual(printed.splitlines()[-1], "PASS user app")

    def test_one_part_alone_may_join_the_directory_of_the_other(self):
        self.assertIsNone(self.run_simulation("--route", "entrance", "--walk", str(self.evidence), "--part", "stores")[0])
        ended, _, worlds, walks = self.run_simulation("--route", "entrance", "--walk", str(self.evidence), "--part", "orders")
        self.assertIsNone(ended)
        self.assertEqual((worlds, walks), (["entrance"], [("usd", "", "orders")]))
        # Without --part a walk still takes a new directory, on either route.
        for route in ("entrance", "registration"):
            ended, _, worlds, _ = self.run_simulation("--route", route, "--walk", str(self.evidence))
            self.assertIn("--walk takes a new directory", ended)
            self.assertEqual(worlds, [])

    def test_failed_flows_of_both_parts_are_counted(self):
        ended, printed, worlds, _ = self.run_simulation("--route", "entrance", "--walk", str(self.evidence), failed={"stores": 1, "orders": 3}.items())
        self.assertEqual(ended, f"FAIL user app: 4 flows failed; see {self.evidence}")
        self.assertEqual(worlds, ["entrance", "entrance"])
        self.assertNotIn("PASS user app", printed)

    def test_a_part_belongs_to_a_walk_of_the_route_without_a_registration(self):
        for arguments in (["--part", "stores"], ["--part", "stores", "--walk", str(self.evidence)], ["--route", "entrance", "--part", "orders"]):
            ended, printed, worlds, _ = self.run_simulation(*arguments)
            self.assertEqual((ended, worlds), (2, []))
            self.assertIn("--part stores and --part orders go with --walk and --route entrance", printed)

    def test_the_won_wallet_is_walked_with_a_registration_and_a_won_token(self):
        ended, printed, worlds, walks = self.run_simulation("--token", "won", "--walk", str(self.evidence), "--part", "won-wallet")
        self.assertIsNone(ended)
        self.assertEqual((worlds, walks), (["registration"], [("won", "")]))
        self.assertEqual(printed.splitlines()[-1], "PASS user app")
        for arguments in (["--walk", str(self.evidence) + "2", "--part", "won-wallet"], ["--token", "won", "--part", "won-wallet"],
                          ["--token", "won", "--route", "entrance", "--walk", str(self.evidence) + "2", "--part", "won-wallet"]):
            ended, printed, worlds, _ = self.run_simulation(*arguments)
            self.assertEqual((ended, worlds), (2, []))
            self.assertIn("--part won-wallet goes with --walk and a won token on the route with a registration", printed)
        # Like the sixteen flows it takes a new directory.
        self.assertIn("--walk takes a new directory", self.run_simulation("--token", "won", "--walk", str(self.evidence), "--part", "won-wallet")[0])

    def test_the_route_with_a_registration_is_walked_as_before(self):
        ended, printed, worlds, walks = self.run_simulation("--walk", str(self.evidence))
        self.assertIsNone(ended)
        self.assertEqual((worlds, walks), (["registration"], [("usd", "")]))
        self.assertEqual(printed.splitlines()[-1], "PASS user app")


class World(unittest.TestCase):
    """What simulate_user_app.world() starts, with its builds, database and processes stood in for."""

    def start(self, route):
        started, waited = [], []
        storage = tempfile.TemporaryDirectory()
        self.addCleanup(storage.cleanup)
        scratch = Path(storage.name).resolve()

        class Process:
            child = mock.Mock()

            def start(self, command, env=None, **rest):
                started.append((Path(command[0]).name, command, env))

            def stop(self):
                pass

        built = {name: scratch / name for name in ("server", "indexer", "contracts", "board", "board_won", "board_krwc")}
        args = mock.Mock(route=route, token="won", serial="emulator-5554", postgres_bin=None)
        with mock.patch.object(simulation.flow, "postgres_tools", lambda directory: scratch), \
                mock.patch.object(simulation, "integration_workspace", lambda *given: nullcontext((scratch, {"PATH": "/bin"}))), \
                mock.patch.object(simulation, "run_command", lambda *given, **rest: mock.Mock(stdout="/go-cache\n")), \
                mock.patch.object(simulation.flow, "database", lambda *given: "dsn"), mock.patch.object(simulation.flow, "build", lambda *given: built), \
                mock.patch.object(simulation, "OwnedProcess", Process), \
                mock.patch.object(simulation, "wait_for", lambda process, log, word, seconds: waited.append((word, len(started)))), redirect_stdout(io.StringIO()):
            with simulation.world(args, adb):
                pass
        return scratch, started, waited

    def test_the_route_without_a_registration_binds_the_entrance_for_the_run_alone(self):
        scratch, started, waited = self.start("entrance")
        (host, _, host_env), (bundler, command, bundler_env) = started
        binding = str(scratch / "order-binding" / "customerEntranceDeployment.js")
        self.assertEqual((host, bundler), ("server", "npx"))
        # The store server opens its customer entrance on a port of its own, and the host writes the binding there.
        self.assertEqual(host_env["P05_CUSTOMER_ADDR"], "127.0.0.1:0")
        self.assertEqual(host_env["NU54_SIMULATOR_ORDER_BINDING"], binding)
        # Only the simulation's bundler configuration is told where, and only after the host wrote it.
        self.assertEqual(bundler_env["NU54_SIMULATOR_ORDER_BINDING"], binding)
        self.assertEqual(command[command.index("--config") + 1], str(simulation.SIMULATOR / "metro.config.js"))
        self.assertNotIn("NU54_CUSTOMER_ORDER_BUILD", bundler_env)
        self.assertEqual(waited, [("SIMULATOR-READY", 1), ("Dev server ready", 2)])
        self.assertTrue((scratch / "order-binding").is_dir())
        self.assertFalse(str(scratch).startswith(str(ROOT)))

    def test_the_route_with_a_registration_binds_nothing(self):
        scratch, started, _ = self.start("registration")
        for _, _, env in started:
            self.assertFalse({"P05_CUSTOMER_ADDR", "NU54_SIMULATOR_ORDER_BINDING", "NU54_CUSTOMER_ORDER_BUILD"} & set(env))
        self.assertFalse((scratch / "order-binding").exists())


if __name__ == "__main__":
    unittest.main()
