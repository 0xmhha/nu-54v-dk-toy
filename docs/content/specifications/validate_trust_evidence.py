"""Finite logical examples only. No keys, accounts, clock or storage are exercised.
The fixture quorum and timestamps are synthetic, not selected product policy.
"""
import copy
import hashlib
import json
from pathlib import Path

P = Path(__file__).resolve().parent
cases = []
def check(name, actual, expected):
    assert actual == expected, (name, actual, expected)
    cases.append(dict(id=name, observed=actual, expected=expected))

def fixture():
    return dict(domain='management-trust', audience='deployment-a', command='recovery.commit',
                subject='body-a', policy='policy-a', request='request-a', case='case-a', generation=1,
                issued=80, nbf=90, exp=200, clock=(100, 102), maxLifetime=150,
                schema=True, proof=True, pop=True, policySelected=True, currentAuthority=True,
                recoveryIndependent=True, requester='controller-a', epoch=4, grant=8, fence=3,
                approvals=[dict(controller='controller-b', key='key-b', subject='body-a', current=True),
                           dict(controller='controller-c', key='key-c', subject='body-a', current=True)],
                quorum=2, trustedClock=True, readAuthority=True, auditDurable=True)

def state():
    return dict(epoch=4, grant=8, fence=3, admission='recovery_prepared', outcome=None,
                reserved=None, consumed=False, commits=0, retired=False, restoreOK=True)

def binding_ok(c):
    return (c['domain'], c['audience'], c['command'], c['case']) == (
        'management-trust', 'deployment-a', 'recovery.commit', 'case-a')

def valid(c, s):
    if not c['schema']: return 'schema_denied'
    if not binding_ok(c):
        return 'binding_denied'
    if not c['policySelected'] or c['policy'] != 'policy-a': return 'policy_hold'
    if not c['proof'] or not c['pop']: return 'proof_denied'
    if not c['recoveryIndependent']: return 'recovery_denied'
    lo, hi = c['clock']
    if (not c['trustedClock'] or lo > hi or not c['issued'] <= c['nbf'] < c['exp'] or
        c['exp'] - c['issued'] > c['maxLifetime'] or lo < c['nbf'] or hi >= c['exp']):
        return 'time_hold'
    if not c['currentAuthority']: return 'authority_denied'
    eligible = [a for a in c['approvals'] if a['current'] and a['subject'] == c['subject']
                and a['controller'] != c['requester']]
    # One independently controlled key per approval in this bounded model.
    # Multiple controllers claiming the same key cannot establish independence.
    bindings = {}
    for a in eligible: bindings.setdefault(a['key'], set()).add(a['controller'])
    controllers = {a['controller'] for a in eligible if len(bindings[a['key']]) == 1}
    if len(controllers) < c['quorum']: return 'approval_hold'
    if (c['epoch'], c['grant'], c['fence']) != (s['epoch'], s['grant'], s['fence']):
        return 'revision_conflict'
    if not c['auditDurable']: return 'audit_hold'
    return 'eligible'

def execute(c, s, response_unknown=False):
    if not binding_ok(c): return 'binding_denied'
    if not s['restoreOK']: return 'restore_hold'
    if s['retired'] or c['generation'] != 1: return 'generation_closed'
    # Identity/digest guards do not return payload; original-result reads use read_original.
    if s['outcome']:
        if s['outcome']['request'] == c['request']:
            return 'query_original' if s['outcome']['subject'] == c['subject'] else 'digest_conflict'
        return 'case_consumed'
    if s['reserved'] and s['reserved'] != (c['request'], c['subject']): return 'case_reserved'
    if s['reserved']: return 'reconcile_original'
    result = valid(c, s)
    if result != 'eligible': return result
    s['reserved'] = (c['request'], c['subject'])
    if response_unknown:
        # We cannot assert whether an external effect happened. Keep fence and reservation.
        s['admission'] = 'restricted'
        return 'unknown'
    s.update(epoch=5, consumed=True, commits=s['commits']+1, admission='recovered_restricted',
             outcome=dict(request=c['request'], subject=c['subject'], payload='committed', epoch=5))
    return 'committed'

def read_original(c, s):
    if not c['readAuthority']: return 'read_denied'
    if not binding_ok(c): return 'binding_denied'
    if not s['restoreOK']: return 'restore_hold'
    if not s['outcome']: return 'unknown' if s['reserved'] else 'not_observed_not_permission'
    if c['request'] != s['outcome']['request']: return 'not_found_in_scope'
    if c['subject'] != s['outcome']['subject']: return 'digest_conflict'
    return 'original_committed' if s['outcome']['payload'] else 'committed_metadata_only'

def retire(s, protected_seal, dependencies_closed):
    if not protected_seal or not dependencies_closed or (s['reserved'] and not s['outcome']):
        return 'retention_hold'
    s.update(retired=True, outcome=None)
    return 'generation_retired'

