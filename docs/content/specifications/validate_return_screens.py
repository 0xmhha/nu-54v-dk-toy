"""Check declarative screen relationships, not rendered UI or live authorization."""
import json
import re
from pathlib import Path
P=Path(__file__).resolve().parent
load=lambda f:json.loads((P/f).read_text())
d=load('return-screen-design.json');screens={x['id']:x for x in load('screen-flows.json')['screens']}
a={x['id']:x for x in d['actions']};states={x['id']:x for x in d['states']};overlays={x['id']:x for x in d['overlays']}
protocol={x['id'] for x in load('return-protocol-candidate.json')['operations']};phases=set(load('return-recovery-contract.json')['jobStates'])
assert len(screens)==37 and len(states)==11 and len(overlays)==6 and len(a)==12
assert set(d['screenRefs'])<=screens.keys()
for action in a.values():
 assert action['guard'] and action['labelKo'] and action['labelEn']
 assert set(action['protocolRefs'])<=protocol
for state in states.values():
 assert set(state['jobPhases'])<=phases
 for surface in ['user','operator']:assert set(state[surface]['actions'])<=a.keys()
 assert state['operator']['projection']=='operator_safe'
 assert not set(state['operator']['actions'])&{'RA-04','RA-05','RA-09'}
 assert state['device']['exposesPrivateHistory'] is False
 if 'RA-11' in state['operator']['actions']:assert state['id']=='RU-07'
 if state['id'] not in ['RU-01','RU-02']:assert not set(state['user']['actions'])&{'RA-04','RA-05'}
for o in overlays.values():assert set(o['forbiddenActions'])<=a.keys()
assert len({x['id'] for x in d['cases']})==13
for case in d['cases']:
 assert case['baseState'] in states and set(case['overlayRefs'])<=overlays.keys()
 assert case['status']=='not_run' and not case['evidenceRefs']
 state=states[case['baseState']]
 # The allowed set is still conditional on current per-action authorization/guard.
 remaining=set(state['user']['actions'])|set(state['operator']['actions'])
 for ref in case['overlayRefs']:remaining-=set(overlays[ref]['forbiddenActions'])
 assert not remaining&set(case['mustNotOffer']),case['id']
assert states['RU-11']['jobPhases']==['completed']
assert 'RA-11' not in states['RU-11']['operator']['actions']
assert {'RA-03','RA-04','RA-05','RA-06','RA-07','RA-11'}<=set(overlays['RO-03']['forbiddenActions'])
for id in d['screenRefs']:assert screens[id]['returnDesignRef']==d['document']
for href in re.findall(r'\]\(([^)]+)\)',(P/d['document']).read_text()):assert (P/href).exists(),href
assert load('return-protocol-candidate.json')['lateAssetRecoveryPolicy']=='unresolved'
assert load('return-recovery-contract.json')['openDecisions'][0]['selection'] is None
print(json.dumps({'baseStates':11,'overlays':6,'guardedActions':12,'declarativeCases':13,'affectedScreens':5,'canonicalScreens':37,'runtimeUiAndAuthorizationTests':'not_run'}))
