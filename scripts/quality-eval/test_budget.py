from pathlib import Path
import tempfile
import unittest

from budget import Budget


class BudgetTests(unittest.TestCase):
    def setUp(self):
        root = Path(__file__).resolve().parents[2] / '.quality-eval'
        root.mkdir(exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(dir=root)
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / 'budget.sqlite'

    def test_unknown_usage_remains_reserved_after_restart(self):
        budget = Budget(self.path, 100)
        identity = budget.reserve(10, 10)
        budget.settle(identity)
        budget.close()
        budget = Budget(self.path, 100)
        self.addCleanup(budget.close)
        with self.assertRaisesRegex(ValueError, 'BUDGET_EXHAUSTED'):
            budget.reserve(1, 1)
        self.assertEqual(budget.totals()['unresolved_reservations_micro_yuan'], 100)

    def test_settlement_is_idempotent_and_releases_only_unused_reserve(self):
        budget = Budget(self.path, 100)
        self.addCleanup(budget.close)
        identity = budget.reserve(10, 10)
        budget.settle(identity, 1, 1)
        budget.settle(identity, 1, 1)
        self.assertEqual(budget.totals()['charged_upper_micro_yuan'], 10)
        with self.assertRaisesRegex(ValueError, 'USAGE_CHANGED'):
            budget.settle(identity, 2, 1)
        budget.reserve(1, 10)

    def test_multiple_connections_share_cap(self):
        first, second = Budget(self.path, 100), Budget(self.path, 100)
        self.addCleanup(first.close)
        self.addCleanup(second.close)
        first.reserve(10, 9)
        with self.assertRaisesRegex(ValueError, 'BUDGET_EXHAUSTED'):
            second.reserve(1, 1)

    def test_exceeded_provider_bound_is_recorded_and_stops_next_call(self):
        budget = Budget(self.path, 1000)
        self.addCleanup(budget.close)
        identity = budget.reserve(1, 1)
        with self.assertRaisesRegex(ValueError, 'PROVIDER_TOKEN_BOUND_EXCEEDED'):
            budget.settle(identity, 10, 10)
        self.assertEqual(budget.totals()['charged_upper_micro_yuan'], 100)
        with self.assertRaisesRegex(ValueError, 'BUDGET_EXHAUSTED'):
            budget.reserve(1, 1)

    def test_invalid_or_changed_cap_rejected(self):
        for cap in [0, -1, 5_000_001, True]:
            with self.assertRaises(ValueError):
                Budget(self.path, cap)
        Budget(self.path, 100).close()
        with self.assertRaisesRegex(ValueError, 'CAP_CHANGED'):
            Budget(self.path, 101)


if __name__ == '__main__':
    unittest.main()
