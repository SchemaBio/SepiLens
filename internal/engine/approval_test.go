package engine_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/engine"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApprovalBindsRuleReportAndImplementation(t *testing.T) {
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	rules, report := []byte("independent rule content"), []byte("independent case report")
	a := engine.Approval{EngineID: engine.EngineCNVLoss2020, Version: "synthetic-test", ContentHash: engine.Hash(rules), ValidationHash: engine.Hash(report), ImplementationHash: "binary", Scope: "synthetic-only", Reviewer: "test reviewer", ApprovedAt: time.Now().Add(-time.Minute).UTC()}
	b, _ := json.Marshal(a)
	a.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, b))
	b, _ = json.Marshal(a)
	p := filepath.Join(t.TempDir(), "approval.json")
	if e = os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	public := base64.StdEncoding.EncodeToString(pub)
	if _, e = engine.LoadApproval(p, public, "binary", rules, report); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		binary        string
		rules, report []byte
	}{{"changed", rules, report}, {"binary", []byte("changed rules"), report}, {"binary", rules, []byte("changed report")}} {
		if _, e = engine.LoadApproval(p, public, c.binary, c.rules, c.report); e == nil {
			t.Fatal("modified approval accepted")
		}
	}
	a.EngineID = engine.EngineSVCv4
	b, _ = json.Marshal(a)
	if e = os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = engine.LoadApproval(p, public, "binary", rules, report); e == nil {
		t.Fatal("SVC approved")
	}
}
