"""Normalize go list JSON stream and collect exact dependency license texts."""
import hashlib
import json
from pathlib import Path
import zipfile

root=Path(__file__).resolve().parents[1]
import argparse
parser=argparse.ArgumentParser();parser.add_argument("--out", default=str(root/"dist/rc2"));args=parser.parse_args()
out=Path(args.out)
raw=(out/"dependencies.json").read_text(encoding="utf-8-sig")
decoder=json.JSONDecoder();modules=[];offset=0
while offset<len(raw):
    while offset<len(raw) and raw[offset].isspace():offset+=1
    if offset==len(raw):break
    module,end=decoder.raw_decode(raw,offset);modules.append(module);offset=end
inventory=[]
with zipfile.ZipFile(out/"dependency-licenses.zip","w",zipfile.ZIP_DEFLATED) as archive:
    for module in modules:
        directory=Path(module.get("Dir",""))
        public={key:value for key,value in module.items() if key not in ("Dir","GoMod")}
        texts=[]
        if module.get("Dir") and directory.is_dir():
            candidates=[path for path in directory.iterdir() if path.is_file() and path.name.upper().startswith(("LICENSE","LICENCE","COPYING","NOTICE","COPYRIGHT"))]
            license_dir=directory/"licenses"
            if license_dir.is_dir():candidates += [path for path in license_dir.rglob("*") if path.is_file()]
            for path in sorted(set(candidates)):
                content=path.read_bytes()
                name=module["Path"]+"@"+module.get("Version","workspace")+"/"+path.relative_to(directory).as_posix()
                archive.writestr(name,content)
                texts.append({"archive_path":name,"sha256":hashlib.sha256(content).hexdigest()})
        public["license_texts"]=texts
        public["license_review_status"]="collected_pending_review" if texts else "missing_pending_review"
        inventory.append(public)
(out/"dependencies.json").write_text(json.dumps(inventory,indent=2)+"\n",encoding="utf-8")
print(f"Collected dependency inventory for {len(inventory)} modules; license approval is separate")
