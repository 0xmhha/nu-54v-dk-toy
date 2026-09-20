"""Local design-shape/relationship checks; never executes a reset or verifies crypto."""
import hashlib
import json
from pathlib import Path
from jsonschema import Draft202012Validator, FormatChecker

ROOT=Path(__file__).resolve().parent
load=lambda n:json.loads((ROOT/n).read_text())
meta=load('return-protocol-candidate.json');schema=load(meta['schema']);samples=load('return-protocol-examples.json')
for name,digest in meta['baselineHashes'].items():
    assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==digest, 'Stale source: '+name
Draft202012Validator.check_schema(schema)
def validator(name):return Draft202012Validator(dict(schema,**{'$ref':'#/$defs/'+name}),format_checker=FormatChecker())
for group in ('valid','invalid'):
    for case in samples[group]:
        errors=list(validator(case['schema']).iter_errors(case['value']))
        assert bool(errors)==(group=='invalid'), (case['name'],[e.message for e in errors])

def semantic(kind,value):
    # Finite illustrative relationships only: no permission engine, proof/profile
    # verifier, clock/nonce enforcement, persistence or device execution is here.
    if kind=='CommitRequest':
        return value['binding']==value['prepareReport']['payload']['binding']
    if kind=='ExecutionGrant':
        return value['payload']['audienceDeviceId']==value['payload']['binding']['deviceId']
    if kind in ('EvidenceSubmit','CleanupSubmit'):
        authority=value['authority']['payload']; action='relay_evidence' if kind=='EvidenceSubmit' else 'submit_cleanup_proof'
        return authority['jobId']==value['jobId'] and action in authority['allowedActions']
    if kind=='ResetEvidence':
        return int(value['payload']['nextEpoch'])>int(value['payload']['binding']['previousEpoch'])
    if kind=='CleanupProof':
        v=value['payload']; return v['cleanupResult']!='complete' or v['markerDigest'] is not None
    if kind=='ReuseGateResult' and value['reuseGate']=='eligible':
        return all(value[k] for k in ('serverReturnCompleted','cleanupVerified','currentDeviceEpochMatches')) and value['deviceEnrollmentState']=='unprovisioned_ready'
    if kind=='JobStatus':
        v=value['progress'];return not (v['grantIssued'] and v['phase'] in ('checking','prepared','aborted'))
    if kind=='EvidenceResult':
        if value['status'] in ('verification_pending','quarantined') and value['serverReturnCompleted']:return False
        if not value['serverReturnCompleted'] and (value['completionAck'] is not None or value['reuseGate']!='closed'):return False
        if value['completionAck'] and value['completionAck']['payload']['jobId']!=value['jobId']:return False
    return True
for c in samples['valid']:assert semantic(c['schema'],c['value']),c['name']
for c in samples['semanticCases']:
    assert validator(c['schema']).is_valid(c['value']),c['name']
    assert semantic(c['schema'],c['value'])==c['expectedValid'],c['name']
for c in samples['retryVectors']:
    assert (c['original']==c['redelivered'])==c['expectedSameGrant'],c['name']
api={x['id'] for x in load('api-catalog.json')['operations']};ble={x['name'] for x in load('ble-catalog.json')['commands']}
wbs=json.loads((ROOT.parent/'planning/work-breakdown.json').read_text());tasks={x['id'] for x in wbs['tasks']}
for op in meta['operations']:
    assert set(op['existingApiRefs'])<=api and set(op['existingBleRefs'])<=ble and set(op['taskRefs'])<=tasks
    assert all(n is None or n in schema['$defs'] for n in (op['requestSchema'],op['responseSchema']))
assert len(api)==107 and len(ble)==34
assert meta['lateAssetRecoveryPolicy']=='unresolved' and load('return-recovery-contract.json')['openDecisions'][0]['selection'] is None
assert wbs['phaseControl']['implementation']=='deferred_by_user' and not meta['canonicalMerged']
print(json.dumps({'candidateMessageGroups':len(meta['operations']),'validShapes':len(samples['valid']),'rejectedShapes':len(samples['invalid']),'semanticExamples':len(samples['semanticCases']),'retryVectors':len(samples['retryVectors']),'canonicalMerged':False,'runtimeCryptoAndDeviceTests':'not_run'}))
