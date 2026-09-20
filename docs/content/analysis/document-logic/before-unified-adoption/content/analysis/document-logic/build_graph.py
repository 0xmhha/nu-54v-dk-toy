"""Build a local, provenance-preserving document graph. No x-theory claim.
Run: python3 content/analysis/document-logic/build_graph.py
Only writes derived analysis artifacts in this directory.
"""
import json,hashlib,re,collections
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]
OUT=Path(__file__).resolve().parent

def read(p):return json.loads((ROOT/p).read_text())
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def save(name,obj):(OUT/name).write_text(json.dumps(obj,ensure_ascii=False,indent=2)+'\n')
paths=sorted(p for p in (ROOT/'content').rglob('*') if p.is_file() and p.suffix in {'.json','.md','.sql','.graphql'} and OUT not in p.parents)
paths=[ROOT/'README.md']+paths
nodes={}; edges=[];seen=set()
def node(id,kind,label,source=None,pointer=None,**extra):
 if id in nodes:return
 nodes[id]=dict(id=id,kind=kind,label=label,source=source,pointer=pointer,**extra)
def edge(a,b,rel,source,pointer=None,basis='explicit'):
 k=(a,b,rel,source,pointer)
 if k not in seen:
  seen.add(k);edges.append(dict(source=a,target=b,relation=rel,provenance=dict(file=source,pointer=pointer),basis=basis))
def category(path):
 n=path.name
 if 'validation' in n or n.startswith('validate_'):return 'verification_record'
 if 'template' in n or 'examples' in n:return 'example_or_template'
 if n.endswith('.schema.json'):return 'schema'
 if 'scope' in n and 'review' not in n:return 'scope'
 if 'decision' in n or 'technology-selection' in n:return 'decision'
 if 'handoff' in n or 'work-' in n or 'implementation-' in n or 'readiness' in n:return 'plan_or_handoff'
 if path.suffix in {'.sql','.graphql'} or any(s in n for s in ['catalog','dto','contract','schema','interface','baseline','storage','adoption','mapping']):return 'contract_or_interface'
 if any(s in n for s in ['design','flow','security','lifecycle']):return 'architecture_or_behavior'
 if any(s in n for s in ['medium','article','intro','ot-','maker']):return 'communication'
 return 'reference_or_unclassified'
parsed={}
for p in paths:
 rel=str(p.relative_to(ROOT)); status=None
 if p.suffix=='.json':
  try:parsed[rel]=json.loads(p.read_text())
  except json.JSONDecodeError:pass
  if isinstance(parsed.get(rel),dict):status=parsed[rel].get('status',parsed[rel].get('phase'))
 node('doc:'+rel,'document',p.name,rel,category=category(p),classificationBasis='filename heuristic; not semantic adjudication',status=status,sha256=sha(p))
for rel,d in parsed.items():
 def scan(x,pointer=''):
  if isinstance(x,dict):
   for k,v in x.items():
    ptr=pointer+'/'+k.replace('~','~0').replace('/','~1')
    if k.startswith('content/') and 'doc:'+k in nodes:edge('doc:'+rel,'doc:'+k,'pins_source' if pointer=='/sourceHashes' else 'references',rel,ptr)
    scan(v,ptr)
  elif isinstance(x,list):
   for i,v in enumerate(x):scan(v,pointer+'/'+str(i))
  elif isinstance(x,str) and 'doc:'+x in nodes:edge('doc:'+rel,'doc:'+x,'references',rel,pointer)
 scan(d)
# Markdown local links are references only; a link is not an adoption or proof edge.
for p in paths:
 if p.suffix!='.md':continue
 rel=str(p.relative_to(ROOT))
 for match in re.finditer(r'\]\(([^)]+)\)',p.read_text()):
  link=match.group(1).split('#')[0]
  if link.startswith(('http:','https:','app:','codex:')):continue
  target=(p.parent/link).resolve()
  try:targetrel=str(target.relative_to(ROOT))
  except ValueError:continue
  if 'doc:'+targetrel in nodes:edge('doc:'+rel,'doc:'+targetrel,'references',rel,'line:'+str(p.read_text()[:match.start()].count('\n')+1))
