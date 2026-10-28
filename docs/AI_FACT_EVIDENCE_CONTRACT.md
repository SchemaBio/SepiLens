# Versioned AI facts and evidence support

`SEPILENS_AI_EVIDENCE_CHAIN_ENABLED` defaults to `false`. Enabling it adds a
rebuilt fact pack to the existing model input and criterion support metadata to
the existing `agent_proposal`. It does not enable formal rules, add a provider,
increase search/repair budgets, change billing, or authorize SVC clinical use.

## Request compatibility

`evidence_package.ai_context.fact_pack` is optional. Its version is
`imported-facts-v1`, and it contains `engine_id`, `reported_is_not_verified` and
`facts`. Each fact has:

- `field`: stable, engine-scoped semantic field name, not a patient identifier;
- `value`: normalized scalar or allowlisted HPO ID list; unknown values are null;
- `source`: `request` for imported data;
- `path`: original request JSON pointer, empty for an unavailable prerequisite;
- `version`: `unversioned` when a dataset version is not independently available;
- `status`: `reported`, `unknown`, `conflicting`, or `not_applicable`.

Octopus builds the pack after fixing the evaluation object and coordinate
convention. It preserves explicit zero, age zero, per-role GT/AD/VAF and available
DP/GQ. Missing parental calls stay unknown. A trio label or phased-looking GT
does not establish confirmed identity, de novo origin, phase or segregation.
Loss requests do not obtain TS prerequisites; Gain requests do not obtain HI
prerequisites. Continuous copy-number estimates remain distinct from absolute
integer copy numbers. Empty/invalid values do not become zero or negative evidence.

SepiLens reconstructs all fields from the normalized request rather than trusting
the received pack. `input_status` in the rebuilt pack is `absent`, `matched`,
`mismatched`, or `unsupported_version`. Unknown pack versions remain compatible
context, not medical evidence. Caller-written `verified` status and provenance
claims are ignored. The exact four-engine Octopus payload fixture is checked
again after SepiLens normalization; protocol updates require both fixtures to
change together.

## Independent support

The model also receives `established_facts` from the deterministic baseline only
when the engine code is valid, its criterion is met, and all supporting sources
have valid identifiers, existing records, object matches, verified material and
verified source status. Source IDs are replaced by the existing opaque aliases
before provider calls. Imported facts cannot establish these conditions.

`agent_proposal.evidence_quality` uses version `evidence-quality-v1`. It records
per-criterion source existence/object/material support, independently established
facts, applicability, and supported/pending/insufficient/conflicting states.
Support requires the same baseline scores, strengths, sources and facts;
duplicate proposal clauses cannot claim support. These audit labels do not replace
the existing rule reconciliation or permit model-written rule facts. The model
proposal, rule result and reviewer decision remain separate.

The reconstructed pack and source alias map are retained in the existing encrypted
agent audit, under its existing task retention lifecycle. Ordinary logs and timing
events do not contain fact values. Public quality summaries report the pack's
schema/coherence state, not patient context. Prompt metadata uses
`four-engine-ai-2.9-evidence-facts` when this path is enabled.

## Tests and remaining work

The four-engine wire fixture, zero/missing/conflict tests, caller-forgery tests,
engine isolation, existing AI/HPO routes, and feature-on/off synthetic HTTP
contracts verify engineering behavior. The 120 parameterized quality replay cases
are engineering cases, not independently reviewed clinical cases or evidence of
clinical accuracy. Claim-specific excerpt/structured-record bindings, directed
retrieval and reviewed quality comparisons remain tracked by the overall plan.

To update the synthetic wire fixture deliberately, run Octopus's
`UPDATE_AI_FACT_CONTRACT=true go test ./internal/assistance -run TestFactPackWireContract`,
copy the resulting fixture to SepiLens `internal/assessment/testdata`, and run
`go test ./internal/assessment -run TestOctopusFactPackWireContract` on the receiver.
These commands do not invoke a model or use clinical records.
