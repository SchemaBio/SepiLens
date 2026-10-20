"""Cross-platform v2 smoke. Synthetic evidence only; no medical database or model."""
import argparse,base64,concurrent.futures,hashlib,json,os,platform,secrets,socket,statistics,subprocess,time,urllib.error,urllib.request
from pathlib import Path
parser=argparse.ArgumentParser();parser.add_argument("--binary",required=True);parser.add_argument("--output",default="tmp/smoke-v2.json");parser.add_argument("--requests",type=int,default=16);parser.add_argument("--assets");args=parser.parse_args()
root=Path(__file__).resolve().parents[1];binary=Path(args.binary).resolve();work=root/"tmp"/("v2-smoke-"+secrets.token_hex(6));work.mkdir(parents=True)
with socket.socket() as sock:sock.bind(("127.0.0.1",0));port=sock.getsockname()[1]
base=f"http://127.0.0.1:{port}";env={k:v for k,v in os.environ.items() if not k.startswith("SEPILENS_") and k not in ("OPENAI_API_KEY","OPENAI_BASE_URL","OPENAI_MODEL")}
env.update(SEPILENS_MASTER_KEY=base64.b64encode(secrets.token_bytes(32)).decode(),SEPILENS_DATA_DIR=str(work/"data"),SEPILENS_ADDR=f"127.0.0.1:{port}",SEPILENS_AUTH_MODE="required",SEPILENS_PRINCIPALS=json.dumps({"submit":{"tenant":"A","actor":"YiJian","role":"submitter","submit_evidence":True},"review":{"tenant":"A","actor":"R","role":"reviewer"},"admin":{"tenant":"A","actor":"ops","role":"admin"},"other":{"tenant":"B","actor":"O","role":"admin"}}),SEPILENS_SOURCE_POLICIES=json.dumps({"synthetic":{"versions":["test"],"actors":["YiJian"],"assemblies":["GRCh38"],"clinical":True}}))
log=(work/"service.log").open("wb");process=None;checks=[]
def call(method,path,body=None,key="submit",idem=None):
 headers={"Authorization":"Bearer "+key} if key else {}
 if idem:headers["Idempotency-Key"]=idem
 if body is not None:headers["Content-Type"]="application/json"
 req=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
 try:response=urllib.request.urlopen(req,timeout=30)
 except urllib.error.HTTPError as e:response=e
 with response:
  raw=response.read();return response.status,json.loads(raw) if raw and response.headers.get("Content-Type","").startswith("application/json") else raw.decode()
def check(name,ok):
 if not ok:raise AssertionError(name)
 checks.append(name)
def start():
 global process
 process=subprocess.Popen([str(binary),"serve"],cwd=work,env=env,stdout=log,stderr=log,creationflags=getattr(subprocess,"CREATE_NO_WINDOW",0))
 for _ in range(200):
  if process.poll() is not None:raise AssertionError("startup failed; inspect local service.log")
  try:
   if call("GET","/readyz",key=None)[0]==200:return
  except (OSError,urllib.error.URLError):pass
  time.sleep(.1)
 raise AssertionError("startup timeout")
def stop():
 if process and process.poll() is None:process.terminate();process.wait(timeout=15)
 log.flush()
def fixture(engine):
 r={"schema_version":"sepilens-evaluation-2.0","engine_id":engine,"mode":"local","clinical_context":{"disease_id":"MONDO:TEST","inheritance":"AD","variant_origin":"germline_confirmed","sample_type":"blood"},"evidence_package":{"sources":[]},"options":{}}
 if engine in ("acmg_amp_2015","svc_v4"):r["variant"]={"assembly":"GRCh38","chrom":"1","pos":100,"ref":"A","alt":"G","gene":"SYNTHETIC_ONLY_MARKER"}
 else:r["cnv"]={"assembly":"GRCh38","chrom":"1","start":1,"end":100,"type":"gain" if engine=="cnv_gain_2020" else "loss","coordinate_system":"1-based-inclusive"}
 if engine=="svc_v4":r["options"]["allow_experimental"]=True
 return r