wpath='content/planning/work-breakdown.json';fpath='content/planning/full-scope-design-review.json';hpath='content/planning/preimplementation-handoff.json'
w=read(wpath);f=read(fpath);h=read(hpath);tasks={t['id']:t for t in w['tasks']}
for i,r in enumerate(f['requirements']):node('req:'+str(r['requirement']),'requirement',r['title'],fpath,f'/requirements/{i}',status='user_scope')
for i,d in enumerate(w['decisions']):node('decision:'+d['id'],'decision',d['id']+' '+d['title'],wpath,f'/decisions/{i}',status=d['status'])
for i,t in enumerate(w['tasks']):
 tid='task:'+t['id'];node(tid,'task',t['id']+' '+t['title'],wpath,f'/tasks/{i}',status=t['status'])
 for req in t['requirements']:edge(tid,'req:'+str(req),'addresses',wpath,f'/tasks/{i}/requirements')
 for dep in t['dependsOn']:edge(tid,'task:'+dep,'depends_on',wpath,f'/tasks/{i}/dependsOn')
 for dec in t['decisionInputs']:edge(tid,'decision:'+dec,'needs_decision',wpath,f'/tasks/{i}/decisionInputs')
 for j,step in enumerate(t['implementationSteps']):
  sid='step:'+step['id'];node(sid,'step',step['id']+' '+step['action'],wpath,f'/tasks/{i}/implementationSteps/{j}',status=step['status']);edge(sid,tid,'part_of',wpath,f'/tasks/{i}/implementationSteps/{j}')
package_impacts=[]
for i,p in enumerate(f['designPackages']):
 pid='package:'+p['id'];node(pid,'package',p['id']+' '+p['title'],fpath,f'/designPackages/{i}',status='candidate')
 for tid in p['primaryTaskRefs']:edge('task:'+tid,pid,'primary_design_home',fpath,f'/designPackages/{i}/primaryTaskRefs')
 for req in p['requirementRefs']:edge(pid,'req:'+str(req),'declared_scope',fpath,f'/designPackages/{i}/requirementRefs')
 union=sorted({r for tid in p['primaryTaskRefs'] for r in tasks[tid]['requirements']})
 for r in union:edge(pid,'req:'+str(r),'transitive_task_impact',wpath,basis='derived_from_task_requirements')
 package_impacts.append(dict(package=p['id'],declared=p['requirementRefs'],transitive=union,transitiveNotDeclared=sorted(set(union)-set(p['requirementRefs'])),declaredNotTransitive=sorted(set(p['requirementRefs'])-set(union))))
for p in h['designPackages']:
 src=p['source'];d=read(src);edge('doc:'+src,'package:'+p['id'],'details',hpath,'/designPackages')
 for section,value in d.items():
  if not isinstance(value,list):continue
  for i,row in enumerate(value):
   if not isinstance(row,dict) or 'id' not in row:continue
   # Only meaningful row objects, with IDs namespaced by source to avoid collisions.
   kind='scenario' if section in {'runtimeCases','evaluationSamples'} else 'transition' if 'Transitions' in section else 'policy_input' if section=='policyInputs' else 'design_item'
   id=src+'#'+row['id'];node(id,kind,row['id']+' '+str(row.get('title',row.get('scenario',row.get('name',row.get('fromState',section))))),src,f'/{section}/{i}',status=row.get('status','proposal'))
   edge(id,'package:'+p['id'],'specifies' if kind!='scenario' else 'planned_check_for',src,f'/{section}/{i}')
   edge('doc:'+src,id,'contains',src,f'/{section}/{i}')
   if 'decisionRef' in row:edge(id,'decision:'+row['decisionRef'],'needs_decision',src,f'/{section}/{i}/decisionRef')
   for api in row.get('apiRefs',row.get('existingApiRefs',[])):edge(id,'api:'+api,'uses_contract',src,f'/{section}/{i}')
