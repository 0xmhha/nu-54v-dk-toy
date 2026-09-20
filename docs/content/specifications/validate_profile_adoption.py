"""Finite protocol model: synthetic evidence and serialized transitions only.
Does not implement a service, distribute messages, or verify runtime capabilities.
"""
import copy, hashlib, json
from pathlib import Path
P=Path(__file__).resolve().parent
CASES=[]

def fixture():
 return dict(head=7,headDigest='old',fence=3,registry=4,readiness=5,plan=2,phase='prepared',
             digest='new',schemaCompatible=True,selected=True,runtimeEvidence=True,authority=True,
             securityAllowed=True,now=100,manifestExpiry=200,
             peers={role:dict(boot=1,digest='new',authenticated=True,challenge='c-'+role,
                             expectedChallenge='c-'+role,expiresAt=150,capability=True,role=role)
                    for role in ['backend','app','device']},
             required=['backend','app','device'],observedBoot={'backend':1,'app':1,'device':1},
             commits={},applied={},continuationAllowed=True,readerTrusted=True)

def request():return dict(id='request-1',digest='new',head=7,fence=3,registry=4,readiness=5,plan=2)

def activate(s,q):
 if not s['authority']:return 'forbidden'
 if q['id'] in s['commits']:
  return 'original_commit_metadata' if s['commits'][q['id']]['request']==q else 'idempotency_conflict'
 if s['phase']!='prepared':return 'not_prepared'
 if q['digest']!=s['digest']:return 'manifest_mismatch'
 if not s['selected']:return 'policy_unselected'
 if not s['schemaCompatible'] or not s['runtimeEvidence']:return 'unsupported'
 if not s['securityAllowed']:return 'restricted'
 if any(q[k]!=s[k] for k in ['head','fence','registry','readiness','plan']):return 'revision_conflict'
 if s['now']>=s['manifestExpiry']:return 'expired'
 for role in s['required']:
  p=s['peers'].get(role)
  if not p:return 'missing_peer'
  if not p['authenticated'] or p['role']!=role or p['challenge']!=p['expectedChallenge']:return 'invalid_peer_proof'
  if not p['capability'] or p['digest']!=s['digest']:return 'incompatible_peer'
  if p['boot']!=s['observedBoot'][role]:return 'stale_boot'
  if s['now']>=p['expiresAt']:return 'expired_peer'
 s['head']+=1;s['headDigest']=s['digest'];s['phase']='active'
 s['commits'][q['id']]={'request':copy.deepcopy(q),'head':s['head'],'digest':s['digest']}
 return 'committed'

def abort(s):
 if not s['authority']:return 'forbidden'
 if s['phase'] in ['active','superseded']:return 'already_committed'
 if s['phase'] not in ['proposed','staging','prepared']:return 'terminal'
 s['phase']='aborted';return 'aborted'

def read_result(s,id,read_authority=True):
 if not read_authority or not s['readerTrusted']:return 'read_denied'
 c=s['commits'].get(id)
 return dict(originalHead=c['head'] if c else None,currentHead=s['head'],securityAllowed=s['securityAllowed'],phase=s['phase'])

def report_applied(s,role,head,digest,boot):
 if role not in s['required']:return 'not_required_peer'
 if boot!=s['observedBoot'][role]:return 'stale_boot'
 if head!=s['head'] or digest!=s['headDigest']:return 'historical_report_only'
 s['applied'][role]=(head,digest,boot);return 'reported'

def allow_effect(s,roles,head,digest,online=True):
 if not s['authority'] or not s['securityAllowed']:return 'held'
 if not online:return 'held_online_required'
 if head!=s['head'] or digest!=s['headDigest']:return 'stale_profile'
 for role in roles:
  if role not in s['required'] or not s['peers'][role]['capability']:return 'unsupported'
  if s['applied'].get(role)!=(head,digest,s['observedBoot'][role]):return 'peer_not_applied'
 return 'eligible_not_executed'

def check(name,observed,expected):
 assert observed==expected,(name,observed,expected)
 CASES.append(dict(id=name,expected=expected,observed=observed))

# Missing choice, capability and stale inputs never advance the head.
for key,value,expected in [('authority',False,'forbidden'),('selected',False,'policy_unselected'),
 ('schemaCompatible',False,'unsupported'),('runtimeEvidence',False,'unsupported'),
 ('securityAllowed',False,'restricted'),('phase','staging','not_prepared'),
 ('now',200,'expired')]:
 s=fixture();s[key]=value;check('activate_'+key,activate(s,request()),expected);check('no_head_change_'+key,s['head'],7)
for key in ['head','fence','registry','readiness','plan']:
 s=fixture();q=request();q[key]+=1;check('stale_'+key,activate(s,q),'revision_conflict')
s=fixture();q=request();q['digest']='other';check('wrong_manifest',activate(s,q),'manifest_mismatch')
for field,value,expected in [('authenticated',False,'invalid_peer_proof'),('role','other','invalid_peer_proof'),
 ('challenge','old','invalid_peer_proof'),('capability',False,'incompatible_peer'),
 ('digest','old','incompatible_peer'),('boot',0,'stale_boot'),('expiresAt',100,'expired_peer')]:
 s=fixture();s['peers']['device'][field]=value
 check('peer_'+field,activate(s,request()),expected)
s=fixture();del s['peers']['device'];s['peers']['optional-observer']={'digest':'new'}
check('optional_cannot_replace_required',activate(s,request()),'missing_peer')
s=fixture();s['peers']['optional-observer']={'digest':'old','capability':False}
check('irrelevant_optional_does_not_block',activate(s,request()),'committed')
s=fixture();s['observedBoot']['device']=2
check('observed_reboot_invalidates_ack',activate(s,request()),'stale_boot')
s=fixture();s['registry']+=1
check('peer_registry_changed_before_commit',activate(s,request()),'revision_conflict')

