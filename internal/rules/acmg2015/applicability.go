package acmg2015

// RequiredFacts are attestations to supporting material, never inferred from missing annotations.
// They implement the baseline criterion prerequisites; published VCEP refinements need separate approval.
var RequiredFacts = map[CriterionCode][]string{
	CodePVS1: {"null_variant", "lof_disease_mechanism", "transcript_relevant", "nmd_or_functional_impact", "moi_consistent"},
	CodePS1:  {"same_amino_acid_as_established_pathogenic", "splice_impact_excluded"},
	CodePS2:  {"maternity_confirmed", "paternity_confirmed", "de_novo", "phenotype_consistent", "no_family_history"},
	CodePS3:  {"well_established_functional_assay", "damaging_effect", "variant_matched"},
	CodePS4:  {"case_control_enrichment", "statistical_significance", "variant_matched"},
	CodePM1:  {"established_hotspot_or_critical_domain", "benign_variation_excluded"},
	CodePM2:  {"population_coverage_adequate", "absent_or_moi_appropriate_rarity"},
	CodePM3:  {"recessive_disorder", "in_trans_pathogenic_variant"},
	CodePM4:  {"protein_length_changed", "nonrepetitive_or_stop_loss", "splice_impact_assessed"},
	CodePM5:  {"different_missense_same_residue_pathogenic", "splice_impact_excluded"},
	CodePM6:  {"assumed_de_novo", "phenotype_consistent", "no_family_history"},
	CodePP1:  {"segregation_disease", "established_gene_disease", "family_independence"},
	CodePP2:  {"missense_disease_mechanism", "low_benign_missense_rate"},
	CodePP3:  {"computational_support", "predictor_dependence_assessed"},
	CodePP4:  {"highly_specific_phenotype", "single_genetic_etiology"},
	CodePP5:  {"reputable_pathogenic_assertion", "independent_evidence_unavailable"},
	CodeBA1:  {"frequency_above_five_percent", "population_coverage_adequate", "ba1_exceptions_excluded"},
	CodeBS1:  {"frequency_exceeds_disease_expectation", "disease_frequency_model_validated"},
	CodeBS2:  {"healthy_individual", "expected_early_full_penetrance", "moi_consistent"},
	CodeBS3:  {"well_established_functional_assay", "normal_function", "variant_matched"},
	CodeBS4:  {"nonsegregation_disease", "phenocopies_excluded"},
	CodeBP1:  {"missense_variant", "primarily_truncating_disease_mechanism"},
	CodeBP2:  {"phase_confirmed", "pathogenic_partner", "cis_or_trans_benign_context"},
	CodeBP3:  {"inframe_indel", "repetitive_region", "no_known_function"},
	CodeBP4:  {"computational_no_impact", "predictor_dependence_assessed"},
	CodeBP5:  {"alternate_molecular_diagnosis", "phenotype_explained"},
	CodeBP6:  {"reputable_benign_assertion", "independent_evidence_unavailable"},
	CodeBP7:  {"synonymous_variant", "no_splice_impact", "not_highly_conserved"},
}