for i,a in enumerate(read('content/specifications/api-catalog.json')['operations']):node('api:'+a['id'],'api',a['id']+' '+a.get('method','')+' '+a.get('path',''),'content/specifications/api-catalog.json',f'/operations/{i}',status='design_contract_not_runtime')
for i,s in enumerate(read('content/specifications/screen-flows.json')['screens']):node('screen:'+s['id'],'screen',s['id']+' '+s.get('title',s.get('name','')),'content/specifications/screen-flows.json',f'/screens/{i}',status='design')
for i,t in enumerate(w['tasks']):
 for ref in t['interfaceRefs']:
  if ref['entry'].startswith('API-'):edge('task:'+t['id'],'api:'+ref['entry'],'uses_contract',wpath,f'/tasks/{i}/interfaceRefs')
 for screen in t['screenRefs']:edge('task:'+t['id'],'screen:'+screen,'appears_in',wpath,f'/tasks/{i}/screenRefs')
# Candidate auth contracts are distinct from the 110 registered design API entries.
ovpath='content/specifications/preimplementation-contract-overlay.json'
ov=read(ovpath)
for i,row in enumerate(ov['routeContracts']):
 id='candidate:'+row['id'];node(id,'candidate_contract',row['id']+' '+row['name'],ovpath,f'/routeContracts/{i}',status='logical_contract_not_registered')
 edge('doc:'+ovpath,id,'contains',ovpath,f'/routeContracts/{i}')
 for api in row['existingApiRefs']:edge(id,'api:'+api,'proposes_extension_to',ovpath,f'/routeContracts/{i}/existingApiRefs')
authpath='content/specifications/social-wallet-recovery-design.json'
for i,row in enumerate(read(authpath)['authTransitions']):
 for ref in row['route'].split('/'):
  if ref.startswith('OC-'):edge(authpath+'#'+row['id'],'candidate:'+ref,'uses_candidate_contract',authpath,f'/authTransitions/{i}/route')
for section in ['mpcTransitions','mpcResumeTransitions']:
 for i,row in enumerate(read(authpath)[section]):
  for ref in row.get('predicateRefs',[]):edge(authpath+'#'+row['id'],authpath+'#'+ref,'requires_predicate',authpath,f'/{section}/{i}/predicateRefs')
cp='content/specifications/credential-paid-resource-design.json'
for i,row in enumerate(read(cp)['paidResourceTransitions']):
 for ref in row.get('predicateRefs',[]):edge(cp+'#'+row['id'],cp+'#'+ref,'requires_predicate',cp,f'/paidResourceTransitions/{i}/predicateRefs')
for i,row in enumerate(read(cp).get('paidResourcePredicates',[])):
 for ref in row.get('includes',[]):edge(cp+'#'+row['id'],cp+'#'+ref,'includes_predicate',cp,f'/paidResourcePredicates/{i}/includes')
refund=read(cp)['paidRefundContract'];refund_id=cp+'#paidRefundContract'
node(refund_id,'design_item','유료자원 환불 노출·권한 계약',cp,'/paidRefundContract',status=refund['adapterStatus'])
edge('doc:'+cp,refund_id,'contains',cp,'/paidRefundContract')
for ref in refund['routes']:edge(refund_id,'candidate:'+ref,'uses_candidate_contract',cp,'/paidRefundContract/routes')
for i,row in enumerate(read(cp)['refundAttemptTransitions']):
 edge(cp+'#'+row['id'],refund_id,'governed_by',cp,f'/refundAttemptTransitions/{i}',basis='attemptStateAuthority.refund + paidRefundContract')

pp='content/specifications/recording-travel-ai-design.json'
for i,row in enumerate(read(pp)['privacyTransitions']):
 for ref in row.get('predicateRefs',[]):edge(pp+'#'+row['id'],pp+'#'+ref,'requires_predicate',pp,f'/privacyTransitions/{i}/predicateRefs')

for i,row in enumerate(read(pp)['recordingStages']):
 for ref in row.get('predicateRefs',[]):edge(pp+'#'+row['id'],pp+'#'+ref,'requires_predicate',pp,f'/recordingStages/{i}/predicateRefs')
for i,row in enumerate(read(pp)['recordingBoundaryRules']):
 if 'predicateRef' in row:edge(pp+'#'+row['id'],pp+'#'+row['predicateRef'],'requires_predicate',pp,f'/recordingBoundaryRules/{i}/predicateRef')

kp='content/specifications/kiosk-commerce-journey-design.json'
for i,row in enumerate(read(kp)['terminalHandoffTransitions']):
 for ref in row.get('predicateRefs',[]):edge(kp+'#'+row['id'],kp+'#'+ref,'requires_predicate',kp,f'/terminalHandoffTransitions/{i}/predicateRefs')
