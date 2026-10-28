"""Persistent reservations for an isolated evaluator, never production billing.

Amounts are integer micro-yuan. The caller must enforce both token ceilings at
the provider boundary. Every retry/repair is a separate reservation.
"""
import sqlite3
import uuid


class Budget:
    def __init__(self, path, cap_micro_yuan=5_000_000):
        if type(cap_micro_yuan) is not int or not 0 < cap_micro_yuan <= 5_000_000:
            raise ValueError('INVALID_CAP')
        self.db = sqlite3.connect(path, timeout=10, isolation_level=None)
        self.db.execute('CREATE TABLE IF NOT EXISTS policy (id INTEGER PRIMARY KEY CHECK(id=1), cap INTEGER NOT NULL)')
        self.db.execute('CREATE TABLE IF NOT EXISTS requests (id TEXT PRIMARY KEY, held INTEGER NOT NULL, charged INTEGER, state TEXT NOT NULL)')
        self.db.execute('INSERT OR IGNORE INTO policy VALUES(1,?)', (cap_micro_yuan,))
        if self.db.execute('SELECT cap FROM policy WHERE id=1').fetchone()[0] != cap_micro_yuan:
            self.db.close()
            raise ValueError('CAP_CHANGED')

    def close(self):
        self.db.close()

    def reserve(self, input_token_ceiling, output_token_ceiling):
        if any(type(v) is not int or v < 0 for v in (input_token_ceiling, output_token_ceiling)) or output_token_ceiling == 0:
            raise ValueError('INVALID_TOKEN_CEILING')
        # Verified Flash peak prices: 2/8 yuan per million tokens, no cache credit.
        held = input_token_ceiling * 2 + output_token_ceiling * 8
        self.db.execute('BEGIN IMMEDIATE')
        try:
            cap = self.db.execute('SELECT cap FROM policy WHERE id=1').fetchone()[0]
            used = self.db.execute('SELECT COALESCE(SUM(COALESCE(charged,held)),0) FROM requests').fetchone()[0]
            violation = self.db.execute("SELECT COUNT(*) FROM requests WHERE state='bound_exceeded'").fetchone()[0]
            if violation or used + held > cap:
                raise ValueError('BUDGET_EXHAUSTED')
            identity = uuid.uuid4().hex
            self.db.execute("INSERT INTO requests VALUES(?,?,NULL,'reserved')", (identity, held))
            self.db.execute('COMMIT')
            return identity
        except BaseException:
            self.db.execute('ROLLBACK')
            raise

    def settle(self, identity, input_tokens=None, output_tokens=None):
        # Transport failures/timeouts/unknown usage retain the full reservation.
        if input_tokens is None or output_tokens is None:
            return
        if any(type(v) is not int or v < 0 for v in (input_tokens, output_tokens)):
            raise ValueError('INVALID_USAGE')
        charged = input_tokens * 2 + output_tokens * 8
        self.db.execute('BEGIN IMMEDIATE')
        try:
            row = self.db.execute('SELECT held,charged FROM requests WHERE id=?', (identity,)).fetchone()
            if row is None:
                raise ValueError('UNKNOWN_RESERVATION')
            if row[1] is not None and row[1] != charged:
                raise ValueError('USAGE_CHANGED')
            state = 'settled' if charged <= row[0] else 'bound_exceeded'
            self.db.execute('UPDATE requests SET charged=?,state=? WHERE id=?', (charged, state, identity))
            self.db.execute('COMMIT')
        except BaseException:
            self.db.execute('ROLLBACK')
            raise
        if state == 'bound_exceeded':
            raise ValueError('PROVIDER_TOKEN_BOUND_EXCEEDED')

    def totals(self):
        cap = self.db.execute('SELECT cap FROM policy WHERE id=1').fetchone()[0]
        charged, pending, requests = self.db.execute('SELECT COALESCE(SUM(charged),0),COALESCE(SUM(CASE WHEN charged IS NULL THEN held ELSE 0 END),0),COUNT(*) FROM requests').fetchone()
        return {'cap_micro_yuan': cap, 'charged_upper_micro_yuan': charged,
                'unresolved_reservations_micro_yuan': pending, 'requests': requests}
