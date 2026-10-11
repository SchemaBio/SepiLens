"""Freeze branch parameters from a reviewed upstream checkout; never execute upstream code.

Usage: python scripts/import_svc_reference.py CHECKOUT COMMIT
The resulting snapshot is a non-authoritative reference implementation, not clinical approval.
"""
import ast
import hashlib
import json
import re
import sys
from pathlib import Path

import argparse
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("checkout")
parser.add_argument("commit")
args = parser.parse_args()
root = Path(__file__).resolve().parents[1]
upstream = Path(args.checkout).resolve()
commit = args.commit
if not re.fullmatch(r"[0-9a-f]{40}", commit):
    raise ValueError("full upstream commit required")
target = root / "internal/rules/svcv4"
target.mkdir(parents=True, exist_ok=True)
branches = {}
for name in ("nonsense", "frameshift", "exon_deletion", "exon_duplication", "start_lost", "stop_lost", "canonical_splice", "intronic_synonymous", "missense_splice"):
    tree = ast.parse((upstream / f"src/svcv4_model/scoring/pfd/{name}.py").read_text(encoding="utf-8"))
    local = {}
    for node in tree.body:
        if isinstance(node, ast.Assign) and isinstance(node.value, ast.Call):
            for var in node.targets:
                if isinstance(var, ast.Name):
                    local[var.id] = node.value
    table = next(node.value for node in tree.body if isinstance(node, ast.AnnAssign) and isinstance(node.target, ast.Name) and node.target.id == "_BRANCH")
    def spec(call):
        spl = call.func.id == "SplBranchSpec"
        fields = ("prd_lo", "prd_hi") if spl else ("parent_code", "prd_lo", "prd_hi")
        result = dict(zip(fields, map(ast.literal_eval, call.args)))
        result.update({kw.arg: ast.literal_eval(kw.value) for kw in call.keywords})
        result["splice"] = spl
        return result
    branches[name] = {key.attr: spec(local[value.id] if isinstance(value, ast.Name) else value) for key, value in zip(table.keys, table.values)}
branch_bytes = (json.dumps(branches, indent=2, sort_keys=True) + "\n").encode()
(target / "branches.json").write_bytes(branch_bytes)
files = sorted((upstream / "src/svcv4_model/scoring").rglob("*.py")) + [upstream / "src/svcv4_model/config.py", upstream / "LICENSE"]
lock = {"repository": "https://github.com/clingen-data-model/svcv4-model", "commit": commit, "status": "experimental_non_authoritative", "clinical_approval": None,
        "branch_hash": hashlib.sha256(branch_bytes).hexdigest(),
        "files": {p.relative_to(upstream).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest() for p in files}}
(target / "upstream-lock.json").write_text(json.dumps(lock, indent=2, sort_keys=True) + "\n", encoding="utf-8")
(target / "UPSTREAM_LICENSE").write_bytes((upstream / "LICENSE").read_bytes())
print(f"Pinned {commit}: {sum(len(v) for v in branches.values())} branch definitions")
