"""Finite scoped authorization/idempotency design examples, not middleware."""
import copy,hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent
D=json.loads((P/'profile-control-adoption-map.json').read_text())
OPS={o['id']:o for o in D['operations']}
RESULTS=[]

def check(name,actual,expected):
 assert actual==expected,(name,actual,expected)
 RESULTS.append(dict(id=name,observed=actual,expected=expected))

def actor():
 return dict(id='operator-a',env='test-a',scope='store-a',authenticated=True,
   grantTrusted=True,grantCurrent=True,entitlements={'PCP-01','PCP-03','PCP-04','PCP-06','PCP-07'},
   readCapability=None,peerBinding=None,relayPermission=False)

def req(op='PRC-01',rid='r1'):
 return dict(op=op,id=rid,env='test-a',scope='store-a',change='change-a',digest='body-a',
   proofValid=True,peerBinding='device-a',boot=1,currentBoot=1,challenge='challenge-a',challengeCurrent=True,
   reportId='device-report-a',activation='activation-a',semanticDigest='evidence-a',expectedRevision=4,currentRevision=4,
   profileSupported=True,choiceSelected=True,restricted=False,restrictionOnly=True,claimedRole='admin')

def auth(a,q):
 if not a['authenticated']:return 'UNAUTHORIZED'
 if not a['grantTrusted'] or not a['grantCurrent'] or a['env']!=q['env'] or a['scope']!=q['scope']:return 'FORBIDDEN'
 policy=OPS[q['op']]['policyRef']
 if policy in a['entitlements']:return 'OK'
 if q['op']=='PRC-04' and a['readCapability']==(q['env'],q['scope'],q['change']):return 'OK'
 return 'FORBIDDEN'

def db():return dict(outcomes={},semanticReports={},effects=0,head=4,cache={},historyTrusted=True)

def perform(a,q,s):
 if q['op'] not in OPS:return 'INVALID_INPUT'
 permission=auth(a,q)
 if permission!='OK':return permission
 if q['op']=='PRC-04':return 'READ_PROJECTION' if s['historyTrusted'] else 'RESULT_PENDING'
 identity=(q['env'],a['id'],q['scope'],q['op'],q['change'],q['id'])
 if identity in s['outcomes']:
  return 'CURRENT_PROJECTION_OF_ORIGINAL' if s['outcomes'][identity]==q['digest'] else 'IDEMPOTENCY_CONFLICT'
 if not s['historyTrusted']:return 'RESULT_PENDING'
 if not q['profileSupported']:return 'UNSUPPORTED_PROFILE'
 if not q['choiceSelected']:return 'POLICY_UNSELECTED'
 if q['restricted']:return 'AUTHORITY_HELD'
 if q['expectedRevision']!=q['currentRevision']:return 'REVISION_CONFLICT'
 if q['op']=='PRC-07' and not q['restrictionOnly']:return 'FORBIDDEN'
 if q['op'] in ['PRC-02','PRC-05']:
  if a['peerBinding']!=q['peerBinding'] and not a['relayPermission']:return 'FORBIDDEN'
  if not q['proofValid']:return 'FORBIDDEN'
  if q['boot']!=q['currentBoot'] or not q['challengeCurrent']:return 'SOURCE_STALE'
  # Identity of original peer report is independent of the HTTP relay actor/id.
  parent=q['change'] if q['op']=='PRC-02' else q['activation']
  semantic=(q['env'],q['scope'],q['op'],parent,q['peerBinding'],q['boot'],q['reportId'])
  if q['op']=='PRC-02':semantic+=(q['challenge'],)
  if semantic in s['semanticReports']:
   return 'ORIGINAL_PEER_REPORT' if s['semanticReports'][semantic]==q['semanticDigest'] else 'IDEMPOTENCY_CONFLICT'
  s['semanticReports'][semantic]=q['semanticDigest']
 s['outcomes'][identity]=q['digest'];s['effects']+=1
 if q['op']=='PRC-03':s['head']+=1
 return 'COMMIT_CANDIDATE'

for op in OPS:
 q=req(op);a=actor();a['entitlements']={OPS[op]['policyRef']};a['peerBinding']='device-a'
 check(op+'_exact_grant',perform(a,q,db()),'READ_PROJECTION' if op=='PRC-04' else 'COMMIT_CANDIDATE')
 for field,value in [('authenticated',False),('grantTrusted',False),('grantCurrent',False),('env','other'),('scope','other')]:
  bad=copy.deepcopy(a);bad[field]=value
  check(op+'_'+field,perform(bad,q,db()),'UNAUTHORIZED' if field=='authenticated' else 'FORBIDDEN')
for role in ['firmware_release_operator','store_terminal_manage','store_owner','ops_audit_read','release_read']:
 a=actor();a['entitlements']={role};check('existing_'+role+'_not_activator',perform(a,req('PRC-03'),db()),'FORBIDDEN')
