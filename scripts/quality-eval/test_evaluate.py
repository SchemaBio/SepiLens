import copy
import unittest

from evaluate import evaluate


def fixture(kind='engineering'):
    case = {'id': 'case-001', 'engine_id': 'acmg_amp_2015',
            'input_sha256': 'a' * 64, 'evidence_sha256': 'b' * 64,
            'label': {'kind': kind, 'source_version': 'fixed-v1',
                      'object_matched': True, 'conflicting': False},
            'expected_classification': 'Pathogenic',
            'clauses': [{'code': 'PS1', 'allowed': True, 'evidence_supported': True,
                         'critical': True, 'conflict': False}]}
    if kind == 'clinvar_reference':
        case['label']['review_stars'] = 2
        case['clauses'] = []
    criterion = {'code': 'PS1', 'status': 'met', 'verified': True,
                 'defaulted': False, 'conflict_identified': False,
                 'foreign_evidence_adopted': False, 'duplicate_evidence_scored': False}
    run = {'case_id': case['id'], 'engine_id': case['engine_id'], 'arm': 'baseline',
           'repeat': 1, 'model_config_sha256': 'c' * 64,
           'input_sha256': case['input_sha256'], 'evidence_sha256': case['evidence_sha256'],
           'state': 'completed', 'classification': 'Pathogenic', 'criteria': [criterion],
           'record_conflict_identified': False}
    return {'version': 'ai-quality-labels-v1', 'evaluation_condition': 'held_out_reference' if kind == 'clinvar_reference' else 'engineering_replay', 'cases': [case]}, {'version': 'ai-quality-captures-v1', 'runs': [run]}


class EvaluationTests(unittest.TestCase):
    def report(self, labels, captures):
        return evaluate(labels, captures)['reports'][0]

    def test_supported_positive_and_missing_arm(self):
        labels, captures = fixture()
        report = evaluate(labels, captures)
        self.assertEqual(report['reports'][0]['evidence_support']['rate'], 1)
        self.assertIsNone(report['reports'][1]['evidence_support']['rate'])
        self.assertEqual(report['reports'][1]['dataset_execution_coverage']['rate'], 0)
        self.assertFalse(report['clinical_accuracy_improvement_proven'])

    def test_all_unknown_cannot_hide_omissions(self):
        labels, captures = fixture()
        captures['runs'][0]['criteria'][0]['status'] = 'insufficient_data'
        report = self.report(labels, captures)
        self.assertEqual(report['critical_evidence_omission']['rate'], 1)
        self.assertEqual(report['useful_suggestion_coverage']['rate'], 0)
        self.assertIsNone(report['false_clause_adoption']['rate'])

    def test_clinvar_has_no_clause_truth(self):
        labels, captures = fixture('clinvar_reference')
        report = self.report(labels, captures)
        self.assertEqual(report['reference_classification_agreement']['rate'], 1)
        self.assertIsNone(report['evidence_support']['rate'])
        self.assertEqual(report['unlabeled_positive_clauses'], 1)
        labels['cases'][0]['clauses'] = fixture()[0]['cases'][0]['clauses']
        with self.assertRaisesRegex(ValueError, 'CLINVAR_CANNOT_LABEL_CRITERIA'):
            evaluate(labels, captures)

    def test_reference_exclusions(self):
        for update in [{'review_stars': 1}, {'object_matched': False}, {'conflicting': True}]:
            with self.subTest(update=update):
                labels, captures = fixture('clinvar_reference')
                labels['cases'][0]['label'].update(update)
                report = self.report(labels, captures)
                self.assertIsNone(report['reference_classification_agreement']['rate'])
                self.assertEqual(report['reference_exclusions'], 1)

    def test_changed_model_or_input_rejected(self):
        for field in ['model_config_sha256', 'input_sha256', 'evidence_sha256']:
            labels, captures = fixture()
            second = copy.deepcopy(captures['runs'][0])
            second.update(arm='evidence_chain')
            second[field] = 'd' * 64
            captures['runs'].append(second)
            with self.assertRaises(ValueError):
                evaluate(labels, captures)

    def test_duplicate_run_rejected(self):
        labels, captures = fixture()
        captures['runs'].append(copy.deepcopy(captures['runs'][0]))
        with self.assertRaisesRegex(ValueError, 'DUPLICATE_RUN'):
            evaluate(labels, captures)

    def test_failed_output_rejected_and_budget_skip_visible(self):
        labels, captures = fixture()
        run = captures['runs'][0]
        run['state'] = 'timeout'
        with self.assertRaises(ValueError):
            evaluate(labels, captures)
        run['criteria'] = []
        with self.assertRaisesRegex(ValueError, 'FAILED_RUN_PUBLISHED_CLASSIFICATION'):
            evaluate(labels, captures)
        run['classification'] = None
        report = self.report(labels, captures)
        self.assertEqual(report['states']['timeout'], 1)
        run['state'] = 'not_run_budget'
        report = self.report(labels, captures)
        self.assertEqual(report['dataset_execution_coverage']['rate'], 0)
        self.assertEqual(report['states']['not_run_budget'], 1)

    def test_defaults_do_not_count_as_supported_suggestions(self):
        labels, captures = fixture()
        captures['runs'][0]['criteria'][0]['defaulted'] = True
        self.assertEqual(self.report(labels, captures)['useful_suggestion_coverage']['rate'], 0)

    def test_repeated_criterion_does_not_erase_failed_audit(self):
        labels, captures = fixture()
        first = captures['runs'][0]['criteria'][0]
        first['duplicate_evidence_scored'] = True
        captures['runs'][0]['criteria'].append(dict(first, duplicate_evidence_scored=False))
        self.assertEqual(self.report(labels, captures)['hard_safety_violations']['duplicate_scoring'], 1)

    def test_false_verification_and_wrong_engine_detected(self):
        labels, captures = fixture()
        labels['cases'][0]['clauses'][0].update(allowed=False, evidence_supported=False)
        captures['runs'][0]['criteria'][0]['foreign_evidence_adopted'] = True
        report = self.report(labels, captures)
        self.assertEqual(report['false_clause_adoption']['rate'], 1)
        self.assertEqual(report['hard_safety_violations']['unwarranted_verification'], 1)
        self.assertEqual(report['hard_safety_violations']['foreign_engine_evidence'], 1)

    def test_unreviewed_independent_label_rejected(self):
        labels, captures = fixture('independent_review')
        with self.assertRaisesRegex(ValueError, 'MISSING_REVIEW_RECORD'):
            evaluate(labels, captures)

    def test_patient_fields_rejected(self):
        labels, captures = fixture()
        labels['cases'][0]['patient_name'] = 'must not be exported'
        with self.assertRaisesRegex(ValueError, 'INVALID_SCHEMA'):
            evaluate(labels, captures)

    def test_mixed_label_kinds_rejected(self):
        labels, captures = fixture()
        second = fixture('clinvar_reference')[0]['cases'][0]
        second['id'] = 'second'
        labels['cases'].append(second)
        with self.assertRaisesRegex(ValueError, 'MIXED_LABEL_KINDS'):
            evaluate(labels, captures)

    def test_label_leakage_condition_must_be_recorded(self):
        labels, captures = fixture('clinvar_reference')
        labels.pop('evaluation_condition')
        with self.assertRaisesRegex(ValueError, 'MISSING_EVALUATION_CONDITION'):
            evaluate(labels, captures)


if __name__ == '__main__':
    unittest.main()