for field in ['terminalClearanceContract','terminalResultIsolation']:
 id=kp+'#'+field;node(id,'design_item',field,kp,'/'+field,status='logical_candidate_not_registered')
 edge('doc:'+kp,id,'contains',kp,'/'+field)
 for ref in read(kp)[field].get('routes',[]):edge(id,'candidate:'+ref,'uses_candidate_contract',kp,'/'+field+'/routes')
 if 'predicateRef' in read(kp)[field]:edge(id,kp+'#'+read(kp)[field]['predicateRef'],'requires_predicate',kp,'/'+field+'/predicateRef')

bp='content/specifications/commerce-consumer-repair-design.json'
for field in ['benefitEffectOwnership','benefitAssessmentContract']:
 id=bp+'#'+field;node(id,'design_item',field,bp,'/'+field,status='logical_candidate_not_registered')
 edge('doc:'+bp,id,'contains',bp,'/'+field)
for i,row in enumerate(read(bp)['benefitHandoffPredicates']):
 id=bp+'#'+row['id'];node(id,'design_item',row['id'],bp,f'/benefitHandoffPredicates/{i}',status='design_candidate')
 edge(bp+'#benefitAssessmentContract',id,'requires_predicate',bp,'/benefitAssessmentContract/predicateRef')
for field in ['assessmentWriter','effectWriter']:
 ref=read(bp)['benefitEffectOwnership'][field];id=bp+'#'+ref
 node(id,'design_item',ref,bp,'/consumers',status='design_candidate')
 edge(bp+'#benefitEffectOwnership',id,field,bp,'/benefitEffectOwnership/'+field)
edge(pp+'#TR-08',bp+'#benefitEffectOwnership','uses_effect_contract',pp,'/benefitEffectBoundary/contractPointer')

for i,row in enumerate(read(pp)['itineraryWriteActions']):
 for ref in row.get('predicateRefs',[]):edge(pp+'#'+row['id'],pp+'#'+ref,'requires_predicate',pp,f'/itineraryWriteActions/{i}/predicateRefs')
for field in ['itineraryGenerationContract','itineraryApplyContract']:
 id=pp+'#'+field;node(id,'design_item',field,pp,'/'+field,status='logical_candidate_not_registered')
 edge('doc:'+pp,id,'contains',pp,'/'+field)
 for ref in read(pp)[field].get('routes',[]):edge(id,'candidate:'+ref,'uses_candidate_contract',pp,'/'+field+'/routes')

# Reviewed change-impact links are not catalog adoption or implementation evidence.
tracepath='content/analysis/document-logic/contract-task-trace.json'
for i,row in enumerate(read(tracepath)['routes']):
 for tid in row['taskRefs']:
  edge('candidate:'+row['contractId'],'task:'+tid,'reviewed_task_impact',tracepath,f'/routes/{i}',basis='reviewed semantic mapping; not canonical adoption')

# Decision briefing is a proposal with provenance, never an implicit choice.
bpath='content/planning/decision-briefing.json'
for i,row in enumerate(read(bpath)['cards']):
 bid='brief:'+row['id'];node(bid,'decision_brief',row['id']+' '+row['title'],bpath,f'/cards/{i}',status=row['status'])
 edge('doc:'+bpath,bid,'contains',bpath,f'/cards/{i}')
 if row['id'].startswith('D'):
  edge(bid,'decision:'+row['id'],'prepares_options_for',bpath,f'/cards/{i}/id')
 for tid in row['taskRefs']:edge(bid,'task:'+tid,'would_affect_if_selected',bpath,f'/cards/{i}/taskRefs',basis='proposal impact; not selected')

# Logical profile and action gates are design candidates, not active runtime config.
profilepath='content/specifications/selection-profile-contract.json'
actionpath='content/specifications/selection-action-gates.json'
for i,row in enumerate(read(profilepath)['profiles']):
 pid='profile:'+row['id'];node(pid,'selection_profile',row['id']+' '+row['title'],profilepath,f'/profiles/{i}',status='unselected')
 edge('doc:'+profilepath,pid,'contains',profilepath,f'/profiles/{i}')
 edge('brief:'+row['decisionRef'],pid,'prepares_profile_for',profilepath,f'/profiles/{i}/decisionRef')
 for j,fld in enumerate(row['fields']):
  fid='field:'+fld['id'];node(fid,'selection_field',fld['id']+' '+fld['title'],profilepath,f'/profiles/{i}/fields/{j}',status='value_null')
  edge(fid,pid,'field_of',profilepath,f'/profiles/{i}/fields/{j}')