# Response loss: same request observes the original commit, not a second commit.
s=fixture();q=request();check('first_commit',activate(s,q),'committed')
check('same_request_after_lost_response',activate(s,q),'original_commit_metadata')
check('exactly_one_head_increment',s['head'],8)
conflict=dict(q,digest='altered');check('same_key_altered_body',activate(s,conflict),'idempotency_conflict')
check('server_commit_not_peer_apply',len(s['applied']),0)
check('partial_apply_not_enabled',allow_effect(s,['backend','app','device'],8,'new'),'peer_not_applied')
for peer in ['backend','app','device']:
 check('report_'+peer,report_applied(s,peer,8,'new',1),'reported')
check('all_bound_peers_ready',allow_effect(s,['backend','app','device'],8,'new'),'eligible_not_executed')
check('duplicate_applied_report',report_applied(s,'device',8,'new',1),'reported')
check('duplicate_report_not_reactivation',s['head'],8)
check('old_digest_request',allow_effect(s,['device'],7,'old'),'stale_profile')
check('wrong_digest_same_revision',allow_effect(s,['device'],8,'old'),'stale_profile')
check('online_mode_offline',allow_effect(s,['device'],8,'new',False),'held_online_required')
s['observedBoot']['device']=2
check('reboot_after_apply',allow_effect(s,['device'],8,'new'),'peer_not_applied')
check('old_boot_report_rejected',report_applied(s,'device',8,'new',1),'stale_boot')
check('new_peer_cannot_inherit',allow_effect(s,['replacement-device'],8,'new'),'unsupported')

# Abort and commit serialize in either order. Timeout is never an abort proof.
s=fixture();check('abort_first',abort(s),'aborted');check('late_activation_after_abort',activate(s,request()),'not_prepared')
check('abort_preserves_old_head',(s['head'],s['headDigest']),(7,'old'))
s=fixture();activate(s,request());check('commit_first_abort',abort(s),'already_committed');check('abort_after_commit_keeps_head',s['head'],8)

# A changed security fence wins over old preparation and over prior activation.
s['securityAllowed']=False;s['fence']+=1
check('active_but_revoked',allow_effect(s,['device'],8,'new'),'held')
before=copy.deepcopy(s);r=read_result(s,'request-1')
check('result_read_preserves_old_commit',r['originalHead'],8)
check('result_read_reports_current_denial',r['securityAllowed'],False)
check('result_read_has_no_mutation',s,before)
check('no_read_authority',read_result(s,'request-1',False),'read_denied')
s['readerTrusted']=False;check('missing_old_reader_not_latest_parse',read_result(s,'request-1'),'read_denied')

# Returning to former content uses a newly prepared higher head; never decrements it.
s=fixture();activate(s,request());old_result=copy.deepcopy(s['commits']['request-1'])
s['phase']='prepared';s['digest']='old';s['readiness']+=1
for p in s['peers'].values():p['digest']='old'
q2=dict(id='request-2',digest='old',head=8,fence=3,registry=4,readiness=6,plan=2)
check('return_old_content_new_commit',activate(s,q2),'committed')
check('rollback_is_new_revision',(s['head'],s['headDigest']),(9,'old'))
check('old_operation_binding_preserved',s['commits']['request-1'],old_result)
check('read_old_success_not_current_activation',read_result(s,'request-1')['currentHead'],9)
check('old_apply_report_cannot_restore_head',report_applied(s,'device',8,'new',1),'historical_report_only')
check('old_apply_keeps_new_head',s['head'],9)
check('old_request_retry_no_restore',activate(s,request()),'original_commit_metadata')
check('old_retry_keeps_new_head',s['head'],9)
s['phase']='prepared';s['securityAllowed']=False
q3=dict(id='request-3',digest='old',head=9,fence=3,registry=4,readiness=6,plan=2)
check('revoked_old_content_not_reactivated',activate(s,q3),'restricted')

# Scope of recovery: no mutation of the original operation is modeled as a read.
for original_known,current_allowed,new_effect,unknown_exposure,expected in [
 (True,True,False,False,'original_continuation_candidate'),
 (False,True,False,False,'hold'),(True,False,False,False,'hold'),
 (True,True,True,False,'hold'),(True,True,False,True,'observe_original_only')]:
 result='hold' if not original_known or not current_allowed or new_effect else 'observe_original_only' if unknown_exposure else 'original_continuation_candidate'
 check('continuation_'+str(len(CASES)+1),result,expected)

# Normalize tuple cases and avoid emitting full snapshots as if they were live data.
for c in CASES:
 for key in ['expected','observed']:
  if isinstance(c[key],dict) and 'peers' in c[key]:c[key]='unchanged_synthetic_state'
report=dict(status='passed',scope='finite_serialized_design_model_only',casesPassed=len(CASES),cases=CASES,
 runtimeVerified=False,productImplementationPerformed=False,activeRollouts=0,canonicalMerged=False,
 sourceHashes={'content/specifications/profile-adoption-protocol.json':hashlib.sha256((P/'profile-adoption-protocol.json').read_bytes()).hexdigest()},
 limitations=['Synthetic peer evidence and sequential CAS model; no distributed race/crypto proof.',
 'Current boot means observed registry state, not instantaneous detection of remote reboot.',
 'Offline revocation and already signed/propagated transaction cancellation are not guaranteed.',
 'No schema/DB migration, deployment, lease implementation or hardware capability was exercised.'])
(P/'profile-adoption-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['cases','sourceHashes','limitations']}))
