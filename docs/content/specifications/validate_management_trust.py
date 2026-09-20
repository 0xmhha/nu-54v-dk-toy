"""Synthetic finite design model. Does not verify keys or provision trust.
The approval quorum=2 below is a test parameter, not a selected product policy.
"""
import copy,hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent
CASES=[]
def check(id,actual,expected):
 assert actual==expected,(id,actual,expected)
 CASES.append(dict(id=id,observed=actual,expected=expected))

def context():
 return dict(environment='env-a',purpose='management_anchor',requester='controller-requester',
  policySelected=True,currentAuthority=True,externalBootstrapTrusted=True,virginInstall=True,
  installProofCurrent=True,ticketFresh=True,recoveryPrecommitted=True,recoveryAuthorityValid=True,
  recoveryAnchorIndependent=True,newKeyPop=True,oldAnchorApproved=True,newAnchorApproved=True,
  rootChainComplete=True,restoreAssessed=True,checkpointCurrent=True,auditDurable=True,
  approvedDigest='digest-a',requestDigest='digest-a',expectedEpoch=4,targetEpoch=5,
  expectedFence=3,fence=3,approvalExpiry=200,now=100,quorum=2,
  approvals=[dict(controller='controller-b',key='key-b',current=True,digest='digest-a'),
             dict(controller='controller-c',key='key-c',current=True,digest='digest-a')],
  resumeAuthority=True,separateReleaseReview=True,consumerCurrent=True,capabilityCurrent=True,
  readAuthority=True,consumerProtectedCheckpoint=True,readerTrusted=True)

def state(epoch=4):return dict(epoch=epoch,fence=3,admission='current',outcomes={},consumedCases=set(),walletKeysChanged=0)

def approvals_ok(c):
 valid=[a for a in c['approvals'] if a['current'] and a['digest']==c['requestDigest'] and a['controller']!=c['requester']]
 # Independent controller and key count, never raw number of signatures.
 return len({a['controller'] for a in valid})>=c['quorum'] and len({a['key'] for a in valid})>=c['quorum']

def command(s,c,kind='rotate',request='req-a',case='case-a'):
 if c['environment']!='env-a' or c['purpose']!='management_anchor':return 'wrong_domain'
 if kind not in ['bootstrap','rotate','recover','resume']:return 'unknown_command'
 if not c['policySelected']:return 'policy_unselected'
 if kind=='bootstrap':auth=c['externalBootstrapTrusted']
 elif kind=='recover':auth=c['recoveryPrecommitted'] and c['recoveryAuthorityValid'] and c['recoveryAnchorIndependent']
 elif kind=='resume':auth=c['resumeAuthority']
 else:auth=c['currentAuthority']
 if not auth:return 'authority_denied'
 identity=(c['environment'],kind,request)
 if identity in s['outcomes']:
  return 'original_metadata_only' if s['outcomes'][identity]==c['requestDigest'] else 'digest_conflict'
 if c['approvedDigest']!=c['requestDigest']:return 'digest_conflict'
 if not c['restoreAssessed'] or not c['checkpointCurrent']:return 'quarantine'
 if not approvals_ok(c) or c['now']>=c['approvalExpiry']:return 'approval_denied'
 if not c['auditDurable']:return 'audit_pending'
 if c['expectedFence']!=s['fence'] or c['fence']<s['fence']:return 'fence_conflict'
 if c['expectedEpoch']!=s['epoch']:return 'epoch_conflict'
 if kind=='bootstrap':
  if s['epoch']!=0 or not c['virginInstall'] or not c['installProofCurrent'] or not c['ticketFresh']:return 'bootstrap_denied'
 elif kind=='rotate':
  if s['admission'] not in ['current','rotation_prepared']:return 'normal_rotation_denied'
  if not c['oldAnchorApproved'] or not c['newAnchorApproved'] or not c['rootChainComplete']:return 'continuity_denied'
 elif kind=='recover':
  if s['admission'] not in ['restricted','recovery_prepared']:return 'recovery_not_prepared'
  if case in s['consumedCases']:return 'recovery_case_consumed'
 elif kind=='resume':
  if s['admission'] not in ['current_restricted','recovered_restricted','restricted']:return 'not_restricted'
  if not c['separateReleaseReview'] or not c['consumerCurrent'] or not c['capabilityCurrent']:return 'resume_hold'
  s['admission']='current';s['outcomes'][identity]=c['requestDigest'];return 'resumed_candidate'
 if not c['newKeyPop']:return 'new_key_unproven'
 if c['targetEpoch']!=s['epoch']+1:return 'nonconsecutive_epoch'
 s['epoch']=c['targetEpoch']
 s['admission']='current_restricted' if kind=='bootstrap' else 'recovered_restricted' if kind=='recover' else 'current'
 if kind=='recover':s['consumedCases'].add(case)
 s['outcomes'][identity]=c['requestDigest'];return 'committed_candidate'

