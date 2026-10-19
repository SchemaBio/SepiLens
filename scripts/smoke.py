"""Exercise a deployed binary with synthetic data; no model/network credentials needed."""
import argparse
import base64
import concurrent.futures
import ctypes
import hashlib
import json
import os
from pathlib import Path
import platform
import secrets
import shutil
import socket
import sqlite3
import statistics
import subprocess
import time
import urllib.error
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument("--binary", required=True)
parser.add_argument("--output", default="tmp/smoke-report.json")
parser.add_argument("--requests", type=int, default=40)
parser.add_argument("--assets", help="Extracted installation directory; defaults to repository assets")
args = parser.parse_args()
root = Path(__file__).resolve().parents[1]
binary = Path(args.binary).resolve()
work = root / "tmp" / ("smoke-" + secrets.token_hex(6))
work.mkdir(parents=True)
with socket.socket() as sock:
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
base = f"http://127.0.0.1:{port}"
key = base64.b64encode(secrets.token_bytes(32)).decode()
env = os.environ.copy()
for name in list(env):
    if name.startswith("SEPILENS_") or name in ("OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_MODEL"):
        env.pop(name)
assets = Path(args.assets).resolve() if args.assets else root
env.update(SEPILENS_MASTER_KEY=key, SEPILENS_DATA_DIR=str(work / "data"), SEPILENS_ADDR=f"127.0.0.1:{port}", SEPILENS_AUTH_MODE="required", SEPILENS_KNOWLEDGE_DIR=str(assets / "knowledge/hpo"), SEPILENS_PRINCIPALS=json.dumps({"submit-test": {"tenant":"A","actor":"cloud","role":"submitter"}, "review-test":{"tenant":"A","actor":"professional","role":"reviewer"}, "admin-test":{"tenant":"A","actor":"ops","role":"admin"}, "other-test":{"tenant":"B","actor":"other","role":"admin"}}))
checks = []
log = (work / "server.log").open("wb")
process = None

