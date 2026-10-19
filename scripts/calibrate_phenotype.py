"""Deterministic synthetic HPOA query holdout; engineering calibration only.

Uses the running SepiLens endpoint so scoring is never reimplemented in Python.
One phenotype term is held out of each synthetic query, not the reference corpus.
This measures internal ranking stability, not independent clinical precision.
"""
import argparse
import csv
import hashlib
import json
import os
from pathlib import Path
import random
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--hpoa", required=True)
    parser.add_argument("--gencc", required=True, help="Locked legacy GenCC TSV with exact disease identifiers")
    parser.add_argument("--url", default="http://127.0.0.1:8080")
    parser.add_argument("--output", required=True)
    parser.add_argument("--seed", type=int, default=20261019)
    parser.add_argument("--limit", type=int, default=200)
    args = parser.parse_args()
    rng = random.Random(args.seed)
    associations = {}
    with open(args.hpoa, encoding="utf-8") as source:
        header = None
        for line in source:
            columns = line.rstrip("\r\n").lstrip("#").split("\t")
            if header is None:
                if "database_id" in columns and "hpo_id" in columns:
                    header = columns
                continue
            if line.startswith("#") or not line.strip():
                continue
            row = dict(zip(header, columns))
            if row.get("qualifier") != "NOT":
                associations.setdefault(row["database_id"], set()).add(row["hpo_id"])
    with open(args.gencc, encoding="utf-8") as source:
        gene_rows = list(csv.DictReader(source, delimiter="\t"))
    mapping = {"Autosomal dominant": "AD", "Autosomal recessive": "AR", "X-linked dominant": "XLD", "X-linked recessive": "XLR"}
    candidates = sorted({(r["gene_symbol"], r["disease_curie"], mapping.get(r["moi_title"], r["moi_title"])) for r in gene_rows if r["classification_title"] in ("Definitive", "Strong", "Moderate") and len(associations.get(r["disease_curie"], [])) >= 3})
    rng.shuffle(candidates)
    token = os.environ.get("SEPILENS_CALIBRATION_TOKEN", "")
    outcomes = []
    for gene, disease, moi in candidates[:args.limit]:
        terms = sorted(associations[disease])
        rng.shuffle(terms)
        terms = terms[:min(6, len(terms))]
        held_out = terms.pop()
        payload = {"gene": gene, "disease_id": disease, "inheritance": moi, "phenotype": {"source": "caller_supplied", "hpo_terms": [{"id": t, "polarity": "present"} for t in sorted(terms)]}}
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        request = urllib.request.Request(args.url.rstrip("/") + "/v1/phenotype/match", data=json.dumps(payload).encode(), headers=headers)
        with urllib.request.urlopen(request, timeout=30) as response:
            result = json.load(response)
        match = result["match"]
        rank = next((s["rank"] for s in match["scores"] if s["disease_id"] == disease), None)
        outcomes.append({"gene": gene, "disease_id": disease, "held_out": held_out, "term_count": len(terms), "rank": rank, "spec_version": match["spec_version"]})
    ranks = [x["rank"] for x in outcomes if x["rank"] is not None]
    report = {"kind": "engineering_query_holdout_not_clinical_validation", "seed": args.seed, "cases": len(outcomes), "mapped_cases": len(ranks), "top1": sum(r == 1 for r in ranks) / len(ranks) if ranks else None, "top3": sum(r <= 3 for r in ranks) / len(ranks) if ranks else None, "mean_reciprocal_rank": sum(1/r for r in ranks) / len(ranks) if ranks else None, "input_sha256": {name: hashlib.sha256(Path(getattr(args, name)).read_bytes()).hexdigest() for name in ("hpoa", "gencc")}, "outcomes": outcomes}
    Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
