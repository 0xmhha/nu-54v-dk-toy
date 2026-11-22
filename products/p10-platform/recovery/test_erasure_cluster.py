"""Unit tests for the erasure rehearsal's private PostgreSQL startup."""

from contextlib import redirect_stderr
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import test_erasure
from integration_workspace import WorkspaceCleanupError


SHM_LIMIT = (
    "2026-11-22 10:02:47 KST [25803] FATAL:  could not create shared memory segment: No space left on device\n"
    "2026-11-22 10:02:47 KST [25803] DETAIL:  Failed system call was shmget(key=476165179, size=56, 03600).\n"
)


class ErasureClusterTest(unittest.TestCase):
    def setUp(self):
        folder = self.enterContext(tempfile.TemporaryDirectory())
        self.parent = Path(folder)
        self.launch = self.enterContext(patch.object(test_erasure.Quarantine, "start", autospec=True))
        self.stop = self.enterContext(patch.object(test_erasure, "stop_postgres"))
        self.sleep = self.enterContext(patch.object(test_erasure.time, "sleep"))
        self.stderr = self.enterContext(redirect_stderr(io.StringIO()))
        self.roots = []

    def start(self, cluster):
        for previous in self.roots:
            self.assertFalse(previous.exists())
        self.roots.append(cluster.root)
        cluster.root.mkdir()
        cluster.data.mkdir()
        (cluster.data / "postmaster.pid").touch()

    def test_capacity_failure_stops_partial_server_then_retries(self):
        def attempt(cluster):
            self.start(cluster)
            if len(self.roots) == 1:
                raise test_erasure.ClusterStartupError("pg_ctl", SHM_LIMIT)

        self.launch.side_effect = attempt
        with test_erasure.isolated(self.parent / "db") as cluster:
            self.assertEqual(cluster.root, self.roots[1])
            self.assertTrue(cluster.data.exists())
            self.stop.assert_called_once()
            self.assertEqual(self.stop.call_args.args[1], self.roots[0] / "data")
        self.assertEqual(len(set(self.roots)), 2)
        self.assertEqual(self.stop.call_count, 2)
        self.sleep.assert_called_once()
        self.assertIn("shmget", self.stderr.getvalue())
        self.assertEqual(list(self.parent.iterdir()), [])

    def test_repeated_capacity_failure_is_bounded(self):
        def attempt(cluster):
            self.start(cluster)
            raise test_erasure.ClusterStartupError("initdb", SHM_LIMIT)

        self.launch.side_effect = attempt
        with self.assertRaisesRegex(RuntimeError, "initdb"):
            with test_erasure.isolated(self.parent / "db"):
                self.fail("persistent startup failure passed")
        self.assertGreater(len(self.roots), 1)
        self.assertEqual(self.stop.call_count, len(self.roots))
        self.assertEqual(self.sleep.call_count, len(self.roots) - 1)
        self.assertEqual(list(self.parent.iterdir()), [])

    def test_disk_capacity_and_other_failures_are_immediate(self):
        for diagnostic in (
            'FATAL: could not write to file "base/3/16384": No space left on device',
            SHM_LIMIT.replace("No space left on device", "Permission denied"),
            SHM_LIMIT.replace("shmget(", "shmat("),
            SHM_LIMIT + '\nFATAL: could not write to file "pg_wal/x": No space left on device',
        ):
            with self.subTest(diagnostic=diagnostic):
                self.launch.side_effect = test_erasure.ClusterStartupError("initdb", diagnostic)
                self.launch.reset_mock()
                with self.assertRaisesRegex(RuntimeError, "initdb"):
                    with test_erasure.isolated(self.parent / "db"):
                        self.fail("startup failure passed")
                self.launch.assert_called_once()
                self.sleep.assert_not_called()
                self.assertIn(diagnostic, self.stderr.getvalue())
                self.assertEqual(list(self.parent.iterdir()), [])

    def test_body_failure_never_retries(self):
        self.launch.side_effect = self.start
        with self.assertRaises(KeyboardInterrupt):
            with test_erasure.isolated(self.parent / "db"):
                raise KeyboardInterrupt
        self.launch.assert_called_once()
        self.stop.assert_called_once()
        self.sleep.assert_not_called()
        self.assertEqual(list(self.parent.iterdir()), [])

    def test_unconfirmed_shutdown_retains_workspace_and_never_retries(self):
        def attempt(cluster):
            self.start(cluster)
            raise test_erasure.ClusterStartupError("pg_ctl", SHM_LIMIT)

        self.launch.side_effect = attempt
        self.stop.side_effect = WorkspaceCleanupError("shutdown unconfirmed")
        with self.assertRaisesRegex(RuntimeError, "shutdown unconfirmed"):
            with test_erasure.isolated(self.parent / "db"):
                self.fail("cleanup failure passed")
        self.launch.assert_called_once()
        self.sleep.assert_not_called()
        self.assertTrue(self.roots[0].exists())


class PostgreSQLStartupDiagnosticTest(unittest.TestCase):
    def test_initdb_classifies_bootstrap_stderr(self):
        cluster = test_erasure.RehearsalQuarantine(Path("/tmp/fixture"), None)
        command = ["initdb"]
        result = subprocess.CompletedProcess(command, 1, "running bootstrap script ... ", SHM_LIMIT)
        with patch.object(test_erasure, "run_command", return_value=result) as launch:
            with self.assertRaises(test_erasure.ClusterStartupError) as failure:
                cluster._initdb(command)
        self.assertTrue(failure.exception.capacity_limited)
        self.assertEqual(launch.call_args.kwargs["env"]["LC_ALL"], "C")

    def test_pg_ctl_classifies_server_log(self):
        with tempfile.TemporaryDirectory() as folder:
            cluster = test_erasure.RehearsalQuarantine(Path(folder), None)
            (cluster.root / "postgres.log").write_text(SHM_LIMIT)
            with patch.object(test_erasure, "start_postgres", side_effect=subprocess.CalledProcessError(1, "pg_ctl")):
                with self.assertRaises(test_erasure.ClusterStartupError) as failure:
                    cluster._start_server("fixture")
            self.assertTrue(failure.exception.capacity_limited)


if __name__ == "__main__":
    unittest.main()
