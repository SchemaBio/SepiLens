"""Emit JSON (a YAML subset) OpenAPI without third-party generator dependencies."""
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCHEMAS = {}

def ref(name):
    return {"$ref": "#/components/schemas/" + name}

def field_type(t):
    t = t.lstrip("*")
    if t == "Point": return {"type": ["string", "null"], "pattern": r"^-?[0-9]+(\.[0-9]{1,6})?$", "description": "Exact decimal points, millionth precision"}
    if t.startswith("[]"):
        return {"type": ["array", "null"], "items": field_type(t[2:])}
    if t.startswith("map[string]"):
        return {"type": "object", "additionalProperties": field_type(t[11:])}
    if t.split(".")[-1] in ("string", "ErrorCode", "EngineID", "Status", "JobStatus", "SourceType", "SourceValidation", "EvidenceStatus", "Confidence", "Polarity", "Protocol", "CriterionCode", "Strength", "Direction", "CriterionStatus"):
        return {"type": "string"}
    if t == "bool":
        return {"type": "boolean"}
    if t in ("int", "int64"):
        return {"type": "integer", "format": "int64"}
    if t == "float64":
        return {"type": "number"}
    if t == "time.Time":
        return {"type": "string", "format": "date-time"}
    if t in ("any", "interface{}", "json.RawMessage"):
        return {}
    return ref(t.split(".")[-1])

# Sources are parsed only for public request/response structs. New packages are added here once their
# types are exposed through an API handler; parsing a package with no exposed struct is harmless.
SOURCE_FILES = ("internal/rules/svcv4/model.go", "internal/api/admission.go", "internal/api/annotation_handler.go", "internal/api/jobs_handler.go", "internal/api/errors.go", "internal/api/server.go", "internal/api/provider_test_handler.go", "internal/evidence/model.go", "internal/hpo/model.go", "internal/jobs/store.go", "internal/engine/registry.go", "internal/rules/acmg2015/evaluator.go", "internal/rules/acmg2015/criteria.go", "internal/rules/cnvrubric/evaluate.go", "internal/api/phenotype_handler.go", "internal/phenotype/model.go", "internal/phenotype/pp4.go", "internal/api/evaluate.go", "internal/rules/acmg2015/derive.go", "internal/rules/acmg2015/points.go", "internal/agent/override.go")

for rel in SOURCE_FILES:
    path = ROOT / rel
    if not path.exists():
        continue
    source = path.read_text(encoding="utf-8")
    for name, body in re.findall(r"type (\w+) struct \{(.*?)\n\}", source, re.S):
        properties, required = {}, []
        for go_name, t, tag in re.findall(r'\s+(\w+)\s+([\w.\[\]*{}]+)\s+`json:"([^\"]+)"`', body):
            key = tag.split(",")[0]
            if key == "-":
                continue
            properties[key] = field_type(t)
            if "omitempty" not in tag:
                required.append(key)
        if properties:
            SCHEMAS[name] = {"type": "object", "properties": properties, "additionalProperties": False}
            if required:
                SCHEMAS[name]["required"] = required

SCHEMAS["EvaluationOptions"]["properties"]["retrieval_mode"]["enum"] = ["off", "targeted", "deep"]
SCHEMAS["CNVRegion"]["properties"]["coordinate_system"]["enum"] = ["1-based-inclusive", "0-based-half-open"]
SCHEMAS["CNVRegion"]["properties"]["type"]["enum"] = ["loss", "gain"]
SCHEMAS["SNVVariant"]["properties"]["allele_frequency"].update(minimum=0, maximum=1)
for name in ("SNVContext", "CNVContext"):
    SCHEMAS[name]["properties"]["variant_origin"]["enum"] = ["germline_confirmed", "unknown"]
    SCHEMAS[name]["properties"]["sample_type"]["enum"] = ["blood", "saliva", "buccal", "fibroblast", "tissue", "other", "unknown"]