for i,row in enumerate(read(profilepath)['crossConstraints']):
 cid='constraint:'+row['id'];node(cid,'profile_constraint',row['id'],profilepath,f'/crossConstraints/{i}',status='logical_candidate')
 for ref in row['fieldRefs']:edge(cid,'field:'+ref,'cross_checks_if_applicable',profilepath,f'/crossConstraints/{i}/fieldRefs',basis='conditional rule operands, not global prerequisites')
for i,row in enumerate(read(actionpath)['actions']):
 aid='gate:'+row['id'];node(aid,'action_gate',row['id']+' '+row['name'],actionpath,f'/actions/{i}',status='candidate_not_implemented')
 edge('doc:'+actionpath,aid,'contains',actionpath,f'/actions/{i}')
 for ref in row['requiredFieldRefs']:edge(aid,'field:'+ref,'requires_for_applicable_branch',actionpath,f'/actions/{i}/requiredFieldRefs',basis='branch-dependent candidate requirement')
 for ref in row['screenRefs']:edge(aid,'screen:'+ref,'proposes_behavior_in',actionpath,f'/actions/{i}/screenRefs')
 for ref in row['apiRefs']:edge(aid,'api:'+ref,'proposes_gate_for',actionpath,f'/actions/{i}/apiRefs')
 for ref in row['candidateContractRefs']:edge(aid,'candidate:'+ref,'proposes_gate_for',actionpath,f'/actions/{i}/candidateContractRefs')

# Adoption protocol nodes remain unexecuted design requirements.
rollpath='content/specifications/profile-adoption-protocol.json'
roll=read(rollpath)
for section,kind in [('records','rollout_record'),('transitions','rollout_transition'),('controlOperations','rollout_control_contract'),('adoptionSteps','adoption_step')]:
 for i,row in enumerate(roll[section]):
  rid='rollout:'+row['id'];label=row['id']+' '+row.get('name',row.get('trigger',''))
  node(rid,kind,label,rollpath,f'/{section}/{i}',status='candidate_not_executed')
  edge('doc:'+rollpath,rid,'contains',rollpath,f'/{section}/{i}')
for tid in roll['taskRefs']:edge('doc:'+rollpath,'task:'+tid,'provides_design_input',rollpath,'/taskRefs')
for ref in roll['relatedGateRefs']:edge('doc:'+rollpath,'gate:'+ref,'constrains_profile_application',rollpath,'/relatedGateRefs')
for ref in roll['decisionRefs']:edge('doc:'+rollpath,'decision:'+ref,'needs_selected_profile',rollpath,'/decisionRefs')
for ref in roll['uiProjection']['screenRefs']:edge('doc:'+rollpath,'screen:'+ref,'proposes_rollout_projection',rollpath,'/uiProjection/screenRefs')

# Management ACL and mapping proposals do not expand canonical API/ACL/BLE counts.
pcpath='content/specifications/profile-control-adoption-map.json'
pc=read(pcpath)
for section,kind in [('policyCandidates','profile_policy_candidate'),('bleCandidates','profile_ble_candidate'),('storageMappings','profile_storage_mapping')]:
 for i,row in enumerate(pc[section]):
  rid='controlmap:'+row['id'];node(rid,kind,row['id']+' '+row.get('name',row.get('proposedCommand',row.get('logicalName',''))),pcpath,f'/{section}/{i}',status='candidate_not_adopted')
  edge('doc:'+pcpath,rid,'contains',pcpath,f'/{section}/{i}')
for i,row in enumerate(pc['operations']):
 rid='rollout:'+row['id']
 edge('doc:'+pcpath,rid,'maps_control_contract',pcpath,f'/operations/{i}')
 edge(rid,'controlmap:'+row['policyRef'],'requires_candidate_policy',pcpath,f'/operations/{i}/policyRef')
 for ref in row['readResourceRefs']:edge(rid,'controlmap:'+ref,'proposed_read_mapping',pcpath,f'/operations/{i}/readResourceRefs')
 for ref in row['writeResourceRefs']:edge(rid,'controlmap:'+ref,'proposed_write_mapping',pcpath,f'/operations/{i}/writeResourceRefs')
