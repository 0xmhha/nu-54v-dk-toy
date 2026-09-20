"""Validate design coverage and illustrative arithmetic, not market execution."""
import hashlib
import json
from decimal import Decimal
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOC = ROOT / 'content/specifications/market-product-design.json'
read = lambda p: json.loads((ROOT / p).read_text())
d = json.loads(DOC.read_text())
assert d['implementation'] == 'deferred_by_user'
assert d['status'] == 'design_proposal_not_merged'
assert not d['canonicalMerged'] and not d['runtimeVerified']
assert d['policySelectionUnchanged']
for path, digest in d['sourceHashes'].items():
    assert hashlib.sha256((ROOT / path).read_bytes()).hexdigest() == digest, path
wbs = read('content/planning/work-breakdown.json')
pkg = next(p for p in read('content/planning/full-scope-design-review.json')['designPackages'] if p['id'] == d['packageRef'])
assert set(d['taskRefs']) == set(pkg['primaryTaskRefs']) <= {t['id'] for t in wbs['tasks']}
assert d['requirementRefs'] == pkg['requirementRefs']
decision_ids = {x['id'] for x in wbs['decisions']}
assert set(d['decisionRefs']) <= decision_ids
assert wbs['taskCount'] == 104 and wbs['subtaskCount'] == 320
assert len(decision_ids) == 19 and all(x['status'] == 'open' for x in wbs['decisions'])
api_ids = {x['id'] for x in read('content/specifications/api-catalog.json')['operations']}
screen_ids = {x['id'] for x in read('content/specifications/screen-flows.json')['screens']}
md = DOC.with_suffix('.md').read_text()
counts = dict(productCards=4, tradeTransitions=16, priceActionMatrix=10, keeperStages=6, policyInputs=8, contractGaps=6, arithmeticExamples=4, runtimeCases=32)
for name, count in counts.items():
    rows = d[name]
    assert len(rows) == len({x['id'] for x in rows}) == count
    assert all(x['id'] in md for x in rows)
for p in d['productCards']:
    assert set(p['screenRefs']) <= screen_ids and set(p['apiRefs']) <= api_ids
    assert p['completion'] in md and p['unselected'] in md
    assert p['acceptance'] == dict(status='not_run',normalControlRequired=True,evidenceRefs=[])
for gap in d['contractGaps']:
    assert set(gap['apiRefs']) <= api_ids and gap['needed'] in md
for policy in d['policyInputs']:
    assert policy['decisionRef'] in decision_ids and policy['selection'] is None
states = {'draft','quoted','allowance_pending','review_ready','execution_pending','result_observed','completed','expired','unknown','cancelled_local','failed_confirmed'}
for t in d['tradeTransitions']:
    assert set(t['fromState'].split('|')) <= states and t['toState'] in states
    assert t['guard'] in md and t['record'] in md
assert all(t['fromState'] == 'result_observed' for t in d['tradeTransitions'] if t['toState'] == 'completed')
assert any('completed' in t['fromState'].split('|') and t['toState'] == 'unknown' for t in d['tradeTransitions'])
for x in d['arithmeticExamples']:
    if x['kind'] in {'exact_input_min','exact_output_max'}:
        q, b = int(x['quoteAtomic']), x['bps']
        assert q > 0 and 0 <= b < 10000
        result = (q*(10000-b)+9999)//10000 if x['kind']=='exact_input_min' else q*(10000+b)//10000
        assert result == int(x['expectedAtomic'])
    elif x['kind'] == 'fx_units':
        n = int(x['amountInAtomic'])*int(x['rateNum'])*10**x['decimalsOut']
        den = int(x['rateDen'])*10**x['decimalsIn']
        assert den > 0 and n//den == int(x['expectedAtomic'])
    elif x['kind'] == 'linear_pnl_example':
        pnl = Decimal(x['quantity'])*(Decimal(x['exit'])-Decimal(x['entry']))
        equity = Decimal(x['collateral'])+pnl-Decimal(x['fees'])+Decimal(x['fundingCashflow'])
        assert pnl == Decimal(x['expectedPnl']) and equity == Decimal(x['expectedEquity'])
    else:
        raise AssertionError('Unknown illustrative arithmetic kind')
assert all(c['status'] == 'not_run' and c['evidenceRefs'] == [] for c in d['runtimeCases'])
assert len(d['externalReferences']) == 3 and all(r['url'] in md for r in d['externalReferences'])
entry = next(e for e in read('content/specifications/approval-source-dispatch.json')['entries'] if e['sourceKind'] == 'market_action')
assert entry['status'] == 'adapter_pending_execution_blocked' and not entry['executionVerified']
print(json.dumps(dict(scope='design_inventory_and_illustrative_arithmetic_only', **counts, preservedSources=len(d['sourceHashes']), runtimeVerified=False)))
