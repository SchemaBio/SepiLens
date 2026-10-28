"""Offline, aggregate-only assessment evaluation. No network or credential access."""
import argparse
from collections import Counter, defaultdict
import hashlib
import json
from pathlib import Path
import re

ENGINES = ('acmg_amp_2015', 'cnv_gain_2020', 'cnv_loss_2020', 'svc_v4')
CLASSES = {'Pathogenic', 'Likely_Pathogenic', 'VUS', 'Likely_Benign', 'Benign'}
SHA = re.compile(r'^[a-f0-9]{64}$')


def require(condition, code):
    if not condition:
        raise ValueError(code)


def exact_keys(value, allowed):
    require(isinstance(value, dict) and set(value).issubset(allowed), 'INVALID_SCHEMA')


def ratio(numerator, denominator):
    return {'numerator': numerator, 'denominator': denominator, 'rate': numerator / denominator if denominator else None}


def validate_dataset(data):
    exact_keys(data, {'version', 'evaluation_condition', 'cases'})
    require(data.get('version') == 'ai-quality-labels-v1', 'UNSUPPORTED_LABEL_VERSION')
    require(data.get('evaluation_condition') in {'engineering_replay', 'held_out_reference', 'annotation_assisted'}, 'MISSING_EVALUATION_CONDITION')
    require(isinstance(data.get('cases'), list) and 0 < len(data['cases']) <= 10000, 'INVALID_CASE_COUNT')
    by_id = {}
    for case in data['cases']:
        exact_keys(case, {'id', 'engine_id', 'input_sha256', 'evidence_sha256', 'label', 'expected_classification', 'clauses'})
        require(isinstance(case.get('id'), str) and 0 < len(case['id']) <= 128 and case['id'] not in by_id, 'DUPLICATE_OR_INVALID_CASE')
        require(case.get('engine_id') in ENGINES, 'INVALID_ENGINE')
        require(all(isinstance(case.get(key), str) and SHA.fullmatch(case[key]) for key in ['input_sha256', 'evidence_sha256']), 'MISSING_INPUT_PIN')
        label = case.get('label')
        exact_keys(label, {'kind', 'source_version', 'object_matched', 'review_stars', 'conflicting', 'review_record_sha256'})
        require(label.get('kind') in {'engineering', 'clinvar_reference', 'independent_review'}, 'INVALID_LABEL_KIND')
        require(isinstance(label.get('source_version'), str) and label['source_version'], 'MISSING_LABEL_VERSION')
        require(type(label.get('object_matched')) is bool and type(label.get('conflicting')) is bool, 'MISSING_LABEL_MATCH_STATE')
        if label['kind'] == 'independent_review':
            require(isinstance(label.get('review_record_sha256'), str) and SHA.fullmatch(label['review_record_sha256']), 'MISSING_REVIEW_RECORD')
        if label['kind'] == 'clinvar_reference':
            require(type(label.get('review_stars')) is int and 0 <= label['review_stars'] <= 4, 'INVALID_REVIEW_STARS')
        require(case.get('expected_classification') is None or case['expected_classification'] in CLASSES, 'INVALID_REFERENCE_CLASS')
        require(isinstance(case.get('clauses'), list) and len(case['clauses']) <= 100, 'INVALID_CLAUSE_LABELS')
        seen = set()
        for clause in case['clauses']:
            exact_keys(clause, {'code', 'allowed', 'evidence_supported', 'critical', 'conflict'})
            require(isinstance(clause.get('code'), str) and clause['code'] and clause['code'] not in seen, 'DUPLICATE_CLAUSE_LABEL')
            require(all(type(clause.get(key)) is bool for key in ['allowed', 'evidence_supported', 'critical', 'conflict']), 'MISSING_CLAUSE_LABEL')
            seen.add(clause['code'])
        # An aggregate ClinVar classification cannot label individual ACMG/CNV/SVC
        # criteria. A professional clause review must be a separate label record.
        require(label['kind'] != 'clinvar_reference' or not case['clauses'], 'CLINVAR_CANNOT_LABEL_CRITERIA')
        by_id[case['id']] = case
    require(len({case['label']['kind'] for case in by_id.values()}) == 1, 'MIXED_LABEL_KINDS')
    return by_id