for i,row in enumerate(pc['storageMappings']):
 if row['sourceRecordRef']:edge('controlmap:'+row['id'],'rollout:'+row['sourceRecordRef'],'maps_logical_record',pcpath,f'/storageMappings/{i}/sourceRecordRef')
for i,row in enumerate(pc['existingBindings']):edge('doc:'+pcpath,'api:'+row['apiRef'],'conditional_reuse_requires_adoption',pcpath,f'/existingBindings/{i}')
for ref in pc['screenRefs']:edge('doc:'+pcpath,'screen:'+ref,'proposes_management_projection',pcpath,'/screenRefs')

# Management trust lifecycle remains a candidate, with no key provisioning.
mtpath='content/specifications/management-trust-lifecycle.json'
mt=read(mtpath)
for section,kind in [('keyDomains','trust_key_domain'),('policyInputs','trust_policy_input'),('records','trust_record'),('invariants','trust_invariant'),('commands','trust_command'),('transitions','trust_transition')]:
 for i,row in enumerate(mt[section]):
  rid='trust:'+row['id'];node(rid,kind,row['id']+' '+row.get('name',row.get('needed',row.get('rule',''))),mtpath,f'/{section}/{i}',status='candidate_not_provisioned')
  edge('doc:'+mtpath,rid,'contains',mtpath,f'/{section}/{i}')
  for ref in row.get('recordRefs',[]):edge(rid,'trust:'+ref,'requires_trust_record',mtpath,f'/{section}/{i}/recordRefs')
  for ref in row.get('fieldRefs',[]):edge(rid,'field:'+ref,'needs_selected_input',mtpath,f'/{section}/{i}/fieldRefs')
  if row.get('commandRef'):edge(rid,'trust:'+row['commandRef'],'uses_candidate_command',mtpath,f'/{section}/{i}/commandRef')
for i,row in enumerate(mt['crossMappings']):
 for ref in row['policyCandidateRefs']:edge('doc:'+mtpath,'controlmap:'+ref,'supplies_current_trust_requirements',mtpath,f'/crossMappings/{i}/policyCandidateRefs')
for ref in mt['taskRefs']:edge('doc:'+mtpath,'task:'+ref,'provides_design_input',mtpath,'/taskRefs')
for ref in mt['decisionRefs']:edge('doc:'+mtpath,'decision:'+ref,'needs_selected_profile',mtpath,'/decisionRefs')
for ref in mt['uiProjection']['screenRefs']:edge('doc:'+mtpath,'screen:'+ref,'proposes_trust_projection',mtpath,'/uiProjection/screenRefs')

# Approval evidence refines logical records without adopting a wire format or authority.
tepath='content/specifications/management-trust-evidence.json'
te=read(tepath)
for section,kind in [('records','trust_evidence_record'),('rules','trust_evidence_rule'),('scenarios','trust_evidence_scenario'),('retention','trust_retention_rule')]:
 for i,row in enumerate(te[section]):
  rid='evidence:'+row['id'];node(rid,kind,row['id']+' '+row['name'],tepath,f'/{section}/{i}',status='design_candidate')
  edge('doc:'+tepath,rid,'contains',tepath,f'/{section}/{i}')
  for ref in row.get('recordRefs',[]):
   prefix='trust:' if section=='records' else 'evidence:'
   edge(rid,prefix+ref,'refines_record' if section=='records' else 'constrains_record',tepath,f'/{section}/{i}/recordRefs')
  for ref in row.get('commandRefs',[]):edge(rid,'trust:'+ref,'illustrates_command',tepath,f'/{section}/{i}/commandRefs')
  if row.get('policyRef'):edge(rid,'trust:'+row['policyRef'],'needs_selected_policy',tepath,f'/{section}/{i}/policyRef')
for i,row in enumerate(te['unresolved']):
 edge('doc:'+tepath,'trust:'+row['policyRef'],'requires_unselected_policy',tepath,f'/unresolved/{i}/policyRef')