# Annotation records are defined in internal/annotation and referenced by the request.
for rel in ("internal/annotation/model.go",):
    source = (ROOT / rel).read_text(encoding="utf-8")
    for name, body in re.findall(r"type (\w+) struct \{(.*?)\n\}", source, re.S):
        properties, required = {}, []
        for go_name, t, tag in re.findall(r'\s+(\w+)\s+([\w.\[\]*{}]+)\s+`json:"([^\"]+)"`', body):
            key = tag.split(",")[0]
            if key == "-":
                continue
            if t.endswith("Record"):
                properties[key] = {"type": "object", "additionalProperties": True, "description": "Provenance envelope: source, version, assembly, retrieved_at, status, reason."}
                continue
            if t.startswith("[]"):
                properties[key] = {"type": "array", "items": field_type(t[2:])}
                continue
            if t.startswith("*"):
                inner = field_type(t[1:])
                inner = dict(inner)
                inner["type"] = [inner.get("type", "string"), "null"] if isinstance(inner.get("type"), str) else inner.get("type", "string")
                properties[key] = inner
                continue
            properties[key] = field_type(t)
            if "omitempty" not in tag:
                required.append(key)
        if properties:
            SCHEMAS[name] = {"type": "object", "properties": properties, "additionalProperties": False}
            if required:
                SCHEMAS[name]["required"] = required
PROVENANCE = {
    "source": {"type": "string", "enum": ["caller_supplied", "sepilens_snapshot", "sepilens_api"]},
    "version": {"type": "string", "description": "Locked data source version; required."},
    "assembly": {"type": "string", "enum": ["GRCh37", "GRCh38"], "description": "Must match variant assembly exactly."},
    "retrieved_at": {"type": "string", "format": "date-time"},
    "status": {"type": "string", "enum": ["found", "not_found", "conflicting", "unavailable", "stale", "caller_supplied"]},
    "reason": {"type": "string"},
    "record_hash": {"type":"string"},
    "query": {"type":"object", "additionalProperties":True, "description":"Exact snapshot query; caller-provided provenance is never trusted."},
}
for name in ("ClinVar", "GnomAD", "AlphaMissense", "Pangolin", "GenCC"):
    if name in SCHEMAS:
        properties = dict(PROVENANCE)
        properties.update(SCHEMAS[name]["properties"])
        SCHEMAS[name]["properties"] = properties
        SCHEMAS[name]["required"] = ["version", "status"]
        SCHEMAS[name]["additionalProperties"] = False
