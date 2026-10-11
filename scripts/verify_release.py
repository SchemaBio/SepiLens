"""Validate delivered artifact checksums, package contents and final smoke identities."""
import hashlib
import json
from pathlib import Path
import tarfile
import zipfile

import argparse
parser=argparse.ArgumentParser();parser.add_argument("--version",default="0.2.0-rc2");parser.add_argument("--out",default="dist/rc2");parser.add_argument("--reports",default="docs/reports/rc2");args=parser.parse_args()
root=Path(__file__).resolve().parents[1];out=root/args.out
digest=lambda data:hashlib.sha256(data).hexdigest()
manifest=json.loads((out/"release-manifest.json").read_text(encoding="utf-8"))
for artifact in manifest["artifacts"]:
    path=out/artifact["name"]
    assert path.is_file() and path.stat().st_size==artifact["bytes"] and digest(path.read_bytes())==artifact["sha256"],"artifact mismatch: "+path.name
for line in (out/"SHA256SUMS").read_text(encoding="ascii").splitlines():
    expected,name=line.split("  ",1)
    assert digest((out/name).read_bytes())==expected,"checksum mismatch: "+name
for platform in ("windows-amd64","linux-amd64","linux-arm64"):
    stem="sepilens-"+args.version+"-"+platform
    binary=out/(stem+(".exe" if platform.startswith("windows") else ""))
    expected=digest(binary.read_bytes())
    if platform.startswith("windows"):
        with zipfile.ZipFile(out/(stem+".zip")) as archive:
            assert digest(archive.read(stem+"/sepilens.exe"))==expected
            assert stem+"/docs/openapi.yaml" in archive.namelist()
    else:
        with tarfile.open(out/(stem+".tar.gz")) as archive:
            member=archive.getmember(stem+"/sepilens")
            assert member.mode & 0o111 and digest(archive.extractfile(member).read())==expected
    report_name="windows-install-smoke.json" if platform.startswith("windows") else "linux-install-smoke.json"
    if platform!="linux-arm64":
        report=json.loads((root/args.reports/report_name).read_text(encoding="utf-8"))
        assert report["status"]=="passed" and report["binary_sha256"]==expected and len(report["checks"])>=30
with tarfile.open(out/("sepilens-"+args.version+"-runtime-images.tar")) as archive:
    platforms=set()
    def visit(node):
        for entry in node.get("manifests",[]):
            info=entry.get("platform",{})
            if info.get("os")=="linux" and info.get("architecture") in ("amd64","arm64"):
                platforms.add(info["architecture"])
            name="blobs/sha256/"+entry["digest"].split(":",1)[1]
            payload=json.loads(archive.extractfile(name).read())
            if "manifests" in payload:visit(payload)
    visit(json.loads(archive.extractfile("index.json").read()))
    assert platforms=={"amd64","arm64"},"OCI platform missing"
containers=json.loads((root/args.reports/"container-smoke.json").read_text(encoding="utf-8"))
assert containers["status"]=="passed"
for result in containers["platforms"]:
    arch=result["platform"].split("/")[1]
    assert result["non_root"] and result["graceful_exit_code"]==0
    assert result["binary_sha256"]==digest((out/("sepilens-"+args.version+"-linux-"+arch)).read_bytes())
spec=json.loads((root/"docs/openapi.yaml").read_text(encoding="utf-8"))
assert spec["openapi"]=="3.1.0" and sum(len(item) for item in spec["paths"].values())==20
def references(value):
    if isinstance(value,dict):
        if "$ref" in value:assert value["$ref"].split("/")[-1] in spec["components"]["schemas"]
        for item in value.values():references(item)
    elif isinstance(value,list):
        for item in value:references(item)
references(spec)
with zipfile.ZipFile(out/("sepilens-"+args.version+"-source.zip")) as source:
    assert all(Path(name).name not in (".env","master.key") and "/.git/" not in name for name in source.namelist())
print("PASS: checksums, three installation packages, source, two OCI platforms and final smoke identities")
