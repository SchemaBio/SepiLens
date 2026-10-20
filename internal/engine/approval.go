package engine

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"
)

type Approval struct {
	SchemaVersion      string    `json:"schema_version,omitempty"`
	PolicyHash         string    `json:"source_policy_hash,omitempty"`
	EngineID           EngineID  `json:"engine_id"`
	Version            string    `json:"version"`
	ContentHash        string    `json:"content_hash"`
	ValidationHash     string    `json:"validation_hash"`
	ImplementationHash string    `json:"implementation_hash"`
	Scope              string    `json:"scope"`
	Reviewer           string    `json:"reviewer"`
	ApprovedAt         time.Time `json:"approved_at"`
	Signature          string    `json:"signature"`
}

func Hash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

// LoadApproval requires independent expert signing; a caller cannot enable an engine by injecting it.
func LoadApproval(path, publicKey, implementation string, content, validation []byte) (Approval, error) {
	var a Approval
	b, err := os.ReadFile(path)
	if err != nil {
		return a, err
	}
	if err = json.Unmarshal(b, &a); err != nil {
		return a, err
	}
	if _, err = Lookup(a.EngineID); err != nil {
		return a, err
	}
	if a.EngineID == EngineSVCv4 || a.Version == "" || a.Scope == "" || a.Reviewer == "" || a.ApprovedAt.IsZero() || a.ApprovedAt.After(time.Now()) || a.ContentHash != Hash(content) || a.ValidationHash != Hash(validation) || a.ImplementationHash != implementation {
		return a, ErrRulesetNotReady
	}
	key, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return a, errors.New("invalid approval public key")
	}
	sig, err := base64.StdEncoding.DecodeString(a.Signature)
	if err != nil {
		return a, ErrRulesetNotReady
	}
	a.Signature = ""
	msg, err := json.Marshal(a)
	if err != nil || !ed25519.Verify(key, msg, sig) {
		return a, ErrRulesetNotReady
	}
	return a, nil
}