SCHEMAS["Set"] = {
    "type": "object",
    "properties": {
        "source": {"type": "string", "enum": ["caller_supplied", "sepilens_snapshot", "sepilens_api"]},
        "clinvar": {"anyOf": [ref("ClinVar"), {"type": "null"}]},
        "gnomad": {"anyOf": [ref("GnomAD"), {"type": "null"}]},
        "alphamissense": {"anyOf": [ref("AlphaMissense"), {"type": "null"}]},
        "pangolin": {"anyOf": [ref("Pangolin"), {"type": "null"}]},
        "gencc": {"anyOf": [ref("GenCC"), {"type": "null"}]},
    },
    "additionalProperties": False,
    "description": "Normalized annotations. Missing or unavailable sources never count as benign evidence.",
}
SCHEMAS["ProviderTestRequest"]["properties"]["protocol"]["enum"] = ["openai_responses", "openai_chat"]
SCHEMAS["ProviderTestRequest"]["required"] = ["base_url", "api_key", "model", "protocol"]
SCHEMAS["AddReviewRequest"]["required"] = ["revision", "decision", "reason"]
SCHEMAS["AddReviewRequest"]["properties"]["decision"]["enum"] = ["accept_engine_result", "reject", "override", "revise_evidence"]
SCHEMAS["JobAccepted"] = {"type": "object", "properties": {"job_id": {"type": "string"}, "status": {"type": "string"}, "result_kind": {"type": "string"}, "is_replayed": {"type": "boolean"}}, "required": ["job_id", "status", "result_kind", "is_replayed"]}
SCHEMAS["ProfilePublic"] = {"type": "object", "properties": {"id": {"type": "string"}, "version": {"type": "integer"}, "model": {"type": "string"}, "protocol": {"type": "string"}, "base_url": {"type": "string", "format": "uri"}}, "required": ["id", "version", "model", "protocol", "base_url"], "additionalProperties": False}
SCHEMAS["AuditExport"] = {"type": "object", "description": "Decrypted tenant-scoped export; contains sensitive structured clinical data. Includes job, reviews, current snapshot, result_history and evidence_history.", "additionalProperties": True}
SCHEMAS["CNVRuleResult"] = SCHEMAS.pop("Result")
SCHEMAS["RuleEngineResult"] = {"oneOf": [ref("EvaluationOutput"), ref("CNVRuleResult"), {"type":"null"}], "description": "Null in evidence_only/simulation/experimental. Only fully verified applicable evidence contributes. CNV score is fixed-point hundredths."}
SCHEMAS["AgentProposal"] = {"type":"object","properties":{"experimental_result":ref("DraftResult"),"evidence":field_type("[]EvidenceItem"),"missing_information":field_type("[]string"),"model":{"type":"object","properties":{"summary":{"type":"string"},"missing_information":field_type("[]string")},"additionalProperties":False}},"additionalProperties":True,"description":"Untrusted suggestions. Legacy unverified data may be wrapped here; it never substitutes for rule_engine_result."}
SCHEMAS["EvaluationResultPayload"]["properties"]["agent_proposal"] = ref("AgentProposal")
SCHEMAS["ProviderTestRequest"]["properties"]["api_key"]["writeOnly"] = True
SCHEMAS["EvaluationResultPayload"]["properties"]["rule_engine_result"] = ref("RuleEngineResult")
SCHEMAS["EvaluationResultPayload"]["properties"]["reviewer_decision"] = {"anyOf": [ref("ReviewDecision"), {"type": "null"}]}
SCHEMAS["EvidenceSnapshot"]["properties"].update(SCHEMAS["SnapshotExtensions"]["properties"])
SCHEMAS["DerivedResult"]["properties"].update(SCHEMAS["EvaluationOutput"]["properties"])
SCHEMAS["RuleEngineResult"] = {"anyOf": [ref("EvaluationOutput"), ref("DerivedResult"), ref("CNVRuleResult"), {"type":"null"}], "description":"Deterministic classification; null outside approved production. AI classifications are ignored."}
SCHEMAS["EvaluationResultPayload"]["properties"]["local_baseline"] = ref("DerivedResult")
SCHEMAS["EvaluationOptions"]["properties"]["annotation_source"]["enum"] = ["caller_supplied", "sepilens_snapshot"]
SCHEMAS["AgentProposal"]["properties"].update({"ai_assessment":ref("Assessment"),"reconciliation":field_type("[]Reconciliation"),"major_override":{"type":"boolean"}})

PATHS = {}
def op(path, method, summary, request=None, response=None, status="200", extra=None, public=False):
    operation = {"summary": summary, "operationId": method + re.sub(r"[^a-zA-Z0-9]", "_", path), "responses": {status: {"description": "Success"}}}
    for code in ("400", "401", "403", "404", "409", "422", "500", "503"):
        operation["responses"][code] = {"description": "Structured error; 409 includes disabled rules or idempotency/review conflicts", "content": {"application/json": {"schema": ref("ErrorEnvelope")}}}
    if public:
        operation["security"] = []
    if response:
        operation["responses"][status]["content"] = {"application/json": {"schema": ref(response)}}
    if request:
        operation["requestBody"] = {"required": True, "content": {"application/json": {"schema": ref(request)}}}
    params = [{"name": name, "in": "path", "required": True, "schema": {"type": "string"}} for name in re.findall(r"\{(\w+)\}", path)]
    if extra:
        params += extra
    if params:
        operation["parameters"] = params
    PATHS.setdefault(path, {})[method] = operation
    return operation

for path in ("/healthz", "/readyz"):
    op(path, "get", "Health/readiness probe", public=True)
op("/v1/capabilities", "get", "Fixed five engine statuses and implemented protocols", response="CapabilitiesResponse")
op("/v1/rulesets", "get", "Rule versions and approval state")
op("/v1/tenant/data", "delete", "Admin: permanently delete current tenant clinical data and profiles")
op("/v1/providers/test", "post", "Admin: paid real connectivity/schema/stream probe; tri-state capabilities", "ProviderTestRequest")
for method in ("get", "put", "delete"):
    op("/v1/providers/{profile_id}", method, "Admin: tenant profile; never returns API key", "ProviderTestRequest" if method == "put" else None, "ProfilePublic" if method != "delete" else None, "201" if method == "put" else "204" if method == "delete" else "200")