def start():
    global process
    process = subprocess.Popen([str(binary), "serve"], cwd=work, env=env, stdout=log, stderr=log, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
    limit = time.monotonic() + 20
    while time.monotonic() < limit:
        if process.poll() is not None:
            raise AssertionError("server exited during startup")
        try:
            if call("GET", "/healthz", token=None)[0] == 200:
                return
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(0.1)
    raise AssertionError("startup timed out")

def stop():
    if process and process.poll() is None:
        process.terminate()
        process.wait(timeout=15)
    log.flush()

def call(method, path, body=None, token="submit-test", headers=None):
    h = dict(headers or {})
    if token:
        h["Authorization"] = "Bearer " + token
    data = None if body is None else json.dumps(body).encode()
    if data is not None:
        h["Content-Type"] = "application/json"
    req = urllib.request.Request(base + path, data=data, headers=h, method=method)
    try:
        response = urllib.request.urlopen(req, timeout=20)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read().decode()
        return response.status, json.loads(raw) if raw and response.headers.get("Content-Type", "").startswith("application/json") else raw

def expect(name, condition):
    if not condition:
        raise AssertionError(name)
    checks.append(name)

body = {"engine_id":"acmg_amp_2015", "variant":{"assembly":"GRCh38","chrom":"1","pos":100,"ref":"A","alt":"G","gene":"STRUCTURED_SENTINEL_TEST"}, "clinical_context":{"disease_id":"MONDO:TEST","inheritance":"AD","variant_origin":"unknown","sample_type":"blood","hpo_terms":[]}, "options":{"retrieval_mode":"off"}}

def wait_result(job_id):
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        code, result = call("GET", "/v1/jobs/" + job_id + "/result")
        if code != 202:
            assert code == 200, "task did not deliver"
            return result
        time.sleep(0.01)
    raise AssertionError("task timeout")

try:
    start()
    expect("auth_required", call("GET", "/v1/capabilities", token=None)[0] == 401)
    code, caps = call("GET", "/v1/capabilities")
    expect("five_disabled_engines", code == 200 and len(caps["engines"]) == 5 and all(not x["enabled"] for x in caps["engines"]))
    code, sources = call("GET", "/v1/annotations/sources")
    expect("no_implicit_annotation_sources", code == 200 and len(sources["sources"]) == 5 and all(not s["enabled"] for s in sources["sources"]))
    annotation = {"engine_id":"acmg_amp_2015", "variant":body["variant"], "annotations":{"source":"sepilens_snapshot", "gnomad":{"version":"synthetic", "status":"found", "faf95":0.00001, "allele_number":100000, "coverage_adequate":True}}}
    code, normalized = call("POST", "/v1/annotations/validate", annotation)
    expect("annotation_source_claim_downgraded", code == 200 and normalized["annotations"]["source"] == "caller_supplied" and normalized["annotations"]["gnomad"]["source"] == "caller_supplied")
    expect("phenotype_requires_locked_graph_and_hpoa", call("POST", "/v1/phenotype/match", {"gene":"TEST","disease_id":"OMIM:1","inheritance":"AD","phenotype":{"hpo_terms":[{"id":"HP:0001945","polarity":"present"}],"source":"caller_supplied"}})[0] == 409)
    expect("ready", call("GET", "/readyz", token=None)[0] == 200)
    clinical = json.loads(json.dumps(body)); clinical["clinical_context"]["variant_origin"] = "germline_confirmed"
    expect("clinical_gate", call("POST", "/v1/variants/snv/evaluations", clinical)[0] == 409)
    clinical["engine_id"] = "svc_v4"
    expect("svc_gate", call("POST", "/v1/variants/snv/evaluations", clinical)[0] == 409)
    draft = json.loads(json.dumps(clinical));draft["options"]["allow_experimental"] = True
    draft["draft_assessment"] = {"moi":"AD", "population":{"faf":"0.00015","daft":"0.00001"}}
    code, svc = call("POST", "/v1/variants/snv/evaluations", draft, headers={"Idempotency-Key":"svc-experimental"})
    expect("svc_experimental_queued", code == 202)
    draft_result = wait_result(svc["job_id"])
    scored = draft_result["agent_proposal"]["experimental_result"]
    expect("svc_exact_boundary_nonclinical", draft_result["result_kind"] == "experimental" and draft_result["rule_engine_result"] is None and not draft_result["settlement_eligible"] and scored["total_points"] == "-6" and not scored["authoritative"] and not scored["inputs_verified"])
    expect("svc_cannot_promote", call("POST", "/v1/evaluations/"+svc["job_id"]+"/reviews", {"revision":1,"decision":"accept_engine_result","reason":"synthetic"}, token="review-test")[0] == 409)
    clinical["engine_id"] = "acmg_amp_2015"; clinical["clinical_context"]["variant_origin"] = "somatic_confirmed"
    expect("somatic_rejected", call("POST", "/v1/variants/snv/evaluations", clinical)[0] == 422)
    raw = "HPO_RAW_SENTINEL_TEST 患者否认癫痫发作。"
    expect("no_implicit_fallback", call("POST", "/v1/hpo/extract", {"text":raw,"options":{}})[0] >= 400)
    code, hpo = call("POST", "/v1/hpo/extract", {"text":raw,"options":{"allow_local_fallback":True}})
    expect("explicit_hpo_span_polarity", code == 200 and hpo["degraded"] and any(x["polarity"] == "absent" and raw[x["span_start"]:x["span_end"]] == x["original_symptom"] for x in (hpo["terms"] or [])))
    code, accepted = call("POST", "/v1/variants/snv/evaluations", body, headers={"Idempotency-Key":"smoke-key"})
    expect("queued", code == 202)
    job_id = accepted["job_id"]
    result = wait_result(job_id)
    expect("evidence_only_no_settlement", result["result_kind"] == "evidence_only" and not result["settlement_eligible"] and result["rule_engine_result"] is None)
    code, replay = call("POST", "/v1/variants/snv/evaluations", body, headers={"Idempotency-Key":"smoke-key"})
    expect("idempotent_replay", code == 200 and replay["job_id"] == job_id and replay["is_replayed"])
    conflict = json.loads(json.dumps(body)); conflict["variant"]["pos"] = 101
    expect("idempotent_conflict", call("POST", "/v1/variants/snv/evaluations", conflict, headers={"Idempotency-Key":"smoke-key"})[0] == 409)
    expect("cross_tenant_not_found", call("GET", "/v1/jobs/" + job_id, token="other-test")[0] == 404)
    code, events = call("GET", "/v1/jobs/" + job_id + "/events")
    expect("sse_completion", code == 200 and "event: completed" in events)
    ids = [int(line[4:]) for line in events.splitlines() if line.startswith("id: ")]
    code, resumed = call("GET", "/v1/jobs/" + job_id + "/events", headers={"Last-Event-ID":str(max(ids))})
    expect("sse_resume", code == 200 and not any(line.startswith("id: ") for line in resumed.splitlines()))
    review = {"revision":1,"decision":"accept_engine_result","reason":"synthetic check"}
    expect("submitter_cannot_review", call("POST", "/v1/evaluations/" + job_id + "/reviews", review)[0] == 403)
    expect("evidence_only_cannot_accept", call("POST", "/v1/evaluations/" + job_id + "/reviews", review, token="review-test")[0] == 409)
    expect("immutable_evidence_available", call("GET", "/v1/evaluations/" + job_id + "/evidence")[0] == 200)
    expect("audit_export", call("GET", "/v1/jobs/" + job_id + "/audit", token="review-test")[0] == 200)
    profile = {"base_url":"https://api.openai.com/v1","api_key":"SYNTHETIC_KEY_NOT_A_VENDOR_SECRET","model":"TEST","protocol":"openai_responses"}
    expect("profile_saved", call("PUT", "/v1/providers/test-profile", profile, token="admin-test")[0] == 201)
    code, public = call("GET", "/v1/providers/test-profile", token="admin-test")
    expect("profile_key_not_readable", code == 200 and "api_key" not in public and "SYNTHETIC_KEY" not in json.dumps(public))
    expect("profile_versioned", call("PUT", "/v1/providers/test-profile", profile, token="admin-test")[1]["version"] == 2)
    expect("profile_deleted", call("DELETE", "/v1/providers/test-profile", token="admin-test")[0] == 204)

    def one(n):
        begin = time.perf_counter()
        code, created = call("POST", "/v1/variants/snv/evaluations", body, headers={"Idempotency-Key":"perf-"+str(n)})
        assert code == 202
        output = wait_result(created["job_id"])
        assert not output["settlement_eligible"]
        return (time.perf_counter() - begin) * 1000
    begin = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        durations = list(pool.map(one, range(args.requests)))
    elapsed = time.perf_counter() - begin
    memory = None
    if os.name == "nt":
        class Counters(ctypes.Structure):
            _fields_ = [("cb",ctypes.c_ulong),("PageFaultCount",ctypes.c_ulong)]+[(name,ctypes.c_size_t) for name in ("PeakWorkingSetSize","WorkingSetSize","QuotaPeakPagedPoolUsage","QuotaPagedPoolUsage","QuotaPeakNonPagedPoolUsage","QuotaNonPagedPoolUsage","PagefileUsage","PeakPagefileUsage")]
        counters = Counters(); counters.cb = ctypes.sizeof(counters)
        if ctypes.windll.psapi.GetProcessMemoryInfo(ctypes.c_void_p(int(process._handle)),ctypes.byref(counters),ctypes.sizeof(counters)):
            memory = counters.PeakWorkingSetSize
    else:
        status = Path(f"/proc/{process.pid}/status")
        if status.exists():
            for line in status.read_text().splitlines():
                if line.startswith("VmHWM:"): memory = int(line.split()[1])*1024
    performance = {"workload":"synthetic evidence_only, retrieval off, no model", "requests":args.requests,"concurrency":4,"workers":2,"seconds":round(elapsed,3),"tasks_per_second":round(args.requests/elapsed,3),"p50_ms":round(statistics.median(durations),3),"p95_ms":round(sorted(durations)[int((len(durations)-1)*.95)],3),"failures":0,"peak_resident_bytes":memory,"cpu":os.environ.get("PROCESSOR_IDENTIFIER",platform.processor()),"logical_cpu_count":os.cpu_count()}
    stop()
    backup = work / "backup"
    shutil.copytree(work / "data", backup)
    # Validate no plaintext sensitive payload survived in database pages or logs.
    pages = (work / "data/sepilens.db").read_bytes()
    expect("encrypted_structured_data", b"STRUCTURED_SENTINEL_TEST" not in pages and b"HPO_RAW_SENTINEL_TEST" not in pages and b"SYNTHETIC_KEY_NOT_A_VENDOR_SECRET" not in pages)
    expect("logs_no_patient_or_keys", b"HPO_RAW_SENTINEL_TEST" not in (work / "server.log").read_bytes() and key.encode() not in (work / "server.log").read_bytes())
    new_key = base64.b64encode(secrets.token_bytes(32)).decode()
    rotated = subprocess.run([str(binary),"rotate-key"],cwd=work,env={**env,"SEPILENS_NEW_MASTER_KEY":new_key},capture_output=True,creationflags=getattr(subprocess,"CREATE_NO_WINDOW",0))
    expect("rotation_transaction", rotated.returncode == 0)
    env["SEPILENS_MASTER_KEY"] = new_key
    start()
    expect("restart_with_new_key", call("GET", "/v1/jobs/"+job_id+"/result")[0] == 200)
    expect("tenant_delete", call("DELETE", "/v1/tenant/data", token="admin-test")[0] == 204)
    expect("deleted_not_readable", call("GET", "/v1/jobs/"+job_id)[0] == 404)
    stop()
    # Restore backup to a different directory, without overwriting the tested database.
    restored = work / "restored"
    shutil.copytree(backup, restored)
    env["SEPILENS_DATA_DIR"] = str(restored); env["SEPILENS_MASTER_KEY"] = key
    start()
    expect("backup_restored_old_key", call("GET", "/v1/jobs/"+job_id+"/result")[0] == 200)
    stop()
    report = {"status":"passed", "checks":checks,"platform":platform.platform(),"python":platform.python_version(),"binary_sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),"performance":performance,"limitations":["No real clinical approval, vendor calls, official full data or hardware power-loss test","Windows terminate is abrupt; graceful shutdown separately checked on Linux","Short synthetic workload does not predict clinical-model latency or accuracy"]}
    output = root / args.output
    output.parent.mkdir(parents=True,exist_ok=True)
    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
    print(f"PASS: {len(checks)} checks; report {output}; {performance['tasks_per_second']} tasks/s (synthetic only)")
finally:
    stop()
    log.close()
