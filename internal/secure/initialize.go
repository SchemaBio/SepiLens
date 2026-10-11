package secure

import (
	"context"
	"database/sql"
	"errors"
)

// Initialize migrates plaintext only once. Subsequent starts validate the key sentinel.
// Payloads are authenticated on read; startup does not rewrite the full database.
func Initialize(ctx context.Context, db *sql.DB, c *Cipher) error {
	var enc string
	e := db.QueryRowContext(ctx, `SELECT sentinel FROM encryption_state WHERE id=1`).Scan(&enc)
	if e == nil {
		plain, e := c.Open(enc, "sepilens:master-key")
		if e != nil || plain != "sepilens-encryption-v1" {
			return errors.New("master key does not match database")
		}
		return nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if e = Reencrypt(ctx, db, c, c, true); e != nil {
		return e
	}
	enc, e = c.Seal("sepilens-encryption-v1", "sepilens:master-key")
	if e != nil {
		return e
	}
	_, e = db.ExecContext(ctx, `INSERT INTO encryption_state(id,sentinel) VALUES(1,?)`, enc)
	return e
}
