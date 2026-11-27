"""Lifecycle checks for the shared K11 SQL fixture."""
from pathlib import Path
import signal
import tempfile
import unittest
from unittest.mock import patch

import pos_upgrade_fixture as fixture


class POSUpgradeLifecycle(unittest.TestCase):
    def setUp(self):
        workspace = tempfile.TemporaryDirectory()
        self.addCleanup(workspace.cleanup)
        self.work = Path(workspace.name)
        self.env = {"PGHOST": str(self.work / "socket"), "PGUSER": "fixture",
                    "PGPORT": "5432", "PGSERVICE": "never", "PGHOSTADDR": "never"}

    def test_success_and_failures_drop_only_the_created_database(self):
        for failure_index in (None, 0, 1, 2, 3, 4, 5, 6):
            with self.subTest(failure_index=failure_index):
                commands = []

                def run(args, env, **kwargs):
                    commands.append(args)
                    self.assertNotIn("PGSERVICE", env)
                    self.assertNotIn("PGHOSTADDR", env)
                    if len(commands) - 1 == failure_index:
                        raise RuntimeError("injected failure")

                with patch.object(fixture, "run", side_effect=run):
                    if failure_index is None:
                        fixture.run_upgrade(self.work, self.env)
                    else:
                        with self.assertRaisesRegex(RuntimeError, "injected failure"):
                            fixture.run_upgrade(self.work, self.env)
                self.assertEqual(commands[0][0], "createdb")
                self.assertEqual(commands[-1][0], "createdb" if failure_index == 0 else "dropdb")

    def test_refuses_external_socket(self):
        with patch.object(fixture, "run") as run:
            with self.assertRaises(ValueError):
                fixture.run_upgrade(self.work, {**self.env, "PGHOST": "/tmp/other"})
            run.assert_not_called()

    def test_term_immediately_after_database_creation_still_drops_it(self):
        commands = []
        previous = signal.signal(signal.SIGTERM, lambda *_: self.interrupt())
        self.addCleanup(signal.signal, signal.SIGTERM, previous)

        def run(args, env, **kwargs):
            commands.append(args[0])
            if args[0] == "createdb":
                signal.raise_signal(signal.SIGTERM)

        with patch.object(fixture, "run", side_effect=run), self.assertRaises(SystemExit) as stopped:
            fixture.run_upgrade(self.work, self.env)
        self.assertEqual(stopped.exception.code, 143)
        self.assertEqual(commands, ["createdb", "dropdb"])

    @staticmethod
    def interrupt():
        raise SystemExit(143)