# Logical operations roles and screen extensions are not assigned identities or active ACLs.
topath='content/specifications/trust-operations-design.json'
to=read(topath)
for section,kind in [('roles','trust_operating_role'),('evidenceResponsibilities','trust_evidence_responsibility'),('panels','trust_panel_candidate'),('actions','trust_operating_action'),('handoffs','trust_operating_handoff'),('invariants','trust_operating_invariant')]:
 for i,row in enumerate(to[section]):
  rid='trustops:'+row['id'];node(rid,kind,row['id']+' '+row.get('name',row.get('rule',row.get('evidenceRef',''))),topath,f'/{section}/{i}',status='candidate_unassigned')
  edge('doc:'+topath,rid,'contains',topath,f'/{section}/{i}')
  for key in ['roleRefs','issuerRoleRefs','verifierRoleRefs','readerRoleRefs']:
   for ref in row.get(key,[]):edge(rid,'trustops:'+ref,key,topath,f'/{section}/{i}/{key}')
  if row.get('custodianRoleRef'):edge(rid,'trustops:'+row['custodianRoleRef'],'custodian_role',topath,f'/{section}/{i}/custodianRoleRef')
  if row.get('evidenceRef'):edge(rid,'evidence:'+row['evidenceRef'],'assigns_logical_responsibility',topath,f'/{section}/{i}/evidenceRef')
  for ref in row.get('evidenceRefs',[]):edge(rid,'evidence:'+ref,'uses_evidence',topath,f'/{section}/{i}/evidenceRefs')
  for ref in row.get('commandRefs',[]):edge(rid,'trust:'+ref,'maps_candidate_command',topath,f'/{section}/{i}/commandRefs')
  if row.get('screenRef'):edge(rid,'screen:'+row['screenRef'],'proposes_screen_extension',topath,f'/{section}/{i}/screenRef')
for i,row in enumerate(to['unresolved']):edge('doc:'+topath,'trust:'+row['policyRef'],'requires_unselected_policy',topath,f'/unresolved/{i}/policyRef')

# Latest handoff addendum distinguishes selection, design, adoption and runtime evidence.
dcpath='content/planning/design-closure-review.json'
dc=read(dcpath)
for section,kind in [('products','product_handoff_view'),('classes','design_closure_class'),('adoptionFamilies','adoption_family'),('closureSequence','closure_sequence'),('reviewCorrections','handoff_correction')]:
 for i,row in enumerate(dc[section]):
  rid='closure:'+row['id'];node(rid,kind,row['id']+' '+row.get('name',row.get('issue','')),dcpath,f'/{section}/{i}',status='candidate_not_implementation_ready')
  edge('doc:'+dcpath,rid,'contains',dcpath,f'/{section}/{i}')
  for ref in row.get('classRefs',[]):edge(rid,'closure:'+ref,'classifies_remaining_work',dcpath,f'/{section}/{i}/classRefs')
  for ref in row.get('dependsOn',[]):edge(rid,'closure:'+ref,'proposed_closure_prerequisite',dcpath,f'/{section}/{i}/dependsOn')
  for ref in row.get('requirementRefs',[]):edge(rid,'req:'+str(ref),'reviews_requirement',dcpath,f'/{section}/{i}/requirementRefs')
  for ref in row.get('packageRefs',[]):edge(rid,'package:'+ref,'uses_logical_design',dcpath,f'/{section}/{i}/packageRefs')
  for ref in row.get('focusDecisionRefs',[]):edge(rid,'brief:'+ref,'focuses_selection_review',dcpath,f'/{section}/{i}/focusDecisionRefs',basis='focused review scope, not universal action prerequisites')
  for ref in row.get('taskImpactRefs',[]):edge(rid,'task:'+ref,'requirement_based_impact',dcpath,f'/{section}/{i}/taskImpactRefs',basis='not ownership or readiness')
  if row.get('source'):edge(rid,'doc:'+row['source'],'uses_candidate_family',dcpath,f'/{section}/{i}/source')

# Cycle detection is scoped to actual completion dependencies, not arbitrary references.
visiting=set();visited=set();cycles=[]
def dfs(id,stack):
 if id in visiting:
  cycles.append(stack[stack.index(id):]+[id]);return
 if id in visited:return
 visiting.add(id)
 for dep in tasks[id]['dependsOn']:dfs(dep,stack+[id])
 visiting.remove(id);visited.add(id)
