"""Generate UPSTREAM PARITY vectors, never independent medical golden cases.

Run with Python+pydantic: script CHECKOUT OUTPUT. Pure reference scoring; no network.
"""
import importlib
import json
import sys
from dataclasses import asdict
from pathlib import Path
from types import SimpleNamespace as NS

import argparse
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("checkout")
parser.add_argument("output")
args = parser.parse_args()
checkout, output = Path(args.checkout), Path(args.output)
sys.path.insert(0, str(checkout / "src"))
from svcv4_model.case import Case, GeneDiseaseValidity, MOI
from svcv4_model.informative import InformativeVariant, VariantClassification
from svcv4_model.mechanism import ExonRelevance, GenccMechanism
from svcv4_model.population import PopulationEvidence
from svcv4_model.scoring import reference_score_population
from svcv4_model.scoring.hod.clinical import reference_score_cln_proband
from svcv4_model.scoring.hod.locus import reference_score_loc_phe, reference_score_loc_seg

root = Path(__file__).resolve().parents[1]
branches = json.loads((root / "internal/rules/svcv4/branches.json").read_text())
vectors = []
for workflow, table in branches.items():
    module = importlib.import_module("svcv4_model.scoring.pfd." + workflow)
    scorer = getattr(module, "reference_score_" + workflow)
    for branch in table:
        for initial in (None, -8, -1, 0, 0.5, 3, 6, 10):
            for mechanism, exon in (("ESTABLISHED", "ALL"), ("LIKELY", "MOST"), ("SUSPECTED", "MOST"), ("UNCERTAIN", "ALL")):
                for fxn in (None, -8, 4, 8):
                    informative = [InformativeVariant(id="iv-1", classification=VariantClassification.PATHOGENIC), InformativeVariant(id="iv-2", classification=VariantClassification.LIKELY_BENIGN)]
                    assessment = NS(prediction_outcome=branch, parent_code=None,
                        predictive=NS(initial_points=initial),
                        mechanism_exon_relevance=NS(gencc_mechanism=GenccMechanism(mechanism), exon_relevance=ExonRelevance(exon)),
                        fxn_points=fxn, spa_points=-1, informative=NS(variants=informative))
                    expected = scorer(assessment, gene_disease_validity=GeneDiseaseValidity.STRONG)
                    vectors.append({"workflow": workflow, "assessment": {"workflow": workflow, "prediction_outcome": branch, "initial_points": initial,
                        "gencc_mechanism": mechanism, "exon_relevance": exon, "fxn_points": fxn, "spa_points": -1,
                        "informative": [{"id": v.id, "classification": v.classification.value} for v in informative]}, "expected": asdict(expected)})

for moi in MOI:
    for fold in (0, 1.4999, 1.5, 4.9999, 5, 14.9999, 15):
        p = PopulationEvidence(faf=fold/100000, daft=1/100000, hmz_eligible="TRUE", homozygote_count=4, hemizygote_count=3)
        vector = {"workflow": "population", "moi": moi.value, "population": {"faf": str(fold/100000), "daft": "0.00001", "hmz_eligible": "TRUE", "homozygote_count": 4, "hemizygote_count": 3}, "expected": asdict(reference_score_population(p, moi=moi))}
        if fold == 15:
            # Explicit source-logic boundary correction, independent of the Go output:
            # Python binary division yields 14.999999999999998; source if fold < 15
            # else -6 places the exact decimal ratio at -6. Keep both results.
            vector["intentional_difference"] = "exact_decimal_POP_FRQ_15x"
            vector["expected_exact"] = dict(vector["expected"])
            vector["expected_exact"]["sub_code_points"] = dict(vector["expected"]["sub_code_points"], POP_FRQ=-6)
            vector["expected_exact"]["parent_total"] = -6 + vector["expected"]["sub_code_points"]["POP_HMZ"]
        vectors.append(vector)
for moi in MOI:
    for phenotype in ("SPECIFIC", "CONSISTENT", "INCONSISTENT"):
        for zygosity in ("HET", "HOM", "HEMI"):
            for thorough in (False, True):
                case = {"id": "c-1", "family_id": "f-1", "sex": "F", "pheno_specificity_for_mde": phenotype, "vbc_zygosity": zygosity,
                    "age_matched_penetrance": "NEAR_100", "confirmed_parental_relationship": "TRUE", "testing": {"covers_all_genes_relevant_to_mde": "TRUE" if thorough else "UNKNOWN", "non_genetic_etiology_excluded": "TRUE" if thorough else "UNKNOWN"},
                    "compound_het_variant": {"classification": "P", "phase_confidence": "HIGH", "co_occurrence_likelihood": "LT_0_0001"},
                    "relatives": [{"parent_of_proband": "TRUE", "vbc_exists": "FALSE"}, {"parent_of_proband": "TRUE", "vbc_exists": "FALSE"}]}
                vectors.append({"workflow": "clinical", "moi": moi.value, "case": case, "expected": asdict(reference_score_cln_proband(Case.model_validate(case), moi=moi))})

output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps({"kind": "upstream_reference_parity_not_clinical_goldens", "commit": "ef66faff51a265fef7b5c4e6439905f3aa540c46", "vectors": vectors}, indent=2) + "\n")
print(f"{len(vectors)} upstream parity vectors")
