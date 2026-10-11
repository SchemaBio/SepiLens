package secure_test

import (
	"context"
	"encoding/base64"
	"github.com/pzweuj/sepilens/internal/secure"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotationIsAtomicAndRejectsOldKey(t *testing.T) {
	ctx := context.Background()
	db, e := sqlite.Open(filepath.Join(t.TempDir(), "rotation.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	old, _ := secure.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	new, _ := secure.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("n", 32))))
	if e = secure.Initialize(ctx, db, old); e != nil {
		t.Fatal(e)
	}
	payload, e := old.Seal(`{"clinical":"marker"}`, "tenant:job:input")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO work_inputs(job_id,tenant_id,payload) VALUES('job','tenant',?)`, payload); e != nil {
		t.Fatal(e)
	}
	if e = secure.Reencrypt(ctx, db, old, new, false); e != nil {
		t.Fatal(e)
	}
	if e = secure.Initialize(ctx, db, new); e != nil {
		t.Fatal(e)
	}
	if e = secure.Initialize(ctx, db, old); e == nil {
		t.Fatal("old key accepted")
	}
	var ciphertext string
	if e = db.QueryRow(`SELECT payload FROM work_inputs WHERE job_id='job'`).Scan(&ciphertext); e != nil {
		t.Fatal(e)
	}
	plain, e := new.Open(ciphertext, "tenant:job:input")
	if e != nil || !strings.Contains(plain, "marker") {
		t.Fatal("data lost during rotation", e)
	}
	if _, e = db.Exec(`INSERT INTO work_inputs(job_id,tenant_id,payload) VALUES('corrupt','tenant','enc1:bad')`); e != nil {
		t.Fatal(e)
	}
	if e = secure.Reencrypt(ctx, db, new, old, false); e == nil {
		t.Fatal("corrupt rotation succeeded")
	}
	if e = secure.Initialize(ctx, db, new); e != nil {
		t.Fatal("failed rotation changed sentinel", e)
	}
	if e = db.QueryRow(`SELECT payload FROM work_inputs WHERE job_id='job'`).Scan(&ciphertext); e != nil {
		t.Fatal(e)
	}
	if _, e = new.Open(ciphertext, "tenant:job:input"); e != nil {
		t.Fatal("failed rotation modified records", e)
	}
}