for id in tasks:dfs(id,[])
dangling=[e for e in edges if e['source'] not in nodes or e['target'] not in nodes]
assert not dangling,dangling[:5]
# Proposed recovery states require outgoing transitions; this is a modeling check, not model checking.
state_audit=[]
for entry in h['designPackages']:
 src=entry['source'];d=read(src)
 for section,rs in d.items():
  if not section.endswith('Transitions') or not isinstance(rs,list):continue
  # The hold-resume supplement and primary MPC table form one state machine.
  if section=='mpcResumeTransitions':continue
  if section=='mpcTransitions':rs=rs+d.get('mpcResumeTransitions',[])
  if not all(isinstance(r,dict) and 'fromState' in r and 'toState' in r for r in rs):continue
  outgoing={s for r in rs for s in r['fromState'].split('|')}; incoming={r['toState'] for r in rs}
  state_audit.append(dict(source=src,section=section,transitionCount=len(rs),sinks=sorted(incoming-outgoing),unknownHasRecovery='unknown' not in incoming or 'unknown' in outgoing))
hashes=[]
for rel,d in parsed.items():
 if not isinstance(d,dict):continue
 for p,digest in d.get('sourceHashes',{}).items():
  if isinstance(digest,str) and re.fullmatch('[0-9a-f]{64}',digest):
   candidates=list(dict.fromkeys([(ROOT/p).resolve(),(ROOT/rel).parent.joinpath(p).resolve()]))
   existing=[x for x in candidates if x.is_file()]
   if len(existing)==1:
    target=existing[0];status='matches' if sha(target)==digest else 'mismatch'
    resolved=str(target.relative_to(ROOT));edge('doc:'+rel,'doc:'+resolved,'pins_source',rel,'/sourceHashes/'+p.replace('~','~0').replace('/','~1')) if 'doc:'+resolved in nodes else None
   else:
    status='missing' if not existing else 'ambiguous_base';resolved=None
   hashes.append(dict(container=rel,source=p,resolvedSource=resolved,status=status))
counts=dict(collections.Counter(n['kind'] for n in nodes.values()))
g=dict(methodology=dict(requested='x-theory',status='awaiting_user_definition',applied='neutral typed provenance graph; not x-theory compliance',epistemicRules=['document reference != adopted design','planned check != execution evidence','open decision != proven contradiction','historical recommendation != current status','dependency cycles evaluated only on depends_on']),scope=dict(documentInventory=len(paths),deepReviewFiles=[p['source'] for p in h['designPackages']]+[wpath,fpath,hpath,'content/specifications/preimplementation-contract-overlay.json',bp],notClaimed='all prose in every inventoried document has been semantically reviewed'),counts=counts,nodes=list(nodes.values()),edges=edges)
save('graph.json',g)
save('structural-audit.json',dict(methodologyStatus=g['methodology']['status'],nodeCount=len(nodes),edgeCount=len(edges),danglingEdges=len(dangling),wbsDependencyEdges=sum(len(t['dependsOn']) for t in tasks.values()),wbsCycles=cycles,packageScopeComparisons=package_impacts,stateAudit=state_audit,sourceHashChecks=len(hashes),hashMismatches=[r for r in hashes if r['status']!='matches'],runtimeVerified=False))
# Full DOT export uses escaped JSON strings as DOT quoted IDs and labels.
quote=lambda x:json.dumps(x,ensure_ascii=False)
lines=['digraph documents {','  graph [rankdir=LR];','  node [shape=box];']
for n in nodes.values():lines.append('  '+quote(n['id'])+' [label='+quote(n['label'])+'];')
for e in edges:lines.append('  '+quote(e['source'])+' -> '+quote(e['target'])+' [label='+quote(e['relation'])+'];')
lines.append('}')
(OUT/'graph.dot').write_text('\n'.join(lines)+'\n')
print(json.dumps(dict(documents=len(paths),nodes=len(nodes),edges=len(edges),counts=counts,wbsCycles=cycles,hashMismatches=sum(r['status']!='matches' for r in hashes)),ensure_ascii=False))
