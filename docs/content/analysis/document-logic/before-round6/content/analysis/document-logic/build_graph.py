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
g=dict(methodology=dict(requested='x-theory',status='awaiting_user_definition',applied='neutral typed provenance graph; not x-theory compliance',epistemicRules=['document reference != adopted design','planned check != execution evidence','open decision != proven contradiction','historical recommendation != current status','dependency cycles evaluated only on depends_on']),scope=dict(documentInventory=len(paths),deepReviewFiles=[p['source'] for p in h['designPackages']]+[wpath,fpath,hpath,'content/specifications/preimplementation-contract-overlay.json'],notClaimed='all prose in every inventoried document has been semantically reviewed'),counts=counts,nodes=list(nodes.values()),edges=edges)
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
