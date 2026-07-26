import importlib.util
import pathlib
import sys
import unittest
from unittest import mock


SCRIPT = pathlib.Path(__file__).with_name("backfill_public_account_ids.py")
SPEC = importlib.util.spec_from_file_location("backfill_public_account_ids", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class BackfillPublicAccountIDsTest(unittest.TestCase):
    def test_generated_ids_are_root_format_and_unique(self):
        values = {MODULE.generate_root_id() for _ in range(1000)}
        self.assertEqual(1000, len(values))
        for value in values:
            self.assertRegex(value, r"^[1-9][0-9]{15}$")

    def test_next_unique_id_retries_existing_value(self):
        cursor = mock.Mock()
        cursor.fetchone.side_effect = [(1,), None]
        counts = MODULE.Counts()

        with mock.patch.object(
            MODULE,
            "generate_root_id",
            side_effect=["1719905235756637", "1719905235756638"],
        ):
            value = MODULE.next_unique_id(cursor, retries=2, counts=counts)

        self.assertEqual("1719905235756638", value)
        self.assertEqual(1, counts.collisions)
        self.assertEqual(2, cursor.execute.call_count)

    def test_run_refuses_partially_populated_rows(self):
        cursor = mock.Mock()
        cursor.fetchone.return_value = (1,)
        connection = mock.Mock()
        connection.cursor.return_value = cursor

        with mock.patch.object(MODULE, "connect", return_value=connection):
            with self.assertRaisesRegex(RuntimeError, "partially populated"):
                MODULE.run(
                    "postgresql://unused",
                    batch_size=10,
                    start_after=0,
                    retries=2,
                    dry_run=True,
                )

        connection.close.assert_called_once_with()

    def test_counts_reports_resume_cursor(self):
        counts = MODULE.Counts(scanned=2, populated=2, last_cursor=42)
        self.assertEqual(42, counts.last_cursor)


if __name__ == "__main__":
    unittest.main()
