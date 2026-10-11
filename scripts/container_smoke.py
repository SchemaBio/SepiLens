"""Verify both loaded OCI platforms, binary identity, HTTP lifecycle and graceful stop."""
import base64
import hashlib
import json
from pathlib import Path
import secrets
import socket
import subprocess
import time
import urllib.error
import urllib.request

root=Path(__file__).resolve().parents[1]
import argparse
parser=argparse.ArgumentParser();parser.add_argument("--version",default="0.2.0-rc2");parser.add_argument("--out",default="dist/rc2");parser.add_argument("--report",default="docs/reports/rc2/container-smoke.json");args=parser.parse_args()
results=[]
for arch in ("amd64","arm64"):
    name="sepilens-smoke-"+secrets.token_hex(6)
    work=root/"tmp"/name;work.mkdir(parents=True)
    with socket.socket() as sock:
        sock.bind(("127.0.0.1",0));port=sock.getsockname()[1]
    key=base64.b64encode(secrets.token_bytes(32)).decode()
    envfile=work/"runtime.env"
    envfile.write_text("SEPILENS_MASTER_KEY="+key+"\nSEPILENS_AUTH_MODE=required\nSEPILENS_PRINCIPALS="+json.dumps({"synthetic-token":{"tenant":"container","actor":"cloud","role":"admin"}})+"\n",encoding="utf-8")
    def docker(*args):
        done=subprocess.run(["docker",*args],capture_output=True)
        if done.returncode:raise AssertionError("Docker command failed: "+done.stderr.decode(errors="replace"))
        return done.stdout.decode().strip()
    def call(method,path,body=None):
        headers={"Authorization":"Bearer synthetic-token"}
        if body is not None:headers["Content-Type"]="application/json"
        req=urllib.request.Request(f"http://127.0.0.1:{port}"+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
        try:response=urllib.request.urlopen(req,timeout=30)
        except urllib.error.HTTPError as e:response=e
        with response:
            data=response.read().decode()
            return response.status,json.loads(data) if data and response.headers.get("Content-Type","").startswith("application/json") else data
    try:
        docker("run","-d","--name",name,"--platform","linux/"+arch,"-p",f"127.0.0.1:{port}:8080","--env-file",str(envfile),"sepilens:"+args.version)
        deadline=time.monotonic()+30
        while time.monotonic()<deadline:
            try:
                if call("GET","/readyz")[0]==200:break
            except (OSError,urllib.error.URLError):pass
            time.sleep(.2)
        else:raise AssertionError("Container startup timed out")
        config=json.loads(docker("inspect",name))[0]
        assert config["Config"]["User"]=="sepilens"
        version=docker("exec",name,"/app/sepilens","version")
        assert version==args.version
        image_hash=docker("exec",name,"sha256sum","/app/sepilens").split()[0]
        binary_hash=hashlib.sha256((root/args.out/f"sepilens-{args.version}-linux-{arch}").read_bytes()).hexdigest()
        assert image_hash==binary_hash,"image and binary differ"
        assert all(not x["enabled"] for x in call("GET","/v1/capabilities")[1]["engines"])
        body={"engine_id":"acmg_amp_2015","variant":{"assembly":"GRCh38","chrom":"1","pos":100,"ref":"A","alt":"G","gene":"TEST"},"clinical_context":{"disease_id":"MONDO:TEST","inheritance":"AD","variant_origin":"unknown","sample_type":"blood","hpo_terms":[]},"options":{"retrieval_mode":"off"}}
        code,accepted=call("POST","/v1/variants/snv/evaluations",body);assert code==202
        deadline=time.monotonic()+20
        while time.monotonic()<deadline:
            code,result=call("GET","/v1/jobs/"+accepted["job_id"]+"/result")
            if code!=202:break
            time.sleep(.02)
        assert code==200 and not result["settlement_eligible"] and result["rule_engine_result"] is None
        assert call("GET","/v1/jobs/"+accepted["job_id"]+"/events")[0]==200
        assert call("GET","/v1/jobs/"+accepted["job_id"]+"/audit")[0]==200
        body["clinical_context"]["variant_origin"]="germline_confirmed"
        assert call("POST","/v1/variants/snv/evaluations",body)[0]==409
        body["engine_id"]="svc_v4";body["options"]["allow_experimental"]=True
        body["draft_assessment"]={"moi":"AD","population":{"faf":"0.00015","daft":"0.00001"}}
        code,svc=call("POST","/v1/variants/snv/evaluations",body);assert code==202
        deadline=time.monotonic()+20
        while time.monotonic()<deadline:
            code,result=call("GET","/v1/jobs/"+svc["job_id"]+"/result")
            if code!=202:break
            time.sleep(.02)
        assert code==200 and result["result_kind"]=="experimental" and result["rule_engine_result"] is None and not result["settlement_eligible"]
        assert result["agent_proposal"]["experimental_result"]["total_points"]=="-6"
        assert call("POST","/v1/evaluations/"+svc["job_id"]+"/reviews",{"revision":1,"decision":"accept_engine_result","reason":"synthetic"})[0]==409
        hpo={"text":"患者否认癫痫发作。","options":{"allow_local_fallback":True}}
        code,extraction=call("POST","/v1/hpo/extract",hpo)
        assert code==200 and any(x["polarity"]=="absent" for x in extraction["terms"])
        assert call("DELETE","/v1/tenant/data")[0]==204
        before=time.monotonic();docker("stop","-t","15",name)
        stop_seconds=time.monotonic()-before
        exit_code=json.loads(docker("inspect",name))[0]["State"]["ExitCode"]
        assert exit_code==0,"SIGTERM shutdown failed"
        results.append({"platform":"linux/"+arch,"status":"passed","version":version,"binary_sha256":binary_hash,"image_id":config["Image"],"non_root":True,"graceful_exit_code":exit_code,"stop_seconds":round(stop_seconds,3),"checks":["OCI load","binary identity","ready","disabled gate","queued result","SSE","audit","SVC exact boundary and no clinical promotion","HPO explicit fallback","tenant delete","SIGTERM shutdown"],"architecture_execution":"native" if arch=="amd64" else "Docker Desktop emulation"})
        print("PASS linux/"+arch)
    finally:
        subprocess.run(["docker","rm","-f","-v",name],capture_output=True)
output=root/args.report;output.parent.mkdir(parents=True,exist_ok=True)
output.write_text(json.dumps({"status":"passed","image":"sepilens:"+args.version,"platforms":results,"limitations":["Synthetic, no real model calls or clinical approval","arm64 is emulated, not actual arm64 deployment hardware"]},indent=2)+"\n",encoding="utf-8")
