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

Octopus sends this optional wire extension only when its server-side
`AI_ASSISTANCE_EVIDENCE_CHAIN_ENABLED=true` flag is enabled (default false).
Browsers cannot set the internal transport flag. Default payloads therefore remain
compatible with older strict SepiLens decoders during rolling upgrades. Upgrade
the receiver before enabling the transport; the managed Octopus child process
maps this opt-in to `SEPILENS_AI_EVIDENCE_CHAIN_ENABLED=true`. A new standalone
receiver can reconstruct facts from old clients even when no incoming pack exists.
Previously stored request payloads are not rewritten when the flag changes.

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
`four-engine-ai-3.0-claim-bindings` when this path is enabled.

## Claim-level bindings and shared correction

With the feature enabled, every provider criterion includes `evidence_bindings`.
Each entry has four required strings: `fact` (the matching `facts_confirmed` name,
or empty for a whole-criterion assertion), `fact_id`, `source_id`, and `excerpt`.
A binding references a supplied fact ID or an exact excerpt from a supplied
source snippet, never both. Structured criterion facts cannot cross criterion or
engine scope; imported request facts remain context only. Source IDs use the
existing opaque aliases at the provider boundary.

A located excerpt of at least 12 characters is traceable material pending human
review. It is never automatically promoted to clinically verified support by
substring matching. Independently established baseline facts may supply an
explicit `rule_engine` binding without asking the model to repeat an already
known fact. Binding reports identify model versus rule provenance, source title
and dataset version, and fixed reasons for rejection or pending review.

Invented fact IDs, wrong-scope facts, uncited/unknown/rejected sources and
unlocatable excerpts trigger the same single correction counter already shared
by schema validation and output truncation. The existing deadline and tool
history are retained, and correction cannot call tools. Diagnostics contain only
fixed reason codes and array offsets, at most 24 entries; no clinical excerpts,
source identities or provider error text enter those diagnostics.

If structurally valid output still contains invalid bindings after the shared
budget, only affected positive clauses are projected to insufficient data with
null scores/points and cleared asserted facts. Valid other clauses remain.
The advisory classification and summary explicitly mark this degradation; the
existing independent rule reconciliation remains authoritative. Original rejected
outputs, raw final output and binding reports remain in the encrypted audit.
`RunContractWithReview` returns unresolved `ReviewIssues` for this projection;
callers must not treat its raw output as independently validated clinical data.

The YiJian read-only panel exposes these source/excerpt associations, versions,
defaulting and review reasons in four languages. It does not add save actions,
overwrite manual edits, or make old records appear verified. It also rejects
unsafe or credential-bearing citation link URLs.

## Tests and remaining work

The four-engine wire fixture, zero/missing/conflict tests, caller-forgery tests,
engine isolation, existing AI/HPO routes, and feature-on/off synthetic HTTP
contracts verify engineering behavior. The 120 parameterized quality replay cases
are engineering cases, not independently reviewed clinical cases or evidence of
clinical accuracy. Additional shared-budget tests and four-engine synthetic HTTP
cases cover correction, repeated invalid output and selective projection.
Directed retrieval and independently reviewed quality comparisons remain tracked
by the overall plan.

To update the synthetic wire fixture deliberately, run Octopus's
`UPDATE_AI_FACT_CONTRACT=true go test ./internal/assistance -run TestFactPackWireContract`,
copy the resulting fixture to SepiLens `internal/assessment/testdata`, and run
`go test ./internal/assessment -run TestOctopusFactPackWireContract` on the receiver.
These commands do not invoke a model or use clinical records.