a=actor();a['entitlements']=set();q=req('PRC-03');q['claimedRole']='profile_change_activate'
check('self_claimed_admin_denied',perform(a,q,db()),'FORBIDDEN')
a['readCapability']=('test-a','store-a','change-a')
check('scoped_read_capability',perform(a,req('PRC-04'),db()),'READ_PROJECTION')
check('read_capability_cannot_activate',perform(a,req('PRC-03'),db()),'FORBIDDEN')
q=req('PRC-04');q['change']='another-change';check('read_capability_exact_change',perform(a,q,db()),'FORBIDDEN')
a=actor();a['grantTrusted']=False
check('target_manifest_cannot_bootstrap_authority',perform(a,req(),db()),'FORBIDDEN')

# Same immutable request, current permission, independent durable outcome retention.
a=actor();q=req('PRC-03');s=db();check('activation_first',perform(a,q,s),'COMMIT_CANDIDATE')
check('activation_retry',perform(a,q,s),'CURRENT_PROJECTION_OF_ORIGINAL')
check('activation_once',s['head'],5)
q2=dict(q,digest='changed-body');check('same_id_changed_body',perform(a,q2,s),'IDEMPOTENCY_CONFLICT')
s['cache'].clear();check('cache_expired_outcome_retained',perform(a,q,s),'CURRENT_PROJECTION_OF_ORIGINAL')
check('no_cache_expiry_reexecution',s['effects'],1)
a['entitlements']={'PCP-04'}
check('lost_mutation_role_no_replay',perform(a,q,s),'FORBIDDEN')
before=copy.deepcopy(s);check('current_read_after_role_loss',perform(a,req('PRC-04'),s),'READ_PROJECTION');check('read_domain_unchanged',s==before,True)
a['grantCurrent']=False;check('lost_all_authority_no_original_payload',perform(a,req('PRC-04'),s),'FORBIDDEN')
a=actor();s=db();s['historyTrusted']=False
check('pruned_unknown_not_new_mutation',perform(a,q,s),'RESULT_PENDING')
check('unknown_history_no_effect',s['effects'],0)
check('unknown_history_read_pending',perform(a,req('PRC-04'),s),'RESULT_PENDING')
for k,v,e in [('profileSupported',False,'UNSUPPORTED_PROFILE'),('choiceSelected',False,'POLICY_UNSELECTED'),
 ('restricted',True,'AUTHORITY_HELD'),('currentRevision',5,'REVISION_CONFLICT')]:
 q=req('PRC-03');q[k]=v;state=db();check(k,perform(actor(),q,state),e);check(k+'_no_effect',state['effects'],0)

# Relayed evidence is authenticated independently; changing HTTP identity cannot reconsume it.
for op in ['PRC-02','PRC-05']:
 a=actor();a['entitlements']={OPS[op]['policyRef']};a['relayPermission']=True
 q=req(op);s=db();check(op+'_relay_first',perform(a,q,s),'COMMIT_CANDIDATE')
 a2=copy.deepcopy(a);a2['id']='relay-b';q2=dict(q,id='different-http-id')
 check(op+'_relay_changed_no_duplicate',perform(a2,q2,s),'ORIGINAL_PEER_REPORT')
 check(op+'_one_evidence_effect',s['effects'],1)
 q3=dict(q2,id='third-http-id',semanticDigest='forged-different-evidence')
 check(op+'_same_semantic_identity_changed_evidence',perform(a2,q3,s),'IDEMPOTENCY_CONFLICT')
 for k,v,e in [('proofValid',False,'FORBIDDEN'),('boot',0,'SOURCE_STALE'),('challengeCurrent',False,'SOURCE_STALE')]:
  bad=dict(q);bad[k]=v;check(op+'_'+k,perform(a,bad,db()),e)
 no_relay=copy.deepcopy(a);no_relay['relayPermission']=False
 check(op+'_http_role_not_peer_identity',perform(no_relay,q,db()),'FORBIDDEN')
q=req('PRC-07');q['restrictionOnly']=False
check('restriction_cannot_release_fence',perform(actor(),q,db()),'FORBIDDEN')
# Unsupported typed readers are a design-adoption guard, not automatically available API reuse.
for kind,registered,expected in [('profile_rollout_control',False,'UNSUPPORTED_PROFILE'),('profile_rollout_control',True,'TYPED_READER_CANDIDATE')]:
 check('typed_reader_registered_'+str(registered),'TYPED_READER_CANDIDATE' if registered else 'UNSUPPORTED_PROFILE',expected)

report=dict(status='passed',scope='finite_authority_and_idempotency_design_examples',casesPassed=len(RESULTS),cases=RESULTS,
 runtimeVerified=False,productImplementationPerformed=False,canonicalMerged=False,
 sourceHashes={'content/specifications/profile-control-adoption-map.json':hashlib.sha256((P/'profile-control-adoption-map.json').read_bytes()).hexdigest()},
 limitations=['Boolean trust/capability flags are synthetic verifier results, never client claims.',
 'No cryptography, HTTP/BLE handler, secure storage, database race or deployment was exercised.',
 'Serialized examples cover listed authority/idempotency branches, not all action semantics or wire schemas.',
 'Original operation/activation peer-state protocol is specified separately and is not replaced by this model.'])
(P/'profile-control-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['cases','limitations','sourceHashes']}))