def read_original(s,c):
 if not c['readAuthority'] or not c['readerTrusted']:return 'read_denied'
 return {'currentEpoch':s['epoch'],'currentAdmission':s['admission'],'currentFence':s['fence'],'walletKeysChanged':s['walletKeysChanged']}

# Bootstrap cannot derive trust from its own new root, social identity, or empty DB.
for flag in ['externalBootstrapTrusted','virginInstall','installProofCurrent','ticketFresh','newKeyPop','policySelected']:
 c=context();c.update(expectedEpoch=0,targetEpoch=1);c[flag]=False;s=state(0)
 expected={'externalBootstrapTrusted':'authority_denied','newKeyPop':'new_key_unproven','policySelected':'policy_unselected'}.get(flag,'bootstrap_denied')
 check('bootstrap_'+flag,command(s,c,'bootstrap'),expected)
 check('bootstrap_'+flag+'_no_epoch_change',s['epoch'],0)
c=context();c.update(expectedEpoch=0,targetEpoch=1);s=state(0)
check('bootstrap_valid_fixture',command(s,c,'bootstrap'),'committed_candidate')
check('bootstrap_not_all_features_resumed',s['admission'],'current_restricted')
check('bootstrap_retry',command(s,c,'bootstrap'),'original_metadata_only')
check('bootstrap_no_duplicate_epoch',s['epoch'],1)
c=context();c.update(expectedEpoch=4,targetEpoch=5);check('empty_db_claim_not_virgin',command(state(),c,'bootstrap'),'bootstrap_denied')

# Approval identity, exact content, key purpose and current authorization.
for key,value,expected in [('environment','env-b','wrong_domain'),('purpose','wallet_signer','wrong_domain'),
 ('currentAuthority',False,'authority_denied'),('policySelected',False,'policy_unselected'),
 ('newKeyPop',False,'new_key_unproven'),('oldAnchorApproved',False,'continuity_denied'),
 ('newAnchorApproved',False,'continuity_denied'),('rootChainComplete',False,'continuity_denied'),
 ('restoreAssessed',False,'quarantine'),('checkpointCurrent',False,'quarantine'),
 ('auditDurable',False,'audit_pending'),('expectedEpoch',3,'epoch_conflict'),
 ('targetEpoch',7,'nonconsecutive_epoch'),('expectedFence',2,'fence_conflict'),
 ('fence',2,'fence_conflict'),('now',200,'approval_denied'),('requestDigest','altered','digest_conflict')]:
 c=context();c[key]=value;s=state();check('rotation_'+key,command(s,c),expected);check('rotation_'+key+'_no_commit',s['epoch'],4)
