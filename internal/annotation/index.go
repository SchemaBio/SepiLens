package annotation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// AssetMetadata binds an index to the verified source bytes and import policy.
type AssetMetadata struct {
	Complete         bool   `json:"complete"`
	Kind             string `json:"kind"`
	Version          string `json:"version"`
	Assembly         string `json:"assembly,omitempty"`
	SHA256           string `json:"sha256"`
	License          string `json:"license"`
	LicenseConfirmed bool   `json:"license_confirmed"`
	RetrievedAt      string `json:"retrieved_at"`
	MaxAgeDays       int    `json:"max_age_days,omitempty"`
}

type Query struct {
	Assembly   string `json:"assembly"`
	Chrom      string `json:"chrom"`
	Pos        int64  `json:"pos"`
	Ref        string `json:"ref"`
	Alt        string `json:"alt"`
	Gene       string `json:"gene"`
	DiseaseID  string `json:"disease_id"`
	MOI        string `json:"moi"`
	Transcript string `json:"transcript,omitempty"`
	Protein    string `json:"protein_change,omitempty"`
}

type IndexedRecord struct {
	Kind  string
	Query Query
	Data  json.RawMessage
}

type Index struct {
	db     *sql.DB
	Hash   string
	Assets []AssetMetadata
}

// BuildIndex publishes only a fully committed index. The old index is never
// removed to make a failed replacement succeed (important on Windows).
func BuildIndex(ctx context.Context, destination, hash string, assets []AssetMetadata, feed func(func(IndexedRecord) error) error) error {
	f, err := os.CreateTemp(filepath.Dir(destination), ".annotations-*.sqlite")
	if err != nil {
		return err
	}
	tmp := f.Name()
	f.Close()
	defer os.Remove(tmp)
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `CREATE TABLE metadata(payload TEXT NOT NULL); CREATE TABLE records(kind TEXT NOT NULL,assembly TEXT,chrom TEXT,pos INTEGER,ref TEXT,alt TEXT,gene TEXT,disease TEXT,moi TEXT,transcript TEXT,protein TEXT,residue TEXT,payload TEXT NOT NULL);`)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	b, _ := json.Marshal(struct {
		Hash   string
		Assets []AssetMetadata
	}{hash, assets})
	if _, err = tx.ExecContext(ctx, "INSERT INTO metadata VALUES (?)", string(b)); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO records VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	err = feed(func(r IndexedRecord) error {
		if !json.Valid(r.Data) {
			return errors.New("invalid indexed JSON")
		}
		q := r.Query
		_, e := stmt.ExecContext(ctx, r.Kind, q.Assembly, strings.TrimPrefix(strings.ToLower(q.Chrom), "chr"), q.Pos, q.Ref, q.Alt, strings.ToUpper(q.Gene), q.DiseaseID, q.MOI, q.Transcript, q.Protein, parseProtein(q.Protein).position, string(r.Data))
		return e
	})
	if err != nil {
		return err
	}
	if err = stmt.Close(); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE INDEX variant_lookup ON records(kind,assembly,chrom,pos,ref,alt); CREATE INDEX gene_lookup ON records(kind,gene,disease,moi); CREATE INDEX protein_lookup ON records(kind,assembly,gene,transcript,residue);`); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	var check string
	if err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
		return errors.New("annotation index integrity failed")
	}
	if err = db.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, destination)
}

func OpenIndex(file, expectedHash string) (*Index, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	if filepath.VolumeName(abs) != "" {
		u.Path = "/" + u.Path
	}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	var raw string
	var meta struct {
		Hash   string
		Assets []AssetMetadata
	}
	if err = db.QueryRow("SELECT payload FROM metadata").Scan(&raw); err == nil {
		err = json.Unmarshal([]byte(raw), &meta)
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("read annotation index metadata: %w", err)
	}
	if meta.Hash != expectedHash {
		db.Close()
		return nil, errors.New("annotation index does not match locked bundle")
	}
	return &Index{db: db, Hash: meta.Hash, Assets: meta.Assets}, nil
}
func (i *Index) Close() error { return i.db.Close() }

func (i *Index) GeneAssertions(ctx context.Context, gene string) ([]GenCC, error) {
	rows, err := i.db.QueryContext(ctx, "SELECT payload FROM records WHERE kind='gencc' AND (?='' OR gene=?) ORDER BY disease,moi,payload", gene, strings.ToUpper(gene))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GenCC
	for rows.Next() {
		var b string
		var g GenCC
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (i *Index) Resolve(ctx context.Context, q Query, now time.Time) (*Set, error) {
	out := &Set{Source: SourceSnapshot}
	for _, kind := range []string{"clinvar", "gnomad", "alphamissense", "gencc", "pangolin"} {
		var meta *AssetMetadata
		for n := range i.Assets {
			if i.Assets[n].Kind == kind && (i.Assets[n].Assembly == "" || i.Assets[n].Assembly == q.Assembly) {
				if meta != nil {
					return nil, fmt.Errorf("multiple active %s assets", kind)
				}
				meta = &i.Assets[n]
			}
		}
		record := Record{Source: SourceSnapshot, Version: "unavailable", Status: StatusUnavailable, Reason: "no licensed snapshot loaded"}
		var payload []byte
		if meta != nil {
			record.Version = meta.Version
			record.Assembly = meta.Assembly
			record.RetrievedAt = meta.RetrievedAt
			if meta.LicenseConfirmed {
				record.Status = StatusNotFound
				record.Reason = "no exact record"
				query := "SELECT payload FROM records WHERE kind=? AND assembly=? AND chrom=? AND pos=? AND ref=? AND alt=? ORDER BY payload"
				args := []any{kind, q.Assembly, strings.TrimPrefix(strings.ToLower(q.Chrom), "chr"), q.Pos, q.Ref, q.Alt}
				if kind == "gencc" {
					query = "SELECT payload FROM records WHERE kind=? AND gene=? AND disease=? AND moi=? ORDER BY payload"
					args = []any{kind, strings.ToUpper(q.Gene), q.DiseaseID, q.MOI}
				}
				rows, err := i.db.QueryContext(ctx, query, args...)
				if err != nil {
					return nil, errors.New("annotation index query failed")
				}
				for rows.Next() {
					var b []byte
					if err = rows.Scan(&b); err != nil {
						rows.Close()
						return nil, err
					}
					if payload == nil {
						payload = b
						record.Status = StatusFound
						record.Reason = ""
					} else if string(payload) != string(b) {
						record.Status = StatusConflicting
						record.Reason = "multiple conflicting snapshot records"
					}
				}
				err = rows.Err()
				rows.Close()
				if err != nil {
					return nil, err
				}
				at, e := time.Parse(time.RFC3339, meta.RetrievedAt)
				age := meta.MaxAgeDays
				if age == 0 {
					age = 90
				}
				if e != nil || at.After(now) {
					record.Status = StatusUnavailable
					record.Reason = "invalid snapshot access date"
				} else if now.Sub(at) > time.Duration(age)*24*time.Hour {
					record.Status = StatusStale
					record.Reason = "snapshot exceeds maximum age"
				}
			}
		}
		queryCopy := q
		record.Query = &queryCopy
		if payload != nil {
			sum := sha256.Sum256(payload)
			record.RecordHash = "sha256:" + hex.EncodeToString(sum[:])
		}
		switch kind {
		case "clinvar":
			v := &ClinVar{}
			if payload != nil {
				if err := json.Unmarshal(payload, v); err != nil {
					return nil, err
				}
			}
			v.Record = record
			out.ClinVar = v
		case "gnomad":
			v := &GnomAD{}
			if payload != nil {
				if err := json.Unmarshal(payload, v); err != nil {
					return nil, err
				}
			}
			v.Record = record
			out.GnomAD = v
		case "alphamissense":
			v := &AlphaMissense{}
			if payload != nil {
				if err := json.Unmarshal(payload, v); err != nil {
					return nil, err
				}
			}
			v.Record = record
			if v.Transcript != "" && v.Transcript != q.Transcript {
				v.Status = StatusUnavailable
				v.Reason = "transcript mismatch"
			}
			out.AlphaMissense = v
		case "pangolin":
			out.Pangolin = &Pangolin{Record: record}
		case "gencc":
			v := &GenCC{}
			if payload != nil {
				if err := json.Unmarshal(payload, v); err != nil {
					return nil, err
				}
			}
			v.Record = record
			if record.Status == StatusConflicting {
				v.Classification = "Disputed"
			}
			out.GenCC = v
		}
	}
	if cv := out.ClinVar; cv != nil && cv.Status == StatusFound && q.Transcript != "" && q.Protein != "" && q.Gene != "" {
		matches, err := i.ProteinMatches(ctx, "clinvar", q)
		if err != nil {
			return nil, err
		}
		for _, r := range matches {
			if r.Query.Chrom == strings.TrimPrefix(strings.ToLower(q.Chrom), "chr") && r.Query.Pos == q.Pos && r.Query.Ref == q.Ref && r.Query.Alt == q.Alt {
				continue
			}
			var other ClinVar
			if err = json.Unmarshal(r.Data, &other); err != nil {
				return nil, err
			}
			diseaseMatched := false
			for _, d := range other.Conditions {
				diseaseMatched = diseaseMatched || d == q.DiseaseID
			}
			if !diseaseMatched || !other.IsPathogenic() || other.Conflicting || other.ReviewStars() < MinReviewStarsForRelativeCriterion {
				continue
			}
			if SameAminoAcid(q.Protein, other.HGVSp) {
				cv.OtherPathogenicSameAminoAcid = append(cv.OtherPathogenicSameAminoAcid, other.VariationID)
			} else if _, ok := SameResidue(q.Protein, other.HGVSp); ok {
				cv.OtherPathogenicSameResidue = append(cv.OtherPathogenicSameResidue, other.VariationID)
			}
		}
	}
	return out, nil
}

// ProteinMatches requires an exact versioned transcript and residue. It never
// infers equivalent transcripts or uses residue numbers across proteins.
func (i *Index) ProteinMatches(ctx context.Context, kind string, q Query) ([]IndexedRecord, error) {
	residue := parseProtein(q.Protein).position
	if residue == "" || q.Transcript == "" || q.Gene == "" {
		return nil, nil
	}
	rows, err := i.db.QueryContext(ctx, "SELECT chrom,pos,ref,alt,protein,payload FROM records WHERE kind=? AND assembly=? AND gene=? AND transcript=? AND residue=? ORDER BY pos,ref,alt,payload", kind, q.Assembly, strings.ToUpper(q.Gene), q.Transcript, residue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IndexedRecord
	for rows.Next() {
		r := IndexedRecord{Kind: kind, Query: Query{Assembly: q.Assembly, Gene: q.Gene, Transcript: q.Transcript}}
		if err = rows.Scan(&r.Query.Chrom, &r.Query.Pos, &r.Query.Ref, &r.Query.Alt, &r.Query.Protein, &r.Data); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