def evaluate(labels, captures):
    cases = validate_dataset(labels)
    exact_keys(captures, {'version', 'runs'})
    require(captures.get('version') == 'ai-quality-captures-v1' and isinstance(captures.get('runs'), list), 'UNSUPPORTED_CAPTURE_VERSION')
    require(len(captures['runs']) <= 100000, 'TOO_MANY_RUNS')
    groups = defaultdict(list)
    run_ids, model_pins = set(), {}
    for run in captures['runs']:
        exact_keys(run, {'case_id', 'engine_id', 'arm', 'repeat', 'model_config_sha256', 'input_sha256', 'evidence_sha256', 'state', 'classification', 'criteria', 'record_conflict_identified'})
        require(run.get('case_id') in cases, 'UNKNOWN_CASE')
        case = cases[run['case_id']]
        require(run.get('engine_id') == case['engine_id'], 'ENGINE_OBJECT_MISMATCH')
        require(run.get('arm') in {'baseline', 'evidence_chain'} and type(run.get('repeat')) is int and run['repeat'] > 0, 'INVALID_RUN_ID')
        identity = (run['case_id'], run['arm'], run['repeat'])
        require(identity not in run_ids, 'DUPLICATE_RUN')
        run_ids.add(identity)
        require(all(run.get(key) == case[key] for key in ['input_sha256', 'evidence_sha256']), 'UNPINNED_INPUT_OR_EVIDENCE')
        model = run.get('model_config_sha256')
        require(isinstance(model, str) and SHA.fullmatch(model), 'MISSING_MODEL_PIN')
        prior = model_pins.setdefault(case['engine_id'], model)
        require(prior == model, 'MODEL_CONFIGURATION_CHANGED')
        require(run.get('state') in {'completed', 'service_failed', 'timeout', 'cancelled', 'not_run_budget'}, 'INVALID_RUN_STATE')
        require(run.get('classification') is None or run['classification'] in CLASSES | {'insufficient_data'}, 'INVALID_RESULT_CLASS')
        require(type(run.get('record_conflict_identified')) is bool and isinstance(run.get('criteria'), list), 'INVALID_CAPTURE')
        require(run['state'] == 'completed' or not run['criteria'], 'FAILED_RUN_PUBLISHED_CRITERIA')
        require(run['state'] == 'completed' or run['classification'] is None, 'FAILED_RUN_PUBLISHED_CLASSIFICATION')
        require(len(run['criteria']) <= 100, 'TOO_MANY_CRITERIA')
        for criterion in run['criteria']:
            exact_keys(criterion, {'code', 'status', 'verified', 'defaulted', 'conflict_identified', 'foreign_evidence_adopted', 'duplicate_evidence_scored'})
            require(isinstance(criterion.get('code'), str) and criterion['code'], 'INVALID_CRITERION')
            require(criterion.get('status') in {'met', 'not_met', 'not_applicable', 'insufficient_data', 'conflicting'}, 'INVALID_CRITERION_STATUS')
            require(all(type(criterion.get(key)) is bool for key in ['verified', 'defaulted', 'conflict_identified', 'foreign_evidence_adopted', 'duplicate_evidence_scored']), 'MISSING_PIPELINE_AUDIT')
        groups[(case['engine_id'], run['arm'])].append((case, run))
    reports = []
    for engine in ENGINES:
        engine_cases = [case for case in cases.values() if case['engine_id'] == engine]
        for arm in ['baseline', 'evidence_chain']:
            rows = groups[(engine, arm)]
            counts = Counter()
            agreement = Counter()
            kinds = Counter(case['label']['kind'] for case in engine_cases)
            seen_cases = set()
            violations = Counter()
            repeats = defaultdict(Counter)
            for case, run in rows:
                counts[run['state']] += 1
                if run['state'] == 'not_run_budget':
                    continue
                seen_cases.add(case['id'])
                if run['state'] != 'completed':
                    continue
                labels_by_code = {item['code']: item for item in case['clauses']}
                positive = [item for item in run['criteria'] if item['status'] == 'met' and not item['defaulted']]
                positive_codes = {item['code'] for item in positive}
                counts['positive_clauses'] += len(positive)
                for item in positive:
                    code = item['code']
                    truth = labels_by_code.get(code)
                    if truth is None:
                        counts['unlabeled_positive_clauses'] += 1
                    else:
                        counts['labeled_positive_clauses'] += 1
                        counts['false_positive_clauses'] += not truth['allowed']
                        counts['evidence_supported_positive_clauses'] += truth['evidence_supported']
                        counts['unwarranted_verified_clauses'] += item['verified'] and not truth['evidence_supported']
                    violations['foreign_engine_evidence'] += item['foreign_evidence_adopted']
                    violations['duplicate_scoring'] += item['duplicate_evidence_scored']
                    repeats[run['repeat']]['positive'] += 1
                for code, truth in labels_by_code.items():
                    if truth['allowed']:
                        counts['expected_useful_clauses'] += 1
                        counts['useful_clauses_returned'] += code in positive_codes
                    if truth['allowed'] and truth['critical']:
                        counts['critical_clauses'] += 1
                        counts['critical_omissions'] += code not in positive_codes
                    if truth['conflict']:
                        counts['conflicting_clauses'] += 1
                        counts['conflicts_identified'] += any(item['code'] == code and (item['status'] == 'conflicting' or item['conflict_identified']) for item in run['criteria'])
                label = case['label']
                if label['conflicting']:
                    counts['record_conflicts'] += 1
                    counts['record_conflicts_identified'] += run['record_conflict_identified']
                reference = case['expected_classification']
                eligible = reference is not None and label['object_matched'] and not label['conflicting']
                if label['kind'] == 'clinvar_reference':
                    eligible = eligible and label['review_stars'] >= 2
                if eligible:
                    agreement['eligible'] += 1
                    agreement['exact'] += run['classification'] == reference
                    agreement['abstained'] += run['classification'] in {None, 'insufficient_data'}
                    repeats[run['repeat']]['reference_eligible'] += 1
                    repeats[run['repeat']]['reference_agreement'] += run['classification'] == reference
                elif reference is not None:
                    agreement['excluded_unmatched_conflicting_or_low_review'] += 1
            violations['unwarranted_verification'] = counts['unwarranted_verified_clauses']
            reports.append({'engine_id': engine, 'arm': arm, 'dataset_cases': len(engine_cases), 'label_kinds': dict(kinds), 'attempted_unique_cases': len(seen_cases),
                            'dataset_execution_coverage': ratio(len(seen_cases), len(engine_cases)), 'states': {key: counts[key] for key in ['completed', 'service_failed', 'timeout', 'cancelled', 'not_run_budget']},
                            'false_clause_adoption': ratio(counts['false_positive_clauses'], counts['labeled_positive_clauses']),
                            'evidence_support': ratio(counts['evidence_supported_positive_clauses'], counts['labeled_positive_clauses']),
                            'critical_evidence_omission': ratio(counts['critical_omissions'], counts['critical_clauses']),
                            'conflict_identification': ratio(counts['conflicts_identified'], counts['conflicting_clauses']),
                            'useful_suggestion_coverage': ratio(counts['useful_clauses_returned'], counts['expected_useful_clauses']),
                            'record_conflict_identification': ratio(counts['record_conflicts_identified'], counts['record_conflicts']),
                            'reference_classification_agreement': ratio(agreement['exact'], agreement['eligible']), 'reference_abstentions': agreement['abstained'],
                            'reference_exclusions': agreement['excluded_unmatched_conflicting_or_low_review'], 'unlabeled_positive_clauses': counts['unlabeled_positive_clauses'],
                            'hard_safety_violations': dict(violations), 'repeat_counts': {str(key): dict(value) for key, value in sorted(repeats.items())}})
    return {'version': 'ai-quality-report-v1', 'evaluation_condition': labels['evaluation_condition'], 'dataset_sha256': hashlib.sha256(json.dumps(labels, sort_keys=True, separators=(',', ':')).encode()).hexdigest(),
            'model_config_pins': model_pins, 'reports': reports, 'clinical_accuracy_improvement_proven': False,
            'limitations': ['ClinVar reference agreement is not independent clinical case adjudication and cannot label individual criteria.',
                            'Null rates indicate missing eligible labels or observations, not zero error.',
                            'Clause metrics are conditional on completed runs with clause labels; execution coverage and service failures are separate.',
                            'Pipeline verification/default/conflict flags must come from protected server audits, never model self-confirmation.',
                            'No network requests or paid provider calls are made by this evaluator.']}


def read_json(path):
    require(Path(path).stat().st_size <= 64 * 1024 * 1024, 'INPUT_TOO_LARGE')
    return json.loads(Path(path).read_text(encoding='utf-8'))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('labels')
    parser.add_argument('captures')
    parser.add_argument('--output')
    args = parser.parse_args()
    try:
        report = evaluate(read_json(args.labels), read_json(args.captures))
        text = json.dumps(report, ensure_ascii=True, indent=2)
        if args.output:
            Path(args.output).write_text(text + '\n', encoding='utf-8')
        print(text)
    except (ValueError, KeyError, TypeError, OSError):
        # Do not echo paths, clinical values, source IDs or parser fragments.
        raise SystemExit('QUALITY_EVALUATION_INPUT_REJECTED')