try:
 start();check("no medical data mounted",not (work/"knowledge").exists());check("API Key required",call("GET","/v2/capabilities",key=None)[0]==401)
 caps=call("GET","/v2/capabilities")[1];check("five fixed engines disabled",len(caps["engines"])==5 and all(not e["enabled"] for e in caps["engines"]))
 check("legacy route removed",call("POST","/v1/variants/snv/evaluations",{})[0]==404)
 request=fixture("acmg_amp_2015");check("medical approval gate",call("POST","/v2/evaluations",request)[0]==409)
 svc=fixture("svc_v4");svc["mode"]="ai_assisted";svc["options"]["provider_profile_id"]="missing";check("AI disabled by default",call("POST","/v2/evaluations",svc)[0]==409)
 stop();env["SEPILENS_SIMULATION"]="true";start()
 for engine in ("acmg_amp_2015","svc_v4","cnv_gain_2020","cnv_loss_2020"):
  req=fixture(engine);code,result=call("POST","/v2/evaluations",req,idem=engine)
  check(engine+" synchronous result",code==200 and result["local_baseline"] is not None)
  check(engine+" no settlement",not result["settlement_eligible"] and result["clinical_use"]=="forbidden")
  if engine=="svc_v4":check("SVC never clinical",result["rule_engine_result"] is None)
  assessment=result["assessment_id"];check(engine+" persistent result",call("GET",f"/v2/jobs/{assessment}/result")[0]==200)
  check(engine+" tenant isolation",call("GET",f"/v2/jobs/{assessment}/result",key="other")[0]==404)
  check(engine+" audit",call("GET",f"/v2/jobs/{assessment}/audit",key="review")[0]==200)
  check(engine+" idempotency",call("POST","/v2/evaluations",req,idem=engine)[1]["assessment_id"]==assessment)
 request=fixture("cnv_loss_2020");request["cnv"]["start"]=-1;check("invalid coordinates",call("POST","/v2/evaluations",request)[0]==422)
 request=fixture("acmg_amp_2015");request["evidence_package"]["verified"]=True;check("forged verification rejected",call("POST","/v2/evaluations",request)[0]==400)
 def one(n):
  before=time.monotonic();code,_=call("POST","/v2/evaluations",fixture("acmg_amp_2015"),idem=f"load-{n}")
  if code!=200:raise AssertionError("load failed")
  return (time.monotonic()-before)*1000
 before=time.monotonic()
 with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:durations=list(pool.map(one,range(args.requests)))
 seconds=time.monotonic()-before
 req=fixture("acmg_amp_2015");code,result=call("POST","/v2/evaluations",req,idem="restart");assessment=result["assessment_id"];stop();start();check("restart replay without reference database",call("POST","/v2/evaluations",req,idem="restart")[1]["assessment_id"]==assessment)
 check("review cannot approve experimental SVC",call("POST","/v2/evaluations",fixture("svc_v4"))[1]["rule_engine_result"] is None)
 check("tenant deletion",call("DELETE","/v2/tenant/data",key="admin")[0]==204);check("deleted result absent",call("GET",f"/v2/jobs/{assessment}/result")[0]==404)
 stop();raw=b"".join(p.read_bytes() for p in (work/"data").glob("*.db*"));check("clinical structured inputs encrypted",b"SYNTHETIC_ONLY_MARKER" not in raw);check("keys and clinical values absent in logs",b"SYNTHETIC_ONLY_MARKER" not in (work/"service.log").read_bytes() and env["SEPILENS_MASTER_KEY"].encode() not in (work/"service.log").read_bytes())
 report={"status":"passed","checks":checks,"platform":platform.platform(),"binary_sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"performance":{"workload":"synthetic local, simulation, no medical database or model","requests":args.requests,"concurrency":4,"seconds":round(seconds,3),"p95_ms":round(sorted(durations)[int((len(durations)-1)*.95)],3)},"limitations":["No real vendor call or clinical approval","Synthetic inputs; no independent clinical accuracy claim"]}
 output=Path(args.output);output.parent.mkdir(parents=True,exist_ok=True);output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n",encoding="utf-8");print(f"PASS {len(checks)} checks: {output}")
finally:stop();log.close()