s = state(); check('valid_recovery', execute(fixture(), s), 'committed')
check('recovery_preserves_restriction', (s['admission'], s['fence']), ('recovered_restricted', 3))
for field, value, expected in [
    ('schema', False, 'schema_denied'), ('domain', 'wallet', 'binding_denied'),
    ('audience', 'deployment-b', 'binding_denied'), ('command', 'trust.resume', 'binding_denied'),
    ('case', 'case-b', 'binding_denied'),
    ('policySelected', False, 'policy_hold'), ('policy', 'other', 'policy_hold'),
    ('proof', False, 'proof_denied'), ('pop', False, 'proof_denied'),
    ('recoveryIndependent', False, 'recovery_denied'), ('currentAuthority', False, 'authority_denied'),
    ('trustedClock', False, 'time_hold'), ('clock', (199, 200), 'time_hold'),
    ('clock', (89, 91), 'time_hold'), ('clock', (103, 102), 'time_hold'),
    ('issued', 95, 'time_hold'), ('exp', 90, 'time_hold'), ('exp', 500, 'time_hold'),
    ('subject', 'altered', 'approval_hold'), ('epoch', 3, 'revision_conflict'),
    ('grant', 7, 'revision_conflict'), ('fence', 2, 'revision_conflict'),
    ('auditDurable', False, 'audit_hold')]:
    c = fixture(); c[field] = value; s = state(); before = copy.deepcopy(s)
    check('deny_'+field+'_'+str(len(cases)), (execute(c, s), s == before), (expected, True))
c = fixture(); c['clock'] = (90, 90)
check('not_before_inclusive', execute(c, state()), 'committed')
c = fixture(); c['clock'] = (199, 199)
check('strictly_before_expiry', execute(c, state()), 'committed')
for name, field, value in [('alias', 'controller', 'controller-b'), ('same_key', 'key', 'key-b'),
                           ('self_approval', 'controller', 'controller-a'), ('revoked', 'current', False)]:
    c = fixture(); c['approvals'][1][field] = value
    check(name, execute(c, state()), 'approval_hold')
c = fixture(); c['approvals'].append(dict(controller='controller-d', key='key-d', subject='other', current=False))
check('invalid_extra_vote_not_counted', execute(c, state()), 'committed')
c = fixture(); s = state(); check('precheck_valid', valid(c, s), 'eligible')
s['grant'] = 9
check('grant_changes_after_precheck', execute(c, s), 'revision_conflict')
c = fixture(); s = state(); valid(c, s); c['clock'] = (200, 201)
check('expires_after_precheck', execute(c, s), 'time_hold')
c = fixture(); s = state(); execute(c, s); c['clock'] = (300, 301); c['currentAuthority'] = False
check('lost_response_no_repeat', (execute(c, s), s['commits']), ('query_original', 1))
before = copy.deepcopy(s)
check('expired_approval_current_reader', read_original(c, s), 'original_committed')
check('read_does_not_resume', s == before, True)
other = copy.deepcopy(c); other['audience'] = 'deployment-b'
check('original_result_other_deployment', read_original(other, s), 'binding_denied')
other = copy.deepcopy(c); other['case'] = 'case-b'
check('original_result_other_case', read_original(other, s), 'binding_denied')
other = copy.deepcopy(c); other['command'] = 'trust.resume'
check('recovery_approval_not_resume', execute(other, s), 'binding_denied')
c['readAuthority'] = False
check('old_approval_not_read_authority', read_original(c, s), 'read_denied')
c['readAuthority'] = True; c['subject'] = 'other'
check('same_identity_different_digest', execute(c, s), 'digest_conflict')
c = fixture(); c['request'] = 'request-b'
check('consumed_case_new_request', execute(c, s), 'case_consumed')
c = fixture(); s['outcome']['payload'] = None
check('pruned_payload_metadata_only', read_original(c, s), 'committed_metadata_only')
check('pruned_payload_no_reexecution', (execute(c, s), s['commits']), ('query_original', 1))
c = fixture(); s = state(); check('external_unknown', execute(c, s, True), 'unknown')
c['clock'] = (400, 401)
check('unknown_expiry_not_release', execute(c, s), 'reconcile_original')
c['request'] = 'request-b'
check('unknown_new_id_cannot_bypass', execute(c, s), 'case_reserved')
check('unknown_prevents_retirement', retire(s, True, True), 'retention_hold')
c = fixture(); s = state(); execute(c, s)
check('tombstone_no_seal', retire(s, False, True), 'retention_hold')
check('tombstone_open_dependencies', retire(s, True, False), 'retention_hold')
check('closed_generation_retirement', retire(s, True, True), 'generation_retired')
check('deleted_outcome_not_fresh', execute(c, s), 'generation_closed')
s = state(); s['restoreOK'] = False
check('old_backup_without_consumption', execute(c, s), 'restore_hold')

report = dict(status='passed', scope='finite_trust_evidence_design_examples', casesPassed=len(cases), cases=cases,
              runtimeVerified=False, productImplementationPerformed=False, canonicalMerged=False,
              fixtureQuorumIsPolicySelection=False, activeTrustChanges=0,
              sourceHashes={'content/specifications/management-trust-evidence.json': hashlib.sha256((P/'management-trust-evidence.json').read_bytes()).hexdigest()},
              limitations=['Synthetic signature, identity, time and checkpoint inputs; no cryptography or live authority verification.',
                           'Serialized single-case model; does not prove distributed commit, concurrent CAS or safe garbage collection.',
                           'Unknown external effects stay unresolved in this model; it cannot manufacture commit evidence.',
                           'No actual retention duration, approval quorum, key suite or public API was selected.'])
(P/'trust-evidence-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['cases','sourceHashes','limitations']}))
