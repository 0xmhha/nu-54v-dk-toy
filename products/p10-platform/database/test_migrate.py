"""Migration inputs must be contiguous and transaction-owned by the runner."""

import tempfile
import unittest
from pathlib import Path

import migrate


class MigrationInputTests(unittest.TestCase):
    def test_snapshots_are_digested_and_names_are_ordered(self):
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            (root / "002_next.sql").write_text("SELECT 2;")
            (root / "001_first.sql").write_text("SELECT 1;")
            result = migrate.migrations(root)
            self.assertEqual([item.version for item in result], [1, 2])
            self.assertEqual([len(item.digest) for item in result], [64, 64])

    def test_embedded_password_is_rejected_without_echoing(self):
        for url in ("postgresql://db:secret@localhost/test", "host=/tmp password=secret dbname=tmp",
                    "postgresql://localhost/tmp?%70assword=secret"):
            with self.subTest(url=url):
                with self.assertRaisesRegex(ValueError, "PGPASSFILE") as result:
                    migrate.check_connection(url)
                self.assertNotIn("secret", str(result.exception))

    def test_migration_gap_must_fail(self):
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            (root / "002_next.sql").write_text("SELECT 2;")
            with self.assertRaisesRegex(ValueError, "contiguous"):
                migrate.migrations(root)

    def test_transaction_escape_and_psql_directives_are_rejected(self):
        for body in ("BEGIN;\nSELECT 1;", "SELECT 1; COMMIT;", "\\include other.sql", "SELECT 1;\nROLLBACK;"):
            with self.subTest(body=body), tempfile.TemporaryDirectory() as scratch:
                root = Path(scratch)
                (root / "001_bad.sql").write_text(body)
                with self.assertRaisesRegex(ValueError, "transaction"):
                    migrate.migrations(root)


if __name__ == "__main__":
    unittest.main()