hpo = op("/v1/hpo/extract", "post", "Transient HPO extraction; explicit local fallback only; no raw text persistence", "HPOExtractRequest", "ExtractionResult")
hpo["responses"]["202"] = {"description": "Simulation queued", "content": {"application/json": {"schema": ref("JobAccepted")}}}
op("/v1/annotations/validate", "post", "Normalize and validate caller-supplied annotations without classifying", "AnnotationRequest")
op("/v1/annotations/sources", "get", "Available annotation sources and their settlement status", response="AnnotationSourcesResponse")
op("/v1/phenotype/match", "post", "Versioned phenotype matching and PP4 gates; never a variant classification", "PhenotypeMatchRequest", "PhenotypeAssessment")
for name, request in (("snv", "SNVEvaluationRequest"), ("cnv", "CNVEvaluationRequest")):
    item = op(f"/v1/variants/{name}/evaluations", "post", "Persist and queue; approved rules required for germline production; unknown origin evidence_only", request, "JobAccepted", "202", [{"name": "Idempotency-Key", "in": "header", "schema": {"type": "string", "maxLength": 256}, "description": "Unique per tenant. Same pinned normalized request replays; conflicting request returns 409."}])
    item["responses"]["200"] = {"description": "Idempotent replay; no new external invocation", "content": {"application/json": {"schema": ref("JobAccepted")}}}
op("/v1/jobs/{job_id}", "get", "Tenant-scoped job status", response="Job")
events = op("/v1/jobs/{job_id}/events", "get", "Persistent SSE; 15s heartbeat and terminal close", extra=[{"name": "Last-Event-ID", "in": "header", "schema": {"type": "integer", "minimum": 0}}])
events["responses"]["200"]["content"] = {"text/event-stream": {"schema": {"type": "string", "description": "id: integer; event: stage; data: JSON. Replay strictly after Last-Event-ID; heartbeat comments."}}}
result = op("/v1/jobs/{job_id}/result", "get", "Three-layer versioned result; reads are free", response="EvaluationResultPayload")
result["responses"]["202"] = {"description": "Still queued/running"}
result["responses"]["410"] = {"description": "Cancelled", "content": {"application/json": {"schema": ref("ErrorEnvelope")}}}
op("/v1/jobs/{job_id}/cancel", "post", "Cancel queued/running; completion cannot overwrite cancellation")
op("/v1/jobs/{job_id}", "delete", "Delete job and associated clinical records")
op("/v1/jobs/{job_id}/audit", "get", "Export encrypted-at-rest history after tenant authorization", response="AuditExport")
op("/v1/evaluations/{id}/reviews", "post", "Reviewer/admin: accept, reject, override or revise evidence at matching revision", "AddReviewRequest", status="201")
op("/v1/evaluations/{id}/evidence", "get", "Latest immutable snapshot; hash/authentication checked", response="EvidenceSnapshot")

# Remove generator-only structures whose dependencies are intentionally private.
used = set()
def walk(value):
    if isinstance(value, dict):
        if "$ref" in value:
            name = value["$ref"].split("/")[-1]
            if name not in used:
                used.add(name)
                if name not in SCHEMAS:
                    raise ValueError("Missing public schema: " + name)
                walk(SCHEMAS[name])
        for k, v in value.items():
            if k != "$ref":
                walk(v)
    elif isinstance(value, list):
        for v in value:
            walk(v)
walk(PATHS)
document = {"openapi": "3.1.0", "info": {"title": "SepiLens API", "version": "0.2.0-rc2", "description": "Engineering candidate. All medical engines default disabled; SVC v4 is available only through explicit experimental opt-in, never for clinical use. Clinical approval and live vendor verification are separate gates."}, "servers": [{"url": "http://127.0.0.1:8080"}], "security": [{"BearerAuth": []}], "paths": PATHS, "components": {"securitySchemes": {"BearerAuth": {"type": "http", "scheme": "bearer", "description": "Binds tenant, actor and role; cannot be overridden in request body."}}, "schemas": {k: SCHEMAS[k] for k in sorted(used)}}}
(ROOT / "docs/openapi.yaml").write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(f"Wrote {sum(len(v) for v in PATHS.values())} operations and {len(used)} schemas")
