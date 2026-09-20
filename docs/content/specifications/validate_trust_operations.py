"""Bounded synthetic review/projection examples; not product authorization code."""
import copy,hashlib,json
from pathlib import Path
P=Path(__file__).resolve().parent
cases=[]
def check(name,actual,expected):
 assert actual==expected,(name,actual,expected)
 cases.append(dict(id=name,observed=actual,expected=expected))

def context():
 return dict(authenticated=True,current=True,environment='a',target='t',controller='reviewer-b',
             grants={'detail','approve','commit','resume','restrict'},readMode='scoped',request='r1',
             policySelected=True,independent=True,expires=False,expectedRevision=3,
             proofValid=True,readiness=True,releaseReview=True)

def request():
 return dict(id='r1',environment='a',target='t',requester='requester-a',digest='digest-a',revision=3,
             approvedDigest=None,approvedController=None,approvalsCurrent=True,committed=False,
             unknown=False,reviewResponses=[],admission='restricted',effectCount=0)

def authorized(c,r,grant):
 return (c['authenticated'] and c['current'] and grant in c['grants'] and
         c['environment']==r['environment'] and c['target']==r['target'])

def review(c,r,digest,approve=True):
 if not authorized(c,r,'approve'):return 'denied'
 if not c['policySelected']:return 'policy_hold'
 if not c['independent'] or c['controller']==r['requester']:return 'not_independent'
 if c['expectedRevision']!=r['revision'] or digest!=r['digest']:return 'stale_review'
 if c['expires'] or not c['proofValid']:return 'evidence_hold'
 if r['committed'] or r['unknown']:return 'query_original'
 if not approve:
  r['reviewResponses'].append('revision_requested');return 'response_recorded_not_cancelled'
 r.update(approvedDigest=digest,approvedController=c['controller']);return 'reviewed_not_active'

def commit(c,r):
 if not authorized(c,r,'commit'):return 'denied'
 if r['committed'] or r['unknown']:return 'query_original'
 if not c['policySelected']:return 'policy_hold'
 if r['reviewResponses']:return 'rejection_policy_hold'
 if c['expectedRevision']!=r['revision']:return 'stale_commit'
 if (r['approvedDigest']!=r['digest'] or not r['approvalsCurrent'] or c['expires'] or
     not c['proofValid'] or not c['independent'] or r['approvedController']==r['requester']):return 'review_hold'
 r.update(committed=True,effectCount=r['effectCount']+1,admission='recovered_restricted')
 return 'committed_restricted'

def projection(c,r,op='detail'):
 if not authorized(c,r,op):return {'status':'denied'}
 if c['readMode']=='exact_request' and (op!='detail' or c['request']!=r['id']):return {'status':'denied'}
 if op in ['list','export','evidence']:return {'status':'scoped_route_only'}
 return dict(status='visible',requestId=r['id'],originalOutcome='committed' if r['committed'] else 'unknown' if r['unknown'] else 'pending',
             currentRestriction=r['admission'])

def restrict(c,r):
 if not authorized(c,r,'restrict'):return 'denied'
 r['admission']='restricted';return 'restricted'

def resume(c,r,new_subject=False):
 if not authorized(c,r,'resume'):return 'denied'
 if not c['policySelected']:return 'policy_hold'
 if not new_subject:return 'separate_approval_required'
 if not c['readiness'] or not c['releaseReview']:return 'release_hold'
 # Deliberately only a gate; real TMC-07 validation is not modeled here.
 return 'eligible_for_separate_resume_review'

c=context();r=request()
check('approved_is_not_active', (review(c,r,r['digest']),r['committed']), ('reviewed_not_active',False))
check('commit_separate_service', commit(c,r), 'committed_restricted')
check('recovery_does_not_resume', r['admission'], 'recovered_restricted')
check('double_click_result_only', (commit(c,r),r['effectCount']), ('query_original',1))
for key,val in [('authenticated',False),('current',False),('environment','b'),('target','other')]:
 c=context();c[key]=val;r=request();old=copy.deepcopy(r)
 check('review_'+key,(review(c,r,r['digest']),r==old),('denied',True))
c=context();c['controller']='requester-a'
check('requester_self_elevation',review(c,request(),'digest-a'),'not_independent')
c=context();c['independent']=False
check('alias_same_controller',review(c,request(),'digest-a'),'not_independent')
for grants in [{'store_owner'},{'firmware_release'},{'detail'}, {'custody'}]:
 c=context();c['grants']=grants
 check('role_not_approval_'+next(iter(grants)),review(c,request(),'digest-a'),'denied')
