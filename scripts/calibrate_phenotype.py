"""Retired: v2 phenotype evidence is supplied inside an evaluation request.

The former calibration used the removed /v1/phenotype/match endpoint and a
local data bundle. Keep historical reports separately; do not use them to
validate the external-evidence API. See docs/YIJIAN_INTEGRATION.md.
"""
raise SystemExit("Retired calibration interface: use versioned external phenotype fixtures and independent cases through /v2/evaluations")
