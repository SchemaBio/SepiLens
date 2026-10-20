"""Package binaries plus public deployment assets; never include .env, keys, DB or tmp."""
import argparse
import hashlib
import json
from pathlib import Path
import tarfile
import subprocess
import zipfile

parser=argparse.ArgumentParser()
parser.add_argument("--version", default="0.2.0-rc2")
parser.add_argument("--out", default="dist/rc2")
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
out=root/args.out
assets=[root/"internal/rules/svcv4/UPSTREAM_LICENSE"]
for name in ("README.md","CHANGELOG.md",".env.example","LICENSE","THIRD_PARTY_NOTICES.md"):
    if (root/name).is_file(): assets.append(root/name)
for name in ("docs","rules/packages","scripts"):
    assets += [p for p in (root/name).rglob("*") if p.is_file() and "__pycache__" not in p.parts and p.suffix != ".pyc"]
for target in ("windows-amd64","linux-amd64","linux-arm64"):
    stem=f"sepilens-{args.version}-{target}"
    binary=out/(stem+(".exe" if target.startswith("windows") else ""))
    if not binary.is_file(): raise SystemExit("Missing binary: "+str(binary))
    if target.startswith("windows"):
        with zipfile.ZipFile(out/(stem+".zip"),"w",zipfile.ZIP_DEFLATED) as package:
            package.write(binary,stem+"/sepilens.exe")
            for path in assets: package.write(path,stem+"/"+path.relative_to(root).as_posix())
    else:
        with tarfile.open(out/(stem+".tar.gz"),"w:gz") as package:
            info=package.gettarinfo(str(binary),stem+"/sepilens");info.mode=0o755
            with binary.open("rb") as stream: package.addfile(info,stream)
            for path in assets: package.add(path,arcname=stem+"/"+path.relative_to(root).as_posix(),recursive=False)
listed=subprocess.run(["git","ls-files","--cached","--others","--exclude-standard","-z"],cwd=root,capture_output=True,check=True).stdout.decode("utf-8").split("\0")
with zipfile.ZipFile(out/f"sepilens-{args.version}-source.zip","w",zipfile.ZIP_DEFLATED) as source:
    for name in sorted(set(listed)):
        if name and (root/name).is_file() and not any(part in (".git",".agents",".codex","secrets","tmp","dist","__pycache__") for part in Path(name).parts) and Path(name).name not in (".env",".env.local"):
            source.write(root/name,"sepilens-source/"+name)
inventory=[{"name":path.name,"sha256":hashlib.sha256(path.read_bytes()).hexdigest(),"bytes":path.stat().st_size} for path in sorted(out.iterdir()) if path.is_file() and path.name not in ("SHA256SUMS","release-manifest.json")]
(out/"release-manifest.json").write_text(json.dumps({"version":args.version,"clinical_engines":"disabled_pending_independent_validation_and_approval","experimental_engines":["svc_v4"],"artifacts":inventory},indent=2)+"\n",encoding="utf-8")
files=[path for path in sorted(out.iterdir()) if path.is_file() and path.name!="SHA256SUMS"]
(out/"SHA256SUMS").write_text("".join(hashlib.sha256(path.read_bytes()).hexdigest()+"  "+path.name+"\n" for path in files),encoding="ascii")
print(f"Packaged {len(files)} artifacts")
