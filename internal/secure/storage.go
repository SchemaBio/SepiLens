package secure

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Reencrypt rewrites all patient/profile payload columns atomically. Metadata and public IDs remain searchable.
func Reencrypt(ctx context.Context, db *sql.DB, old, new *Cipher, allowLegacy bool) error {
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	type spec struct {
		table   string
		keys    []string
		fields  []string
		binding func([]string, int) string
	}
	specs := []spec{
		{"jobs", []string{"tenant_id", "job_id"}, []string{"result_json", "error_json"}, func(k []string, i int) string {
			suffix := []string{"result", "error"}[i]
			return k[0] + ":" + k[1] + ":" + suffix
		}},
		{"work_inputs", []string{"tenant_id", "job_id"}, []string{"payload"}, func(k []string, i int) string { return k[0] + ":" + k[1] + ":input" }},
		{"reviews", []string{"tenant_id", "evaluation_id", "id"}, []string{"reason"}, func(k []string, i int) string { return k[0] + ":" + k[1] + ":review" }},
		{"evidence_snapshots", []string{"tenant_id", "snapshot_id"}, []string{"sources_json", "items_json", "provenance_json"}, func(k []string, i int) string {
			return k[0] + ":" + k[1] + ":" + []string{"sources", "items", "provenance"}[i]
		}},
		{"provider_profiles", []string{"tenant_id", "profile_id", "version"}, []string{"config"}, func(k []string, i int) string { return k[0] + ":" + k[1] + ":profile" }},
		{"audit_versions", []string{"tenant_id", "evaluation_id", "version"}, []string{"payload"}, func(k []string, i int) string { return k[0] + ":" + k[1] + ":result" }},
	}
	for _, s := range specs {
		fields := append(append([]string(nil), s.keys...), s.fields...)
		coalesced := make([]string, len(fields))
		for i, f := range fields {
			coalesced[i] = "COALESCE(CAST(" + f + " AS TEXT),'')"
		}
		rows, e := tx.QueryContext(ctx, "SELECT "+strings.Join(coalesced, ",")+" FROM "+s.table)
		if e != nil {
			return e
		}
		var records [][]string
		for rows.Next() {
			v := make([]string, len(fields))
			dest := make([]any, len(v))
			for i := range v {
				dest[i] = &v[i]
			}
			if e = rows.Scan(dest...); e != nil {
				rows.Close()
				return e
			}
			records = append(records, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, v := range records {
			var sets, where []string
			var args []any
			for i, f := range s.fields {
				value := v[len(s.keys)+i]
				if value == "" {
					continue
				}
				binding := s.binding(v[:len(s.keys)], i)
				if strings.HasPrefix(value, "enc1:") {
					plain, err := old.Open(value, binding)
					if err != nil {
						return err
					}
					value = plain
				} else if !allowLegacy {
					return errors.New("unencrypted legacy data encountered")
				}
				value, e = new.Seal(value, binding)
				if e != nil {
					return e
				}
				sets = append(sets, f+"=?")
				args = append(args, value)
			}
			if len(sets) == 0 {
				continue
			}
			for i, k := range s.keys {
				where = append(where, k+"=?")
				args = append(args, v[i])
			}
			if _, e = tx.ExecContext(ctx, "UPDATE "+s.table+" SET "+strings.Join(sets, ",")+" WHERE "+strings.Join(where, " AND "), args...); e != nil {
				return e
			}
		}
	}
	var sentinel string
	if e = tx.QueryRowContext(ctx, `SELECT sentinel FROM encryption_state WHERE id=1`).Scan(&sentinel); e == nil {
		plain, err := old.Open(sentinel, "sepilens:master-key")
		if err != nil {
			return err
		}
		enc, err := new.Seal(plain, "sepilens:master-key")
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE encryption_state SET sentinel=? WHERE id=1`, enc); err != nil {
			return err
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	return tx.Commit()
}
