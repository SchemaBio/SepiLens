"""Generate the v2 contract from Go JSON fields; --check detects drift."""
import argparse
import json
import re
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
PACKAGES={"assessment":"Assessment","annotation":"Annotation","api":"API","acmg2015":"ACMG","cnv":"CNV","svcv4":"SVC","hpo":"HPO","phenotype":"Phenotype","evidence":"Evidence","jobs":"Jobs","engine":"Engine"}
DIRS={name:ROOT/"internal"/("rules/"+name if name in ("acmg2015","svcv4") else "variant/cnv" if name=="cnv" else name) for name in PACKAGES}
structs={};aliases={}
for package,directory in DIRS.items():
 for path in sorted(directory.glob("*.go")):
  if path.name.endswith("_test.go"):continue
  source=path.read_text(encoding="utf-8")
  for name,body in re.findall(r"type (\w+) struct \{(.*?)\n\}",source,re.S):structs[(package,name)]=body
  for name,kind in re.findall(r"type (\w+) (string|int|int64|bool)\b",source):aliases[(package,name)]=kind
schemas={};building=set()
def ref(package,name):
 key=PACKAGES[package]+name
 if key not in schemas and (package,name) not in building:build(package,name)
 return {"$ref":"#/components/schemas/"+key}
def field(package,t):
 if t.startswith("*"):return {"anyOf":[field(package,t[1:]),{"type":"null"}]}
 if t.startswith("[]"):return {"type":["array","null"],"items":field(package,t[2:])}
 if t.startswith("map[string]"):return {"type":"object","additionalProperties":field(package,t[11:])}
 if t in ("any","interface{}","json.RawMessage"):return {}
 if t=="time.Time":return {"type":"string","format":"date-time"}
 if "." in t:package,t=t.split(".",1)
 if (package,t) in aliases:return field(package,aliases[(package,t)])
 if t=="Point":return {"type":"string","pattern":r"^-?[0-9]+(?:\.[0-9]{1,6})?$"}
 if t in ("int","int64"):return {"type":"integer","format":"int64"}
 if t=="float64":return {"type":"number"}
 if t in ("string","bool"):return {"type":"boolean" if t=="bool" else "string"}
 if (package,t) not in structs:raise ValueError("Unknown Go JSON type: "+package+"."+t)
 return ref(package,t)
def build(package,name):
 building.add((package,name));props={};required=[]
 for line in structs[(package,name)].splitlines():
  match=re.match(r'\s*\w+\s+([\w.\[\]*{}]+)\s+`json:"([^\"]+)"`',line)
  if match:
   t,tag=match.groups();key=tag.split(",")[0]
   if key=="-":continue
   props[key]=field(package,t)
   if "omitempty" not in tag and "omitzero" not in tag:required.append(key)
  else:
   embed=re.match(r"\s*([A-Z]\w*|\w+\.[A-Z]\w*)\s*$",line)
   if embed:
    t=embed.group(1);ep,en=t.split(".",1) if "." in t else (package,t)
    build(ep,en);base=schemas[PACKAGES[ep]+en];props.update(base["properties"]);required.extend(base.get("required",[]))
 schemas[PACKAGES[package]+name]={"type":"object","properties":props,"additionalProperties":False}
 if required:schemas[PACKAGES[package]+name]["required"]=sorted(set(required))
 building.remove((package,name))
paths={}
def op(path,method,summary,request=None,response=None,status="200",responses=None):
 item={"summary":summary,"responses":{status:{"description":"Success"}},"parameters":[{"name":n,"in":"path","required":True,"schema":{"type":"string"}} for n in re.findall(r"\{(\w+)\}",path)]}
 if request:item["requestBody"]={"required":True,"content":{"application/json":{"schema":ref(*request)}}}
 if response:item["responses"][status]["content"]={"application/json":{"schema":ref(*response)}}
 if responses:item["responses"].update(responses)
 for code in ("400","401","403","404","409","413","422","500","503"):item["responses"][code]={"description":"Structured error","content":{"application/json":{"schema":ref("api","ErrorEnvelope")}}}
 paths.setdefault(path,{})[method]=item
op("/v2/evaluations","post","Evaluate external JSON evidence: local 200; LLM queued 202",("assessment","Request"),("api","EvaluationResultPayload"),responses={"202":{"description":"LLM job accepted; job_id, assessment_id, status, is_replayed"}})
paths["/v2/evaluations"]["post"]["parameters"].append({"name":"Idempotency-Key","in":"header","required":False,"schema":{"type":"string","maxLength":256}})
for path,response in [("capabilities",("api","CapabilitiesResponse")),("rulesets",None),("jobs/{job_id}",("jobs","Job")),("jobs/{job_id}/result",("api","EvaluationResultPayload")),("jobs/{job_id}/audit",None),("evaluations/{id}/evidence",("evidence","EvidenceSnapshot"))]:op("/v2/"+path,"get",path,response=response)
for method,path,req,status in [("put","providers/{profile_id}",("api","ProviderTestRequest"),"201"),("get","providers/{profile_id}",None,"200"),("delete","providers/{profile_id}",None,"204"),("post","providers/test",("api","ProviderTestRequest"),"200"),("post","jobs/{job_id}/cancel",None,"200"),("delete","jobs/{job_id}",None,"204"),("delete","tenant/data",None,"204"),("post","evaluations/{id}/reviews",("api","AddReviewRequest"),"201")]:op("/v2/"+path,method,path,request=req,status=status)
op("/v2/jobs/{job_id}/events","get","Server-sent job events")
ref("assessment","Result")
request=schemas["AssessmentRequest"]
request["properties"]["schema_version"]["const"]="sepilens-evaluation-2.0"
request["properties"]["mode"]["enum"]=["local","ai_assisted"]
request["properties"]["engine_id"]["enum"]=["acmg_amp_2015","svc_v4","cnv_gain_2020","cnv_loss_2020"]
schemas["AssessmentRegion"]["properties"]["coordinate_system"]["const"]="1-based-inclusive"
schemas["APIAddReviewRequest"]["required"]=["revision","decision","reason"]
document={"openapi":"3.1.0","info":{"title":"SepiLens external evidence API","version":"0.3.0-dev","description":"No medical database dependency. Medical approval remains required; SVC v4 is experimental. Local responses are synchronous; LLM jobs are asynchronous."},"security":[{"BearerAuth":[]}],"paths":paths,"components":{"securitySchemes":{"BearerAuth":{"type":"http","scheme":"bearer"}},"schemas":dict(sorted(schemas.items()))}}
output=json.dumps(document,ensure_ascii=False,indent=2)+"\n"
parser=argparse.ArgumentParser();parser.add_argument("--check",action="store_true");args=parser.parse_args();path=ROOT/"docs/openapi.yaml"
if args.check:
 if path.read_text(encoding="utf-8")!=output:raise SystemExit("OpenAPI drift: run python scripts/openapi.py")
 print("OpenAPI has no drift")
else:path.write_text(output,encoding="utf-8");print(f"Wrote {sum(map(len,paths.values()))} operations and {len(schemas)} schemas")