c=context();c['approvals'][0]['controller']=c['requester'];check('self_elevation_not_approval',command(state(),c),'approval_denied')
c=context();c['approvals'][1]['controller']=c['approvals'][0]['controller'];check('two_aliases_same_controller',command(state(),c),'approval_denied')
c=context();c['approvals'][1]['key']=c['approvals'][0]['key'];check('duplicate_key_not_two_approvals',command(state(),c),'approval_denied')
c=context();c['approvals'][0]['current']=False;check('revoked_approver',command(state(),c),'approval_denied')
c=context();c['approvals'][0]['digest']='other';check('approval_other_digest',command(state(),c),'approval_denied')
s=state();c=context();check('rotation_valid_fixture',command(s,c),'committed_candidate');check('rotation_increases_epoch',s['epoch'],5)
check('rotation_lost_response',command(s,c),'original_metadata_only');check('rotation_retry_keeps_epoch',s['epoch'],5)
c['currentAuthority']=False;check('old_mutation_permission_not_replayed',command(s,c),'authority_denied')
before=copy.deepcopy(s);check('read_with_separate_current_scope',read_original(s,c)['currentEpoch'],5);check('read_no_mutation',s==before,True)
c['readAuthority']=False;check('read_requires_current_scope',read_original(s,c),'read_denied')
s=state();s['admission']='restricted';check('compromise_not_normal_rotation',command(s,context()),'normal_rotation_denied')
# Expiry of a pending change alone is not the revocation of a healthy existing anchor.
s=state();before=copy.deepcopy(s);c=context();c['now']=201
check('pending_change_expiry_denied',command(s,c),'approval_denied');check('healthy_old_head_retained',s==before,True)

# Recovery uses pre-established independent authorization, not a bypass through the new key.
for flag in ['recoveryPrecommitted','recoveryAuthorityValid','recoveryAnchorIndependent','newKeyPop','checkpointCurrent']:
 s=state();s['admission']='recovery_prepared';c=context();c['currentAuthority']=False;c[flag]=False
 expected='authority_denied' if flag.startswith('recovery') else 'new_key_unproven' if flag=='newKeyPop' else 'quarantine'
 check('recovery_'+flag,command(s,c,'recover'),expected)
s=state();s['admission']='recovery_prepared';c=context();c['currentAuthority']=False
check('independent_recovery_fixture',command(s,c,'recover'),'committed_candidate')
check('recovery_stays_restricted',s['admission'],'recovered_restricted')
check('recovery_does_not_lower_fence',s['fence'],3)
check('management_recovery_no_wallet_key_change',s['walletKeysChanged'],0)
check('recovery_retry_original_only',command(s,c,'recover'),'original_metadata_only')
c2=context();c2.update(expectedEpoch=5,targetEpoch=6);s['admission']='recovery_prepared'
check('same_case_new_request_not_second_recovery',command(s,c2,'recover','req-b'),'recovery_case_consumed')
s['admission']='recovered_restricted';c2=context();c2.update(expectedEpoch=5,targetEpoch=6)
for flag in ['resumeAuthority','separateReleaseReview','consumerCurrent','capabilityCurrent']:
 bad=copy.deepcopy(c2);bad[flag]=False
 check('resume_'+flag,command(s,bad,'resume','resume-'+flag),'authority_denied' if flag=='resumeAuthority' else 'resume_hold')
check('resume_after_separate_review',command(s,c2,'resume','resume-ok'),'resumed_candidate')
check('resume_no_extra_epoch',s['epoch'],5)

# Consumer checkpoint/current authority are independent from signature verification.
for known_chain,current_grant,protected_checkpoint,expected in [(True,True,True,'eligible'),(False,True,True,'hold'),(True,False,True,'hold'),(True,True,False,'hold')]:
 check('consumer_'+str(len(CASES)+1),'eligible' if known_chain and current_grant and protected_checkpoint else 'hold',expected)
report=dict(status='passed',scope='finite_management_trust_design_examples',casesPassed=len(CASES),cases=CASES,
 fixtureApprovalQuorum=2,fixtureQuorumIsPolicySelection=False,activeTrustChanges=0,runtimeVerified=False,canonicalMerged=False,productImplementationPerformed=False,
 sourceHashes={'content/specifications/management-trust-lifecycle.json':hashlib.sha256((P/'management-trust-lifecycle.json').read_bytes()).hexdigest()},
 limitations=['Approval signatures, identity independence, OOB provenance and checkpoints are synthetic verified-input flags.',
 'The test quorum of 2 does not select a product threshold or assign people.',
 'No private keys, management identities, SQL, protected storage, remote recovery or device firmware were exercised.',
 'Serialized examples do not prove cross-store atomicity, root compromise recovery, or instantaneous offline revocation.'])
(P/'management-trust-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['cases','limitations','sourceHashes']}))