c=context();r=request();review(c,r,'digest-a');r['digest']='digest-b'
check('edited_subject_old_approval',commit(c,r),'review_hold')
check('stale_tab_old_digest',review(c,r,'digest-a'),'stale_review')
c=context();r=request();review(c,r,'digest-a');r['revision']=4
check('concurrent_revision_change',commit(c,r),'stale_commit')
c=context();r=request();review(c,r,'digest-a');r['approvalsCurrent']=False
check('approval_revoked_after_review',commit(c,r),'review_hold')
c=context();r=request();review(c,r,'digest-a');c['expires']=True
check('approval_expires_before_commit',commit(c,r),'review_hold')
c=context();r=request();review(c,r,'digest-a');c['current']=False
check('service_permission_revoked',commit(c,r),'denied')
c=context();r=request();review(c,r,'digest-a')
check('review_comment_not_cancel',review(c,r,'digest-a',False),'response_recorded_not_cancelled')
check('comment_preserves_approved_digest',r['approvedDigest'],'digest-a')
check('rejection_aggregation_unselected',commit(c,r),'rejection_policy_hold')
# Whether a rejection has veto effect is unselected; no effect is committed after it.
c=context();r=request();r['unknown']=True
check('unknown_not_restart',commit(c,r),'query_original')
check('unknown_not_reapprove',review(c,r,'digest-a'),'query_original')
c=context();r=request();review(c,r,'digest-a');commit(c,r);c['expires']=True;c['policySelected']=False
before=copy.deepcopy(r)
check('expired_mutation_evidence_read_still_scoped',projection(c,r)['originalOutcome'],'committed')
check('read_no_effect',r==before,True)
for op in ['list','export','evidence']:
 check('detail_not_'+op,projection(c,r,op),{'status':'denied'})
c=context();c.update(readMode='exact_request',grants={'detail','list','export','evidence'})
for op in ['list','export','evidence']:
 check('exact_request_not_'+op,projection(c,r,op),{'status':'denied'})
c['request']='r2';check('exact_request_other_id',projection(c,r),{'status':'denied'})
c=context();c['environment']='b';check('other_environment_projection',projection(c,r),{'status':'denied'})
c=context();r.update(secret='DO_NOT_DISCLOSE',proof='RAW_PROOF',controllerTopology=['private'],audio='PRIVATE_AUDIO')
check('projection_allowlist',set(projection(c,r)),{'status','requestId','originalOutcome','currentRestriction'})
c['current']=False;check('session_revoked_projection',projection(c,r),{'status':'denied'})
c=context();c['policySelected']=False;r=request()
check('policy_missing_blocks_new_review',review(c,r,'digest-a'),'policy_hold')
check('independent_restriction_still_allowed',restrict(c,r),'restricted')
c=context();r=request();review(c,r,'digest-a');commit(c,r)
check('recovery_approval_not_resume',resume(c,r),'separate_approval_required')
c['readiness']=False;check('consumer_not_ready',resume(c,r,True),'release_hold')
c['readiness']=True;c['releaseReview']=False;check('incident_not_closed',resume(c,r,True),'release_hold')
c['releaseReview']=True;c['grants']={'detail'};check('observer_cannot_resume',resume(c,r,True),'denied')

# Normalize sets/tuples for readable, deterministic JSON evidence.
def serial(x):
 if isinstance(x,set):return sorted(x)
 if isinstance(x,(list,tuple)):return [serial(y) for y in x]
 if isinstance(x,dict):return {k:serial(v) for k,v in x.items()}
 return x
report=dict(status='passed',scope='finite_operating_workflow_and_projection_examples',casesPassed=len(cases),cases=serial(cases),
 runtimeVerified=False,canonicalMerged=False,assignedPeople=[],productImplementationPerformed=False,
 sourceHashes={'content/specifications/trust-operations-design.json':hashlib.sha256((P/'trust-operations-design.json').read_bytes()).hexdigest()},
 limitations=['Synthetic current authority, identity independence and signature validity; no IAM, cryptography or browser execution.',
 'One eligible approval represents a prevalidated approval set, not a selected quorum.',
 'Rejection/veto policy remains unselected; comment records do not implement a cancellation protocol.',
 'Subset model does not prove bootstrap/recovery channel security, concurrent transactions, retention or actual resume execution.'])
(P/'trust-operations-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['cases','sourceHashes','limitations']}))
