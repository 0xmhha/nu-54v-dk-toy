"""Verify the two-file W10 commissioning publication and recovery."""

import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

SCRIPTS = Path(__file__).resolve().parents[1] / "scripts"
sys.path.insert(0, str(SCRIPTS))
from enroll_board import publish_w10_commissioning


class W10CommissioningPublicationTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="nu54-w10-publication-")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.grant = self.root / "board.n54q"
        self.publication = self.root / "w10.json"

    def publish(self):
        publish_w10_commissioning(self.grant, b"N54Q grant", self.publication, b"N54V publication")

    def test_publish_grant_before_roster_and_keep_outputs_private(self):
        link = os.link
        def check(source, destination):
            if destination == self.publication:
                self.assertEqual(self.grant.read_bytes(), b"N54Q grant")
            return link(source, destination)
        with patch("enroll_board.os.link", side_effect=check):
            self.publish()
        self.assertEqual(self.publication.read_bytes(), b"N54V publication")
        self.assertEqual(self.grant.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.publication.stat().st_mode & 0o777, 0o600)
        self.assertCountEqual(self.root.iterdir(), [self.grant, self.publication])

    def test_existing_grant_is_preserved_without_new_roster(self):
        self.grant.write_bytes(b"existing board")
        with self.assertRaises(FileExistsError):
            self.publish()
        self.assertEqual(self.grant.read_bytes(), b"existing board")
        self.assertCountEqual(self.root.iterdir(), [self.grant])

    def test_existing_roster_is_preserved_and_failed_grant_is_removed(self):
        self.publication.write_bytes(b"existing roster")
        with self.assertRaises(FileExistsError):
            self.publish()
        self.assertEqual(self.publication.read_bytes(), b"existing roster")
        self.assertCountEqual(self.root.iterdir(), [self.publication])

    def test_transient_roster_failure_can_retry_same_filenames(self):
        link = os.link
        def fail_publication(source, destination):
            if destination == self.publication:
                raise PermissionError("publication unavailable")
            return link(source, destination)
        with patch("enroll_board.os.link", side_effect=fail_publication):
            with self.assertRaises(PermissionError):
                self.publish()
        self.assertEqual(list(self.root.iterdir()), [])
        self.publish()
        self.assertCountEqual(self.root.iterdir(), [self.grant, self.publication])


if __name__ == "__main__":
    unittest.main()
