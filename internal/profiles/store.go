package profiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"github.com/pzweuj/sepilens/internal/secure"
	"github.com/pzweuj/sepilens/internal/ssrf"
	"time"
)

var ErrNotFound = errors.New("provider profile not found")

type Profile struct {
	ID      string           `json:"id"`
	Version int              `json:"version"`
	Config  responses.Config `json:"-"`
}
type Store struct {
	DB           *sql.DB
	Cipher       *secure.Cipher
	AllowPrivate bool
}

func (s *Store) Save(ctx context.Context, tenant, id string, c responses.Config) (Profile, error) {
	if id == "" || c.APIKey == "" || c.Model == "" {
		return Profile{}, errors.New("profile requires id, model and API key")
	}
	c.AllowPrivate = s.AllowPrivate
	if _, e := ssrf.ValidateEndpoint(c.BaseURL, s.AllowPrivate); e != nil {
		return Profile{}, e
	}
	if _, e := responses.NewClient(c); e != nil {
		return Profile{}, e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return Profile{}, e
	}
	defer tx.Rollback()
	var deleted int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_deleted WHERE tenant_id=? AND profile_id=?`, tenant, id).Scan(&deleted); e != nil {
		return Profile{}, e
	}
	if deleted != 0 {
		return Profile{}, errors.New("deleted profile identity cannot be reused")
	}
	var v int
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM provider_profiles WHERE tenant_id=? AND profile_id=?`, tenant, id).Scan(&v); e != nil {
		return Profile{}, e
	}
	b, e := json.Marshal(c)
	if e != nil {
		return Profile{}, e
	}
	enc, e := s.Cipher.Seal(string(b), tenant+":"+id+":profile")
	if e != nil {
		return Profile{}, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO provider_profiles VALUES(?,?,?,?,?)`, tenant, id, v, enc, time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		return Profile{}, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM profile_deleted WHERE tenant_id=? AND profile_id=?`, tenant, id); e != nil {
		return Profile{}, e
	}
	return Profile{id, v, c}, tx.Commit()
}
func (s *Store) Get(ctx context.Context, t, id string, v int) (Profile, error) {
	var p Profile
	var enc string
	q := `SELECT profile_id,version,config FROM provider_profiles WHERE tenant_id=? AND profile_id=? AND NOT EXISTS(SELECT 1 FROM profile_deleted WHERE tenant_id=? AND profile_id=?)`
	args := []any{t, id, t, id}
	if v > 0 {
		q += " AND version=?"
		args = append(args, v)
	}
	q += " ORDER BY version DESC LIMIT 1"
	if e := s.DB.QueryRowContext(ctx, q, args...).Scan(&p.ID, &p.Version, &enc); e != nil {
		return p, ErrNotFound
	}
	plain, e := s.Cipher.Open(enc, t+":"+id+":profile")
	if e != nil {
		return p, e
	}
	if e = json.Unmarshal([]byte(plain), &p.Config); e != nil {
		return p, e
	}
	p.Config.AllowPrivate = s.AllowPrivate
	return p, nil
}
func (s *Store) Delete(ctx context.Context, t, id string) error {
	if _, e := s.Get(ctx, t, id, 0); e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT OR IGNORE INTO profile_deleted VALUES(?,?)`, t, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM provider_profiles WHERE tenant_id=? AND profile_id=?`, t, id); e != nil {
		return e
	}
	return tx.Commit()
}
